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
import copy
import inspect
import multiprocessing
import os
import pickle
import threading
import time
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import Mock

import pytest

from kserve import AsyncModel
from kserve import async_model
from kserve import context as kserve_context
from kserve.predictor_config import PredictorConfig
from kserve import ModelRepository
from kserve.constants.constants import FASTAPI_APP_IMPORT_STRING
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.server import RESTServer


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


class SerializablePredictMixin:
    def predict(self, payload, headers=None, response_headers=None):
        response_headers["x-model"] = self.name
        return {
            "value": payload["value"] * self.factor,
            "host": self.predictor_config.predictor_host,
            "thread": threading.get_ident(),
        }


class SerializableModel(SerializablePredictMixin, AsyncModel):
    __slots__ = ("factor",)

    def __init__(self, name):
        super().__init__(name, return_response_headers=True)
        self.factor = 2
        self.ready = True


class SerializableChildModel(SerializableModel):
    def predict(self, payload, headers=None, response_headers=None):
        result = super().predict(payload, headers, response_headers)
        result["value"] += 1
        return result


class SerializableAsyncModel(SerializableModel):
    async def predict(self, payload, headers=None, response_headers=None):
        return super().predict(payload, headers, response_headers)


def _predict_in_spawned_server(server, connection):
    async def predict():
        kserve_context.set_predictor_config(PredictorConfig("child-host"))
        loop_thread = threading.get_ident()
        model = server.dataplane.model_registry.get_model("spawned")
        result, headers = await model({"value": 3})
        connection.send((os.getpid(), loop_thread, result, headers))

    try:
        asyncio.run(predict())
    finally:
        connection.close()


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


@pytest.mark.asyncio
@pytest.mark.parametrize("workers", [None, "1", "2"])
async def test_predict_preserves_request_context_without_leaking_changes(
    monkeypatch, workers
):
    if workers is None:
        monkeypatch.delenv("ASYNC_MODEL_WORKERS", raising=False)
    else:
        monkeypatch.setenv("ASYNC_MODEL_WORKERS", workers)
    request_id = contextvars.ContextVar("request_id", default="outside")

    class ContextModel(AsyncModel):
        def predict(self, payload, headers=None):
            config = self.predictor_config
            value = request_id.get()
            request_id.set("worker-only")
            kserve_context.set_predictor_config(PredictorConfig("worker-only"))
            return config, value, threading.get_ident()

    model = ContextModel("context-model")
    loop_thread = threading.get_ident()

    async def request(name):
        config = PredictorConfig(name)
        kserve_context.set_predictor_config(config)
        request_id.set(name)
        result, _ = await model({"instances": [1]})
        assert result[0] is config
        assert result[1] == name
        assert result[2] != loop_thread
        assert model.predictor_config is config
        assert request_id.get() == name

    # Reuse the same model/worker while each task has its own configuration.
    await asyncio.gather(request("first"), request("second"))
    await asyncio.gather(request("third"), request("fourth"))
    assert request_id.get() == "outside"


@pytest.mark.asyncio
async def test_sync_super_calls_keep_return_values_and_instance_binding():
    class Parent(AsyncModel):
        def predict(self, payload, headers=None):
            return [self.name, threading.get_ident(), headers["x-request-id"]]

    class Child(Parent):
        def predict(self, payload, headers=None):
            return super().predict(payload, headers) + ["child"]

    class Grandchild(Child):
        def predict(self, payload, headers=None):
            return super().predict(payload, headers) + ["grandchild"]

    for cls, suffix in ((Child, ["child"]), (Grandchild, ["child", "grandchild"])):
        first, second = cls("first"), cls("second")
        for model in (first, second, first):
            result, _ = await model({}, {"x-request-id": model.name})
            assert result[0] == result[2] == model.name
            assert result[1] != threading.get_ident()
            assert result[3:] == suffix


@pytest.mark.asyncio
async def test_inherited_sync_mixin_predict_is_offloaded():
    class SyncPredictMixin:
        def predict(self, payload, headers=None):
            return threading.get_ident()

    class ServedModel(SyncPredictMixin, AsyncModel):
        pass

    class InheritedModel(ServedModel):
        pass

    for cls in (ServedModel, InheritedModel):
        result, _ = await cls("inherited")({})
        assert result != threading.get_ident()


