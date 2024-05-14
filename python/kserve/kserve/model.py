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
import time
from abc import ABC
from enum import Enum
from typing import Any, AsyncIterator, Dict, List, Optional, Union

import grpc
import httpx
import orjson
from cloudevents.http import CloudEvent
from httpx import HTTPStatusError

from .errors import InvalidInput
from .logging import logger, trace_logger
from .metrics import (
    EXPLAIN_HIST_TIME,
    POST_HIST_TIME,
    PRE_HIST_TIME,
    PREDICT_HIST_TIME,
    get_labels,
)
from .protocol.grpc import grpc_predict_v2_pb2_grpc
from .protocol.grpc.grpc_predict_v2_pb2 import ModelInferRequest, ModelInferResponse
from .protocol.infer_type import InferRequest, InferResponse

PREDICTOR_URL_FORMAT = "{0}://{1}/v1/models/{2}:predict"
EXPLAINER_URL_FORMAT = "{0}://{1}/v1/models/{2}:explain"
PREDICTOR_V2_URL_FORMAT = "{0}://{1}/v2/models/{2}/infer"
EXPLAINER_V2_URL_FORMAT = "{0}://{1}/v2/models/{2}/explain"


class BaseKServeModel(ABC):
    """
    A base class to inherit all of the kserve models from.

    This class implements the expectations of model repository and model server.
    """

    def __init__(self, name: str):
        """
        Adds the required attributes

        Args:
            name: The name of the model.
        """
        self.name = name
        self.ready = False

    def stop(self):
        """Stop handler can be overridden to perform model teardown"""
        pass


class InferenceVerb(Enum):
    EXPLAIN = 1
    PREDICT = 2
    GENERATE = 3


class PredictorProtocol(Enum):
    REST_V1 = "v1"
    REST_V2 = "v2"
    GRPC_V2 = "grpc-v2"


def is_v2(protocol: PredictorProtocol) -> bool:
    return protocol != PredictorProtocol.REST_V1


def get_latency_ms(start: float, end: float) -> float:
    return round((end - start) * 1000, 9)


class PredictorConfig:
    def __init__(
        self,
        predictor_host: str,
        predictor_protocol: str = PredictorProtocol.REST_V1.value,
        predictor_use_ssl: bool = False,
        predictor_request_timeout_seconds: int = 600,
    ):
        """The configuration for the http call to the predictor

        Args:
            predictor_host: The host name of the predictor
            predictor_protocol: The inference protocol used for predictor http call
            predictor_use_ssl: Enable using ssl for http connection to the predictor
            predictor_request_timeout_seconds: The request timeout seconds for the predictor http call
        """
        self.predictor_host = predictor_host
        self.predictor_protocol = predictor_protocol
        self.predictor_use_ssl = predictor_use_ssl
        self.predictor_request_timeout_seconds = predictor_request_timeout_seconds


