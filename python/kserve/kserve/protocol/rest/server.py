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

import asyncio
import logging
import socket
from importlib import metadata
from typing import Dict, List, Optional, Union

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.responses import ORJSONResponse
from fastapi.routing import APIRoute as FastAPIRoute
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
)
from kserve.logging import trace_logger
from kserve.protocol.dataplane import DataPlane

from .v1_endpoints import V1Endpoints
from .v2_datamodels import (
    InferenceResponse,
    ModelMetadataResponse,
    ModelReadyResponse,
    ServerLiveResponse,
    ServerMetadataResponse,
    ServerReadyResponse,
    ListModelsResponse,
)
from .v2_endpoints import V2Endpoints
from .openai_model import OpenAIModel


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
    def __init__(
        self, data_plane: DataPlane, model_repository_extension, enable_docs_url=False
    ):
        self.dataplane = data_plane
        self.model_repository_extension = model_repository_extension
        self.enable_docs_url = enable_docs_url

    def create_application(self) -> FastAPI:
        """Create a KServe ModelServer application with API routes.

        Returns:
            FastAPI: An application instance.
        """
        v1_endpoints = V1Endpoints(self.dataplane, self.model_repository_extension)
        v2_endpoints = V2Endpoints(self.dataplane, self.model_repository_extension)
        openai_model = OpenAIModel()

        return FastAPI(
            title="KServe ModelServer",
            version=metadata.version("kserve"),
            docs_url="/docs" if self.enable_docs_url else None,
            redoc_url=None,
            default_response_class=ORJSONResponse,
            routes=[
                # Server Liveness API returns 200 if server is alive.
                FastAPIRoute(r"/", self.dataplane.live),
                # Metrics
                FastAPIRoute(r"/metrics", metrics_handler, methods=["GET"]),
                # V1 Inference Protocol
                FastAPIRoute(r"/v1/models", v1_endpoints.models, tags=["V1"]),
                # Model Health API returns 200 if model is ready to serve.
                FastAPIRoute(
                    r"/v1/models/{model_name}", v1_endpoints.model_ready, tags=["V1"]
                ),
                # Note: Set response_model to None to resolve fastapi Response issue
                # https://fastapi.tiangolo.com/tutorial/response-model/#disable-response-model
                FastAPIRoute(
                    r"/v1/models/{model_name}:predict",
                    v1_endpoints.predict,
                    methods=["POST"],
                    tags=["V1"],
                    response_model=None,
                ),
                FastAPIRoute(
                    r"/v1/models/{model_name}:explain",
                    v1_endpoints.explain,
                    methods=["POST"],
                    tags=["V1"],
                    response_model=None,
                ),
                # V2 Inference Protocol
                # https://github.com/kserve/kserve/tree/master/docs/predict-api/v2
                FastAPIRoute(
                    r"/v2",
                    v2_endpoints.metadata,
                    response_model=ServerMetadataResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/health/live",
                    v2_endpoints.live,
                    response_model=ServerLiveResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/health/ready",
                    v2_endpoints.ready,
                    response_model=ServerReadyResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models",
                    v2_endpoints.models,
                    response_model=ListModelsResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}",
                    v2_endpoints.model_metadata,
                    response_model=ModelMetadataResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/versions/{model_version}",
                    v2_endpoints.model_metadata,
                    tags=["V2"],
                    include_in_schema=False,
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/ready",
                    v2_endpoints.model_ready,
                    response_model=ModelReadyResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"v2/models/{model_name}/versions/{model_version}/ready",
                    v2_endpoints.model_ready,
                    response_model=ModelReadyResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/infer",
                    v2_endpoints.infer,
                    methods=["POST"],
                    response_model=InferenceResponse,
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/generate",
                    v2_endpoints.generate,
                    methods=["POST"],
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/generate_stream",
                    v2_endpoints.generate_stream,
                    methods=["POST"],
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/models/{model_name}/versions/{model_version}/infer",
                    v2_endpoints.infer,
                    methods=["POST"],
                    tags=["V2"],
                    include_in_schema=False,
                ),
                FastAPIRoute(
                    r"/v2/repository/models/{model_name}/load",
                    v2_endpoints.load,
                    methods=["POST"],
                    tags=["V2"],
                ),
                FastAPIRoute(
                    r"/v2/repository/models/{model_name}/unload",
                    v2_endpoints.unload,
                    methods=["POST"],
                    tags=["V2"],
                ),
                # OpenAI Protocol
                FastAPIRoute(
                    r"/v1/chat/completions",
                    openai_model.create_chat_completion,
                    methods=["POST"],
                    tags=["OpenAI"],
                ),
                FastAPIRoute(
                    r"/v1/completions",
                    openai_model.create_completion,
                    methods=["POST"],
                    tags=["OpenAI"],
                ),
            ],
            exception_handlers={
                InvalidInput: invalid_input_handler,
                InferenceError: inference_error_handler,
                ModelNotFound: model_not_found_handler,
                ModelNotReady: model_not_ready_handler,
                NotImplementedError: not_implemented_error_handler,
                Exception: generic_exception_handler,
            },
        )


class UvicornServer:
    def __init__(
        self,
        http_port: int,
        sockets: List[socket.socket],
        data_plane: DataPlane,
        model_repository_extension,
        enable_docs_url,
        log_config: Optional[Union[str, Dict]] = None,
        access_log_format: Optional[str] = None,
    ):
        super().__init__()
        self.sockets = sockets
        rest_server = RESTServer(
            data_plane, model_repository_extension, enable_docs_url
        )
        app = rest_server.create_application()
        app.add_middleware(
            TimingMiddleware,
            client=PrintTimings(),
            metric_namer=StarletteScopeToName(prefix="kserve.io", starlette_app=app),
        )
        self.cfg = uvicorn.Config(
            app=app,
            host="0.0.0.0",
            log_config=log_config,
            port=http_port,
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

    def run_sync(self):
        server = uvicorn.Server(config=self.cfg)
        asyncio.run(server.serve(sockets=self.sockets))

    async def run(self):
        await self.server.serve()

    async def stop(self, sig: Optional[int] = None):
        if self.server:
            self.server.handle_exit(sig=sig, frame=None)
