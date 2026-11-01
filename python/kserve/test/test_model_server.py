# Copyright 2024 The KServe Authors.
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
import signal
import threading
from concurrent.futures import ThreadPoolExecutor
from unittest.mock import Mock

import pytest
from kserve import Model, ModelRepository, ModelServer, model_server

UNKNOWN_MODEL_TYPE_ERR_MESSAGE = "Unknown model collection type"


def test_model_server_start_no_models():
    server = ModelServer()

    with pytest.raises(RuntimeError) as exc:
        server.start(models=None)

    assert exc.value.args[0] == UNKNOWN_MODEL_TYPE_ERR_MESSAGE


@pytest.mark.parametrize("configured_workers", [1, 2, None])
def test_start_configures_executor_on_serving_loop(monkeypatch, configured_workers):
    monkeypatch.delenv("ASYNC_MODEL_WORKERS", raising=False)
    monkeypatch.setattr(Model, "_shared_executor", None)
    monkeypatch.setattr(Model, "_max_workers", None)
    monkeypatch.setattr(model_server.utils, "cpu_count", lambda: 0)
    expected_workers = configured_workers if configured_workers is not None else 4
    count = expected_workers + 1
    release = threading.Event()
    submitted = []
    state_lock = threading.Lock()
    active = peak = 0
    serving_loop = None
    started = None
    all_submitted = None
    original_submit = ThreadPoolExecutor.submit

    class BlockingModel(Model):
        def predict(self, payload, headers=None):
            nonlocal active, peak
            with state_lock:
                active += 1
                peak = max(peak, active)
                if active >= expected_workers:
                    serving_loop.call_soon_threadsafe(started.set)
            try:
                if not release.wait(timeout=5):
                    raise TimeoutError("prediction was never released")
                return payload
            finally:
                with state_lock:
                    active -= 1

    def submit(executor, fn, *args, **kwargs):
        future = original_submit(executor, fn, *args, **kwargs)
        submitted.append(future)
        if len(submitted) == count:
            all_submitted.set()
        return future

    model = BlockingModel("blocking", offload_sync_predict=True)
    model.ready = True
    previous_config = model_server.kserve_context.get_predictor_config()
    server = ModelServer(max_asyncio_workers=configured_workers, enable_grpc=False)
    handler = Mock()
    server.register_exception_handler(handler)
    monkeypatch.setattr(server, "register_signal_handler", Mock())

    async def serve():
        nonlocal serving_loop, started, all_submitted
        serving_loop = asyncio.get_running_loop()
        started, all_submitted = asyncio.Event(), asyncio.Event()
        tasks = [asyncio.create_task(model({"value": i})) for i in range(count)]
        try:
            await asyncio.wait_for(all_submitted.wait(), timeout=2)
            await asyncio.wait_for(started.wait(), timeout=2)
            assert sum(future.running() for future in submitted) == expected_workers
            assert serving_loop.get_exception_handler() is handler
        finally:
            release.set()
            results = await asyncio.wait_for(asyncio.gather(*tasks), timeout=5)
        assert [result[0] for result in results] == [{"value": i} for i in range(count)]

    monkeypatch.setattr(ThreadPoolExecutor, "submit", submit)
    monkeypatch.setattr(
        model_server, "RESTServer", Mock(return_value=Mock(start=serve))
    )
    # start() must configure asyncio.run's loop, not this pre-existing one.
    previous_loop = asyncio.new_event_loop()
    asyncio.set_event_loop(previous_loop)
    try:
        server.start([model])
        assert serving_loop is not previous_loop
        assert peak <= expected_workers
        assert Model._shared_executor is None
    finally:
        release.set()
        previous_loop.run_until_complete(previous_loop.shutdown_default_executor())
        previous_loop.close()
        asyncio.set_event_loop(None)
        model_server.kserve_context.set_predictor_config(previous_config)


def test_model_server_ssl_switches_to_https_port():
    server = ModelServer(
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 8443


def test_model_server_ssl_keeps_explicit_port():
    server = ModelServer(
        http_port=9000,
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 9000


def test_model_server_ssl_keeps_explicit_default_http_port():
    server = ModelServer(
        http_port=8080,
        ssl_certfile="/etc/tls/private/tls.crt",
        ssl_keyfile="/etc/tls/private/tls.key",
    )
    assert server.http_port == 8080


def test_model_server_no_ssl_keeps_default_port():
    server = ModelServer()
    assert server.http_port == 8080


@pytest.mark.parametrize(
    "ssl_certfile,ssl_keyfile",
    [
        ("/etc/tls/private/tls.crt", None),
        (None, "/etc/tls/private/tls.key"),
    ],
)
def test_model_server_rejects_partial_ssl_configuration(ssl_certfile, ssl_keyfile):
    with pytest.raises(
        ValueError, match="ssl_certfile and ssl_keyfile must be configured together"
    ):
        ModelServer(ssl_certfile=ssl_certfile, ssl_keyfile=ssl_keyfile)


class _RecordingModel(Model):
    def __init__(self, name, events):
        super().__init__(name)
        self.events = events
        self.ready = True

    def stop(self):
        self.events.append("model stopped")
        super().stop()


class _DrainingServer:
    def __init__(self, events, drain):
        self.events = events
        self.drain = drain

    async def stop(self, sig):
        self.events.append("drain started")
        await self.drain.wait()
        self.events.append("drain finished")


async def _wait_until(predicate):
    for _ in range(50):
        if predicate():
            return
        await asyncio.sleep(0.01)


@pytest.mark.asyncio
@pytest.mark.parametrize("server_attr", ["_rest_multiprocess_server", "_grpc_server"])
async def test_model_server_stop_unloads_models_after_drain(server_attr):
    events = []
    drain = asyncio.Event()
    server = ModelServer(registered_models=ModelRepository())
    server.register_model(_RecordingModel("test-model", events))
    setattr(server, server_attr, _DrainingServer(events, drain))

    server.stop(signal.SIGTERM)
    await asyncio.sleep(0.1)
    # In-flight requests may still use the model while the server drains.
    assert events == ["drain started"]

    drain.set()
    await _wait_until(lambda: "model stopped" in events)
    assert events == ["drain started", "drain finished", "model stopped"]


@pytest.mark.asyncio
async def test_model_server_stop_unloads_models_when_server_stop_fails():
    events = []

    class FailingGRPCServer:
        async def stop(self, sig):
            raise RuntimeError("grpc stop failed")

    server = ModelServer(registered_models=ModelRepository())
    server.register_model(_RecordingModel("test-model", events))
    server._grpc_server = FailingGRPCServer()

    server.stop(signal.SIGTERM)
    await _wait_until(lambda: "model stopped" in events)
    assert events == ["model stopped"]
