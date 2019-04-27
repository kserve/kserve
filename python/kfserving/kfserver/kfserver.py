import tornado.ioloop
import tornado.web

class KFServer():
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

        self._server.listen(8000)
        tornado.ioloop.IOLoop.current().start()

    def load(self):
        raise NotImplementedError

    # predict must be overriden by the implementing class
    def predict(self, tensor):
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