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

import argparse
import asyncio
import logging
from distutils.util import strtobool
from typing import List, Dict, Union

import pkg_resources
import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.routing import APIRoute as FastAPIRoute
from fastapi.responses import ORJSONResponse
from prometheus_client import REGISTRY, exposition
from ray import serve
from ray.serve.api import Deployment, RayServeHandle

import kserve.errors as errors
from kserve import Model
from kserve.handlers import V1Endpoints, V2Endpoints
from kserve.handlers.dataplane import DataPlane
from kserve.handlers.model_repository_extension import ModelRepositoryExtension
from kserve.handlers.v2_datamodels import InferenceResponse, ServerMetadataResponse, ServerLiveResponse, \
    ServerReadyResponse, ModelMetadataResponse
from kserve.model_repository import ModelRepository


DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument("--http_port", default=DEFAULT_HTTP_PORT, type=int,
                    help="The HTTP Port listened to by the model server.")
parser.add_argument("--grpc_port", default=DEFAULT_GRPC_PORT, type=int,
                    help="The GRPC Port listened to by the model server.")
parser.add_argument("--workers", default=1, type=int,
                    help="The number of works to fork.")
parser.add_argument("--enable_docs_url", default=False, type=lambda x: bool(strtobool(x)),
                    help="Enable docs url '/docs' to display Swagger UI.")
parser.add_argument("--enable_latency_logging", default=False, type=lambda x: bool(strtobool(x)),
                    help="Output a log per request with latency metrics.")

args, _ = parser.parse_known_args()


async def metrics_handler(request: Request) -> Response:
    encoder, content_type = exposition.choose_encoder(request.headers.get("accept"))
    return Response(content=encoder(REGISTRY), headers={"content-type": content_type})


