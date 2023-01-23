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

from typing import Dict, Union, Tuple, Optional

import cloudevents.exceptions as ce
import orjson
import pkg_resources
from cloudevents.http import CloudEvent, from_http
from cloudevents.sdk.converters.util import has_binary_headers
from ray.serve.api import RayServeHandle

from ..model import Model
from ..errors import InvalidInput, ModelNotFound
from ..model import ModelType
from ..model_repository import ModelRepository
from ..utils.utils import create_response_cloudevent
from .infer_type import InferRequest
from ..constants import constants
from .grpc import grpc_predict_v2_pb2 as pb
import time
import logging


class DataPlane:
    """KServe DataPlane
    """

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry
        self._server_name = constants.KSERVE_MODEL_SERVER_NAME

        # Dynamically fetching version of the installed 'kserve' distribution. The assumption is
        # that 'kserve' will already be installed by the time this class is instantiated.
        self._server_version = pkg_resources.get_distribution("kserve").version

    @property
    def model_registry(self):
        return self._model_registry

    def get_model_from_registry(self, name: str) -> Union[Model, RayServeHandle]:
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)

        return model

    def get_model(self, name: str) -> Union[Model, RayServeHandle]:
        """Get the model instance with the given name.

        The instance can be either ``Model`` or ``RayServeHandle``.

        Args:
            name (str): Model name.

        Returns:
            Model|RayServeHandle: Instance of the model.
        """
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)
        if not self._model_registry.is_model_ready(name):
            model.load()
        return model

    @staticmethod
    def get_binary_cloudevent(body: Union[str, bytes, None], headers: Dict[str, str]) -> CloudEvent:
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
        except (ce.MissingRequiredFields, ce.InvalidRequiredFields, ce.InvalidStructuredJSON,
                ce.InvalidHeadersFormat, ce.DataMarshallerError, ce.DataUnmarshallerError) as e:
            raise InvalidInput(f"Cloud Event Exceptions: {e}")

    @staticmethod
    async def live() -> Dict[str, str]:
        """Server live.

        Returns ``{"status": "alive"}`` on successful invocation.
        Primarily meant to be used for Kubernetes liveness check.

        Returns:
            Dict: {"status": "alive"}
        """
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
            "extensions": ["model_repository_extension"]
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

        if not isinstance(model, RayServeHandle):
            input_types = model.get_input_types()
            output_types = model.get_output_types()
        else:
            model_handle: RayServeHandle = model
            input_types = await model_handle.get_input_types.remote()
            output_types = await model_handle.get_output_types.remote()
        return {
            "name": model_name,
            "platform": "",
            "inputs": input_types,
            "outputs": output_types
        }

    @staticmethod
    async def ready() -> bool:
        """Server ready.

        Returns ``True``. Primarily meant to be used as Kubernetes readiness check.

        Returns:
            bool: True
        """
        return True

    def model_ready(self, model_name: str) -> bool:
        """Check if a model is ready.

        Args:
            model_name (str): name of the model

        Returns:
            bool: True if the model is ready, False otherwise.

        Raises:
            ModelNotFound: exception if model is not found
        """
        if self._model_registry.get_model(model_name) is None:
            raise ModelNotFound(model_name)

        return self._model_registry.is_model_ready(model_name)

    def decode(self, body, headers) -> Union[Dict, InferRequest]:
        t1 = time.time()
        if isinstance(body, InferRequest):
            return body
        if headers and has_binary_headers(headers):
            body = self.get_binary_cloudevent(body, headers)
        else:
            if type(body) is bytes:
                try:
                    body = orjson.loads(body)
                except orjson.JSONDecodeError as e:
                    raise InvalidInput(f"Unrecognized request format: {e}")
        t2 = time.time()
        logging.debug(f"decoded request in {round((t2 - t1) * 1000, 9)}ms")
        return body

    def encode(self, model_name, body, response, headers) -> Tuple[Dict, Dict[str, str]]:
        response_headers = {}
        # if we received a cloudevent, then also return a cloudevent
        is_cloudevent = False
        is_binary_cloudevent = False
        if headers:
            if has_binary_headers(headers):
                is_cloudevent = True
                is_binary_cloudevent = True
            if headers.get("content-type", "") == "application/cloudevents+json":
                is_cloudevent = True
        if is_cloudevent:
            response_headers, response = create_response_cloudevent(model_name, body, response,
                                                                    is_binary_cloudevent)

            if is_binary_cloudevent:
                response_headers["content-type"] = "application/json"
            else:
                response_headers["content-type"] = "application/cloudevents+json"
        return response, response_headers

    async def infer(
            self,
            model_name: str,
            body: Union[bytes, Dict, InferRequest],
            headers: Optional[Dict[str, str]] = None
    ) -> Tuple[Union[str, bytes, Dict, pb.ModelInferResponse], Dict[str, str]]:
        """Performs inference on the specified model with the provided body and headers.

        If the ``body`` contains an encoded `CloudEvent`_, then it will be decoded and processed.
        The response body/headers will also be encoded as CloudEvents.

        Args:
            model_name (str): Model name.
            body (bytes|Dict): Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            Tuple[Union[str, bytes, Dict], Dict[str, str]]:
                - response: The inference result.
                - response_headers: Headers to construct the HTTP response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.

        .. _CloudEvent: https://cloudevents.io/
        """
        body = self.decode(body, headers)

        # call model locally or remote model workers
        model = self.get_model(model_name)
        if not isinstance(model, RayServeHandle):
            response = await model(body, headers=headers)
        else:
            model_handle: RayServeHandle = model
            response = await model_handle.remote(body)

        response, response_headers = self.encode(model_name, body, response, headers)
        return response, response_headers

    async def explain(self, model_name: str,
                      body: Union[bytes, Dict, InferRequest],
                      headers: Optional[Dict[str, str]] = None
                      ) -> Tuple[Union[str, bytes, Dict], Dict[str, str]]:
        """Performs explanation for the specified model.

        Args:
            model_name (str): Model name to be used for explanation.
            body (bytes|Dict): Request body data.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            Dict: Explanation result.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        body = self.decode(body, headers)

        # call model locally or remote model workers
        model = self.get_model(model_name)
        if not isinstance(model, RayServeHandle):
            response = await model(body, model_type=ModelType.EXPLAINER)
        else:
            model_handle = model
            response = await model_handle.remote(body, model_type=ModelType.EXPLAINER)
        response, response_headers = self.encode(model_name, body, response, headers)
        return response, response_headers
