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

from typing import Dict
import sys

import json
import tornado.web
from tornado.httpclient import AsyncHTTPClient
from cloudevents.http import CloudEvent
from http import HTTPStatus

PREDICTOR_URL_FORMAT = "http://{0}/v1/models/{1}:predict"
EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"
PREDICTOR_V2_URL_FORMAT = "http://{0}/v2/models/{1}/infer"
EXPLAINER_V2_URL_FORMAT = "http://{0}/v2/models/{1}/explain"


# KFModel is intended to be subclassed by various components within KFServing.
class KFModel:

    def __init__(self, name: str):
        self.name = name
        self.ready = False
        self.protocol = "v1"
        self.predictor_host = None
        self.explainer_host = None
        # The timeout matches what is set in generated Istio resources.
        # We generally don't want things to time out at the request level here,
        # timeouts should be handled elsewhere in the system.
        self.timeout = 600
        self._http_client_instance = None

    @property
    def _http_client(self):
        if self._http_client_instance is None:
            self._http_client_instance = AsyncHTTPClient(max_clients=sys.maxsize)
        return self._http_client_instance

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def preprocess(self, request: Dict) -> Dict:
        # If cloudevent dict, then parse 'data' field. Otherwise, pass through.
        response = request

        if(isinstance(request, CloudEvent)):
            response = request.data
            if(isinstance(response, bytes)):
                try:
                    response = json.loads(response.decode('UTF-8'))
                except (json.decoder.JSONDecodeError, UnicodeDecodeError) as e:
                    attributes = request._attributes
                    if "content-type" in attributes:
                        if attributes["content-type"] == "application/cloudevents+json" or attributes["content-type"] == "application/json":
                            raise tornado.web.HTTPError(
                                status_code=HTTPStatus.BAD_REQUEST,
                                reason="Unrecognized request format: %s" % e
                            )

        elif(isinstance(request, dict)): #CE structured - https://github.com/cloudevents/sdk-python/blob/8773319279339b48ebfb7b856b722a2180458f5f/cloudevents/http/http_methods.py#L126 
 
            if "time" in request \
                and "type" in request \
                and "source" in request \
                and "id" in request \
                and "specversion" in request \
                and "data" in request:
                response = request["data"]
                
        return response

    def postprocess(self, request: Dict) -> Dict:
        return request

    async def predict(self, request: Dict) -> Dict:
        if not self.predictor_host:
            raise NotImplementedError
        predict_url = PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == "v2":
            predict_url = PREDICTOR_V2_URL_FORMAT.format(self.predictor_host, self.name)
        response = await self._http_client.fetch(
            predict_url,
            method='POST',
            request_timeout=self.timeout,
            body=json.dumps(request)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)

    async def explain(self, request: Dict) -> Dict:
        if self.explainer_host is None:
            raise NotImplementedError
        explain_url = EXPLAINER_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == "v2":
            explain_url = EXPLAINER_V2_URL_FORMAT.format(self.predictor_host, self.name)
        response = await self._http_client.fetch(
            url=explain_url,
            method='POST',
            request_timeout=self.timeout,
            body=json.dumps(request)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)

