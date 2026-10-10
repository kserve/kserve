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

import httpx
import pytest
from fastapi import FastAPI

from huggingfaceserver.vllm.engine_health import VLLMEngineHealth
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


class RecoveringEngineClient:
    def __init__(self, failure: str):
        self.failure: str | None = failure
        self._never_set = asyncio.Event()

    async def check_health(self) -> None:
        if self.failure == "timeout":
            await self._never_set.wait()
        elif self.failure == "exception":
            raise RuntimeError("transient health check failure")


class EngineHealthModel(BaseKServeModel):
    server_health_check_enabled = True

    def __init__(self, health: VLLMEngineHealth):
        super().__init__("health-model")
        self.health = health

    async def is_live(self) -> bool:
        return await self.health.is_live()

    async def healthy(self) -> bool:
        return await self.health.is_ready()


@pytest.mark.asyncio
@pytest.mark.parametrize("endpoint", ["live", "ready"])
@pytest.mark.parametrize("failure", ["timeout", "exception"])
async def test_health_endpoints_recover_after_transient_failure(endpoint, failure):
    engine_client = RecoveringEngineClient(failure)
    health = VLLMEngineHealth(timeout_seconds=0.01)
    health.set_engine_client(engine_client)
    health.mark_ready()

    repository = ModelRepository()
    repository.update(EngineHealthModel(health))
    app = FastAPI()
    register_v2_endpoints(app, DataPlane(repository), None)
    app.add_exception_handler(ServerNotLive, server_not_live_handler)
    app.add_exception_handler(ServerNotReady, server_not_ready_handler)

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://health-test"
    ) as client:
        response = await client.get(f"/v2/health/{endpoint}")
        assert response.status_code == 503

        engine_client.failure = None
        assert (await client.get("/v2/health/live")).status_code == 200
        assert (await client.get("/v2/health/ready")).status_code == 200
