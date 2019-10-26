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

from typing import List, Dict
import requests
import tornado.web

PREDICTOR_URL_FORMAT = "http://{0}/v1/models/{1}:predict"
EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"

# KFModel is intended to be subclassed by various components within KFServing.
class KFModel(object):

    def __init__(self, name: str):
        self.name = name
        self.ready = False

    def load(self):
        self.ready = True

    def predict(self, request: Dict, predictor_host: str = None) -> Dict:
        if predictor_host is None:
            raise NotImplementedError

        response = requests.post(PREDICTOR_URL_FORMAT.format(predictor_host, self.name), request)
        if response.status_code != 200:
            raise tornado.web.HTTPError(
                status_code=response.status_code,
                reason=response.reason)
        return response.json()

    def explain(self, request: Dict, explainer_host: str = None) -> Dict:
        if explainer_host is None:
            raise NotImplementedError

        response = requests.post(EXPLAINER_URL_FORMAT.format(explainer_host, self.name), request)
        if response.status_code != 200:
            raise tornado.web.HTTPError(
                status_code=response.status_code,
                reason=response.reason)
        return response.json()
