from concurrent import futures
import logging

from kserve.grpc import grpc_predict_v2_pb2_grpc
from kserve.grpc.servicer import InferenceServicer
from kserve.handlers.dataplane import DataPlane

from grpc import aio


class GRPCServer:
    def __init__(
        self,
        port: int,
        data_plane: DataPlane
    ):
        self._port = port
        self._data_plane = data_plane

    def _create_server(self, max_workers):
        self._inference_servicer = InferenceServicer(self._data_plane)
        self._server = aio.server(futures.ThreadPoolExecutor(max_workers=max_workers))
        grpc_predict_v2_pb2_grpc.add_GRPCInferenceServiceServicer_to_server(
            self._inference_servicer, self._server
        )
        self._server.add_insecure_port(f'[::]:{self._port}')
        return self._server

    async def start(self, max_workers):
        self._create_server(max_workers)

        await self._server.start()
        logging.info(
            "gRPC server running on "
            f"[::]:{self._port}"
        )
        await self._server.wait_for_termination()

    async def stop(self):
        await self._server.stop(grace=10)
