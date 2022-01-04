import tornado.web
from kserve.handlers.base import BaseHandler
from kserve.model_repository import ModelRepository


class HealthHandler(BaseHandler):
    def initialize(self, models: ModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self, name: str):
        model = self.models.get_model(name)
        if model is None:
            raise tornado.web.HTTPError(
                status_code=404,
                reason="Model with name %s does not exist." % name
            )

        if self.models.is_model_ready(name):
            self.write({
                "name": name,
                "ready": True
            })
        else:
            self.set_status(503)
            self.write({
                "name": name,
                "ready": False
            })
