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

import inspect
import tornado.web
import typing
import json
import pytz
import cloudevents.exceptions as ce
from cloudevents.http import CloudEvent, from_http, is_binary, is_structured, to_binary, to_structured
from cloudevents.sdk.converters.util import has_binary_headers
from http import HTTPStatus
from kfserving.kfmodel_repository import KFModelRepository
from datetime import datetime


class HTTPHandler(tornado.web.RequestHandler):
    def initialize(self, models: KFModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get_model(self, name: str):
        model = self.models.get_model(name)
        if model is None:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.NOT_FOUND,
                reason="Model with name %s does not exist." % name
            )
        if not model.ready:
            model.load()
        return model

    def validate(self, request):
        if(isinstance(request, dict)):
            if ("instances" in request and not isinstance(request["instances"], list)) or \
               ("inputs" in request and not isinstance(request["inputs"], list)):
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Expected \"instances\" or \"inputs\" to be a list"
                )
        return request

class PredictHandler(HTTPHandler):
    async def post(self, name: str):
        if has_binary_headers(self.request.headers):            
            try:
                #Use default unmarshaller if contenttype is set in header
                if "ce-contenttype" in self.request.headers:
                    body = from_http(self.request.headers, self.request.body)
                else:
                    body = from_http(self.request.headers, self.request.body, lambda x: x)
            except (ce.MissingRequiredFields, ce.InvalidRequiredFields, ce.InvalidStructuredJSON, ce.InvalidHeadersFormat, ce.DataMarshallerError, ce.DataUnmarshallerError) as e:
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Cloud Event Exceptions: %s" % e
                )
        else:
            try:
                body = json.loads(self.request.body)
            except json.decoder.JSONDecodeError as e:
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Unrecognized request format: %s" % e
                )

        model = self.get_model(name)
        request = model.preprocess(body)
        request = self.validate(request)
        response = (await model.predict(request)) if inspect.iscoroutinefunction(model.predict) else model.predict(request)
        response = model.postprocess(response)

        if has_binary_headers(self.request.headers):
            event = CloudEvent(body._attributes, response)
            if is_binary(self.request.headers):
                eventheader, eventbody = to_binary(event)
            elif is_structured(self.request.headers):
                eventheader, eventbody = to_structured(event)
            for k, v in eventheader.items():
                if k != "ce-time":
                    self.set_header(k, v)
                else: #utc now() timestamp
                    self.set_header('ce-time', datetime.utcnow().replace(tzinfo=pytz.utc).strftime('%Y-%m-%dT%H:%M:%S.%f%z'))
            response = eventbody

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
        response = (await model.explain(request)) if inspect.iscoroutinefunction(model.explain) else model.explain(request)
        response = model.postprocess(response)
        self.write(response)
