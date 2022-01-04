from kserve.handlers.base import BaseHandler
from kserve.model_repository import ModelRepository


class ListHandler(BaseHandler):
    def initialize(self, models: ModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self):
        self.write({"models": list(self.models.get_models().keys())})
