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
import multiprocessing
from concurrent import futures

from grpc import aio

from kserve.logging import logger
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension

from . import grpc_predict_v2_pb2_grpc
from .interceptors import LoggingInterceptor, ExceptionToStatusInterceptor
from .servicer import InferenceServicer


class GRPCServer:
    def __init__(
        self,
        port: int,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
        kwargs: dict,
        grace_period: int = 30,
    ):
        self._port = port
        self._data_plane = data_plane
        self._model_repository_extension = model_repository_extension
        self._server = None
        self._grace_period = grace_period
        self._kwargs = kwargs

    async def start(self, max_workers):
        inference_servicer = InferenceServicer(
            self._data_plane, self._model_repository_extension
        )
        self._server = aio.server(
            futures.ThreadPoolExecutor(max_workers=max_workers),
            interceptors=(LoggingInterceptor(), ExceptionToStatusInterceptor()),
            options=[
                (
                    "grpc.max_send_message_length",
                    self._kwargs.get("grpc_max_send_message_length"),
                ),
                (
                    "grpc.max_receive_message_length",
                    self._kwargs.get("grpc_max_receive_message_length"),
                ),
            ],
        )
        grpc_predict_v2_pb2_grpc.add_GRPCInferenceServiceServicer_to_server(
            inference_servicer, self._server
        )

        listen_addr = f"[::]:{self._port}"
        self._server.add_insecure_port(listen_addr)
        logger.info(f"Starting gRPC server with {max_workers} workers")
        logger.info("Starting gRPC server on %s", listen_addr)
        await self._server.start()
        await self._server.wait_for_termination()

    async def stop(self, sig: int = None):
        if self._server:
            logger.info("Waiting for gRPC server shutdown")
            await self._server.stop(grace=self._grace_period)
            logger.info("gRPC server shutdown complete")


class GRPCProcess(multiprocessing.Process):

    def __init__(
        self,
        port: int,
        max_threads: int,
        data_plane: DataPlane,
        model_repository_extension: ModelRepositoryExtension,
        kwargs: dict,
    ):
        super().__init__()
        self._data_plane = data_plane
        self._model_repository_extension = model_repository_extension
        self._port = port
        self._max_threads = max_threads
        self._server = None
        self._kwargs = kwargs

    def stop(self):
        if self._server:
            self._server.stop()

    def run(self):
        self._server = GRPCServer(
            self._port,
            self._data_plane,
            self._model_repository_extension,
            kwargs=self._kwargs,
        )
        asyncio.run(self._server.start(self._max_threads))
