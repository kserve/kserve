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
from typing import Any

import tornado.web
from http import HTTPStatus
from kserve.model_repository import ModelRepository


class BaseHandler(tornado.web.RequestHandler):
    def write_error(self, status_code: int, **kwargs: Any) -> None:
        """This method is called when there are unhandled tornado.web.HTTPErrors"""
        self.set_status(status_code)

        reason = "An error occurred"

        exc_info = kwargs.get("exc_info", None)
        if exc_info is not None:
            if hasattr(exc_info[1], "reason"):
                reason = exc_info[1].reason
            else:
                reason = str(exc_info[1])

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
