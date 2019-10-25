import tornado.web
import json
from kfserving.model import KFModel
from http import HTTPStatus


class PredictHandler(V1Handler):
    def initialize(self, models: Dict[str, KFModel]):
        self.models = models

    def post(self, name: str):
        model = self.get_model(name)
        self.validate(model)
        self.write(model.predict(self.request.body))
