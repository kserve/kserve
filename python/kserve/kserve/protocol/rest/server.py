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
from typing import Dict, Optional, Union

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.routing import APIRouter
from prometheus_client import REGISTRY, exposition
from timing_asgi import TimingClient, TimingMiddleware
from timing_asgi.integrations import StarletteScopeToName

from kserve.errors import (
    InferenceError,
    InvalidInput,
    ModelNotFound,
    ModelNotReady,
    generic_exception_handler,
    inference_error_handler,
    invalid_input_handler,
    model_not_found_handler,
    model_not_ready_handler,
    not_implemented_error_handler,
    UnsupportedProtocol,
    unsupported_protocol_error_handler,
)
from kserve.logging import trace_logger
from kserve.protocol.dataplane import DataPlane

from .openai.config import maybe_register_openai_endpoints
from .v1_endpoints import register_v1_endpoints
from .v2_endpoints import register_v2_endpoints


async def metrics_handler(request: Request) -> Response:
    encoder, content_type = exposition.choose_encoder(request.headers.get("accept"))
    return Response(content=encoder(REGISTRY), headers={"content-type": content_type})


class PrintTimings(TimingClient):
    def timing(self, metric_name, timing, tags):
        trace_logger.info(f"{metric_name}: {timing}")


class _NoSignalUvicornServer(uvicorn.Server):
    def install_signal_handlers(self) -> None:
        pass


class RESTServer:
    def __init__(self, app: FastAPI, data_plane: DataPlane, model_repository_extension):
        self.app = app
        self.dataplane = data_plane
        self.model_repository_extension = model_repository_extension

    def create_application(self):
        """Create a KServe ModelServer application with API routes."""
        root_router = APIRouter()
        root_router.add_api_route(r"/", self.dataplane.live)
        root_router.add_api_route(r"/metrics", metrics_handler, methods=["GET"])
        self.app.include_router(root_router)
        register_v1_endpoints(self.app, self.dataplane, self.model_repository_extension)
        register_v2_endpoints(self.app, self.dataplane, self.model_repository_extension)
        # Register OpenAI endpoints if any of the models in the registry implement the OpenAI interface
        # This adds /openai/v1/completions and /openai/v1/chat/completions routes to the
        # REST server.
        maybe_register_openai_endpoints(self.app, self.dataplane.model_registry)

        # Add exception handlers
        self.app.add_exception_handler(InvalidInput, invalid_input_handler)
        self.app.add_exception_handler(InferenceError, inference_error_handler)
        self.app.add_exception_handler(ModelNotFound, model_not_found_handler)
        self.app.add_exception_handler(ModelNotReady, model_not_ready_handler)
        self.app.add_exception_handler(
            NotImplementedError, not_implemented_error_handler
        )
        self.app.add_exception_handler(
            UnsupportedProtocol, unsupported_protocol_error_handler
        )
        self.app.add_exception_handler(Exception, generic_exception_handler)


class UvicornServer:
    def __init__(
        self,
        app: FastAPI,
        http_port: int,
        data_plane: DataPlane,
        model_repository_extension,
        log_config: Optional[Union[str, Dict]] = None,
        access_log_format: Optional[str] = None,
        workers: int = 1,
    ):
        super().__init__()
        rest_server = RESTServer(app, data_plane, model_repository_extension)
        rest_server.create_application()
        app.add_middleware(
            TimingMiddleware,
            client=PrintTimings(),
            metric_namer=StarletteScopeToName(prefix="kserve.io", starlette_app=app),
        )
        self.cfg = uvicorn.Config(
            app="kserve.model_server:app",
            host="0.0.0.0",
            log_config=log_config,
            port=http_port,
            workers=workers,
        )

        # More context in https://github.com/encode/uvicorn/pull/947
        # At the time of writing the ASGI specs are not clear when it comes
        # to change the access log format, and hence the Uvicorn upstream devs
        # chose to create a custom middleware for this.
        # The allowed log format is specified in https://github.com/Kludex/asgi-logger#usage
        if access_log_format:
            from asgi_logger import AccessLoggerMiddleware

            # As indicated by the asgi-logger docs, we need to clear/unset
            # any setting for uvicorn.access to avoid log duplicates.
            logging.getLogger("uvicorn.access").handlers = []
            app.add_middleware(AccessLoggerMiddleware, format=access_log_format)
            # The asgi-logger settings don't set propagate to False,
            # so we get duplicates if we don't set it explicitly.
            logging.getLogger("access").propagate = False

        self.server = _NoSignalUvicornServer(config=self.cfg)

    async def run(self):
        await self.server.serve()

    async def stop(self, sig: Optional[int] = None):
        if self.server:
            self.server.handle_exit(sig=sig, frame=None)
