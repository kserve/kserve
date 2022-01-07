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

import json
from http import HTTPStatus

import tornado.web
from ray.serve.api import RayServeHandle

from kserve.handlers.base import HTTPHandler
from kserve.model import ModelType


class ExplainHandler(HTTPHandler):
    async def post(self, name: str):
        try:
            body = json.loads(self.request.body)
        except json.decoder.JSONDecodeError as e:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Unrecognized request format: %s" % e
            )
        # call model locally or remote model workers
        model = self.get_model(name)
        if not isinstance(model, RayServeHandle):
            response = await model(body, model_type=ModelType.EXPLAINER)
        else:
            model_handle = model
            response = await model_handle.remote(body, model_type=ModelType.EXPLAINER)
        self.write(response)
