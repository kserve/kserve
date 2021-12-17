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

from typing import Any

import tornado.web
import json
import pytz
import cloudevents.exceptions as ce
from cloudevents.http import CloudEvent, from_http, is_binary, is_structured, to_binary, to_structured
from cloudevents.sdk.converters.util import has_binary_headers
from http import HTTPStatus
from kserve.model_repository import ModelRepository
from kserve.model import ModelType
from datetime import datetime

from ray.serve.api import RayServeHandle


class BaseHandler(tornado.web.RequestHandler):
    def write_error(self, status_code: int, **kwargs: Any) -> None:
        """This method is called when there are unhandled tornado.web.HTTPErrors"""
        self.set_status(status_code)

        reason = "An error occurred"

        exc_info = kwargs.get("exc_info", None)
        if exc_info is not None:
            if hasattr(exc_info[1], "reason"):
                reason = exc_info[1].reason

        self.write({"error": reason})


class NotFoundHandler(tornado.web.RequestHandler):
    def write_error(self, status_code: int, **kwargs: Any) -> None:
        self.set_status(HTTPStatus.NOT_FOUND)
        self.write({"error": "invalid path"})


class HTTPHandler(BaseHandler):
    def initialize(self, models: ModelRepository):
        self.models = models  # pylint:disable=attribute-defined-outside-init

    def get_model(self, name: str):
        model = self.models.get_model(name)
        if model is None:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.NOT_FOUND,
                reason="Model with name %s does not exist." % name
            )
        if not self.models.is_model_ready(name):
            model.load()
        return model


class PredictHandler(HTTPHandler):
    async def post(self, name: str):
        if has_binary_headers(self.request.headers):
            try:
                # Use default unmarshaller if contenttype is set in header
                if "ce-contenttype" in self.request.headers:
                    body = from_http(self.request.headers, self.request.body)
                else:
                    body = from_http(self.request.headers, self.request.body, lambda x: x)
            except (ce.MissingRequiredFields, ce.InvalidRequiredFields, ce.InvalidStructuredJSON,
                    ce.InvalidHeadersFormat, ce.DataMarshallerError, ce.DataUnmarshallerError) as e:
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
        # call model locally or remote model workers
        model = self.get_model(name)
        if not isinstance(model, RayServeHandle):
            response = await model(body)
        else:
            model_handle = model
            response = await model_handle.remote(body)
        # process response from the model
        if has_binary_headers(self.request.headers):
            event = CloudEvent(body._attributes, response)
            if is_binary(self.request.headers):
                eventheader, eventbody = to_binary(event)
            elif is_structured(self.request.headers):
                eventheader, eventbody = to_structured(event)
            for k, v in eventheader.items():
                if k != "ce-time":
                    self.set_header(k, v)
                else:  # utc now() timestamp
                    self.set_header('ce-time', datetime.utcnow().replace(tzinfo=pytz.utc).
                                    strftime('%Y-%m-%dT%H:%M:%S.%f%z'))
            response = eventbody

        self.write(response)


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
