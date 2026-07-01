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
import threading
import time
from concurrent.futures import ThreadPoolExecutor

import pytest

from kserve import AsyncModel


class SyncPredictModel(AsyncModel):
    def __init__(self, name="sync-model"):
        super().__init__(name)
        self.ready = True

    def predict(self, payload, headers=None):
        return threading.get_ident()


class GatedModel(AsyncModel):
    """Blocks inside predict() until released.

    Used to prove that a long-running prediction does not block the event loop
    or new prediction requests.
    """

    def __init__(self, name="gated-model"):
        super().__init__(name)
        self.ready = True
        self.started = threading.Event()
        self.release = threading.Event()

    def predict(self, payload, headers=None):
        if payload.get("block"):
            self.started.set()
            # Block the worker thread (not the event loop) until released.
            if not self.release.wait(timeout=5):
                raise TimeoutError("blocking predict was never released")
            return "slow"
        return "fast"


class SlowModel(AsyncModel):
    """Sleeps inside predict() to measure parallel throughput."""

    def __init__(self, name="slow-model", delay=0.4):
        super().__init__(name)
        self.ready = True
        self.delay = delay

    def predict(self, payload, headers=None):
        time.sleep(self.delay)
        return threading.get_ident()


_MODEL_CLASSES = (SyncPredictModel, GatedModel, SlowModel)


def _reset_state(cls):
    cls._cleanup_executor()
    cls._shared_executor = None
    cls._max_workers = None
    cls._cleanup_registered = False


@pytest.fixture(autouse=True)
def reset_async_model_state():
    for cls in _MODEL_CLASSES:
        _reset_state(cls)
    yield
    for cls in _MODEL_CLASSES:
        _reset_state(cls)


@pytest.mark.asyncio
async def test_async_model_offloads_sync_predict():
    model = SyncPredictModel()
    loop_thread_id = threading.get_ident()
    result = await model.predict({"instances": [1]})
    assert result != loop_thread_id


@pytest.mark.asyncio
async def test_async_model_env_workers(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    executor = SyncPredictModel._get_executor()
    assert isinstance(executor, ThreadPoolExecutor)
    assert executor._max_workers == 1


def test_async_model_invalid_env_workers(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "invalid")
    executor = SyncPredictModel._get_executor()
    assert executor is None
    assert SyncPredictModel._max_workers == -1


def test_async_model_cleanup_shared_executor(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    executor = SyncPredictModel._get_executor()
    assert isinstance(executor, ThreadPoolExecutor)
    SyncPredictModel._cleanup_executor()
    assert SyncPredictModel._shared_executor is None


@pytest.mark.asyncio
async def test_long_predict_does_not_block_new_requests(monkeypatch):
    # Two workers: one is held by the long prediction, the other must stay
    # available to serve a new request concurrently.
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "2")
    model = GatedModel()

    # Start a long-running prediction that blocks inside its worker thread.
    slow_task = asyncio.create_task(model.predict({"block": True}))

    # Spin until the slow predict is actually running. The event loop staying
    # responsive enough to run this loop already proves it is not blocked.
    while not model.started.is_set():
        await asyncio.sleep(0.01)

    # A new request must complete while the long prediction is still in flight.
    fast_result = await asyncio.wait_for(model.predict({"block": False}), timeout=2)
    assert fast_result == "fast"
    assert not slow_task.done()

    # Release the long prediction and confirm it finishes cleanly.
    model.release.set()
    assert await asyncio.wait_for(slow_task, timeout=2) == "slow"


@pytest.mark.asyncio
async def test_async_model_runs_predicts_concurrently(monkeypatch):
    num_requests = 5
    delay = 0.4
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", str(num_requests))
    model = SlowModel(delay=delay)

    start = time.perf_counter()
    results = await asyncio.gather(
        *(model.predict({"instances": [i]}) for i in range(num_requests))
    )
    elapsed = time.perf_counter() - start

    # Each predict ran on its own worker thread, none on the event-loop thread.
    assert len(set(results)) == num_requests
    assert threading.get_ident() not in results
    # Concurrent execution finishes in ~delay; serialized would be num*delay.
    assert elapsed < num_requests * delay * 0.5
