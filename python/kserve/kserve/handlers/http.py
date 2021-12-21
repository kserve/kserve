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
import cloudevents.exceptions as ce
from cloudevents.http import CloudEvent, from_http
from cloudevents.sdk.converters.util import has_binary_headers
from http import HTTPStatus
from kserve.kfmodel_repository import KFModelRepository
from kserve.kfmodel import ModelType
from kserve.utils.utils import is_structured_cloudevent, create_response_cloudevent

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
    def initialize(self, models: KFModelRepository):
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
    def get_binary_cloudevent(self) -> CloudEvent:
        try:
            # Use default unmarshaller if contenttype is set in header
            if "ce-contenttype" in self.request.headers:
                event = from_http(self.request.headers, self.request.body)
            else:
                event = from_http(self.request.headers, self.request.body, lambda x: x)

            return event
        except (ce.MissingRequiredFields, ce.InvalidRequiredFields, ce.InvalidStructuredJSON,
                ce.InvalidHeadersFormat, ce.DataMarshallerError, ce.DataUnmarshallerError) as e:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Cloud Event Exceptions: %s" % e
            )

    async def post(self, name: str):
        is_cloudevent = False
        is_binary_cloudevent = False

        if has_binary_headers(self.request.headers):
            is_cloudevent = True
            is_binary_cloudevent = True
            body = self.get_binary_cloudevent()
        else:
            try:
                body = json.loads(self.request.body)
            except json.decoder.JSONDecodeError as e:
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Unrecognized request format: %s" % e
                )

            if is_structured_cloudevent(body):
                is_cloudevent = True

        # call model locally or remote model workers
        model = self.get_model(name)
        if not isinstance(model, RayServeHandle):
            response = await model(body)
        else:
            model_handle = model
            response = await model_handle.remote(body)

        # if we received a cloudevent, then also return a cloudevent
        if is_cloudevent:
            headers, response = create_response_cloudevent(name, body, response, is_binary_cloudevent)

            if is_binary_cloudevent:
                self.set_header("Content-Type", "application/json")
            else:
                self.set_header("Content-Type", "application/cloudevents+json")

            for k, v in headers.items():
                self.set_header(k, v)

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
