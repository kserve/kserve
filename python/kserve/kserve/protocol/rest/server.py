# Copyright 2022 The KServe Authors.
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

import logging
from socket import socket
import sys
from typing import List, Optional

import fastapi
import uvicorn
from fastapi import Request, Response
from fastapi.routing import APIRouter
from prometheus_client import REGISTRY, exposition
from timing_asgi import TimingClient, TimingMiddleware
from timing_asgi.integrations import StarletteScopeToName
from uvicorn.importer import import_from_string, ImportFromStringError

from kserve.errors import (
    InferenceError,
    InvalidInput,
    ModelNotFound,
    ModelNotReady,
    ServerNotLive,
    ServerNotReady,
    UnsupportedProtocol,
    generic_exception_handler,
    inference_error_handler,
    invalid_input_handler,
    model_not_found_handler,
    model_not_ready_handler,
    not_implemented_error_handler,
    server_not_live_handler,
    server_not_ready_handler,
    unsupported_protocol_error_handler,
)
from kserve.logging import trace_logger, logger
from kserve.protocol.dataplane import DataPlane

from .v1_endpoints import register_v1_endpoints
from .v2_endpoints import register_v2_endpoints
from ..model_repository_extension import ModelRepositoryExtension


async def metrics_handler(request: Request) -> Response:
    encoder, content_type = exposition.choose_encoder(request.headers.get("accept"))
    return Response(content=encoder(REGISTRY), headers={"content-type": content_type})


class PrintTimings(TimingClient):
    def timing(self, metric_name, timing, tags):
        trace_logger.info(f"{metric_name}: {timing} {tags}")


class RESTServer:
    def __init__(
        self,
        app: str,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
        http_port: int,
        access_log_format: Optional[str] = None,
        workers: int = 1,
        grace_period: int = 30,
    ):
        self.dataplane = data_plane
        self.model_repository_extension = model_repository_extension
        self.access_log_format = access_log_format
        self.config = uvicorn.Config(
            app,
            host="0.0.0.0",
            port=http_port,
            workers=workers,
            # By setting log_config to None we tell Uvicorn not to configure logging as it is already
            # configured by kserve.
            log_config=None,
            timeout_graceful_shutdown=grace_period,
            loop="asyncio",
        )
        self._server = uvicorn.Server(self.config)

    def _register_endpoints(self, app: fastapi.FastAPI):
        root_router = APIRouter()
        root_router.add_api_route(r"/", self.dataplane.live)
        root_router.add_api_route(r"/metrics", metrics_handler, methods=["GET"])
        app.include_router(root_router)
        register_v1_endpoints(app, self.dataplane, self.model_repository_extension)
        register_v2_endpoints(app, self.dataplane, self.model_repository_extension)
        # Register OpenAI endpoints if any of the models in the registry implement the OpenAI interface
        # This adds /openai/v1/completions and /openai/v1/chat/completions routes to the
        # REST server.
        try:
            from kserve.protocol.rest.openai.config import (
                maybe_register_openai_endpoints,
            )

            maybe_register_openai_endpoints(app, self.dataplane.model_registry)
            logger.info("OpenAI endpoints registered")
        except ImportError:
            logger.info("OpenAI endpoints not registered")

    def _add_exception_handlers(self, app: fastapi.FastAPI):
        app.add_exception_handler(InvalidInput, invalid_input_handler)
        app.add_exception_handler(InferenceError, inference_error_handler)
        app.add_exception_handler(ModelNotFound, model_not_found_handler)
        app.add_exception_handler(ModelNotReady, model_not_ready_handler)
        app.add_exception_handler(NotImplementedError, not_implemented_error_handler)
        app.add_exception_handler(
            UnsupportedProtocol, unsupported_protocol_error_handler
        )
        app.add_exception_handler(ServerNotLive, server_not_live_handler)
        app.add_exception_handler(ServerNotReady, server_not_ready_handler)
        app.add_exception_handler(Exception, generic_exception_handler)

    def _add_middlewares(self, app: fastapi.FastAPI):
        app.add_middleware(
            TimingMiddleware,
            client=PrintTimings(),
            metric_namer=StarletteScopeToName(prefix="kserve.io", starlette_app=app),
        )

        # More context in https://github.com/encode/uvicorn/pull/947
        # At the time of writing the ASGI specs are not clear when it comes
        # to change the access log format, and hence the Uvicorn upstream devs
        # chose to create a custom middleware for this.
        # The allowed log format is specified in https://github.com/Kludex/asgi-logger#usage
        if self.access_log_format:
            from asgi_logger import AccessLoggerMiddleware

            # As indicated by the asgi-logger docs, we need to clear/unset
            # any setting for uvicorn.access to avoid log duplicates.
            logging.getLogger("uvicorn.access").handlers = []
            app.add_middleware(AccessLoggerMiddleware, format=self.access_log_format)
            # The asgi-logger settings don't set propagate to False,
            # so we get duplicates if we don't set it explicitly.
            logging.getLogger("access").propagate = False

    def create_application(self):
        try:
            app = import_from_string("kserve.model_server:app")
        except ImportFromStringError as exc:
            logger.error("Error loading ASGI app. %s", exc)
            sys.exit(1)
        self._add_middlewares(app)
        self._register_endpoints(app)
        self._add_exception_handlers(app)

    async def start(self):
        """Starts the server without configuring the event loop."""
        self.create_application()
        logger.info("Starting uvicorn with %s workers", self.config.workers)
        await self._server.serve()

    def run(self, sockets: Optional[List[socket]] = None):
        """Configures event loop and starts the server."""
        self.create_application()
        self._server.run(sockets=sockets)
