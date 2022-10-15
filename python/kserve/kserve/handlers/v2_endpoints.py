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
from typing import Optional, Dict

from kserve.handlers.v2_datamodels import (
    InferenceRequest, ServerMetadataResponse, ServerLiveResponse, ServerReadyResponse,
    ModelMetadataResponse, InferenceResponse
)
from kserve.handlers.dataplane import DataPlane
from kserve.handlers.model_repository_extension import ModelRepositoryExtension


class V2Endpoints:
    """V2 Endpoints
    """

    def __init__(self, dataplane: DataPlane, model_repository_extension: Optional[ModelRepositoryExtension] = None):
        self.model_repository_extension = model_repository_extension
        self.dataplane = dataplane

    async def metadata(self) -> ServerMetadataResponse:
        return ServerMetadataResponse.parse_obj(self.dataplane.metadata())

    async def live(self) -> ServerLiveResponse:
        return ServerLiveResponse.parse_obj(self.dataplane.live())

    async def ready(self) -> ServerReadyResponse:
        return ServerReadyResponse.parse_obj(self.dataplane.ready())

    async def model_metadata(self, model_name: str, model_version: Optional[str] = None) -> ModelMetadataResponse:
        # TODO: support model_version
        if model_version:
            raise NotImplementedError("Model versioning not supported yet.")

        metadata = await self.dataplane.model_metadata(model_name)
        return ModelMetadataResponse.parse_obj(metadata)

    async def infer(self, model_name: str, request_body: InferenceRequest,
                    model_version: Optional[str] = None) -> InferenceResponse:
        # TODO: support model_version
        if model_version:
            raise NotImplementedError("Model versioning not supported yet.")

        response = await self.dataplane.infer(model_name=model_name, body=request_body.dict())
        return InferenceResponse.parse_obj(response)

    async def load(self, model_name: str) -> Dict:
        self.model_repository_extension.load(model_name)
        return {"name": model_name, "load": True}

    async def unload(self, model_name: str) -> Dict:
        self.model_repository_extension.unload(model_name)
        return {"name": model_name, "unload": True}
