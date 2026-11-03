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

import pytest
from kserve import Model, ModelRepository, ModelServer

UNKNOWN_MODEL_TYPE_ERR_MESSAGE = "Unknown model collection type"


def test_model_server_start_no_models():
    server = ModelServer()

    with pytest.raises(RuntimeError) as exc:
        server.start(models=None)

    assert exc.value.args[0] == UNKNOWN_MODEL_TYPE_ERR_MESSAGE


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
