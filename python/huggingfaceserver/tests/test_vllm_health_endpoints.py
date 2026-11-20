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
from types import SimpleNamespace
from unittest.mock import AsyncMock

import httpx
import pytest
from fastapi import FastAPI
from vllm.v1.engine.exceptions import EngineDeadError

from huggingfaceserver.vllm.engine_health import VLLMEngineHealth
from huggingfaceserver.vllm.vllm_model import VLLMModel
from kserve.model import BaseKServeModel
from kserve.model_repository import ModelRepository
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest.openai.types import Completion
from kserve.protocol.rest.server import RESTServer


class RecoveringEngineClient:
    def __init__(self, failure: str):
        self.failure: str | None = failure
        self._never_set = asyncio.Event()
        self.check_count = 0

    async def check_health(self) -> None:
        self.check_count += 1
        if self.failure == "timeout":
            await self._never_set.wait()
        elif self.failure == "exception":
            raise RuntimeError("transient health check failure")
        elif self.failure == "dead":
            raise EngineDeadError()


class EngineHealthModel(BaseKServeModel):
    server_health_check_enabled = True

    def __init__(self, health: VLLMEngineHealth):
        super().__init__("health-model")
        self.health = health

    async def is_live(self) -> bool:
        return await self.health.is_live()

    async def healthy(self) -> bool:
        return self.health.ready

    async def is_ready(self) -> bool:
        return await self.health.is_ready()


def create_test_app(model: BaseKServeModel) -> FastAPI:
    repository = ModelRepository()
    repository.update(model)
    app = FastAPI()
    server = RESTServer(
        "kserve.model_server:app",
        DataPlane(repository),
        ModelRepositoryExtension(repository),
        http_port=8080,
    )
    server._register_endpoints(app)
    server._add_exception_handlers(app)
    return app


@pytest.mark.asyncio
@pytest.mark.parametrize("endpoint", ["/", "/v2/health/live", "/v2/health/ready"])
@pytest.mark.parametrize("failure", ["timeout", "exception"])
async def test_health_endpoints_recover_after_transient_failure(endpoint, failure):
    engine_client = RecoveringEngineClient(failure)
    health = VLLMEngineHealth(timeout_seconds=0.01)
    health.set_engine_client(engine_client)
    health.mark_ready()

    app = create_test_app(EngineHealthModel(health))

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://health-test"
    ) as client:
        response = await client.get(endpoint)
        assert response.status_code == (503 if endpoint.endswith("ready") else 200)

        engine_client.failure = None
        assert (await client.get("/")).status_code == 200
        assert (await client.get("/v2/health/live")).status_code == 200
        assert (await client.get("/v2/health/ready")).status_code == 200


@pytest.mark.asyncio
@pytest.mark.parametrize("endpoint", ["/", "/v2/health/live", "/v2/health/ready"])
async def test_engine_death_is_latched_across_all_health_routes(endpoint):
    engine_client = RecoveringEngineClient("dead")
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(engine_client)
    health.mark_ready()
    app = create_test_app(EngineHealthModel(health))

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://health-test"
    ) as client:
        assert (await client.get(endpoint)).status_code == 503
        assert isinstance(health.failure, EngineDeadError)
        engine_client.failure = None
        for route in ("/", "/v2/health/live", "/v2/health/ready"):
            assert (await client.get(route)).status_code == 503

    assert engine_client.check_count == 1


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "state", ["initializing", "ready", "probe_timed_out", "stopping", "dead"]
)
async def test_openai_requests_only_check_engine_state(state):
    engine_client = RecoveringEngineClient("timeout")
    health = VLLMEngineHealth(timeout_seconds=0.01)
    health.set_engine_client(engine_client)
    if state != "initializing":
        health.mark_ready()
    if state == "stopping":
        health.mark_stopping()
    elif state == "dead":
        health.mark_failed(EngineDeadError())

    model = VLLMModel.__new__(VLLMModel)
    BaseKServeModel.__init__(model, "health-model")
    model._engine_health = health
    completion = Completion(
        model=model.name,
        choices=[{"index": 0, "text": "ok", "finish_reason": "stop"}],
        usage={"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    )
    backend = AsyncMock(return_value=completion)
    model.openai_serving_completion = SimpleNamespace(create_completion=backend)
    app = create_test_app(model)

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://health-test"
    ) as client:
        if state == "probe_timed_out":
            assert (await client.get("/v2/health/ready")).status_code == 503
        checks_before_request = engine_client.check_count
        response = await client.post(
            "/openai/v1/completions",
            json={"model": model.name, "prompt": "hello"},
        )

    can_serve = state in ("ready", "probe_timed_out")
    assert response.status_code == (200 if can_serve else 503)
    assert engine_client.check_count == checks_before_request
    if can_serve:
        assert response.json()["choices"][0]["text"] == "ok"
        backend.assert_awaited_once()
    else:
        backend.assert_not_awaited()
