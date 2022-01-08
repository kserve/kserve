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

import tornado.web
from kserve.handlers.base import BaseHandler
from kserve.model_repository import ModelRepository


class LivenessHandler(BaseHandler):  # pylint:disable=too-few-public-methods
    def get(self):
        self.write({"status": "alive"})


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
