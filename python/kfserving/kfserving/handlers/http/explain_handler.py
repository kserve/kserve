from http import HTTPStatus
import tornado.web
import json
from kfserving.model import KFModel


class ExplainHandler(V1Handler):
    def initialize(self, models: Dict[str, KFModel]):
        self.protocol = protocol
        self.models = models

    def post(self, name: str):
        model = self.get_model(name)
        self.validate(model)
        self.write(model.explain(self.request.body))