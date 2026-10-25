# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import atexit
import asyncio
import contextvars
import functools
import inspect
import os
import threading
from concurrent.futures import ThreadPoolExecutor
from typing import Optional

from .model import Model


class AsyncModel(Model):
    """Model base class that offloads sync predict() to a thread executor.

    Example
    -------
    from kserve import AsyncModel

    class MyModel(AsyncModel):
        def predict(self, payload, headers=None):
            return slow_inference(payload)

    Why
    Many custom predictors are written with sync predict() and include blocking
    I/O or long-running inference. When KServe's async __call__() executes a sync
    predict() directly, the event loop can be blocked, reducing concurrency and
    causing timeouts. Rewriting the full stack to async is often impractical, so
    AsyncModel provides a safe default that keeps the event loop responsive while
    preserving existing sync code.

    How it works
    1) Subclass defines sync predict() as usual.
    2) __init__ wraps the instance's bound predict in async + run_in_executor.
    3) KServe's __call__ sees async predict and awaits it.
    4) Sync code runs in a thread pool with a copy of the request's context.
       Class methods remain unchanged so synchronous super() calls still work.

    Configuration
    - A positive ASYNC_MODEL_WORKERS sets a single per-process worker limit shared
      by all AsyncModel subclasses and instances.
    - Otherwise, use the event loop's default executor, configured by ModelServer
      --max_asyncio_workers (default: min(32, cpu_count + 4)).
    """

    _shared_executor: Optional[ThreadPoolExecutor] = None
    _max_workers: Optional[int] = None
    _cleanup_registered: bool = False
    _executor_lock = threading.Lock()

    @staticmethod
    def _cleanup_executor():
        with AsyncModel._executor_lock:
            if AsyncModel._shared_executor is not None:
                AsyncModel._shared_executor.shutdown(wait=False, cancel_futures=True)
                AsyncModel._shared_executor = None

    @staticmethod
    def _get_executor() -> Optional[ThreadPoolExecutor]:
        # State belongs to AsyncModel, not to the subclass invoking this method.
        # Serialize first use across model classes and event-loop threads.
        with AsyncModel._executor_lock:
            if AsyncModel._shared_executor is not None:
                return AsyncModel._shared_executor

            if AsyncModel._max_workers is None:
                env_workers = os.getenv("ASYNC_MODEL_WORKERS")
                if env_workers:
                    try:
                        AsyncModel._max_workers = int(env_workers)
                    except ValueError:
                        AsyncModel._max_workers = -1

            if AsyncModel._max_workers and AsyncModel._max_workers > 0:
                AsyncModel._shared_executor = ThreadPoolExecutor(
                    max_workers=AsyncModel._max_workers
                )
                if not AsyncModel._cleanup_registered:
                    atexit.register(AsyncModel._cleanup_executor)
                    AsyncModel._cleanup_registered = True
                return AsyncModel._shared_executor

            return None

    def __init_subclass__(cls, **kwargs):
        super().__init_subclass__(**kwargs)
        predict_attr = inspect.getattr_static(cls, "predict")
        if isinstance(predict_attr, (staticmethod, classmethod)):
            raise TypeError("AsyncModel.predict must be an instance method.")

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        # Resolve the effective method, including methods inherited from mixins,
        # without replacing class attributes used by synchronous super() calls.
        original_predict = self.predict

        if inspect.iscoroutinefunction(original_predict):
            return

        @functools.wraps(original_predict)
        async def async_predict(*args, **kwargs):
            loop = asyncio.get_running_loop()
            context = contextvars.copy_context()
            bound = functools.partial(original_predict, *args, **kwargs)
            return await loop.run_in_executor(self._get_executor(), context.run, bound)

        self.predict = async_predict
