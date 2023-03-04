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

from typing import Optional, Dict

from fastapi.requests import Request
from fastapi.responses import Response

from ..infer_type import InferInput, InferRequest
from .v2_datamodels import (
    InferenceRequest, ServerMetadataResponse, ServerLiveResponse, ServerReadyResponse,
    ModelMetadataResponse, InferenceResponse, ModelReadyResponse
)
from ..dataplane import DataPlane
from ..model_repository_extension import ModelRepositoryExtension
from ...errors import ModelNotReady


class V2Endpoints:
    """KServe V2 Endpoints
    """

    def __init__(self, dataplane: DataPlane, model_repository_extension: Optional[ModelRepositoryExtension] = None):
        self.model_repository_extension = model_repository_extension
        self.dataplane = dataplane

    async def metadata(self) -> ServerMetadataResponse:
        """Server metadata endpoint.

        Returns:
            ServerMetadataResponse: Server metadata JSON object.
        """
        return ServerMetadataResponse.parse_obj(self.dataplane.metadata())

    @staticmethod
    async def live() -> ServerLiveResponse:
        """Server live endpoint.

        Returns:
            ServerLiveResponse: Server live message.
        """
        return ServerLiveResponse(live=True)

    @staticmethod
    async def ready() -> ServerReadyResponse:
        """Server ready endpoint.

        Returns:
            ServerReadyResponse: Server ready message.
        """
        return ServerReadyResponse(ready=True)

    async def model_metadata(self, model_name: str, model_version: Optional[str] = None) -> ModelMetadataResponse:
        """Model metadata handler. It provides information about a model.

        Args:
            model_name (str): Model name.
            model_version (Optional[str]): Model version (optional).

        Returns:
            ModelMetadataResponse: Model metadata object.
        """
        # TODO: support model_version
        if model_version:
            raise NotImplementedError("Model versioning not supported yet.")

        metadata = await self.dataplane.model_metadata(model_name)
        return ModelMetadataResponse.parse_obj(metadata)

    async def model_ready(self, model_name: str, model_version: Optional[str] = None) -> ModelReadyResponse:
        """Check if a given model is ready.

        Args:
            model_name (str): Model name.
            model_version (str): Model version.

        Returns:
            ModelReadyResponse: Model ready object
        """
        # TODO: support model_version
        if model_version:
            raise NotImplementedError("Model versioning not supported yet.")

        model_ready = self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        return ModelReadyResponse.parse_obj({"name": model_name, "ready": model_ready})

    async def infer(
        self,
        raw_request: Request,
        raw_response: Response,
        model_name: str,
        request_body: InferenceRequest,
        model_version: Optional[str] = None
    ) -> InferenceResponse:
        """Infer handler.

        Args:
            raw_request (Request): fastapi request object,
            raw_response (Response): fastapi response object,
            model_name (str): Model name.
            request_body (InferenceRequest): Inference request body.
            model_version (Optional[str]): Model version (optional).

        Returns:
            InferenceResponse: Inference response object.
        """
        # TODO: support model_version
        if model_version:
            raise NotImplementedError("Model versioning not supported yet.")

        request_headers = dict(raw_request.headers)
        infer_inputs = [InferInput(name=input.name, shape=input.shape, datatype=input.datatype,
                                   data=input.data) for input in request_body.inputs]
        infer_request = InferRequest(model_name=model_name, infer_inputs=infer_inputs)
        response, response_headers = await self.dataplane.infer(
            model_name=model_name, body=infer_request, headers=request_headers)

        if response_headers:
            raw_response.headers.update(response_headers)
        res = InferenceResponse.parse_obj(response)
        return res

    async def load(self, model_name: str) -> Dict:
        """Model load handler.

        Args:
            model_name (str): Model name.

        Returns:
            Dict: {"name": model_name, "load": True}
        """
        await self.model_repository_extension.load(model_name)
        return {"name": model_name, "load": True}

    async def unload(self, model_name: str) -> Dict:
        """Model unload handler.

        Args:
            model_name (str): Model name.

        Returns:
            Dict: {"name": model_name, "unload": True}
        """
        await self.model_repository_extension.unload(model_name)
        return {"name": model_name, "unload": True}
