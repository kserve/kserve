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

import pytest

from huggingfaceserver.vllm.engine_health import VLLMEngineHealth


class FakeEngineClient:
    def __init__(self, error: Exception | None = None, block: bool = False):
        self.error = error
        self.block = block
        self.check_count = 0
        self._never_set = asyncio.Event()

    async def check_health(self) -> None:
        self.check_count += 1
        if self.block:
            await self._never_set.wait()
        if self.error is not None:
            raise self.error


@pytest.mark.asyncio
async def test_healthy_engine_is_live_and_ready():
    client = FakeEngineClient()
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)
    health.mark_ready()

    assert await health.is_live() is True
    assert await health.is_ready() is True
    assert client.check_count == 2


@pytest.mark.asyncio
async def test_initializing_engine_is_live_but_not_ready():
    client = FakeEngineClient()
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)

    assert await health.is_live() is True
    assert await health.is_ready() is False
    assert client.check_count == 1


@pytest.mark.asyncio
async def test_initializing_before_client_exists_is_live_but_not_ready():
    health = VLLMEngineHealth(timeout_seconds=0.1)

    assert await health.is_live() is True
    assert await health.is_ready() is False


@pytest.mark.asyncio
async def test_worker_failure_during_initialization_fails_liveness():
    client = FakeEngineClient(error=RuntimeError("worker exited during startup"))
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)

    assert await health.is_live() is False
    assert await health.is_ready() is False
    assert client.check_count == 1


@pytest.mark.asyncio
async def test_background_loop_failure_is_latched_for_live_and_ready():
    client = FakeEngineClient(error=RuntimeError("background loop failed"))
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)
    health.mark_ready()

    assert await health.is_ready() is False
    assert await health.is_live() is False
    assert client.check_count == 1
    assert isinstance(health.failure, RuntimeError)


@pytest.mark.asyncio
async def test_worker_exit_is_latched_for_live_and_ready():
    class FakeEngineDeadError(RuntimeError):
        pass

    client = FakeEngineClient(error=FakeEngineDeadError("worker exited"))
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)
    health.mark_ready()

    assert await health.is_live() is False
    assert await health.is_ready() is False
    assert client.check_count == 1
    assert isinstance(health.failure, FakeEngineDeadError)


@pytest.mark.asyncio
async def test_health_check_timeout_fails_closed_without_hanging():
    client = FakeEngineClient(block=True)
    health = VLLMEngineHealth(timeout_seconds=0.01)
    health.set_engine_client(client)
    health.mark_ready()

    assert await asyncio.wait_for(health.is_live(), timeout=0.2) is False
    assert await health.is_ready() is False
    assert client.check_count == 1
    assert isinstance(health.failure, TimeoutError)


@pytest.mark.asyncio
async def test_health_check_exception_fails_closed():
    client = FakeEngineClient(error=ValueError("scheduler failed"))
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)
    health.mark_ready()

    assert await health.is_live() is False
    assert await health.is_ready() is False
    assert client.check_count == 1


@pytest.mark.asyncio
async def test_concurrent_success_cannot_race_past_a_latched_failure():
    class RacingEngineClient:
        def __init__(self):
            self.check_count = 0
            self.first_check_started = asyncio.Event()
            self.release_first_check = asyncio.Event()

        async def check_health(self) -> None:
            self.check_count += 1
            if self.check_count == 1:
                self.first_check_started.set()
                await self.release_first_check.wait()
                return
            raise RuntimeError("concurrent worker failure")

    client = RacingEngineClient()
    health = VLLMEngineHealth(timeout_seconds=1.0)
    health.set_engine_client(client)
    health.mark_ready()

    first_probe = asyncio.create_task(health.is_live())
    await client.first_check_started.wait()
    assert await health.is_ready() is False
    client.release_first_check.set()

    assert await first_probe is False
    assert isinstance(health.failure, RuntimeError)


@pytest.mark.asyncio
async def test_startup_failure_fails_live_and_ready():
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.mark_failed(RuntimeError("engine startup failed"))

    assert await health.is_live() is False
    assert await health.is_ready() is False


@pytest.mark.asyncio
async def test_missing_public_health_api_fails_closed():
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(object())  # type: ignore[arg-type]
    health.mark_ready()

    assert await health.is_live() is False
    assert await health.is_ready() is False
    assert isinstance(health.failure, RuntimeError)


@pytest.mark.asyncio
async def test_graceful_shutdown_is_not_latched_as_engine_failure():
    client = FakeEngineClient(error=RuntimeError("client is shutting down"))
    health = VLLMEngineHealth(timeout_seconds=0.1)
    health.set_engine_client(client)
    health.mark_ready()
    health.mark_stopping()

    assert await health.is_live() is True
    assert await health.is_ready() is False
    assert health.failure is None
    assert client.check_count == 0
