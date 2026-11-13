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

import asyncio
from unittest.mock import AsyncMock

import httpx
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from kserve.model import BaseKServeModel
from kserve.model_repository import ModelRepository
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.grpc import grpc_predict_v2_pb2 as pb
from kserve.protocol.grpc.servicer import InferenceServicer
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.server import RESTServer


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

    async def healthy(self) -> bool:
        return False


def create_test_app(model_repository: ModelRepository) -> FastAPI:
    app = FastAPI()
    server = RESTServer(
        "kserve.model_server:app",
        DataPlane(model_repository),
        ModelRepositoryExtension(model_repository),
        http_port=8080,
    )
    server._register_endpoints(app)
    server._add_exception_handlers(app)
    return app


def create_test_client(model: BaseKServeModel) -> TestClient:
    model_repository = ModelRepository()
    model_repository.update(model)
    app = create_test_app(model_repository)
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


@pytest.mark.parametrize("live_status", [True, False])
def test_root_route_returns_the_liveness_http_status(live_status):
    model = ServerHealthModel("health-model")
    model.live_status = live_status
    client = create_test_client(model)

    response = client.get("/")

    assert response.status_code == (200 if live_status else 503)
    if live_status:
        assert response.json() == {"status": "alive"}


def test_non_opted_in_model_preserves_server_health_behavior():
    client = create_test_client(LegacyModel("legacy-model"))

    assert client.get("/v2/health/live").status_code == 200
    assert client.get("/v2/health/ready").status_code == 200


def test_readiness_opt_in_does_not_disable_liveness():
    model = ServerHealthModel("health-model")
    model.server_health_check_enabled = False
    model.live_status = False
    model.ready_status = False
    client = create_test_client(model)

    assert client.get("/").status_code == 503
    assert client.get("/v2/health/live").status_code == 503
    assert client.get("/v2/health/ready").status_code == 200


def test_server_readiness_uses_the_active_probe_hook():
    class ActiveReadinessModel(ServerHealthModel):
        async def healthy(self) -> bool:
            return True

        async def is_ready(self) -> bool:
            return False

    client = create_test_client(ActiveReadinessModel("health-model"))

    assert client.get("/v2/models/health-model/ready").status_code == 200
    assert client.get("/v2/health/ready").status_code == 503


@pytest.mark.asyncio
async def test_instance_liveness_checks_the_repository():
    model = ServerHealthModel("health-model")
    model.live_status = False
    repository = ModelRepository()
    repository.update(model)

    assert await DataPlane.live() == {"status": "alive"}
    assert await DataPlane(repository).is_live() is False


@pytest.mark.asyncio
@pytest.mark.parametrize("live_status", [True, False])
@pytest.mark.parametrize("readiness_opt_in", [True, False])
async def test_grpc_liveness_checks_the_repository(live_status, readiness_opt_in):
    model = ServerHealthModel("health-model")
    model.live_status = live_status
    model.server_health_check_enabled = readiness_opt_in
    repository = ModelRepository()
    repository.update(model)
    servicer = InferenceServicer(DataPlane(repository), None)

    response = await servicer.ServerLive(pb.ServerLiveRequest(), None)

    assert response.live is live_status


@pytest.mark.asyncio
@pytest.mark.parametrize("probe_name", ["is_server_live", "is_server_ready"])
async def test_server_health_checks_aliased_models_once(probe_name):
    model = ServerHealthModel("health-model")
    model.is_live = AsyncMock(return_value=True)
    model.healthy = AsyncMock(return_value=True)
    repository = ModelRepository()
    repository.update(model)
    repository.update(model, "alias")

    assert await getattr(repository, probe_name)() is True
    check = model.is_live if probe_name == "is_server_live" else model.healthy
    check.assert_awaited_once()


@pytest.mark.asyncio
@pytest.mark.parametrize("endpoint", ["/", "/v2/health/live", "/v2/health/ready"])
@pytest.mark.parametrize("mutation", ["load", "unload"])
async def test_model_repository_changes_during_a_probe(endpoint, mutation):
    started = asyncio.Event()
    release = asyncio.Event()

    async def paused_health_check():
        started.set()
        await release.wait()
        return True

    first_model = ServerHealthModel("paused-model")
    first_model.is_live = paused_health_check
    first_model.healthy = paused_health_check
    other_model = ServerHealthModel("other-model")
    repository = ModelRepository()
    repository.update(first_model)
    repository.update(other_model)
    app = create_test_app(repository)

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app, raise_app_exceptions=False),
        base_url="http://health-test",
    ) as client:
        probe = asyncio.create_task(client.get(endpoint))
        try:
            await asyncio.wait_for(started.wait(), timeout=2)
            if mutation == "load":
                new_model = ServerHealthModel("new-model")
                new_model.live_status = False
                new_model.ready_status = False
                repository.update(new_model)
            else:
                repository.unload(other_model.name)
        finally:
            release.set()
            response = await asyncio.wait_for(probe, timeout=2)

        assert response.status_code == 200
        assert (await client.get(endpoint)).status_code == (
            503 if mutation == "load" else 200
        )


@pytest.mark.asyncio
@pytest.mark.parametrize("probe_name", ["is_server_live", "is_server_ready"])
async def test_server_health_stops_after_the_first_failed_model(probe_name):
    first_model = ServerHealthModel("failed-model")
    first_model.live_status = False
    first_model.ready_status = False
    other_model = ServerHealthModel("other-model")
    other_model.is_live = AsyncMock(return_value=True)
    other_model.healthy = AsyncMock(return_value=True)
    repository = ModelRepository()
    repository.update(first_model)
    repository.update(other_model)

    assert await getattr(repository, probe_name)() is False
    other_model.is_live.assert_not_awaited()
    other_model.healthy.assert_not_awaited()


@pytest.mark.asyncio
@pytest.mark.parametrize("probe_name", ["is_server_live", "is_server_ready"])
async def test_server_health_preserves_cancellation(probe_name):
    model = ServerHealthModel("health-model")
    model.is_live = AsyncMock(side_effect=asyncio.CancelledError)
    model.healthy = AsyncMock(side_effect=asyncio.CancelledError)
    repository = ModelRepository()
    repository.update(model)

    with pytest.raises(asyncio.CancelledError):
        await getattr(repository, probe_name)()
