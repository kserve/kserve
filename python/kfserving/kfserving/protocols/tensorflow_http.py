# Copyright 2019 kubeflow.org.
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

from http import HTTPStatus
import tornado
from typing import Dict, List
from kfserving.protocols.request_handler import RequestHandler # pylint: disable=no-name-in-module


class TensorflowRequestHandler(RequestHandler):

    def __init__(self, request: Dict): #pylint: disable=useless-super-delegation
        super().__init__(request)

    def validate(self):
        if "instances" not in self.request:
            raise tornado.web.HTTPError(
                status_code=HTTPStatus.BAD_REQUEST,
                reason="Expected key \"instances\" in request body"
            )

    def extract_request(self) -> List:
        return self.request["instances"]

    def wrap_response(self, response: List) -> Dict:
        return {"predictions": response}
