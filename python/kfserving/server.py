from http import HTTPStatus

import tornado.ioloop
import tornado.web
import argparse
import os
import logging
import json

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081
DEFAULT_PROTOCOL = "tensorflow/serving/proto/predict.proto"

parser = argparse.ArgumentParser()
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT,
                    help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT,
                    help='The GRPC Port listened to by the model server.')
args, _ = parser.parse_known_args()

KFSERVER_LOGLEVEL = os.environ.get('KFSERVER_LOGLEVEL', 'INFO').upper()
logging.basicConfig(level=KFSERVER_LOGLEVEL)


class KFServer(object):
    def __init__(self):
        self.http_port = args.http_port
        self.grpc_port = args.grpc_port
        self.protocol = DEFAULT_PROTOCOL

        self.model_name = args.model_name
        self.model_dir = args.model_dir
        self.local_model_dir = args.local_model_dir

    def start(self, models):
        # TODO add a GRPC server

        self._http_server = tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            # Protocol Discovery API that returns the serving protocol supported by this server.
            (r"/protocol", ProtocolHandler),
            # Prometheus Metrics API that returns metrics for model servers
            (r"/metrics", MetricsHandler, dict(models=models)),
            # Model Health API returns 200 if model is ready to serve.
            (r"/model/([a-zA-Z0-9_-]+)",
             ModelHealthHandler, dict(models=models)),
            # Predict API executes executes predict on input tensors
            (r"/model/([a-zA-Z0-9_-]+)",
             ModelPredictHandler, dict(models=models)),
            # Optional Custom Predict Verb for Tensorflow compatibility
            (r"/model/([a-zA-Z0-9_-]+):predict",
             ModelPredictHandler, dict(models=models)),
        ])

        self._http_server.listen(self.http_port)
        logging.info("Listening on port %s" % self.http_port)
        tornado.ioloop.IOLoop.current().start()


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

        self.write({
            "predictions": outputs
        })
