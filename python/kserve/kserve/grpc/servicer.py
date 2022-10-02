from kserve.grpc import grpc_predict_v2_pb2 as pb
from kserve.grpc import grpc_predict_v2_pb2_grpc
from kserve.handlers.dataplane import DataPlane
from kserve.utils.utils import to_headers

from grpc import ServicerContext


class InferenceServicer(grpc_predict_v2_pb2_grpc.GRPCInferenceServiceServicer):

    def __init__(
        self, data_plane: DataPlane
    ):
        super().__init__()
        self._data_plane = data_plane

    async def ServerLive(
        self, request: pb.ServerLiveRequest, context
    ) -> pb.ServerLiveResponse:
        is_live = await self._data_plane.live()
        return pb.ServerLiveResponse(live=is_live)

    async def ServerReady(
        self, request: pb.ServerReadyRequest, context
    ) -> pb.ServerLiveResponse:
        is_ready = await self._data_plane.ready()
        return pb.ServerReadyResponse(ready=is_ready)

    async def ModelReady(
        self, request: pb.ModelReadyRequest, context
    ) -> pb.ModelMetadataResponse:
        is_ready = await self._data_plane.model_ready(model_name=request.name)
        return pb.ModelReadyResponse(ready=is_ready)

    async def ModelMetadata(
        self, request: pb.ModelMetadataRequest, context
    ) -> pb.ModelMetadataResponse:
        metadata = await self._data_plane.model_metadata(model_name=request.name)
        return pb.ModelMetadataResponse(
            name=metadata["name"],
            versions=metadata["versions"],
            platform=metadata["platform"],
            inputs=metadata["inputs"],
            outputs=metadata["outputs"]
        )

    async def RepositoryModelLoad(
        self, request: pb.RepositoryModelLoadRequest, context
    ) -> pb.RepositoryModelLoadResponse:
        response = await self._data_plane.load(name=request.model_name)
        return pb.RepositoryModelLoadResponse(model_name=response["name"], isLoaded=response["load"])

    async def RepositoryModelUnload(
        self, request: pb.RepositoryModelUnloadRequest, context
    ) -> pb.RepositoryModelUnloadResponse:
        response = await self._data_plane.unload(name=request.model_name)
        return pb.RepositoryModelUnloadResponse(model_name=response["name"], isUnloaded=response["unload"])

    async def ModelInfer(
        self, request: pb.ModelInferRequest, context: ServicerContext
    ) -> pb.ModelInferResponse:
        headers = to_headers(context)
        response = await self._data_plane.infer(payload=request, headers=headers, model_name=request.model_name)
        # NOTE: response must be either ModelInferResponse or Predict Protocol v2 infer response structure
        if isinstance(response, pb.ModelInferResponse):
            return response
        return pb.ModelInferResponse(id=response["id"],
                                     model_name=response["model_name"],
                                     outputs=response["outputs"])
