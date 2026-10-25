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
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import Mock

import pytest

from kserve import AsyncModel
from kserve import async_model


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
    atexit.unregister(cls._cleanup_executor)
    if cls is AsyncModel:
        cls._shared_executor = None
        cls._max_workers = None
        cls._cleanup_registered = False
    else:
        for name in ("_shared_executor", "_max_workers", "_cleanup_registered"):
            if name in cls.__dict__:
                delattr(cls, name)


@pytest.fixture(autouse=True)
def reset_async_model_state():
    for cls in (AsyncModel, *_MODEL_CLASSES):
        _reset_state(cls)
    yield
    for cls in (AsyncModel, *_MODEL_CLASSES):
        _reset_state(cls)


def test_executor_is_shared_across_model_classes(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "2")
    executor = SyncPredictModel._get_executor()

    assert GatedModel._get_executor() is executor
    assert SlowModel._get_executor() is executor
    assert AsyncModel._get_executor() is executor


def test_concurrent_first_use_creates_one_executor(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "2")
    factory = Mock(wraps=ThreadPoolExecutor)
    register = Mock(wraps=atexit.register)
    monkeypatch.setattr(async_model, "ThreadPoolExecutor", factory)
    monkeypatch.setattr(async_model.atexit, "register", register)
    barrier = threading.Barrier(6)

    def get_executor(cls):
        barrier.wait(timeout=5)
        return cls._get_executor()

    with ThreadPoolExecutor(max_workers=6) as callers:
        futures = [callers.submit(get_executor, cls) for cls in _MODEL_CLASSES * 2]
        executors = [future.result(timeout=5) for future in futures]

    assert all(executor is executors[0] for executor in executors)
    factory.assert_called_once_with(max_workers=2)
    register.assert_called_once_with(AsyncModel._cleanup_executor)


def test_cleanup_from_another_class_closes_the_shared_pool(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    executor = SyncPredictModel._get_executor()
    GatedModel._cleanup_executor()

    with pytest.raises(RuntimeError, match="shutdown"):
        executor.submit(lambda: None)
    assert AsyncModel._shared_executor is None
    replacement = SlowModel._get_executor()
    assert replacement is not executor
    assert SyncPredictModel._get_executor() is replacement


@pytest.mark.parametrize("workers", [None, "invalid", "0", "-1"])
def test_default_executor_fallback_is_shared(monkeypatch, workers):
    if workers is None:
        monkeypatch.delenv("ASYNC_MODEL_WORKERS", raising=False)
    else:
        monkeypatch.setenv("ASYNC_MODEL_WORKERS", workers)

    assert all(cls._get_executor() is None for cls in _MODEL_CLASSES)
    assert AsyncModel._shared_executor is None
    assert not AsyncModel._cleanup_registered


@pytest.mark.asyncio
@pytest.mark.parametrize("workers", [1, 2])
async def test_worker_limit_applies_across_model_classes(monkeypatch, workers):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", str(workers))
    loop = asyncio.get_running_loop()
    release = threading.Event()
    workers_started = asyncio.Event()
    all_submitted = asyncio.Event()
    state_lock = threading.Lock()
    active = 0
    peak = 0
    submitted = []
    original_submit = ThreadPoolExecutor.submit

    def predict(self, payload, headers=None):
        nonlocal active, peak
        with state_lock:
            active += 1
            peak = max(peak, active)
            if active >= workers:
                loop.call_soon_threadsafe(workers_started.set)
        try:
            if not release.wait(timeout=5):
                raise TimeoutError("prediction was never released")
            return self.name
        finally:
            with state_lock:
                active -= 1

    def submit(executor, fn, *args, **kwargs):
        future = original_submit(executor, fn, *args, **kwargs)
        submitted.append(future)
        if len(submitted) == 3:
            all_submitted.set()
        return future

    monkeypatch.setattr(ThreadPoolExecutor, "submit", submit)
    classes = [
        type(f"GatedModel{i}", (AsyncModel,), {"predict": predict}) for i in range(3)
    ]
    tasks = [
        asyncio.create_task(cls(str(i)).predict({})) for i, cls in enumerate(classes)
    ]
    try:
        await asyncio.wait_for(all_submitted.wait(), timeout=2)
        await asyncio.wait_for(workers_started.wait(), timeout=2)
        # All three calls have reached their executors; excess work must queue.
        assert sum(future.running() for future in submitted) == workers
        with state_lock:
            assert active == workers
    finally:
        release.set()
        results = await asyncio.wait_for(asyncio.gather(*tasks), timeout=5)
        for cls in classes:
            cls._cleanup_executor()
            atexit.unregister(cls._cleanup_executor)

    assert results == ["0", "1", "2"]
    assert peak <= workers


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
