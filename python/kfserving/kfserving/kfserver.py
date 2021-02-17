# Copyright 2020 kubeflow.org.
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
import json
import inspect
import sys
from typing import List, Optional
import tornado.ioloop
import tornado.web
import tornado.httpserver
import tornado.log

from kfserving.handlers.http import PredictHandler, ExplainHandler
from kfserving import KFModel
from kfserving.kfmodel_repository import KFModelRepository

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
args, _ = parser.parse_known_args()

tornado.log.enable_pretty_logging()


class KFServer:
    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 max_buffer_size: int = args.max_buffer_size,
                 workers: int = args.workers,
                 registered_models: KFModelRepository = KFModelRepository()):
        self.registered_models = registered_models
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.max_buffer_size = max_buffer_size
        self.workers = workers
        self._http_server: Optional[tornado.httpserver.HTTPServer] = None

    def create_application(self):
        return tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            (r"/v2/health/live", LivenessHandler),
            (r"/v1/models",
             ListHandler, dict(models=self.registered_models)),
            (r"/v2/models",
             ListHandler, dict(models=self.registered_models)),
            # Model Health API returns 200 if model is ready to serve.
            (r"/v1/models/([a-zA-Z0-9_-]+)",
             HealthHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/status",
             HealthHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):predict",
             PredictHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/infer",
             PredictHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):explain",
             ExplainHandler, dict(models=self.registered_models)),
            (r"/v2/models/([a-zA-Z0-9_-]+)/explain",
             ExplainHandler, dict(models=self.registered_models)),
            (r"/v2/repository/models/([a-zA-Z0-9_-]+)/load",
             LoadHandler, dict(models=self.registered_models)),
            (r"/v2/repository/models/([a-zA-Z0-9_-]+)/unload",
             UnloadHandler, dict(models=self.registered_models)),
        ])

    def start(self, models: List[KFModel], nest_asyncio: bool = False):
        for model in models:
            self.register_model(model)

        self._http_server = tornado.httpserver.HTTPServer(
            self.create_application(), max_buffer_size=self.max_buffer_size)

        logging.info("Listening on port %s", self.http_port)
        self._http_server.bind(self.http_port)
        logging.info("Will fork %d workers", self.workers)
        self._http_server.start(self.workers)

        # Need to start the IOLoop after workers have been started
        # https://github.com/tornadoweb/tornado/issues/2426
        # The nest_asyncio package needs to be installed by the downstream module
        if nest_asyncio:
            import nest_asyncio
            nest_asyncio.apply()

        tornado.ioloop.IOLoop.current().start()

    def register_model(self, model: KFModel):
        if not model.name:
            raise Exception(
                "Failed to register model, model.name must be provided.")
        self.registered_models.update(model)
        logging.info("Registering model: %s", model.name)


class LivenessHandler(tornado.web.RequestHandler):  # pylint:disable=too-few-public-methods
    def get(self):
        self.write("Alive")


class HealthHandler(tornado.web.RequestHandler):
    def initialize(self, models: KFModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self, name: str):
        model = self.models.get_model(name)
        if model is None:
            raise tornado.web.HTTPError(
                status_code=404,
                reason="Model with name %s does not exist." % name
            )

        if not model.ready:
            raise tornado.web.HTTPError(
                status_code=503,
                reason="Model with name %s is not ready." % name
            )

        self.write(json.dumps({
            "name": model.name,
            "ready": model.ready
        }))


class ListHandler(tornado.web.RequestHandler):
    def initialize(self, models: KFModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self):
        self.write(json.dumps([ob.name for ob in self.models.get_models()]))


class LoadHandler(tornado.web.RequestHandler):
    def initialize(self, models: KFModelRepository):  # pylint:disable=attribute-defined-outside-init
        self.models = models

    async def post(self, name: str):
        try:
            (await self.models.load(name)) if inspect.iscoroutinefunction(self.models.load) else self.models.load(name)
        except Exception as e:
            ex_type, ex_value, ex_traceback = sys.exc_info()
            raise tornado.web.HTTPError(
                status_code=500,
                reason=f"Model with name {name} is not ready. "
                       f"Error type: {ex_type} error msg: {ex_value}"
            )

        if not self.models.is_model_ready(name):
            raise tornado.web.HTTPError(
                status_code=503,
                reason=f"Model with name {name} is not ready."
            )
        self.write(json.dumps({
            "name": name,
            "load": True
        }))


class UnloadHandler(tornado.web.RequestHandler):
    def initialize(self, models: KFModelRepository):  # pylint:disable=attribute-defined-outside-init
        self.models = models

    def post(self, name: str):
        try:
            self.models.unload(name)
        except KeyError:
            raise tornado.web.HTTPError(
                status_code=404,
                reason="Model with name %s does not exist." % name
            )
        self.write(json.dumps({
            "name": name,
            "unload": True
        }))
