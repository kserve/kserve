# Copyright 2019 kubeflow.org.
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
from typing import List, Dict
import tornado.ioloop
import tornado.web
import tornado.httpserver
import tornado.log
from kfserving.handlers.http import PredictHandler, ExplainHandler
from kfserving import KFModel

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT, type=int,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT, type=int,
                    help='The GRPC Port listened to by the model server.')
parser.add_argument('--workers', default=0, type=int,
                    help='The number of works to fork')
args, _ = parser.parse_known_args()

tornado.log.enable_pretty_logging()

class KFServer:
    def __init__(self, http_port: int = args.http_port,
                 grpc_port: int = args.grpc_port,
                 workers: int = args.workers):
        self.registered_models = {}
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.workers = workers
        self._http_server = None

    def create_application(self):
        return tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            (r"/v1/models",
             ListHandler, dict(models=self.registered_models)),
            # Model Health API returns 200 if model is ready to serve.
            (r"/v1/models/([a-zA-Z0-9_-]+)",
             HealthHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):predict",
             PredictHandler, dict(models=self.registered_models)),
            (r"/v1/models/([a-zA-Z0-9_-]+):explain",
             ExplainHandler, dict(models=self.registered_models)),
        ])

    def start(self, models: List[KFModel]):
        for model in models:
            self.register_model(model)

        self._http_server = tornado.httpserver.HTTPServer(
            self.create_application())

        logging.info("Listening on port %s", self.http_port)
        self._http_server.bind(self.http_port)
        logging.info("Will fork %d workers", self.workers)
        self._http_server.start(self.workers)
        tornado.ioloop.IOLoop.current().start()

    def register_model(self, model: KFModel):
        if not model.name:
            raise Exception(
                "Failed to register model, model.name must be provided.")
        self.registered_models[model.name] = model
        logging.info("Registering model: %s", model.name)


class LivenessHandler(tornado.web.RequestHandler):  # pylint:disable=too-few-public-methods
    def get(self):
        self.write("Alive")


class HealthHandler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str, KFModel]):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self, name: str):
        if name not in self.models:
            raise tornado.web.HTTPError(
                status_code=404,
                reason="Model with name %s does not exist." % name
            )

        model = self.models[name]
        self.write(json.dumps({
            "name": model.name,
            "ready": model.ready
        }))


class ListHandler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str, KFModel]):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self):
        self.write(json.dumps(list(self.models.values())))
