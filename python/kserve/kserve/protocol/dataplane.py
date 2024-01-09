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

import time
import logging
from importlib import metadata
from typing import Dict, Optional, Tuple, Union, cast
from inspect import iscoroutinefunction

import cloudevents.exceptions as ce
import orjson

from cloudevents.http import CloudEvent, from_http
from cloudevents.sdk.converters.util import has_binary_headers

from .rest.v2_datamodels import InferenceRequest
from ..constants import constants
from ..constants.constants import INFERENCE_CONTENT_LENGTH_HEADER, PredictorProtocol
from .grpc import grpc_predict_v2_pb2 as pb
from ..model import Model, PredictorProtocol
from ..errors import InvalidInput, ModelNotFound
from ..logging import logger
from ..model import InferenceVerb, BaseKServeModel, InferenceModel
from ..model_repository import ModelRepository
from ..utils.utils import create_response_cloudevent, is_structured_cloudevent, get_grpc_client, get_http_client, \
    get_liveness_endpoint, get_readiness_endpoint, get_model_ready_endpoint
from .infer_type import InferRequest, InferResponse
from .rest.openai import OpenAIModel

JSON_HEADERS = [
    "application/json",
    "application/cloudevents+json",
    "application/ld+json",
    "application/octet-stream",
]


class DataPlane:
    """KServe DataPlane"""

    predictor_host: str = None
    predictor_protocol: str = None
    predictor_use_ssl: bool = None
    predictor_health_check: bool = None

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry
        self._server_name = constants.KSERVE_MODEL_SERVER_NAME

        # Dynamically fetching version of the installed 'kserve' distribution. The assumption is
        # that 'kserve' will already be installed by the time this class is instantiated.
        self._server_version = metadata.version("kserve")

    @property
    def model_registry(self):
        return self._model_registry

    def get_model_from_registry(self, name: str) -> BaseKServeModel:
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)

        return model

    async def get_model(self, name: str) -> BaseKServeModel:
        """Get the model instance with the given name.

        Args:
            name (str): Model name.

        Returns:
            ModelHandleType: Instance of the model.
        """
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)
        is_ready = await self._model_registry.is_model_ready(name)
        if not is_ready:
            model.load()
        return model

    @staticmethod
    def get_binary_cloudevent(
        body: Union[str, bytes, None], headers: Dict[str, str]
    ) -> CloudEvent:
        """Helper function to parse CloudEvent body and headers.

        Args:
            body (str|bytes|None): Request body.
            headers (Dict[str, str]): Request headers.

        Returns:
            CloudEvent: A CloudEvent instance parsed from http body and headers.

        Raises:
            InvalidInput: An error when CloudEvent failed to parse.
        """
        try:
            # Use default unmarshaller if contenttype is set in header
            if "ce-contenttype" in headers:
                event = from_http(headers, body)
            else:
                event = from_http(headers, body, lambda x: x)

            return event
        except (
            ce.MissingRequiredFields,
            ce.InvalidRequiredFields,
            ce.InvalidStructuredJSON,
            ce.InvalidHeadersFormat,
            ce.DataMarshallerError,
            ce.DataUnmarshallerError,
        ) as e:
            raise InvalidInput(f"Cloud Event Exceptions: {e}")

    @staticmethod
    async def live() -> Dict[str, str]:
        """Server live.

        Returns ``{"status": "alive"}`` on successful invocation.
        Primarily meant to be used for Kubernetes liveness check.

        Returns:
            Dict: {"status": "alive"}
        """
        # If predictor host is present, then it means this is a transformer,
        # We should also need to check the predictor server's health if predictor health check is enabled.
        if DataPlane.predictor_health_check and DataPlane.predictor_host:
            if DataPlane.predictor_protocol == PredictorProtocol.GRPC_V2.value:
                grpc_client = get_grpc_client(DataPlane.predictor_host, DataPlane.predictor_use_ssl)
                res = await grpc_client.ServerLive(pb.ServerLiveRequest())
                if not res.live:
                    return {"status": "error"}
            elif (DataPlane.predictor_protocol == PredictorProtocol.REST_V1.value or
                  DataPlane.predictor_protocol == PredictorProtocol.REST_V2.value):
                res = await get_http_client().get(get_liveness_endpoint(DataPlane.predictor_host,
                                                                        DataPlane.predictor_protocol,
                                                                        DataPlane.predictor_use_ssl))
                if DataPlane.predictor_protocol == PredictorProtocol.REST_V1.value:
                    if not res.is_success or res.json().get("status", "error").lower() != "alive":
                        return {"status": "error"}
                elif DataPlane.predictor_protocol == PredictorProtocol.REST_V2.value:
                    if not res.is_success or not res.json().get("live", False):
                        return {"status": "error"}
        return {"status": "alive"}

    def metadata(self) -> Dict:
        """Server metadata.

        Note:
            Supports ``model_repository_extension`` as defined at Triton Server `Model Repository Extension`_.

        Returns:
            Returns a dict object with following fields:
                - name (str): name of the server.
                - version (str): server version number.
                - extension (list[str]): list of protocol extensions supported by this server.

        .. _Model Repository Extension:
            https://github.com/triton-inference-server/server/blob/main/docs/protocol/extension_model_repository.md
        """
        return {
            "name": self._server_name,
            "version": self._server_version,
            "extensions": ["model_repository_extension"],
        }

    async def model_metadata(self, model_name: str) -> Dict:
        """Get metadata for a specific model.

        Args:
            model_name (str): Model name

        Returns:
            Dict: dictionary with following fields:

                - name (str): name of the model
                - platform: "" (Empty String)
                - inputs: Dict with below fields
                    - name (str): name of the input
                    - datatype (str): Eg. INT32, FP32
                    - shape ([]int): The shape of the tensor.
                                   Variable-size dimensions are specified as -1.
                - outputs: Same as inputs described above.

        Note:
            For more information, check KServe v2 `Model Metadata`_ documentation.

        Raises:
            ModelNotFound: exception will be raised if the model with model_name is not found.

        .. _Model Metadata:
            https://github.com/kserve/kserve/blob/master/docs/predict-api/v2/required_api.md#model-metadata
        """
        # TODO: model versioning is not supported yet
        model = self.get_model_from_registry(model_name)

        if not isinstance(model, InferenceModel):
            raise ValueError(
                f"Model of type {type(model).__name__} does not support inference"
            )
        input_types = (
            await model.get_input_types()
            if iscoroutinefunction(model.get_input_types)
            else model.get_input_types()
        )
        output_types = (
            await model.get_output_types()
            if iscoroutinefunction(model.get_output_types)
            else model.get_output_types()
        )

        return {
            "name": model_name,
            "platform": "",
            "inputs": input_types,
            "outputs": output_types,
        }

    @staticmethod
    async def ready() -> bool:
        """Server ready.

        Returns ``True``. Primarily meant to be used as Kubernetes readiness check.

        Returns:
            bool: True
        """
        # If predictor host is present, then it means this is a transformer,
        # We should also need to check the predictor server's health if predictor health check is enabled.
        if DataPlane.predictor_health_check and DataPlane.predictor_host:
            if DataPlane.predictor_protocol == PredictorProtocol.GRPC_V2.value:
                grpc_client = get_grpc_client(DataPlane.predictor_host, DataPlane.predictor_use_ssl)
                res = await grpc_client.ServerReady(pb.ServerReadyRequest())
                if not res.ready:
                    return False
            elif (DataPlane.predictor_protocol == PredictorProtocol.REST_V1.value or
                  DataPlane.predictor_protocol == PredictorProtocol.REST_V2.value):
                res = await get_http_client().get(get_readiness_endpoint(DataPlane.predictor_host,
                                                                         DataPlane.predictor_protocol,
                                                                         DataPlane.predictor_use_ssl))
                if DataPlane.predictor_protocol == PredictorProtocol.REST_V1.value:
                    if not res.is_success or res.json().get("status", "error").lower() != "alive":
                        return False
                elif DataPlane.predictor_protocol == PredictorProtocol.REST_V2.value:
                    if not res.is_success or not res.json().get("ready", False):
                        return False
        return True

    async def model_ready(self, model_name: str) -> bool:
        """Check if a model is ready.

        Args:
            model_name (str): name of the model

        Returns:
            bool: True if the model is ready, False otherwise.

        Raises:
            ModelNotFound: exception if model is not found
        """

        # If predictor host is present, then it means this is a transformer,
        # We should also check the predictor model's health if predictor health check is enabled.
        if DataPlane.predictor_health_check and DataPlane.predictor_host:
            if DataPlane.predictor_protocol == PredictorProtocol.GRPC_V2.value:
                grpc_client = get_grpc_client(DataPlane.predictor_host, DataPlane.predictor_use_ssl)
                res = await grpc_client.ModelReady(pb.ModelReadyRequest(name=model_name))
                if not res.ready:
                    return False
            elif (DataPlane.predictor_protocol == PredictorProtocol.REST_V1.value or
                  DataPlane.predictor_protocol == PredictorProtocol.REST_V2.value):
                res = await get_http_client().get(get_model_ready_endpoint(DataPlane.predictor_host,
                                                                           DataPlane.predictor_protocol,
                                                                           DataPlane.predictor_use_ssl,
                                                                           model_name))
                # Need to convert the response to str, because v2 endpoint returns a bool value while v1 endpoint
                # returns a string value.
                if not res.is_success or str(res.json().get("ready", "False")).lower() == "false":
                    return False
        if self._model_registry.get_model(model_name) is None:
            raise ModelNotFound(model_name)

        return await self._model_registry.is_model_ready(model_name)

    def decode(
        self,
        body,
        headers,
        protocol_version: str = PredictorProtocol.REST_V1.value,
        model_name: str = None,
    ) -> Tuple[Union[Dict, InferRequest], Dict]:
        t1 = time.time()
        attributes = {}
        if isinstance(body, InferRequest):
            return body, attributes
        elif isinstance(body, InferenceRequest) or (
            protocol_version.lower() == PredictorProtocol.REST_V2.value
            and isinstance(body, bytes)
        ):
            return self.decode_inference_request(body, headers, model_name), attributes
        if headers:
            if has_binary_headers(headers):
                # returns CloudEvent
                body = self.get_binary_cloudevent(body, headers)
            elif (
                "content-type" in headers
                and headers["content-type"] not in JSON_HEADERS
            ):
                return body, attributes
        if type(body) is bytes:
            try:
                body = orjson.loads(body)
            except orjson.JSONDecodeError as e:
                raise InvalidInput(f"Unrecognized request format: {e}")

        decoded_body, attributes = self.decode_cloudevent(body)
        t2 = time.time()
        logger.debug(f"decoded request in {round((t2 - t1) * 1000, 9)}ms")
        return decoded_body, attributes

    def decode_cloudevent(self, body) -> Tuple[Union[Dict, InferRequest], Dict]:
        decoded_body = body
        attributes = {}
        if isinstance(body, CloudEvent):
            attributes = body._get_attributes()
            decoded_body = body.get_data()
            try:
                decoded_body = orjson.loads(decoded_body.decode("UTF-8"))
            except (orjson.JSONDecodeError, UnicodeDecodeError) as e:
                # If decoding or parsing failed, check if it was supposed to be JSON UTF-8
                if "content-type" in body._attributes and (
                    body._attributes["content-type"] == "application/cloudevents+json"
                    or body._attributes["content-type"] == "application/json"
                ):
                    raise InvalidInput(
                        f"Failed to decode or parse binary json cloudevent: {e}"
                    )

        elif isinstance(body, dict):
            if is_structured_cloudevent(body):
                decoded_body = body["data"]
                attributes = body
                del attributes["data"]
        return decoded_body, attributes

    def decode_inference_request(
        self, body: Union[bytes, InferenceRequest], headers: Dict, model_name: str
    ) -> InferRequest:
        if isinstance(body, bytes):
            json_length = headers.get(INFERENCE_CONTENT_LENGTH_HEADER, None)
            if json_length is None:
                raise InvalidInput(
                    f"received byte inputs, but the"
                    f"'{INFERENCE_CONTENT_LENGTH_HEADER}' header is missing."
                )
            return InferRequest.from_bytes(body, int(json_length), model_name)
        else:
            return InferRequest.from_inference_request(body, model_name)

    def encode(
        self,
        model_name,
        response,
        headers,
        req_attributes: Dict,
    ) -> Tuple[Dict, Dict[str, str]]:
        response_headers = {}
        # if we received a cloudevent, then also return a cloudevent
        is_cloudevent = False
        is_binary_cloudevent = False
        if isinstance(response, InferResponse):
            response, json_size = response.to_rest()
            if json_size is not None:
                response_headers[INFERENCE_CONTENT_LENGTH_HEADER] = str(json_size)
        if headers:
            if has_binary_headers(headers):
                is_cloudevent = True
                is_binary_cloudevent = True
            if headers.get("content-type", "") == "application/cloudevents+json":
                is_cloudevent = True
        if is_cloudevent:
            response_headers, response = create_response_cloudevent(
                model_name, response, req_attributes, is_binary_cloudevent
            )

            if is_binary_cloudevent:
                response_headers["content-type"] = "application/json"
            else:
                response_headers["content-type"] = "application/cloudevents+json"
        return response, response_headers

    async def infer(
        self,
        model_name: str,
        request: Union[Dict, InferRequest],
        headers: Optional[Dict[str, str]] = None,
    ) -> Tuple[Union[Dict, InferResponse], Dict[str, str]]:
        """Performs inference on the specified model with the provided body and headers.

        If the ``body`` contains an encoded `CloudEvent`_, then it will be decoded and processed.
        The response body/headers will also be encoded as CloudEvents.

        Args:
            model_name (str): Model name.
            request (bytes|Dict): Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            Tuple[Union[str, bytes, Dict], Dict[str, str]]:
                - response: The inference result.
                - response_headers: Headers to construct the HTTP response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.

        .. _CloudEvent: https://cloudevents.io/
        """
        # call model locally or remote model workers
        response_headers = {}
        model = await self.get_model(model_name)
        if isinstance(model, OpenAIModel):
            error_msg = f"Model {model_name} is of type OpenAIModel. It does not support the infer method."
            raise InvalidInput(reason=error_msg)
        if not isinstance(model, InferenceModel):
            raise ValueError(
                f"Model of type {type(model).__name__} does not support inference"
            )
        model = cast(InferenceModel, model)
        response, res_headers = await model(request, headers=headers)
        response_headers.update(res_headers)
        return response, response_headers

    async def explain(
        self,
        model_name: str,
        request: Union[bytes, Dict, InferRequest],
        headers: Optional[Dict[str, str]] = None,
    ) -> Tuple[Union[str, bytes, Dict, InferResponse], Dict[str, str]]:
        """Performs explanation for the specified model.

        Args:
            model_name (str): Model name to be used for explanation.
            request (bytes|Dict): Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            Dict: Explanation result.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        # call model locally or remote model workers
        response_headers = headers if headers else {}
        model = await self.get_model(model_name)
        if isinstance(model, OpenAIModel):
            logger.warning(
                f"Model {model_name} is of type OpenAIModel. It does not support the explain method."
                " A request exercised this path and will cause a server crash."
            )
        if not isinstance(model, InferenceModel):
            raise ValueError(
                f"Model of type {type(model).__name__} does not support inference"
            )
        response, res_headers = await model(
            request, verb=InferenceVerb.EXPLAIN, headers=headers
        )
        response_headers.update(res_headers)
        return response, response_headers