class Model(BaseKServeModel):
    def __init__(self, name: str, predictor_config: Optional[PredictorConfig] = None):
        """KServe Model Public Interface

        Model is intended to be subclassed to implement the model handlers.

        Args:
            name: The name of the model.
            predictor_config: The configurations for http call to the predictor.
        """
        super().__init__(name)

        # The predictor config member fields are kept for backwards compatibility as they could be set outside
        self.protocol = (
            predictor_config.predictor_protocol
            if predictor_config
            else PredictorProtocol.REST_V1.value
        )
        self.predictor_host = (
            predictor_config.predictor_host if predictor_config else None
        )
        # The default timeout matches what is set in generated Istio virtual service resources.
        # We generally don't want things to time out at the request level here,
        # timeouts should be handled elsewhere in the system.
        self.timeout = (
            predictor_config.predictor_request_timeout_seconds
            if predictor_config
            else 600
        )
        self.use_ssl = predictor_config.predictor_use_ssl if predictor_config else False
        self.explainer_host = None
        self._http_client_instance = None
        self._grpc_client_stub = None
        self.enable_latency_logging = False

    async def __call__(
        self,
        body: Union[Dict, CloudEvent, InferRequest],
        verb: InferenceVerb = InferenceVerb.PREDICT,
        headers: Dict[str, str] = None,
    ) -> Union[Dict, InferResponse, List[str]]:
        """Method to call predictor or explainer with the given input.

        Args:
            body: Request body.
            verb: The inference verb for predict/generate/explain
            headers: Request headers.

        Returns:
            Response output from preprocess -> predict/generate/explain -> postprocess
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
            payload = (
                await self.preprocess(body, headers)
                if inspect.iscoroutinefunction(self.preprocess)
                else self.preprocess(body, headers)
            )
            preprocess_ms = get_latency_ms(start, time.time())
        payload = self.validate(payload)
        if verb == InferenceVerb.EXPLAIN:
            with EXPLAIN_HIST_TIME.labels(**prom_labels).time():
                start = time.time()
                response = (
                    (await self.explain(payload, headers))
                    if inspect.iscoroutinefunction(self.explain)
                    else self.explain(payload, headers)
                )
                explain_ms = get_latency_ms(start, time.time())
        elif verb == InferenceVerb.PREDICT:
            with PREDICT_HIST_TIME.labels(**prom_labels).time():
                start = time.time()
                response = (
                    (await self.predict(payload, headers))
                    if inspect.iscoroutinefunction(self.predict)
                    else self.predict(payload, headers)
                )
                predict_ms = get_latency_ms(start, time.time())
        else:
            raise NotImplementedError

        with POST_HIST_TIME.labels(**prom_labels).time():
            start = time.time()
            response = (
                await self.postprocess(response, headers)
                if inspect.iscoroutinefunction(self.postprocess)
                else self.postprocess(response, headers)
            )
            postprocess_ms = get_latency_ms(start, time.time())

        if self.enable_latency_logging is True:
            trace_logger.info(
                f"requestId: {request_id}, preprocess_ms: {preprocess_ms}, "
                f"explain_ms: {explain_ms}, predict_ms: {predict_ms}, "
                f"postprocess_ms: {postprocess_ms}"
            )

        return response

    @property
    def _http_client(self):
        if self._http_client_instance is None:
            self._http_client_instance = httpx.AsyncClient()
        return self._http_client_instance

    @property
    def _grpc_client(self):
        if self._grpc_client_stub is None:
            # requires appending the port to the predictor host for gRPC to work
            if ":" not in self.predictor_host:
                port = 443 if self.use_ssl else 80
                self.predictor_host = f"{self.predictor_host}:{port}"
            if self.use_ssl:
                _channel = grpc.aio.secure_channel(
                    self.predictor_host, grpc.ssl_channel_credentials()
                )
            else:
                _channel = grpc.aio.insecure_channel(self.predictor_host)
            self._grpc_client_stub = grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub(
                _channel
            )
        return self._grpc_client_stub

    def validate(self, payload):
        if isinstance(payload, ModelInferRequest):
            return payload
        if isinstance(payload, InferRequest):
            return payload
        # TODO: validate the request if self.get_input_types() defines the input types.
        if self.protocol == PredictorProtocol.REST_V2.value:
            if "inputs" in payload and not isinstance(payload["inputs"], list):
                raise InvalidInput('Expected "inputs" to be a list')
        elif self.protocol == PredictorProtocol.REST_V1.value:
            if (
                isinstance(payload, Dict)
                and "instances" in payload
                and not isinstance(payload["instances"], list)
            ):
                raise InvalidInput('Expected "instances" to be a list')
        return payload

    def load(self) -> bool:
        """Load handler can be overridden to load the model from storage.
        The `self.ready` should be set to True after the model is loaded. The flag is used for model health check.

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

    async def preprocess(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferRequest]:
        """`preprocess` handler can be overridden for data or feature transformation.
        The model decodes the request body to `Dict` for v1 endpoints and `InferRequest` for v2 endpoints.

        Args:
            payload: Payload of the request.
            headers: Request headers.

        Returns:
            A Dict or InferRequest in KServe Model Transformer mode which is transmitted on the wire to predictor.
            Tensors in KServe Predictor mode which is passed to predict handler for performing the inference.
        """

        return payload

    async def postprocess(
        self, result: Union[Dict, InferResponse], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        """The `postprocess` handler can be overridden for inference result or response transformation.
        The predictor sends back the inference result in `Dict` for v1 endpoints and `InferResponse` for v2 endpoints.

        Args:
            result: The inference result passed from `predict` handler or the HTTP response from predictor.
            headers: Request headers.

        Returns:
            A Dict or InferResponse after post-process to return back to the client.
        """
        return result

    async def _http_predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Dict:
        protocol = "https" if self.use_ssl else "http"
        predict_url = PREDICTOR_URL_FORMAT.format(
            protocol, self.predictor_host, self.name
        )
        if self.protocol == PredictorProtocol.REST_V2.value:
            predict_url = PREDICTOR_V2_URL_FORMAT.format(
                protocol, self.predictor_host, self.name
            )

        # Adjusting headers. Inject content type if not exist.
        # Also, removing host, as the header is the one passed to transformer and contains transformer's host
        predict_headers = {"Content-Type": "application/json"}
        if headers is not None:
            if "x-request-id" in headers:
                predict_headers["x-request-id"] = headers["x-request-id"]
            if "x-b3-traceid" in headers:
                predict_headers["x-b3-traceid"] = headers["x-b3-traceid"]
        if isinstance(payload, InferRequest):
            payload = payload.to_rest()
        data = orjson.dumps(payload)

        try:
            response = await self._http_client.post(
                predict_url, timeout=self.timeout, headers=predict_headers, content=data
            )
        except Exception as exc:
            request_id = predict_headers.get("x-request-id", "N.A.")
            logger.error(
                f"Could not send a request to predictor at url {predict_url} "
                f"for {request_id=} "
                f"due to exception {exc}"
            )
            raise exc

        if not response.is_success:
            message = (
                "{error_message}, '{0.status_code} {0.reason_phrase}' for url '{0.url}'"
            )
            error_message = ""
            if (
                "content-type" in response.headers
                and response.headers["content-type"] == "application/json"
            ):
                error_message = response.json()
                if "error" in error_message:
                    error_message = error_message["error"]
            message = message.format(response, error_message=error_message)
            raise HTTPStatusError(message, request=response.request, response=response)
        return orjson.loads(response.content)

    async def _grpc_predict(
        self,
        payload: Union[ModelInferRequest, InferRequest],
        headers: Dict[str, str] = None,
    ) -> ModelInferResponse:
        if isinstance(payload, InferRequest):
            payload = payload.to_grpc()
        async_result = await self._grpc_client.ModelInfer(
            request=payload,
            timeout=self.timeout,
            metadata=(
                ("request_type", "grpc_v2"),
                ("response_type", "grpc_v2"),
                ("x-request-id", headers.get("x-request-id", "")),
            ),
        )
        return async_result

    async def predict(
        self,
        payload: Union[Dict, InferRequest, ModelInferRequest],
        headers: Dict[str, str] = None,
    ) -> Union[Dict, InferResponse, AsyncIterator[Any]]:
        """The `predict` handler can be overridden for performing the inference.
            By default, the predict handler makes call to predictor for the inference step.

        Args:
            payload: Model inputs passed from `preprocess` handler.
            headers: Request headers.

        Returns:
            Inference result or a Response from the predictor.

        Raises:
            HTTPStatusError when getting back an error response from the predictor.
        """
        if not self.predictor_host:
            raise NotImplementedError("Could not find predictor_host.")
        if self.protocol == PredictorProtocol.GRPC_V2.value:
            res = await self._grpc_predict(payload, headers)
            return InferResponse.from_grpc(res)
        else:
            res = await self._http_predict(payload, headers)
            # return an InferResponse if this is REST V2, otherwise just return the dictionary
            return (
                InferResponse.from_rest(self.name, res)
                if is_v2(PredictorProtocol(self.protocol))
                else res
            )

    async def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        """`explain` handler can be overridden to implement the model explanation.
        The default implementation makes call to the explainer if ``explainer_host`` is specified.

        Args:
            payload: Explainer model inputs passed from preprocess handler.
            headers: Request headers.

        Returns:
            An Explanation for the inference result.

        Raises:
            HTTPStatusError when getting back an error response from the explainer.
        """
        if self.explainer_host is None:
            raise NotImplementedError("Could not find explainer_host.")

        protocol = "https" if self.use_ssl else "http"
        # Currently explainer only supports the kserve v1 endpoints
        explain_url = EXPLAINER_URL_FORMAT.format(
            protocol, self.explainer_host, self.name
        )
        response = await self._http_client.post(
            url=explain_url, timeout=self.timeout, content=orjson.dumps(payload)
        )

        response.raise_for_status()
        return orjson.loads(response.content)
