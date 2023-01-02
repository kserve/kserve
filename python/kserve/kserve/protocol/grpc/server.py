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

import asyncio
import logging
import multiprocessing
from concurrent import futures

from . import grpc_predict_v2_pb2_grpc
from .interceptors import LoggingInterceptor
from .servicer import InferenceServicer
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension

from grpc import aio


MAX_GRPC_MESSAGE_LENGTH = -1


class GRPCServer:
    def __init__(
        self,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension
    ):
        self._data_plane = data_plane
        self._model_repository_extension = model_repository_extension
        self._server = None

    async def start(self, max_workers, bind_address):
        inference_servicer = InferenceServicer(
            self._data_plane,
            self._model_repository_extension)
        self._server = aio.server(
            futures.ThreadPoolExecutor(max_workers=max_workers),
            interceptors=(LoggingInterceptor(),),
            options=[
                ("grpc.max_message_length", MAX_GRPC_MESSAGE_LENGTH),
                ("grpc.max_send_message_length", MAX_GRPC_MESSAGE_LENGTH),
                ("grpc.max_receive_message_length", MAX_GRPC_MESSAGE_LENGTH),
                ('grpc.so_reuseport', 1),
                ("grpc.use_local_subchannel_pool", 1),
            ]
        )
        grpc_predict_v2_pb2_grpc.add_GRPCInferenceServiceServicer_to_server(
            inference_servicer, self._server)

        self._server.add_insecure_port(bind_address)
        logging.info("Starting gRPC server on %s", bind_address)
        await self._server.start()
        await self._server.wait_for_termination()

    async def stop(self, sig: int = None):
        logging.info("Waiting for gRPC server shutdown")
        await self._server.stop(grace=10)
        logging.info("gRPC server shutdown complete")


class GRPCProcess(multiprocessing.Process):

    def __init__(self,
                 bind_address: str,
                 max_threads: int,
                 data_plane: DataPlane,
                 model_repository_extension: ModelRepositoryExtension,
                 ):
        super().__init__()
        self._data_plane = data_plane
        self._model_repository_extension = model_repository_extension
        self._bind_address = bind_address
        self._max_threads = max_threads
        self._server = None

    def stop(self):
        self._server.stop()

    def run(self):
        self._server = GRPCServer(self._data_plane, self._model_repository_extension)
        asyncio.run(self._server.start(self._max_threads, self._bind_address))
