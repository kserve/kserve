from http import HTTPStatus
import tornado.web
import json
from kfserving.model import KFModel


class V1Handler(tornado.web.RequestHandler):
    def initialize(self, models: Dict[str, KFModel]):
        self.protocol = protocol
        self.models = models

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

    def validate(self):
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
        
        if not isinstance(body["instances"], list):
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected \"instances\" to be a list"
            )

