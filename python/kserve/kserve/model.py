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
import logging
import time
from enum import Enum
from typing import Dict, Union, List

import grpc

import httpx
from httpx import HTTPStatusError
import orjson
from cloudevents.http import CloudEvent

from .protocol.infer_type import InferRequest
from .metrics import PRE_HIST_TIME, POST_HIST_TIME, PREDICT_HIST_TIME, EXPLAIN_HIST_TIME, get_labels
from .protocol.grpc import grpc_predict_v2_pb2_grpc
from .protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, ModelInferResponse

from .errors import InvalidInput
from .utils.utils import convert_grpc_response_to_dict, is_structured_cloudevent

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


def get_latency_ms(start: float, end: float) -> float:
    return round((end - start) * 1000, 9)


class Model:
    def __init__(self, name: str):
        """KServe Model Public Interface

        Model is intended to be subclassed by various components within KServe.

        Args:
            name (str): Name of the model.
        """
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

    async def __call__(self, body: Union[Dict, CloudEvent, ModelInferRequest],
                       model_type: ModelType = ModelType.PREDICTOR,
                       headers: Dict[str, str] = None) -> Dict:
        """Method to call predictor or explainer with the given input.

        Args:
            body (Dict|CloudEvent): Request payload body.
            model_type (ModelType): Model type enum. Can be either predictor or explainer.
            headers (Dict): Request headers.

        Returns:
            Dict: Response output from preprocess -> predictor/explainer -> postprocess
        """
        request_id = headers.get("x-request-id", "N.A.") if headers else "N.A."

        # latency vars
        preprocess_ms = 0
        explain_ms = 0
        predict_ms = 0
        postprocess_ms = 0
        prom_labels = get_labels(self.name)

        with PRE_HIST_TIME.labels(**prom_labels).time():
            start = time.time()
            payload = await self.preprocess(body, headers) if inspect.iscoroutinefunction(self.preprocess) \
                else self.preprocess(body, headers)
            preprocess_ms = get_latency_ms(start, time.time())
        payload = self.validate(payload)
        if model_type == ModelType.EXPLAINER:
            with EXPLAIN_HIST_TIME.labels(**prom_labels).time():
                start = time.time()
                response = (await self.explain(payload, headers)) if inspect.iscoroutinefunction(self.explain) \
                    else self.explain(payload, headers)
                explain_ms = get_latency_ms(start, time.time())
        elif model_type == ModelType.PREDICTOR:
            with PREDICT_HIST_TIME.labels(**prom_labels).time():
                start = time.time()
                response = (await self.predict(payload, headers)) if inspect.iscoroutinefunction(self.predict) \
                    else self.predict(payload, headers)
                predict_ms = get_latency_ms(start, time.time())
        else:
            raise NotImplementedError

        with POST_HIST_TIME.labels(**prom_labels).time():
            start = time.time()
            response = self.postprocess(response, headers)
            postprocess_ms = get_latency_ms(start, time.time())

        if self.enable_latency_logging is True:
            logging.info(f"requestId: {request_id}, preprocess_ms: {preprocess_ms}, "
                         f"explain_ms: {explain_ms}, predict_ms: {predict_ms}, "
                         f"postprocess_ms: {postprocess_ms}")

        return response

    @property
    def _http_client(self):
        if self._http_client_instance is None:
            self._http_client_instance = httpx.AsyncClient()
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
        if isinstance(payload, InferRequest):
            return payload
        # TODO: validate the request if self.get_input_types() defines the input types.
        if self.protocol == PredictorProtocol.REST_V2.value:
            if "inputs" in payload and not isinstance(payload["inputs"], list):
                raise InvalidInput("Expected \"inputs\" to be a list")
        elif self.protocol == PredictorProtocol.REST_V1.value:
            if isinstance(payload, Dict) and "instances" in payload and not isinstance(payload["instances"], list):
                raise InvalidInput("Expected \"instances\" to be a list")
        return payload

    def load(self) -> bool:
        """Load handler can be overridden to load the model from storage
        ``self.ready`` flag is used for model health check

        Returns:
            bool: True if model is ready, False otherwise
        """
        self.ready = True
        return self.ready

    def get_input_types(self) -> List[Dict]:
        # Override this function to return appropriate input format expected by your model.
        # Refer https://kserve.github.io/website/0.9/modelserving/inference_api/#model-metadata-response-json-object

        # Eg.
        # return [{ "name": "", "datatype": "INT32", "shape": [1,5], }]
        return []

    def get_output_types(self) -> List[Dict]:
        # Override this function to return appropriate output format returned by your model.
        # Refer https://kserve.github.io/website/0.9/modelserving/inference_api/#model-metadata-response-json-object

        # Eg.
        # return [{ "name": "", "datatype": "INT32", "shape": [1,5], }]
        return []

    async def preprocess(self, payload: Union[Dict, CloudEvent, ModelInferRequest],
                         headers: Dict[str, str] = None) -> Union[Dict, ModelInferRequest]:
        """`preprocess` handler can be overridden for data or feature transformation.
        The default implementation decodes to Dict if it is a binary CloudEvent
        or gets the data field from a structured CloudEvent.

        Args:
            payload (Dict|CloudEvent|ModelInferRequest): Body of the request.
            headers (Dict): Request headers.

        Returns:
            Transformed Dict|ModelInferRequest which passes to ``predict`` handler
        """
        response = payload

        if isinstance(payload, CloudEvent):
            response = payload.data
            # Try to decode and parse JSON UTF-8 if possible, otherwise
            # just pass the CloudEvent data on to the predict function.
            # This is for the cases that CloudEvent encoding is protobuf, avro etc.
            try:
                response = orjson.loads(response.decode('UTF-8'))
            except (orjson.JSONDecodeError, UnicodeDecodeError) as e:
                # If decoding or parsing failed, check if it was supposed to be JSON UTF-8
                if "content-type" in payload._attributes and \
                        (payload._attributes["content-type"] == "application/cloudevents+json" or
                         payload._attributes["content-type"] == "application/json"):
                    raise InvalidInput(f"Failed to decode or parse binary json cloudevent: {e}")

        elif isinstance(payload, dict):
            if is_structured_cloudevent(payload):
                response = payload["data"]

        return response

    def postprocess(self, response: Union[Dict, ModelInferResponse], headers: Dict[str, str] = None) \
            -> Union[Dict, ModelInferResponse]:
        """The postprocess handler can be overridden for inference response transformation

        Args:
            response (Dict|ModelInferResponse): The response passed from ``predict`` handler.
            headers (Dict): Request headers.

        Returns:
            Dict: post-processed response.
        """
        if headers:
            if "grpc" in headers.get("user-agent", "") and isinstance(response, ModelInferResponse):
                return response
            if "application/json" in headers.get("content-type", "") and isinstance(response, ModelInferResponse):
                return convert_grpc_response_to_dict(response)
        return response

    async def _http_predict(self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None) -> Dict:
        predict_url = PREDICTOR_URL_FORMAT.format(self.predictor_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            predict_url = PREDICTOR_V2_URL_FORMAT.format(self.predictor_host, self.name)

        # Adjusting headers. Inject content type if not exist.
        # Also, removing host, as the header is the one passed to transformer and contains transformer's host
        predict_headers = {'Content-Type': 'application/json'}
        if headers is not None:
            if 'x-request-id' in headers:
                predict_headers['x-request-id'] = headers['x-request-id']
            if 'x-b3-traceid' in headers:
                predict_headers['x-b3-traceid'] = headers['x-b3-traceid']
        if isinstance(payload, InferRequest):
            payload = payload.to_rest()
        data = orjson.dumps(payload)
        response = await self._http_client.post(
            predict_url,
            timeout=self.timeout,
            headers=predict_headers,
            content=data
        )
        if not response.is_success:
            message = (
                "{error_message}, '{0.status_code} {0.reason_phrase}' for url '{0.url}'"
            )
            error_message = ""
            if "content-type" in response.headers and response.headers["content-type"] == "application/json":
                error_message = response.json()
                if "error" in error_message:
                    error_message = error_message["error"]
            message = message.format(response, error_message=error_message)
            raise HTTPStatusError(message, request=response.request, response=response)
        return orjson.loads(response.content)

    async def _grpc_predict(self, payload: Union[ModelInferRequest, InferRequest], headers: Dict[str, str] = None) \
            -> ModelInferResponse:
        if isinstance(payload, InferRequest):
            payload = payload.to_grpc()
        async_result = await self._grpc_client.ModelInfer(
            request=payload,
            timeout=self.timeout,
            metadata=(('request_type', 'grpc_v2'),
                      ('response_type', 'grpc_v2'),
                      ('x-request-id', headers.get('x-request-id', '')))
        )
        return async_result

    async def predict(self, payload: Union[Dict, ModelInferRequest, InferRequest],
                      headers: Dict[str, str] = None) -> Union[Dict, ModelInferResponse]:
        """

        Args:
            payload (Dict|ModelInferRequest): Prediction data passed from ``preprocess`` handler.
            headers (Dict): Request headers.

        Returns:
            Dict|ModelInferResponse: Prediction result from the model.
        """
        if not self.predictor_host:
            raise NotImplementedError("Could not find predictor_host.")
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            return await self._grpc_predict(payload, headers)
        else:
            return await self._http_predict(payload, headers)

    async def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        """`explain` handler can be overridden to implement the model explanation.
        The default implementation makes call to the explainer if ``explainer_host`` is specified

        Args:
            payload (Dict): Dict passed from preprocess handler.
            headers (Dict): Request headers.

        Returns:
            Dict: Response from the explainer.
        """
        if self.explainer_host is None:
            raise NotImplementedError("Could not find explainer_host.")
        explain_url = EXPLAINER_URL_FORMAT.format(self.explainer_host, self.name)
        if self.protocol == PredictorProtocol.REST_V2.value:
            explain_url = EXPLAINER_V2_URL_FORMAT.format(self.explainer_host, self.name)
        response = await self._http_client.post(
            url=explain_url,
            timeout=self.timeout,
            content=orjson.dumps(payload)
        )

        response.raise_for_status()
        return orjson.loads(response.content)