class ModelServer:
    """KServe ModelServer

    Args:
        http_port (int): HTTP port. Default: ``8080``.
        grpc_port (int): GRPC port. Default: ``8081``.
        workers (int): Number of workers for uvicorn. Default: ``1``.
        registered_models (ModelRepository): Model repository with registered models.
        enable_docs_url (bool): Whether to turn on ``/docs`` Swagger UI. Default: ``False``.
        enable_latency_logging (bool): Whether to log latency metric. Default: ``False``.
    """

    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 workers: int = args.workers,
                 registered_models: ModelRepository = ModelRepository(),
                 enable_docs_url: bool = args.enable_docs_url,
                 enable_latency_logging: bool = args.enable_latency_logging):
        dataplane = handlers.DataPlane(model_registry=registered_models)
        self._grpc_server = GRPCServer(port=grpc_port, data_plane=dataplane)
        self.registered_models = registered_models
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.workers = workers
        self._server = None
        self.enable_docs_url = enable_docs_url
        self.enable_latency_logging = enable_latency_logging

    def create_application(self) -> FastAPI:
        """Create a KServe ModelServer application with API routes.

        Returns:
            FastAPI: An application instance.
        """
        dataplane = DataPlane(model_registry=self.registered_models)
        model_repository_extension = ModelRepositoryExtension(model_registry=self.registered_models)
        v1_endpoints = V1Endpoints(dataplane, model_repository_extension)
        v2_endpoints = V2Endpoints(dataplane, model_repository_extension)

        return FastAPI(
            title="KServe ModelServer",
            version=pkg_resources.get_distribution("kserve").version,
            docs_url="/docs" if self.enable_docs_url else None,
            redoc_url=None,
            default_response_class=ORJSONResponse,
            routes=[
                # Server Liveness API returns 200 if server is alive.
                FastAPIRoute(r"/", dataplane.live),
                # Metrics
                FastAPIRoute(r"/metrics", metrics_handler, methods=["GET"]),
                # V1 Inference Protocol
                FastAPIRoute(r"/v1/models", v1_endpoints.models, tags=["V1"]),
                # Model Health API returns 200 if model is ready to serve.
                FastAPIRoute(r"/v1/models/{model_name}", v1_endpoints.model_ready, tags=["V1"]),
                FastAPIRoute(r"/v1/models/{model_name}:predict",
                             v1_endpoints.predict, methods=["POST"], tags=["V1"]),
                FastAPIRoute(r"/v1/models/{model_name}:explain",
                             v1_endpoints.explain, methods=["POST"], tags=["V1"]),
                # V2 Inference Protocol
                # https://github.com/kserve/kserve/tree/master/docs/predict-api/v2
                FastAPIRoute(r"/v2", v2_endpoints.metadata, response_model=ServerMetadataResponse, tags=["V2"]),
                FastAPIRoute(r"/v2/health/live", v2_endpoints.live, response_model=ServerLiveResponse, tags=["V2"]),
                FastAPIRoute(r"/v2/health/ready", v2_endpoints.ready, response_model=ServerReadyResponse, tags=["V2"]),
                FastAPIRoute(r"/v2/models/{model_name}",
                             v2_endpoints.model_metadata, response_model=ModelMetadataResponse, tags=["V2"]),
                FastAPIRoute(r"/v2/models/{model_name}/versions/{model_version}",
                             v2_endpoints.model_metadata, tags=["V2"], include_in_schema=False),
                FastAPIRoute(r"/v2/models/{model_name}/infer",
                             v2_endpoints.infer, methods=["POST"], response_model=InferenceResponse, tags=["V2"]),
                FastAPIRoute(r"/v2/models/{model_name}/versions/{model_version}/infer",
                             v2_endpoints.infer, methods=["POST"], tags=["V2"], include_in_schema=False),
                FastAPIRoute(r"/v2/repository/models/{model_name}/load",
                             v2_endpoints.load, methods=["POST"], tags=["V2"]),
                FastAPIRoute(r"/v2/repository/models/{model_name}/unload",
                             v2_endpoints.unload, methods=["POST"], tags=["V2"]),
            ], exception_handlers={
                errors.InvalidInput: errors.invalid_input_handler,
                errors.InferenceError: errors.inference_error_handler,
                errors.ModelNotFound: errors.model_not_found_handler,
                errors.ModelNotReady: errors.model_not_ready_handler,
                NotImplementedError: errors.not_implemented_error_handler,
                Exception: errors.exception_handler
            }
        )

    async def start(self, models: Union[List[Model], Dict[str, Deployment]]) -> None:
        if isinstance(models, list):
            for model in models:
                if isinstance(model, Model):
                    self.register_model(model)
                    # pass whether to log request latency into the model
                    model.enable_latency_logging = self.enable_latency_logging
                else:
                    raise RuntimeError("Model type should be 'Model'")
        elif isinstance(models, dict):
            if all([isinstance(v, Deployment) for v in models.values()]):
                # TODO: make this port number a variable
                serve.start(detached=True, http_options={"host": "0.0.0.0", "port": 9071})
                for key in models:
                    models[key].deploy()
                    handle = models[key].get_handle()
                    self.register_model_handle(key, handle)
            else:
                raise RuntimeError("Model type should be RayServe Deployment")
        else:
            raise RuntimeError("Unknown model collection types")

        cfg = uvicorn.Config(
            self.create_application(),
            host="0.0.0.0",
            port=self.http_port,
            workers=self.workers
        )

        self._server = uvicorn.Server(cfg)
        servers = [self._server.serve(), self._grpc_server.start(self.workers)]
        servers_task = asyncio.gather(*servers)
        await servers_task

    def register_model_handle(self, name: str, model_handle: RayServeHandle):
        self.registered_models.update_handle(name, model_handle)
        logging.info("Registering model handle: %s", name)

    def register_model(self, model: Model):
        if not model.name:
            raise Exception(
                "Failed to register model, model.name must be provided.")
        self.registered_models.update(model)
        logging.info("Registering model: %s", model.name)
