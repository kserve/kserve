from http import HTTPStatus

import tornado.ioloop
import tornado.web
import tornado.httpserver
import argparse
import os
import logging
import json
from kfserving.model import KFModel
from typing import List, Dict, Optional

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081
TFSERVING_HTTP_PROTOCOL = "tensorflow.http"
SELDON_HTTP_PROTOCOL = "seldon.http"
PROTOCOLS = [TFSERVING_HTTP_PROTOCOL,SELDON_HTTP_PROTOCOL]

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT, type=int,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT, type=int,
                    help='The GRPC Port listened to by the model server.')
parser.add_argument('--protocol', default=TFSERVING_HTTP_PROTOCOL, choices=PROTOCOLS, type=str,
                    help='The protocol served by the model server')
args, _ = parser.parse_known_args()

KFSERVER_LOGLEVEL = os.environ.get('KFSERVER_LOGLEVEL', 'INFO').upper()
logging.basicConfig(level=KFSERVER_LOGLEVEL)


class KFServer(object):
    def __init__(self,protocol: str = args.protocol,http_port: int = args.http_port,grpc_port: int = args.grpc_port):
        self.registered_models: Dict[str,KFModel] = {}
        self.http_port = http_port
        self.grpc_port = grpc_port
        self.protocol = protocol
        self._http_server: Optional[tornado.httpserver.HTTPServer] = None

    def createApplication(self):
        return tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            # Protocol Discovery API that returns the serving protocol supported by this server.
            (r"/protocol", ProtocolHandler, dict(protocol=self.protocol)),
            # Prometheus Metrics API that returns metrics for model servers
            (r"/metrics", MetricsHandler, dict(models=self.registered_models)),
            # Model Health API returns 200 if model is ready to serve.
            (r"/models/([a-zA-Z0-9_-]+)",
             ModelHealthHandler, dict(models=self.registered_models)),
            # Predict API executes executes predict on input tensors
            (r"/models/([a-zA-Z0-9_-]+)",
             ModelPredictHandler, dict(models=self.registered_models)),
            # Optional Custom Predict Verb for Tensorflow compatibility
            (r"/models/([a-zA-Z0-9_-]+):predict",
             ModelPredictHandler, dict(models=self.registered_models)),
        ])

    def start(self, models: List[KFModel] = []):
        # TODO add a GRPC server
        for model in models:
            self.register_model(model)

        self._http_server = tornado.httpserver.HTTPServer(self.createApplication())

        logging.info("Listening on port %s" % self.http_port)
        self._http_server.bind(self.http_port)
        self._http_server.start(0) # Forks workers equal to host's cores
        tornado.ioloop.IOLoop.current().start()

    def register_model(self, model: KFModel):
        if not model.name:
            raise Exception("Failed to register model, model.name must be provided.")
        self.registered_models[model.name] = model


class ModelPredictHandler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str,KFModel]):
        self.models = models

    def post(self, name: str):
        # TODO Add metrics
        if name not in self.models:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.NOT_FOUND,
                reason="Model with name %s does not exist." % name
            )

        model = self.models[name]
        if not model.ready:
            model.load()

        try:
            body = json.loads(self.request.body)
        except json.decoder.JSONDecodeError as e:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Unrecognized request format: %s" % e
            )

        results = model.predict(body)

        self.write(str(results))

class LivenessHandler(tornado.web.RequestHandler):
    def get(self):
        self.write("Alive")


class ProtocolHandler(tornado.web.RequestHandler):
    def initialize(self, protocol: str):
        self.protocol = protocol

    def get(self):
        self.write(self.protocol)


class MetricsHandler(tornado.web.RequestHandler):
    def get(self):
        self.write("Not Implemented")


class ModelHealthHandler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str,KFModel]):
        self.models = models

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


if __name__ == "__main__":
    s = KFServer()
    s.start()