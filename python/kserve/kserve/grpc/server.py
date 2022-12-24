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


import logging
from concurrent import futures

from kserve.grpc import grpc_predict_v2_pb2_grpc
from kserve.grpc.interceptors import LoggingInterceptor
from kserve.grpc.servicer import InferenceServicer
from kserve.handlers.dataplane import DataPlane
from kserve.handlers.model_repository_extension import ModelRepositoryExtension

from grpc import aio


MAX_GRPC_MESSAGE_LENGTH = 8388608


class GRPCServer:
    def __init__(
        self,
        port: int,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension
    ):
        self._port = port
        self._data_plane = data_plane
        self._model_repository_extension = model_repository_extension
        self._server = None

    async def start(self, max_workers):
        inference_servicer = InferenceServicer(
            self._data_plane,
            self._model_repository_extension)
        self._server = aio.server(
            futures.ThreadPoolExecutor(max_workers=max_workers),
            interceptors=(LoggingInterceptor(),),
            options=[
                ("grpc.max_message_length", MAX_GRPC_MESSAGE_LENGTH),
                ("grpc.max_send_message_length", MAX_GRPC_MESSAGE_LENGTH),
                ("grpc.max_receive_message_length", MAX_GRPC_MESSAGE_LENGTH)
            ]
        )
        grpc_predict_v2_pb2_grpc.add_GRPCInferenceServiceServicer_to_server(
            inference_servicer, self._server)

        listen_addr = f'[::]:{self._port}'
        self._server.add_insecure_port(listen_addr)
        logging.info("Starting gRPC server on %s", listen_addr)
        await self._server.start()
        await self._server.wait_for_termination()

    async def stop(self, sig: int = None):
        logging.info("Waiting for gRPC server shutdown")
        await self._server.stop(grace=10)
        logging.info("gRPC server shutdown complete")
