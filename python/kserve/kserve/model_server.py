# Copyright 2021 The KServe Authors.
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
from typing import List, Dict, Union
from distutils.util import strtobool

import uvicorn
from fastapi import FastAPI, Request, Response
from fastapi.routing import APIRoute as FastAPIRoute
from prometheus_client import REGISTRY, exposition
from ray import serve
from ray.serve.api import Deployment, RayServeHandle

import kserve.errors as errors
import kserve.handlers as handlers
from kserve import Model
from kserve.model_repository import ModelRepository

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT, type=int,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT, type=int,
                    help='The GRPC Port listened to by the model server.')
parser.add_argument('--workers', default=1, type=int,
                    help='The number of works to fork')
parser.add_argument("--enable_latency_logging", default=False, type=lambda x: bool(strtobool(x)),
                    help="Output a log per request with latency metrics")

args, _ = parser.parse_known_args()


async def metrics_handler(request: Request):
    encoder, content_type = exposition.choose_encoder(request.headers.get('accept'))
    return Response(content=encoder(REGISTRY), headers={'content-type': content_type})


class ModelServer:
    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 workers: int = args.workers,
                 registered_models: ModelRepository = ModelRepository(),
                 enable_latency_logging: bool = args.enable_latency_logging):
        self.registered_models = registered_models
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.workers = workers
        self._server = None
        self.enable_latency_logging = enable_latency_logging

    def create_application(self):
        dataplane = handlers.DataPlane(model_registry=self.registered_models)
        return FastAPI(routes=[
            # Metrics
            FastAPIRoute(r"/metrics", metrics_handler, methods=['GET']),
            # Server Liveness API returns 200 if server is alive.
            FastAPIRoute(r"/", dataplane.live),
            # V1 Inference Protocol
            FastAPIRoute(r"/v1/models", dataplane.model_metadata),
            # Model Health API returns 200 if model is ready to serve.
            FastAPIRoute(r"/v1/models/{model_name}", dataplane.model_ready),
            FastAPIRoute(r"/v1/models/{model_name}:predict", dataplane.infer, methods=["POST"]),
            FastAPIRoute(r"/v1/models/{model_name}:explain", dataplane.infer, methods=["POST"]),
            # V2 Inference Protocol
            # https://github.com/kserve/kserve/tree/master/docs/predict-api/v2
            FastAPIRoute(r"/v2", dataplane.metadata),
            FastAPIRoute(r"/v2/health/live", dataplane.live),
            FastAPIRoute(r"/v2/health/ready", dataplane.ready),
            # TODO: Finish model metadata with version dataplane handler
            FastAPIRoute(r"/v2/models/{model_name}", dataplane.model_metadata),
            FastAPIRoute(r"/v2/models/{model_name}/versions/{model_version}", dataplane.model_metadata),
            # TODO: Finish model infer with version dataplane handler
            FastAPIRoute(r"/v1/models/{model_name}/infer", dataplane.infer, methods=["POST"]),
            FastAPIRoute(r"/v1/models/{model_name}/versions/{model_version}/infer", dataplane.infer, methods=["POST"]),
            # TODO: Finish Triton Multi-Model Serving
            FastAPIRoute(r"/v2/repository/models/{model_name}/load", dataplane.load),
            FastAPIRoute(r"/v2/repository/models/{model_name}/unload", dataplane.unload),
        ], exception_handlers={
            errors.InvalidInput: errors.invalid_input_handler,
            errors.InferenceError: errors.inference_error_handler,
            errors.ModelNotFound: errors.model_not_found_handler,
            Exception: errors.exception_handler
        })

    async def start(self, models: Union[List[Model], Dict[str, Deployment]]):
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
            port=self.http_port,
            workers=self.workers
        )

        self._server = uvicorn.Server(cfg)
        servers = [self._server.serve()]
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
