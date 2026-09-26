# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from fastapi import FastAPI
from fastapi.testclient import TestClient

from kserve.errors import (
    ServerNotLive,
    ServerNotReady,
    server_not_live_handler,
    server_not_ready_handler,
)
from kserve.model import BaseKServeModel
from kserve.model_repository import ModelRepository
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.rest.v2_endpoints import register_v2_endpoints


class ServerHealthModel(BaseKServeModel):
    server_health_check_enabled = True

    def __init__(self, name: str):
        super().__init__(name)
        self.live_status = True
        self.ready_status = True
        self.health_error: Exception | None = None

    async def is_live(self) -> bool:
        if self.health_error is not None:
            raise self.health_error
        return self.live_status

    async def healthy(self) -> bool:
        if self.health_error is not None:
            raise self.health_error
        return self.ready_status


class LegacyModel(BaseKServeModel):
    def __init__(self, name: str):
        super().__init__(name)

    async def is_live(self) -> bool:
        return False

    async def healthy(self) -> bool:
        return False


def create_test_client(model: BaseKServeModel) -> TestClient:
    model_repository = ModelRepository()
    model_repository.update(model)
    app = FastAPI()
    register_v2_endpoints(app, DataPlane(model_repository), None)
    app.add_exception_handler(ServerNotLive, server_not_live_handler)
    app.add_exception_handler(ServerNotReady, server_not_ready_handler)
    return TestClient(app)


def test_opted_in_model_controls_server_health():
    model = ServerHealthModel("health-model")
    client = create_test_client(model)

    assert client.get("/v2/health/live").status_code == 200
    assert client.get("/v2/health/ready").status_code == 200

    model.ready_status = False
    assert client.get("/v2/health/live").status_code == 200
    assert client.get("/v2/health/ready").status_code == 503

    model.live_status = False
    assert client.get("/v2/health/live").status_code == 503
    assert client.get("/v2/health/ready").status_code == 503

    model.health_error = RuntimeError("health check failed")
    assert client.get("/v2/health/live").status_code == 503
    assert client.get("/v2/health/ready").status_code == 503


def test_non_opted_in_model_preserves_server_health_behavior():
    client = create_test_client(LegacyModel("legacy-model"))

    assert client.get("/v2/health/live").status_code == 200
    assert client.get("/v2/health/ready").status_code == 200
