# Copyright 2022 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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


class ListHandler(BaseHandler):
    def initialize(self, models: ModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get(self):
        self.write({"models": list(self.models.get_models().keys())})
