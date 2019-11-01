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

from typing import Dict

import json
import requests
import tornado.web

PREDICTOR_URL_FORMAT = "http://{0}/v1/models/{1}:predict"
EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"

# KFModel is intended to be subclassed by various components within KFServing.
class KFModel():

    def __init__(self, name: str):
        self.name = name
        self.ready = False
        self.predictor_host = None
        self.explainer_host = None

    def load(self):
        self.ready = True

    def preprocess(self, request: Dict) -> Dict:
        return request

    def postprocess(self, request: Dict) -> Dict:
        return request

    def predict(self, request: Dict) -> Dict:
        if self.predictor_host is None:
            raise NotImplementedError

        response = requests.post(
            PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name),
            json.dumps(request)
        )
        if response.status_code != 200:
            raise tornado.web.HTTPError(
                status_code=response.status_code,
                reason=response.content)
        return response.json()

    def explain(self, request: Dict) -> Dict:
        if self.explainer_host is None:
            raise NotImplementedError

        response = requests.post(
            EXPLAINER_URL_FORMAT.format(self.explainer_host, self.name),
            json.dumps(request)
        )
        if response.status_code != 200:
            raise tornado.web.HTTPError(
                status_code=response.status_code,
                reason=response.content)
        return response.json()
