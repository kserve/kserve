# Copyright 2021 The KServe Authors.
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

from typing import Dict, Union
import sys
import inspect
import json
import tornado.web
from tornado.httpclient import AsyncHTTPClient
from cloudevents.http import CloudEvent
from http import HTTPStatus
from enum import Enum
from kserve.utils.utils import is_structured_cloudevent
import grpc
from tritonclient.grpc import InferResult, service_pb2_grpc
from tritonclient.grpc.service_pb2 import ModelInferRequest, ModelInferResponse


PREDICTOR_URL_FORMAT = "http://{0}/v1/models/{1}:predict"
EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"
PREDICTOR_V2_URL_FORMAT = "http://{0}/v2/models/{1}/infer"
EXPLAINER_V2_URL_FORMAT = "http://{0}/v2/models/{1}/explain"


class ModelType(Enum):
    EXPLAINER = 1
    PREDICTOR = 2


class PredictorProtocol(Enum):
    REST_V1 = "v1"
    REST_V2 = "v2"
    GRPC_V2 = "grpc-v2"


class ModelMissingError(Exception):
    def __init__(self, path):
        self.path = path

    def __str__(self):
        return self.path


class InferenceError(RuntimeError):
    def __init__(self, reason):
        self.reason = reason

    def __str__(self):
        return self.reason


# Model is intended to be subclassed by various components within KServe.
class Model:
    def __init__(self, name: str):
        self.name = name
        self.ready = False
        self.protocol = PredictorProtocol.REST_V1.value
        self.predictor_host = None
        self.explainer_host = None
        # The timeout matches what is set in generated Istio resources.
        # We generally don't want things to time out at the request level here,
        # timeouts should be handled elsewhere in the system.
        self.timeout = 600
        self._http_client_instance = None
        self._grpc_client_stub = None

    async def __call__(self, body, model_type: ModelType = ModelType.PREDICTOR):
        request = await self.preprocess(body) if inspect.iscoroutinefunction(self.preprocess) \
            else self.preprocess(body)
        request = self.validate(request)
        if model_type == ModelType.EXPLAINER:
            response = (await self.explain(request)) if inspect.iscoroutinefunction(self.explain) \
                else self.explain(request)
        elif model_type == ModelType.PREDICTOR:
            response = (await self.predict(request)) if inspect.iscoroutinefunction(self.predict) \
                else self.predict(request)
        else:
            raise NotImplementedError
        response = self.postprocess(response)
        return response

    @property
    def _http_client(self):
        if self._http_client_instance is None:
            self._http_client_instance = AsyncHTTPClient(max_clients=sys.maxsize)
        return self._http_client_instance

    @property
    def _grpc_client(self):
        if self._grpc_client_stub is None:
            # requires appending ":80" to the predictor host for gRPC to work
            if ":" not in self.predictor_host:
                self.predictor_host = self.predictor_host + ":80"
            _channel = grpc.aio.insecure_channel(self.predictor_host)
            self._grpc_client_stub = service_pb2_grpc.GRPCInferenceServiceStub(_channel)
        return self._grpc_client_stub

    def validate(self, request):
        if self.protocol == PredictorProtocol.REST_V2:
            if "inputs" in request and not isinstance(request["inputs"], list):
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Expected \"inputs\" to be a list"
                )
        elif isinstance(request, Dict) or self.protocol == PredictorProtocol.REST_V1:
            if "instances" in request and not isinstance(request["instances"], list):
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Expected \"instances\" to be a list"
                )
        return request

    def load(self) -> bool:
        """
        Load handler can be overridden to load the model from storage
        self.ready flag is used for model health check
        :return: bool
        """
        self.ready = True
        return self.ready

    async def preprocess(self, request: Union[Dict, CloudEvent]) -> Union[Dict, ModelInferRequest]:
        """
        The preprocess handler can be overridden for data or feature transformation.
        The default implementation decodes to Dict if it is a binary CloudEvent
        or gets the data field from a structured CloudEvent.
        :param request: Dict|CloudEvent|ModelInferRequest
        :return: Transformed Dict|ModelInferRequest which passes to predict handler
        """
        response = request

        if isinstance(request, CloudEvent):
            response = request.data
            # Try to decode and parse JSON UTF-8 if possible, otherwise
            # just pass the CloudEvent data on to the predict function.
            # This is for the cases that CloudEvent encoding is protobuf, avro etc.
            try:
                response = json.loads(response.decode('UTF-8'))
            except (json.decoder.JSONDecodeError, UnicodeDecodeError) as e:
                # If decoding or parsing failed, check if it was supposed to be JSON UTF-8
                if "content-type" in request._attributes and \
                        (request._attributes["content-type"] == "application/cloudevents+json" or
                         request._attributes["content-type"] == "application/json"):
                    raise tornado.web.HTTPError(
                        status_code=HTTPStatus.BAD_REQUEST,
                        reason=f"Failed to decode or parse binary json cloudevent: {e}"
                    )

        elif isinstance(request, dict):
            if is_structured_cloudevent(request):
                response = request["data"]

        return response

    def postprocess(self, response: Union[Dict, ModelInferResponse]) -> Dict:
        """
        The postprocess handler can be overridden for inference response transformation
        :param response: Dict|ModelInferResponse passed from predict handler
        :return: Dict
        """
        if isinstance(response, ModelInferResponse):
            response = InferResult(response)
            return response.get_response(as_json=True)
        return response

    async def _http_predict(self, request: Dict) -> Dict:
        predict_url = PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            predict_url = PREDICTOR_V2_URL_FORMAT.format(self.predictor_host, self.name)
        json_header = {'Content-Type': 'application/json'}
        response = await self._http_client.fetch(
            predict_url,
            method='POST',
            request_timeout=self.timeout,
            headers=json_header,
            body=json.dumps(request)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)

    async def _grpc_predict(self, request: ModelInferRequest) -> ModelInferResponse:
        async_result = await self._grpc_client.ModelInfer(request=request, timeout=self.timeout)
        return async_result

    async def predict(self, request: Union[Dict, ModelInferRequest]) -> Union[Dict, ModelInferResponse]:
        """
        The predict handler can be overridden to implement the model inference.
        The default implementation makes a call to the predictor if predictor_host is specified
        :param request: Dict|ModelInferRequest passed from preprocess handler
        :return: Dict|ModelInferResponse
        """
        if not self.predictor_host:
            raise NotImplementedError
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            return await self._grpc_predict(request)
        else:
            return await self._http_predict(request)

    async def explain(self, request: Dict) -> Dict:
        """
        The explain handler can be overridden to implement the model explanation.
        The default implementation makes an call to the explainer if explainer_host is specified
        :param request: Dict passed from preprocess handler
        :return: Dict
        """
        if self.explainer_host is None:
            raise NotImplementedError
        explain_url = EXPLAINER_URL_FORMAT.format(self.explainer_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            explain_url = EXPLAINER_V2_URL_FORMAT.format(self.explainer_host, self.name)
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
