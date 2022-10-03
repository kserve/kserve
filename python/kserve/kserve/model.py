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
import logging
import time
import sys
import inspect
import json
import tornado.web
import tornado.log
from tornado.httpclient import AsyncHTTPClient
from cloudevents.http import CloudEvent
from http import HTTPStatus
from enum import Enum
from kserve.utils.utils import is_structured_cloudevent
import grpc
from prometheus_client import Histogram
from google.protobuf.json_format import MessageToJson
from kserve.grpc import grpc_predict_v2_pb2_grpc
from kserve.grpc.grpc_predict_v2_pb2 import (ModelInferRequest,
                                             ModelInferResponse)

tornado.log.enable_pretty_logging()

PREDICTOR_URL_FORMAT = "http://{0}/v1/models/{1}:predict"
EXPLAINER_URL_FORMAT = "http://{0}/v1/models/{1}:explain"
PREDICTOR_V2_URL_FORMAT = "http://{0}/v2/models/{1}/infer"
EXPLAINER_V2_URL_FORMAT = "http://{0}/v2/models/{1}/explain"

PRE_HIST_TIME = Histogram('request_preprocessing_seconds', 'pre-processing request latency')
POST_HIST_TIME = Histogram('request_postprocessing_seconds', 'post-processing request latency')
PREDICT_HIST_TIME = Histogram('request_predict_processing_seconds', 'prediction request latency')
EXPLAIN_HIST_TIME = Histogram('request_explain_processing_seconds', 'explain request latency')


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


def get_latency_ms(start, end):
    return round((end - start) * 1000, 9)


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
        self.enable_latency_logging = False

    async def __call__(self, body, model_type: ModelType = ModelType.PREDICTOR, headers: Dict[str, str] = None):
        request_id = headers.get("X-Request-Id")
        # latency vars
        preprocess_ms = 0
        explain_ms = 0
        predict_ms = 0
        postprocess_ms = 0

        with PRE_HIST_TIME.time():
            start = time.time()
            payload = await self.preprocess(body, headers) if inspect.iscoroutinefunction(self.preprocess) \
                else self.preprocess(body, headers)
            preprocess_ms = get_latency_ms(start, time.time())
        payload = self.validate(payload)
        if model_type == ModelType.EXPLAINER:
            with EXPLAIN_HIST_TIME.time():
                start = time.time()
                response = (await self.explain(payload, headers)) if inspect.iscoroutinefunction(self.explain) \
                    else self.explain(payload, headers)
                explain_ms = get_latency_ms(start, time.time())
        elif model_type == ModelType.PREDICTOR:
            with PREDICT_HIST_TIME.time():
                start = time.time()
                response = (await self.predict(payload, headers)) if inspect.iscoroutinefunction(self.predict) \
                    else self.predict(payload, headers)
                predict_ms = get_latency_ms(start, time.time())
        else:
            raise NotImplementedError

        with POST_HIST_TIME.time():
            start = time.time()
            response = self.postprocess(response, headers)
            postprocess_ms = get_latency_ms(start, time.time())

        if self.enable_latency_logging is True:
            logging.info(f"requestId: {request_id}, preprocess_ms: {preprocess_ms}, explain_ms: {explain_ms}, "
                         f"predict_ms: {predict_ms}, postprocess_ms: {postprocess_ms}")

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
            self._grpc_client_stub = grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub(_channel)
        return self._grpc_client_stub

    def validate(self, payload):
        if isinstance(payload, ModelInferRequest):
            return payload
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

    async def preprocess(
        self,
        payload: Union[Dict, CloudEvent, ModelInferRequest],
        headers: Dict[str, str] = None
    ) -> Union[Dict, ModelInferRequest]:
        """
        The preprocess handler can be overridden for data or feature transformation.
        The default implementation decodes to Dict if it is a binary CloudEvent
        or gets the data field from a structured CloudEvent.
        :param payload: Dict|CloudEvent|ModelInferRequest body
        :param headers: Dict
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

    def postprocess(
        self,
        response: Union[Dict, ModelInferResponse],
        headers: Dict[str, str] = None
    ) -> Union[Dict, ModelInferResponse]:
        """
        The postprocess handler can be overridden for inference response transformation
        :param response: Dict|ModelInferResponse passed from predict handler
        :param headers: Dict
        :return: Dict
        """
        if headers:
            if "grpc" in headers.get("user-agent", "") and isinstance(response, ModelInferResponse):
                return response
            if "application/json" in headers.get("Content-Type", "") and isinstance(response, ModelInferResponse):
                return json.loads(
                    MessageToJson(response, preserving_proto_field_name=True, including_default_value_fields=True))
        return response

    async def _http_predict(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        predict_url = PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            predict_url = PREDICTOR_V2_URL_FORMAT.format(self.predictor_host, self.name)

        # Adjusting headers. Inject content type if not exist.
        # Also, removing host, as the header is the one passed to transformer and contains transformer's host
        predict_headers = {'Content-Type': 'application/json'}
        if headers is not None:
            if 'X-Request-Id' in headers:
                predict_headers['X-Request-Id'] = headers['X-Request-Id']
            if 'X-B3-Traceid' in headers:
                predict_headers['X-B3-Traceid'] = headers['X-B3-Traceid']

        response = await self._http_client.fetch(
            predict_url,
            method='POST',
            request_timeout=self.timeout,
            headers=predict_headers,
            body=json.dumps(payload)
        )
        if response.code != 200:
            raise tornado.web.HTTPError(
                status_code=response.code,
                reason=response.body)
        return json.loads(response.body)

    async def _grpc_predict(self, payload: ModelInferRequest, headers: Dict[str, str] = None) -> ModelInferResponse:
        async_result = await self._grpc_client.ModelInfer(
            request=payload,
            timeout=self.timeout,
            metadata=(('request_type', 'grpc_v2'), ('response_type', 'grpc_v2'))
        )
        return async_result

    async def predict(self, payload: Union[Dict, ModelInferRequest],
                      headers: Dict[str, str] = None) -> Union[Dict, ModelInferResponse]:
        """
        The predict handler can be overridden to implement the model inference.
        The default implementation makes a call to the predictor if predictor_host is specified
        :param payload: Dict|ModelInferRequest body passed from preprocess handler
        :param headers: Dict
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
        :param payload: Dict passed from preprocess handler
        :param headers: Dict
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

    async def metadata(self):
        return {
            "name": self.name,
            "versions": [],
            "platform": "",
            "inputs": [],
            "outputs": []
        }
