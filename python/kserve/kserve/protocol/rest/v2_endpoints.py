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

from typing import Optional, Dict, Union

from fastapi import FastAPI, APIRouter
from fastapi.requests import Request
from fastapi.responses import Response

from .v2_datamodels import (
    InferenceRequest,
    ServerMetadataResponse,
    ServerLiveResponse,
    ServerReadyResponse,
    ModelMetadataResponse,
    InferenceResponse,
    ModelReadyResponse,
    ListModelsResponse,
)
from ..dataplane import DataPlane
from ..model_repository_extension import ModelRepositoryExtension
from ...constants.constants import V2_ROUTE_PREFIX, PredictorProtocol
from ...errors import ModelNotReady


class V2Endpoints:
    """KServe V2 Endpoints"""

    def __init__(
        self,
        dataplane: DataPlane,
        model_repository_extension: Optional[ModelRepositoryExtension] = None,
    ):
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

    async def models(self) -> ListModelsResponse:
        """Get a list of models in the model registry.

        Returns:
            ListModelsResponse: List of models object.
        """
        models = list(self.dataplane.model_registry.get_models().keys())
        return ListModelsResponse.parse_obj({"models": models})

    async def model_metadata(
        self, model_name: str, model_version: Optional[str] = None
    ) -> ModelMetadataResponse:
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

    async def model_ready(
        self, model_name: str, model_version: Optional[str] = None
    ) -> ModelReadyResponse:
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
        request_body: Union[InferenceRequest, bytes],
        model_version: Optional[str] = None,
    ) -> Union[InferenceResponse, Response]:
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

        model_ready = self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        request_headers = dict(raw_request.headers)

        infer_request, _ = self.dataplane.decode(
            request_body,
            request_headers,
            protocol_version=PredictorProtocol.REST_V2.value,
            model_name=model_name,
        )
        response, response_headers = await self.dataplane.infer(
            model_name=model_name,
            request=infer_request,
            headers=request_headers,
        )

        response, response_headers = self.dataplane.encode(
            model_name=model_name,
            response=response,
            headers=response_headers,
            req_attributes={},
            raw_bytes=infer_request.use_binary_outputs,
        )

        if response_headers:
            raw_response.headers.update(response_headers)
        if infer_request.use_binary_outputs:
            raw_response.status_code = 200
            raw_response.body = response
            res = raw_response
        else:
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


def register_v2_endpoints(
    app: FastAPI,
    dataplane: DataPlane,
    model_repository_extension: Optional[ModelRepositoryExtension],
):
    """Register V2 endpoints.

    Args:
        app (FastAPI): FastAPI app.
        dataplane (DataPlane): DataPlane object.
        model_repository_extension (Optional[ModelRepositoryExtension]): Model repository extension.
    """
    v2_endpoints = V2Endpoints(
        dataplane=dataplane, model_repository_extension=model_repository_extension
    )
    v2_router = APIRouter(prefix=V2_ROUTE_PREFIX, tags=["V2"])
    v2_router.add_api_route(
        r"",
        v2_endpoints.metadata,
        response_model=ServerMetadataResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/health/live",
        v2_endpoints.live,
        response_model=ServerLiveResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/health/ready",
        v2_endpoints.ready,
        response_model=ServerReadyResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/models",
        v2_endpoints.models,
        response_model=ListModelsResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/models/{model_name}",
        v2_endpoints.model_metadata,
        response_model=ModelMetadataResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/models/{model_name}/versions/{model_version}",
        v2_endpoints.model_metadata,
        response_model=ModelMetadataResponse,
        methods=["GET"],
        include_in_schema=False,
    )
    v2_router.add_api_route(
        r"/models/{model_name}/ready",
        v2_endpoints.model_ready,
        response_model=ModelReadyResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/models/{model_name}/versions/{model_version}/ready",
        v2_endpoints.model_ready,
        response_model=ModelReadyResponse,
        methods=["GET"],
    )
    v2_router.add_api_route(
        r"/models/{model_name}/infer",
        v2_endpoints.infer,
        response_model=InferenceResponse,
        methods=["POST"],
    )
    v2_router.add_api_route(
        r"/models/{model_name}/versions/{model_version}/infer",
        v2_endpoints.infer,
        response_model=InferenceResponse,
        methods=["POST"],
        include_in_schema=False,
    )
    v2_router.add_api_route(
        r"/repository/models/{model_name}/load", v2_endpoints.load, methods=["POST"]
    )
    v2_router.add_api_route(
        r"/repository/models/{model_name}/unload", v2_endpoints.unload, methods=["POST"]
    )
    app.include_router(v2_router)
