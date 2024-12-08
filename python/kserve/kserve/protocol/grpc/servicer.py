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

from grpc import ServicerContext

from . import grpc_predict_v2_pb2 as pb
from . import grpc_predict_v2_pb2_grpc
from ..infer_type import InferRequest, InferResponse
from ..dataplane import DataPlane
from ..model_repository_extension import ModelRepositoryExtension
from ...errors import InvalidInput
from ...utils.utils import to_headers


class InferenceServicer(grpc_predict_v2_pb2_grpc.GRPCInferenceServiceServicer):

    def __init__(
        self,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
    ):
        super().__init__()
        self._data_plane = data_plane
        self._mode_repository_extension = model_repository_extension

    @classmethod
    def validate_grpc_request(cls, request: pb.ModelInferRequest):
        raw_inputs_length = len(request.raw_input_contents)
        if raw_inputs_length != 0 and len(request.inputs) != raw_inputs_length:
            raise InvalidInput(
                f"the number of inputs ({len(request.inputs)}) does not match the expected number of "
                f"raw input contents ({raw_inputs_length}) for model '{request.model_name}'."
            )
        if raw_inputs_length != 0:
            for input_ in request.inputs:
                if input_.HasField("contents"):
                    raise InvalidInput(
                        f"contents field must not be specified when using raw_input_contents for input '{input_.name}' for model '{request.model_name}'"
                    )

    async def ServerMetadata(self, request: pb.ServerMetadataRequest, context):
        metadata = self._data_plane.metadata()
        return pb.ServerMetadataResponse(
            name=metadata["name"],
            version=metadata["version"],
            extensions=metadata["extensions"],
        )

    async def ServerLive(
        self, request: pb.ServerLiveRequest, context
    ) -> pb.ServerLiveResponse:
        response = await self._data_plane.live()
        is_live = response["status"] == "alive"
        return pb.ServerLiveResponse(live=is_live)

    async def ServerReady(
        self, request: pb.ServerReadyRequest, context
    ) -> pb.ServerReadyResponse:
        is_ready = await self._data_plane.ready()
        return pb.ServerReadyResponse(ready=is_ready)

    async def ModelReady(
        self, request: pb.ModelReadyRequest, context
    ) -> pb.ModelReadyResponse:
        is_ready = await self._data_plane.model_ready(model_name=request.name)
        return pb.ModelReadyResponse(ready=is_ready)

    async def ModelMetadata(
        self, request: pb.ModelMetadataRequest, context
    ) -> pb.ModelMetadataResponse:
        metadata = await self._data_plane.model_metadata(model_name=request.name)
        return pb.ModelMetadataResponse(
            name=metadata["name"],
            platform=metadata["platform"],
            inputs=metadata["inputs"],
            outputs=metadata["outputs"],
        )

    async def RepositoryModelLoad(
        self, request: pb.RepositoryModelLoadRequest, context
    ) -> pb.RepositoryModelLoadResponse:
        response = await self._mode_repository_extension.load(
            model_name=request.model_name
        )
        return pb.RepositoryModelLoadResponse(
            model_name=response["name"], isLoaded=response["load"]
        )

    async def RepositoryModelUnload(
        self, request: pb.RepositoryModelUnloadRequest, context
    ) -> pb.RepositoryModelUnloadResponse:
        response = await self._mode_repository_extension.unload(
            model_name=request.model_name
        )
        return pb.RepositoryModelUnloadResponse(
            model_name=response["name"], isUnloaded=response["unload"]
        )

    async def ModelInfer(
        self, request: pb.ModelInferRequest, context: ServicerContext
    ) -> pb.ModelInferResponse:
        headers = to_headers(context)
        self.validate_grpc_request(request)
        infer_request = InferRequest.from_grpc(request)
        response_body, _ = await self._data_plane.infer(
            request=infer_request, headers=headers, model_name=request.model_name
        )
        if isinstance(response_body, pb.ModelInferResponse):
            return response_body
        elif isinstance(response_body, InferResponse):
            return response_body.to_grpc()
        else:
            return pb.ModelInferResponse(
                id=response_body["id"],
                model_name=response_body["model_name"],
                outputs=response_body["outputs"],
            )
