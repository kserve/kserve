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

import inspect
import json
import sys
from enum import Enum
from http import HTTPStatus
from typing import Dict, Union

import grpc
import tornado.web
from cloudevents.http import CloudEvent
from tornado.httpclient import AsyncHTTPClient
from tritonclient.grpc import InferResult, service_pb2_grpc
from tritonclient.grpc.service_pb2 import ModelInferRequest, ModelInferResponse

from kserve.utils.utils import is_structured_cloudevent

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

    async def __call__(self, body, model_type: ModelType = ModelType.PREDICTOR, headers: Dict[str, str] = None):
        payload = await self.preprocess(body, headers) if inspect.iscoroutinefunction(self.preprocess) \
            else self.preprocess(body, headers)
        payload = self.validate(payload)
        if model_type == ModelType.EXPLAINER:
            response = (await self.explain(payload, headers)) if inspect.iscoroutinefunction(self.explain) \
                else self.explain(payload, headers)
        elif model_type == ModelType.PREDICTOR:
            response = (await self.predict(payload, headers)) if inspect.iscoroutinefunction(self.predict) \
                else self.predict(payload, headers)
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

    def validate(self, payload):
        if self.protocol == PredictorProtocol.REST_V2.value:
            if "inputs" in payload and not isinstance(payload["inputs"], list):
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Expected \"inputs\" to be a list"
                )
        elif isinstance(payload, Dict) or self.protocol == PredictorProtocol.REST_V1.value:
            if "instances" in payload and not isinstance(payload["instances"], list):
                raise tornado.web.HTTPError(
                    status_code=HTTPStatus.BAD_REQUEST,
                    reason="Expected \"instances\" to be a list"
                )
        return payload

    def load(self) -> bool:
        """
        Load handler can be overridden to load the model from storage
        self.ready flag is used for model health check
        :return: bool
        """
        self.ready = True
        return self.ready

    async def preprocess(self, payload: Union[Dict, CloudEvent], headers: Dict[str, str] = None) -> Union[
            Dict, ModelInferRequest]:
        """
        The preprocess handler can be overridden for data or feature transformation.
        The default implementation decodes to Dict if it is a binary CloudEvent
        or gets the data field from a structured CloudEvent.
        :param headers: Dict|ModelInferRequest headers
        :param payload: Dict|CloudEvent|ModelInferRequest body
        :return: Transformed Dict|ModelInferRequest which passes to predict handler
        """
        response = payload

        if isinstance(payload, CloudEvent):
            response = payload.data
            # Try to decode and parse JSON UTF-8 if possible, otherwise
            # just pass the CloudEvent data on to the predict function.
            # This is for the cases that CloudEvent encoding is protobuf, avro etc.
            try:
                response = json.loads(response.decode('UTF-8'))
            except (json.decoder.JSONDecodeError, UnicodeDecodeError) as e:
                # If decoding or parsing failed, check if it was supposed to be JSON UTF-8
                if "content-type" in payload._attributes and \
                        (payload._attributes["content-type"] == "application/cloudevents+json" or
                         payload._attributes["content-type"] == "application/json"):
                    raise tornado.web.HTTPError(
                        status_code=HTTPStatus.BAD_REQUEST,
                        reason=f"Failed to decode or parse binary json cloudevent: {e}"
                    )

        elif isinstance(payload, dict):
            if is_structured_cloudevent(payload):
                response = payload["data"]

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

    async def _http_predict(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        predict_url = PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            predict_url = PREDICTOR_V2_URL_FORMAT.format(self.predictor_host, self.name)
        if headers is None:
            headers = {'Content-Type': 'application/json'}
        response = await self._http_client.fetch(
            predict_url,
            method='POST',
            request_timeout=self.timeout,
            headers=headers,
            body=json.dumps(payload)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)

    async def _grpc_predict(self, payload: ModelInferRequest, headers: Dict[str, str] = None) -> ModelInferResponse:
        async_result = await self._grpc_client.ModelInfer(request=payload, timeout=self.timeout, headers=headers)
        return async_result

    async def predict(self, payload: Union[Dict, ModelInferRequest],
                      headers: Dict[str, str] = None) -> Union[Dict, ModelInferResponse]:
        """
        The predict handler can be overridden to implement the model inference.
        The default implementation makes a call to the predictor if predictor_host is specified
        :param headers: Dict|ModelInferenceRequest headers
        :param payload: Dict|ModelInferRequest body passed from preprocess handler
        :return: Dict|ModelInferResponse
        """
        if not self.predictor_host:
            raise NotImplementedError
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            return await self._grpc_predict(payload, headers)
        else:
            return await self._http_predict(payload, headers)

    async def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        """
        The explain handler can be overridden to implement the model explanation.
        The default implementation makes an call to the explainer if explainer_host is specified
        :param headers: Dict|ModelInferRequest headers
        :param payload: Dict passed from preprocess handler
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
            body=json.dumps(payload)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)