@pytest.mark.asyncio
async def test_async_predict_and_response_header_pipeline_stay_on_event_loop(
    monkeypatch,
):
    class AsyncPredictMixin:
        async def predict(self, payload, headers=None, response_headers=None):
            response_headers["x-model"] = self.name
            return {"thread": threading.get_ident(), "value": payload["value"]}

    class ServedModel(AsyncPredictMixin, AsyncModel):
        pass

    model = ServedModel("native-async", return_response_headers=True)
    monkeypatch.setattr(
        model,
        "_get_executor",
        Mock(side_effect=AssertionError("offloaded async predict")),
    )
    assert inspect.iscoroutinefunction(model.predict)
    result, headers = await model({"value": 42})
    assert result == {"thread": threading.get_ident(), "value": 42}
    assert headers == {"x-model": "native-async"}


@pytest.mark.asyncio
async def test_sync_predict_response_headers_and_exceptions():
    class HeaderModel(AsyncModel):
        def predict(self, payload, headers=None, response_headers=None):
            if payload.get("fail"):
                raise ValueError("prediction failed")
            response_headers["x-model"] = self.name
            return payload

    model = HeaderModel("sync-headers", return_response_headers=True)
    assert await model({"value": 42}) == ({"value": 42}, {"x-model": "sync-headers"})
    with pytest.raises(ValueError, match="prediction failed"):
        await model({"fail": True})
    assert await model({"value": 43}) == ({"value": 43}, {"x-model": "sync-headers"})


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_type", [SerializableModel, SerializableChildModel, SerializableAsyncModel]
)
@pytest.mark.parametrize("transport", ["pickle-4", "pickle-5", "deepcopy"])
async def test_serialized_model_keeps_prediction_behavior(
    monkeypatch, model_type, transport
):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    original = model_type("original")
    # Exercise serialization after first use, when the shared pool already exists.
    config = kserve_context.get_predictor_config()
    kserve_context.set_predictor_config(PredictorConfig("parent-host"))
    try:
        await original({"value": 1})
        if transport == "deepcopy":
            restored = copy.deepcopy(original)
        else:
            restored = pickle.loads(pickle.dumps(original, protocol=int(transport[-1])))
        restored.name = "restored"
        restored.factor = 3
        assert restored.ready
        assert inspect.iscoroutinefunction(restored.predict)
        result, headers = await restored({"value": 4})
        assert result["value"] == (13 if model_type is SerializableChildModel else 12)
        assert result["host"] == "parent-host"
        assert (result["thread"] == threading.get_ident()) == (
            model_type is SerializableAsyncModel
        )
        assert headers == {"x-model": "restored"}
        original_result, original_headers = await original({"value": 4})
        assert original_result["value"] == (
            9 if model_type is SerializableChildModel else 8
        )
        assert original_headers == {"x-model": "original"}
    finally:
        kserve_context.set_predictor_config(config)


def test_registered_async_model_can_predict_in_spawned_workers(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    registry = ModelRepository()
    registry.update(SerializableChildModel("spawned"))
    server = RESTServer(
        FASTAPI_APP_IMPORT_STRING,
        DataPlane(registry),
        ModelRepositoryExtension(registry),
        http_port=0,
        workers=2,
    )
    context = multiprocessing.get_context("spawn")
    children = []
    try:
        for _ in range(server.config.workers):
            reader, writer = context.Pipe(duplex=False)
            process = context.Process(
                target=_predict_in_spawned_server, args=(server, writer)
            )
            children.append((process, reader, writer))
            process.start()
            writer.close()
        for process, reader, _ in children:
            assert reader.poll(30), "spawned worker did not return a prediction"
            pid, loop_thread, result, headers = reader.recv()
            assert pid == process.pid != os.getpid()
            assert result["value"] == 7
            assert result["host"] == "child-host"
            assert result["thread"] != loop_thread
            assert headers == {"x-model": "spawned"}
            process.join(timeout=10)
            assert process.exitcode == 0
    finally:
        for process, reader, writer in children:
            if process.pid is not None:
                if process.is_alive():
                    process.kill()
                process.join(timeout=10)
                process.close()
            reader.close()
            writer.close()
