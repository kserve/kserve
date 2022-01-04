import sys
import inspect
import tornado.web
from kserve.handlers.base import BaseHandler
from kserve.model_repository import ModelRepository


class LoadHandler(BaseHandler):
    def initialize(self, models: ModelRepository):  # pylint:disable=attribute-defined-outside-init
        self.models = models

    async def post(self, name: str):
        try:
            if inspect.iscoroutinefunction(self.models.load):
                await self.models.load(name)
            else:
                self.models.load(name)
        except Exception:
            ex_type, ex_value, ex_traceback = sys.exc_info()
            raise tornado.web.HTTPError(
                status_code=500,
                reason=f"Model with name {name} is not ready. "
                       f"Error type: {ex_type} error msg: {ex_value}"
            )

        if not self.models.is_model_ready(name):
            raise tornado.web.HTTPError(
                status_code=503,
                reason=f"Model with name {name} is not ready."
            )
        self.write({
            "name": name,
            "load": True
        })


class UnloadHandler(BaseHandler):
    def initialize(self, models: ModelRepository):  # pylint:disable=attribute-defined-outside-init
        self.models = models

    def post(self, name: str):
        try:
            self.models.unload(name)
        except KeyError:
            raise tornado.web.HTTPError(
                status_code=404,
                reason="Model with name %s does not exist." % name
            )
        self.write({
            "name": name,
            "unload": True
        })
