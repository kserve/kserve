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

import asyncio
import functools
import inspect
import os
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
    2) __init_subclass__ wraps it in async + run_in_executor.
    3) KServe's __call__ sees async predict and awaits it.
    4) Sync code runs in a thread pool (non-blocking).

    Configuration
    - ASYNC_MODEL_WORKERS env var for max workers, or ModelServer --max_asyncio_workers.
    - Default (when using ModelServer): min(32, cpu_count + 4).
    """

    _shared_executor: Optional[ThreadPoolExecutor] = None
    _max_workers: Optional[int] = None

    @classmethod
    def _get_executor(cls) -> Optional[ThreadPoolExecutor]:
        if cls._shared_executor is not None:
            return cls._shared_executor

        if cls._max_workers is None:
            env_workers = os.getenv("ASYNC_MODEL_WORKERS")
            if env_workers:
                try:
                    cls._max_workers = int(env_workers)
                except ValueError:
                    cls._max_workers = None

        if cls._max_workers and cls._max_workers > 0:
            cls._shared_executor = ThreadPoolExecutor(max_workers=cls._max_workers)
            return cls._shared_executor

        return None

    def __init_subclass__(cls, **kwargs):
        super().__init_subclass__(**kwargs)
        predict_attr = cls.__dict__.get("predict")
        if predict_attr is None:
            return

        if isinstance(predict_attr, (staticmethod, classmethod)):
            raise TypeError("AsyncModel.predict must be an instance method.")

        original_predict = predict_attr

        if inspect.iscoroutinefunction(original_predict):
            return

        @functools.wraps(original_predict)
        async def async_predict(self, *args, **kwargs):
            loop = asyncio.get_running_loop()
            executor = cls._get_executor()
            bound = functools.partial(original_predict, self, *args, **kwargs)
            return await loop.run_in_executor(executor, bound)

        setattr(cls, "predict", async_predict)
