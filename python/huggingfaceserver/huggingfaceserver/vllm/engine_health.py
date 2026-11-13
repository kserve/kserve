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
from typing import Optional, Protocol

from kserve.logging import logger


DEFAULT_VLLM_HEALTH_CHECK_TIMEOUT_SECONDS = 0.5


class HealthCheckableEngineClient(Protocol):
    """Public portion of vLLM's EngineClient used by health probes."""

    async def check_health(self) -> None:
        """Raise when the engine cannot continue serving requests."""


class VLLMEngineHealth:
    """Tracks vLLM initialization and fatal engine failures.

    vLLM's public ``EngineClient.check_health`` method reports both a failed
    AsyncLLM output handler and an EngineCore/worker process failure through
    ``EngineDeadError``. This explicit engine-death error is latched. Timeouts
    and other transient exceptions only fail readiness, allowing later probes
    to retry and recover without triggering a container restart.
    """

    def __init__(self, timeout_seconds: float):
        if timeout_seconds <= 0:
            raise ValueError("vLLM health check timeout must be greater than zero")

        # Other backends import this module for the default timeout, so require
        # the vLLM exception API only when an engine health tracker is created.
        try:
            from vllm.v1.engine.exceptions import EngineDeadError
        except ImportError as error:
            message = (
                "Cannot initialize vLLM health checks: EngineDeadError is "
                "unavailable. Install a supported vLLM version."
            )
            logger.error(message, exc_info=error)
            raise RuntimeError(message) from error

        self._engine_dead_error = EngineDeadError
        self._timeout_seconds = timeout_seconds
        self._engine_client: Optional[HealthCheckableEngineClient] = None
        self._ready = False
        self._stopping = False
        self._failure: Optional[BaseException] = None

    @property
    def failure(self) -> Optional[BaseException]:
        return self._failure

    @property
    def ready(self) -> bool:
        """Return inference readiness without actively probing the engine."""
        return self._ready and not self._stopping and self._failure is None

    def set_engine_client(self, engine_client: HealthCheckableEngineClient) -> None:
        self._engine_client = engine_client

    def mark_ready(self) -> None:
        if self._engine_client is None:
            raise RuntimeError("vLLM engine cannot become ready without a client")
        if self._failure is None and not self._stopping:
            self._ready = True

    def mark_failed(self, error: BaseException) -> None:
        if self._stopping or self._failure is not None:
            return
        self._ready = False
        self._failure = error
        logger.error("vLLM engine entered an unrecoverable state: %s", error)

    def mark_stopping(self) -> None:
        self._ready = False
        self._stopping = True

    async def is_live(self) -> bool:
        if self._failure is not None:
            return False
        if self._stopping:
            # Preserve graceful shutdown semantics. Kubernetes already stops
            # probing a container after it begins termination.
            return True
        if self._engine_client is None:
            # Model initialization before a client exists is a readiness
            # concern, not a reason to restart a container loading a model.
            return True
        # Once the client exists, check it even before readiness so a worker
        # that dies during the remainder of initialization is not hidden.
        await self._check_engine()
        return self._failure is None

    async def is_ready(self) -> bool:
        if not self.ready:
            return False
        return await self._check_engine()

    async def _check_engine(self) -> bool:
        if self._failure is not None:
            return False
        if self._stopping:
            return False

        engine_client = self._engine_client
        if engine_client is None:
            self.mark_failed(RuntimeError("vLLM engine client is unavailable"))
            return False

        check_health = getattr(engine_client, "check_health", None)
        if not callable(check_health):
            self.mark_failed(
                RuntimeError(
                    "vLLM EngineClient does not expose the public check_health API"
                )
            )
            return False

        try:
            await asyncio.wait_for(check_health(), timeout=self._timeout_seconds)
        except asyncio.CancelledError:
            raise
        except asyncio.TimeoutError as error:
            logger.error(
                "vLLM engine health check timed out after %g seconds",
                self._timeout_seconds,
                exc_info=error,
            )
            return False
        except Exception as error:
            if isinstance(error, self._engine_dead_error):
                self.mark_failed(error)
            logger.error("vLLM engine health check failed", exc_info=error)
            return False

        # Another concurrent probe may have latched a failure while this check
        # was in flight. Never overwrite or race past that terminal state.
        return self._failure is None and not self._stopping
