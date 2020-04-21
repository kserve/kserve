# Copyright 2020 kubeflow.org.
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
        if "instances" in request and not isinstance(request["instances"], list):
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected \"instances\" to be a list"
            )
        return request


class PredictHandler(HTTPHandler):
    async def post(self, name: str):
        model = self.get_model(name)
        try:
            body = json.loads(self.request.body)
        except json.decoder.JSONDecodeError as e:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Unrecognized request format: %s" % e
            )
        request = model.preprocess(body)
        request = self.validate(request)
        response = await model.predict(request)
        response = model.postprocess(response)
        self.write(response)


class ExplainHandler(HTTPHandler):
    async def post(self, name: str):
        model = self.get_model(name)
        try:
            body = json.loads(self.request.body)
        except json.decoder.JSONDecodeError as e:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Unrecognized request format: %s" % e
            )
        request = model.preprocess(body)
        request = self.validate(request)
        response = await model.explain(request)
        response = model.postprocess(response)
        self.write(response)
