import json
from typing import Dict, Union, Tuple

import cloudevents.exceptions as ce
import pkg_resources
from cloudevents.http import CloudEvent, from_http
from cloudevents.sdk.converters.util import has_binary_headers
from ray.serve.api import RayServeHandle

from ..constants import constants
from kserve import Model
from kserve.errors import InvalidInput, ModelNotFound
from kserve.model import ModelType
from kserve.model_repository import ModelRepository

from ray.serve.api import RayServeHandle
from kserve.utils.utils import is_structured_cloudevent, create_response_cloudevent


class DataPlane:

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry
        self._server_name = constants.KSERVE_MODEL_SERVER_NAME

        # Dynamically fetching version of the installed 'kserve' distribution. The assumption is
        # that 'kserve' will already be installed by the time this class is instantiated.
        self._server_version = pkg_resources.get_distribution("kserve").version

    def get_model_from_registry(self, name: str) -> Union[Model, RayServeHandle]:
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)

        return model

    def get_model(self, name: str) -> Union[Model, RayServeHandle]:
        model = self._model_registry.get_model(name)
        if model is None:
            raise ModelNotFound(name)
        if not self._model_registry.is_model_ready(name):
            model.load()
        return model

    @staticmethod
    def get_binary_cloudevent(body: Union[str, bytes, None], headers: Dict[str, str]) -> CloudEvent:
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
    async def live() -> dict:
        """
        Returns status alive on successful invocation. Primarily meant to be used as
        kubernetes liveness check

        Returns:
            Dict: {"status": "alive"}
        """
        response = {"status": "alive"}
        return response

    async def metadata(self) -> dict:
        """
        Returns metadata for this server
        Returns:
            Returns a dict object with following fields:
                - name (str): name of the server
                - version (str): server version number
                - extension (list[str]): list of protocol extensions supported by this server
        """
        return {
            "name": self._server_name,
            "version": self._server_version,
            # supports model_repository_extension as defined at
            # https://github.com/triton-inference-server/server/blob/main/docs/protocol
            # /extension_model_repository.md#model-repository-extension
            "extensions": ["model_repository_extension"]
        }

    async def model_metadata(self, model_name: str) -> Dict:
        """
        Returns metadata for a specific model.

        Args:
            model_name (str): Model name

        Returns:
            A dictionary with following fields:
                - name (str): name of the model
                - platform: "" (Empty String)
                - inputs: Dict with below fields
                    - name (str): name of the input
                    - datatype (str): Eg. INT32, FP32
                    - shape ([]int): The shape of the tensor.
                                   Variable-size dimensions are specified as -1.
}
                - outputs: Same as inputs described above.
            For more info refer:
            https://kserve.github.io/website/0.9/modelserving/inference_api/#model-metadata

        Raises: 'ModelNotFound' exception will be raised if the model with model_name is not found.

        """

        # Note: model versioning is not supported
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

    async def ready(self) -> bool:
        """
        Returns 'True' when all the loaded models are ready to receive requests.
        Primarily meant to be used as kubernetes readiness check.

        Returns:
            True, when all registered models are ready
            False, otherwise
        """

        models = self._model_registry.get_models().values()
        is_ready = all([model.ready for model in models])
        return is_ready

    async def model_ready(self, model_name: str) -> bool:
        """
        Returns if the model specified by 'model_name' is ready

        Args:
            model_name (str): name of the model

        Returns:
            False, if the model is not ready or model is not found
            True, otherwise
        """
        is_ready = self._model_registry.is_model_ready(model_name)
        return is_ready

    async def infer(
            self,
            model_name: str,
            body: bytes,
            headers: Dict[str, str] = None
    ) -> Tuple[Union[str, bytes, Dict], Dict[str, str]]:
        """
        Performs inference on the specified model with the provided body and headers.
        If the 'body' contains an encoded [CloudEvent](https://cloudevents.io/),
        then it will be decoded and processed. The response body/headers will also be encoded as
        CloudEvents.

        Args:
            model_name (str): Model name
            body (bytes): The raw body of the received HTTP request.
            headers (Dict[str,str]): The headers received for HTTP request

        Returns:
            A tuple with below elements:
                - response: The inference result
                - response_headers: Headers to construct the HTTP response
        """

        is_cloudevent = False
        is_binary_cloudevent = False

        if headers and has_binary_headers(headers):
            is_cloudevent = True
            is_binary_cloudevent = True
            body = self.get_binary_cloudevent(body, headers)
        else:
            try:
                body = json.loads(body)
            except json.decoder.JSONDecodeError as e:
                raise InvalidInput(f"Unrecognized request format: {e}")

            if is_structured_cloudevent(body):
                is_cloudevent = True

        # call model locally or remote model workers
        model = self.get_model(model_name)
        if not isinstance(model, RayServeHandle):
            response = await model(body)
        else:
            model_handle: RayServeHandle = model
            response = await model_handle.remote(body)

        response_headers = {}
        # if we received a cloudevent, then also return a cloudevent
        if is_cloudevent:
            response_headers, response = create_response_cloudevent(model_name, body, response,
                                                                    is_binary_cloudevent)

            if is_binary_cloudevent:
                response_headers["Content-Type"] = "application/json"
            else:
                response_headers["Content-Type"] = "application/cloudevents+json"

        return response, response_headers

    async def explain(self, model_name: str, body: bytes) -> Dict:
        """
        Performs explanation for the specified model.

        Args:
            model_name (str): Model name to be used for explanation
            body (bytes): Raw HTTP request body

        Returns:
            Result explanation result
        """
        try:
            body = json.loads(body)
        except json.decoder.JSONDecodeError as e:
            raise InvalidInput(f"Unrecognized request format: {e}")
        # call model locally or remote model workers
        model = self.get_model(model_name)
        if not isinstance(model, RayServeHandle):
            response = await model(body, model_type=ModelType.EXPLAINER)
        else:
            model_handle = model
            response = await model_handle.remote(body, model_type=ModelType.EXPLAINER)
        return response
