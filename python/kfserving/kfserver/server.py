import tornado.ioloop
import tornado.web
import argparse


parser = argparse.ArgumentParser(description='Process some integers.')

parser.add_argument('--model_uri', help='A URI pointer to the model file')

class KFServer(object):
    def start(self):
        args = parser.parse_args()
        self.port = args.port
        print(self.port)


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

        self.load()
        self._server.listen(8000)
        tornado.ioloop.IOLoop.current().start()

    # load attempts to download the arg-specified model-file to local storage
    def load(self):
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