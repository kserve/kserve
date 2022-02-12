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
import logging
from typing import List, Optional, Dict, Union
import tornado.ioloop
import tornado.web
import tornado.httpserver
import tornado.log
import asyncio
from tornado import concurrent

from .utils import utils

import kserve.handlers as handlers
from kserve import Model
from kserve.model_repository import ModelRepository
from ray.serve.api import Deployment, RayServeHandle
from ray import serve

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081
DEFAULT_MAX_BUFFER_SIZE = 104857600

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT, type=int,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT, type=int,
                    help='The GRPC Port listened to by the model server.')
parser.add_argument('--max_buffer_size', default=DEFAULT_MAX_BUFFER_SIZE, type=int,
                    help='The max buffer size for tornado.')
parser.add_argument('--workers', default=1, type=int,
                    help='The number of works to fork')
parser.add_argument('--max_asyncio_workers', default=None, type=int,
                    help='Max number of asyncio workers to spawn')

args, _ = parser.parse_known_args()

tornado.log.enable_pretty_logging()


class ModelServer:
    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 max_buffer_size: int = args.max_buffer_size,
                 workers: int = args.workers,
                 max_asyncio_workers: int = args.max_asyncio_workers,
                 registered_models: ModelRepository = ModelRepository()):
        self.registered_models = registered_models
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.max_buffer_size = max_buffer_size
        self.workers = workers
        self.max_asyncio_workers = max_asyncio_workers
        self._http_server: Optional[tornado.httpserver.HTTPServer] = None

    def create_application(self):
        return tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", handlers.LivenessHandler),
            (r"/v2/health/live", handlers.LivenessHandler),
            (r"/v1/models",
             handlers.ListHandler, dict(models=self.registered_models)),
            (r"/v2/models",
             handlers.ListHandler, dict(models=self.registered_models)),
            # Model Health API returns 200 if model is ready to serve.
            (r"/v1/models/([a-zA-Z0-9_-]+)",
             handlers.HealthHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/status",
             handlers.HealthHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):predict",
             handlers.PredictHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/infer",
             handlers.PredictHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):explain",
             handlers.ExplainHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/explain",
             handlers.ExplainHandler, dict(models=self.registered_models)),
            (r"/v2/repository/models/([a-zA-Z0-9_-]+)/load",
             handlers.LoadHandler, dict(models=self.registered_models)),
            (r"/v2/repository/models/([a-zA-Z0-9_-]+)/unload",
             handlers.UnloadHandler, dict(models=self.registered_models)),
        ], default_handler_class=handlers.NotFoundHandler)

    def start(self, models: Union[List[Model], Dict[str, Deployment]], nest_asyncio: bool = False):
        if isinstance(models, list):
            for model in models:
                if isinstance(model, Model):
                    self.register_model(model)
                else:
                    raise RuntimeError("Model type should be Model")
        elif isinstance(models, dict):
            if all([isinstance(v, Deployment) for v in models.values()]):
                serve.start(detached=True, http_options={"host": "0.0.0.0", "port": 9071})
                for key in models:
                    models[key].deploy()
                    handle = models[key].get_handle()
                    self.register_model_handle(key, handle)
            else:
                raise RuntimeError("Model type should be RayServe Deployment")
        else:
            raise RuntimeError("Unknown model collection types")

        if self.max_asyncio_workers is None:
            # formula as suggest in https://bugs.python.org/issue35279
            self.max_asyncio_workers = min(32, utils.cpu_count()+4)

        self._http_server = tornado.httpserver.HTTPServer(
            self.create_application(), max_buffer_size=self.max_buffer_size)

        logging.info("Listening on port %s", self.http_port)
        self._http_server.bind(self.http_port)
        logging.info("Will fork %d workers", self.workers)
        self._http_server.start(self.workers)

        logging.info(f"Setting max asyncio worker threads as {self.max_asyncio_workers}")
        asyncio.get_event_loop().set_default_executor(
            concurrent.futures.ThreadPoolExecutor(max_workers=self.max_asyncio_workers))

        # Need to start the IOLoop after workers have been started
        # https://github.com/tornadoweb/tornado/issues/2426
        # The nest_asyncio package needs to be installed by the downstream module
        if nest_asyncio:
            import nest_asyncio
            nest_asyncio.apply()

        tornado.ioloop.IOLoop.current().start()

    def register_model_handle(self, name: str, model_handle: RayServeHandle):
        self.registered_models.update_handle(name, model_handle)
        logging.info("Registering model handle: %s", name)

    def register_model(self, model: Model):
        if not model.name:
            raise Exception(
                "Failed to register model, model.name must be provided.")
        self.registered_models.update(model)
        logging.info("Registering model: %s", model.name)
