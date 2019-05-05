from http import HTTPStatus

import tornado.ioloop
import tornado.web
import argparse
import os
import logging
import json

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081
DEFAULT_PROTOCOL = "tensorflow/HTTP"

parser = argparse.ArgumentParser(add_help=False)
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT,
                    help='The GRPC Port listened to by the model server.')
args, _ = parser.parse_known_args()

KFSERVER_LOGLEVEL = os.environ.get('KFSERVER_LOGLEVEL', 'INFO').upper()
logging.basicConfig(level=KFSERVER_LOGLEVEL)


class KFServer(object):
    def __init__(self):
        self.registered_models = {}
        self.http_port = args.http_port
        self.grpc_port = args.grpc_port
        self.protocol = DEFAULT_PROTOCOL

        self._http_server = None

    def start(self, models=[]):
        # TODO add a GRPC server
        for model in models:
            self.register_model(model)

        self._http_server = tornado.httpserver.HTTPServer(tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            # Protocol Discovery API that returns the serving protocol supported by this server.
            (r"/protocol", ProtocolHandler),
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
        ]))

        logging.info("Listening on port %s" % self.http_port)
        self._http_server.bind(self.http_port)
        self._http_server.start(0) # Forks workers equal to host's cores
        tornado.ioloop.IOLoop.current().start()

    def register_model(self, model):
        if not model.name:
            raise Exception("Failed to register model, model.name must be provided.")
        self.registered_models[model.name] = model


class LivenessHandler(tornado.web.RequestHandler):
    def get(self):
        self.write("Alive")


class ProtocolHandler(tornado.web.RequestHandler):
    def initialize(self, protocol):
        self.protocol = protocol

    def get(self):
        self.write(self.protocol)


class MetricsHandler(tornado.web.RequestHandler):
    def get(self):
        self.write("Not Implemented")


class ModelHealthHandler(tornado.web.RequestHandler):
    def initialize(self, models):
        self.models = models

    def get(self, name):
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


class ModelPredictHandler(tornado.web.RequestHandler):
    def initialize(self, models):
        self.models = models

    def post(self, name):
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

        if "instances" not in body:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"instances\" in request body"
            )

        inputs = model.preprocess(body["instances"])
        results = model.predict(inputs)
        outputs = model.postprocess(results)

        self.write(str({
            "predictions": outputs
        }))
