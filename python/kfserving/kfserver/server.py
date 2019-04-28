import tornado.ioloop
import tornado.web
import argparse
import kfserving.storage as storage
import os
from .protocols import TENSORFLOW_PROTOCOL

DEFAULT_HTTP_PORT = 8080
DEFAULT_GRPC_PORT = 8081
DEFAULT_MODEL_NAME = "default"
DEFAULT_LOCAL_MODEL_DIR = "/tmp/model"
DEFAULT_PROTOCOL = TENSORFLOW_PROTOCOL

parser = argparse.ArgumentParser()
parser.add_argument('--model_uri', required=True, help='A URI pointer to the model directory')
parser.add_argument('--model_name', default=DEFAULT_MODEL_NAME, help='The name that the model is served under.')
parser.add_argument("--local_model_dir", default=DEFAULT_LOCAL_MODEL_DIR, help="The local path to copy model_uri.")
parser.add_argument('--http_port', default=DEFAULT_HTTP_PORT, help='The HTTP Port listened to by the model server.')
parser.add_argument('--grpc_port', default=DEFAULT_GRPC_PORT, help='The GRPC Port listened to by the model server.')
parser.add_argument('--protocol', default=DEFAULT_PROTOCOL, help='The Serving Protocol supported by the model server.')
args = parser.parse_args()

class KFServer(object):
    def __init__(self):
        self.args = args
        self.load_model(args.model_uri, args.local_model_dir)
    
    def start(self):
        self._server = tornado.web.Application([
            # Server Liveness API returns 200 if server is alive.
            (r"/", LivenessHandler),
            # Model Health API returns 200 if model is ready to serve.
            (r"/model/([a-zA-Z0-9_-]+)", ModelHealthHandler),
            # Predict API executes executes predict on input tensors
            (r"/model/([a-zA-Z0-9_-]+)", ModelPredictHandler, dict(predict=self.predict)),
            # Optional Custom Predict Verb for Tensorflow compatibility
            (r"/model/([a-zA-Z0-9_-]+):predict", ModelPredictHandler, dict(predict=self.predict)),
        ])

        self._server.listen(args.http_port)
        print("Listening on " + str(args.http_port))
        tornado.ioloop.IOLoop.current().start()

    # load attempts to download the arg-specified model-file to local storage
    def load_model(self, model_uri, local_path):
        storage.uri_to_local(model_uri, local_path)
        print("Downloading model")

    # predict must be overriden by the implementing class
    def predict(self, request):
        raise NotImplementedError

class LivenessHandler(tornado.web.RequestHandler):
    def get(self):
        self.write("Alive")

class ModelHealthHandler(tornado.web.RequestHandler):
    def get(self, name):
        self.write(name + " is healthy")

class ModelPredictHandler(tornado.web.RequestHandler):
    def initialize(self, predict):
        self.predict = predict
    def post(self, name):
        self.write(self.predict(name))