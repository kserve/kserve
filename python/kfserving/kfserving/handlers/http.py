import tornado.web
import json
from typing import Dict
from http import HTTPStatus
from kfserving.kfmodel import KFModel


class HTTPHandler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str, KFModel]):
        self.models = models # pylint:disable=attribute-defined-outside-init

    def get_model(self, name: str):
        if name not in self.models:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.NOT_FOUND,
                reason="Model with name %s does not exist." % name
            )
        model = self.models[name]
        if not model.ready:
            model.load()
        return model

    def validate(self, request):
        try:
            body = json.loads(request.body)
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

        if not isinstance(body["instances"], list):
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected \"instances\" to be a list"
            )
        return body


class PredictHandler(HTTPHandler):
    def post(self, name: str):
        model = self.get_model(name)
        request = self.validate(self.request)
        request = model.preprocess(request)
        response = model.predict(request)
        response = model.postprocess(response)
        self.write(json.dumps(response))


class ExplainHandler(HTTPHandler):
    def post(self, name: str):
        model = self.get_model(name)
        request = self.validate(self.request)
        request = model.preprocess(request)
        response = model.explain(request)
        response = model.postprocess(response)
        self.write(json.dumps(response))
