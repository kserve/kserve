# Copyright 2026 The KServe Authors.
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

from unittest.mock import AsyncMock

import grpc
import pytest

from kserve import ModelRepository
from kserve.errors import (
    InvalidInput,
    ModelNotFound,
    ModelNotReady,
    ServerNotLive,
    ServerNotReady,
)
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.grpc import grpc_predict_v2_pb2, grpc_predict_v2_pb2_grpc
from kserve.protocol.grpc.interceptors import ExceptionToStatusInterceptor
from kserve.protocol.grpc.servicer import InferenceServicer
from kserve.protocol.model_repository_extension import ModelRepositoryExtension


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "exception, code, details",
    [
        (InvalidInput("bad input"), grpc.StatusCode.INVALID_ARGUMENT, "bad input"),
        (
            ModelNotFound("m"),
            grpc.StatusCode.NOT_FOUND,
            "Model with name m does not exist.",
        ),
        (
            ModelNotReady("m"),
            grpc.StatusCode.UNAVAILABLE,
            "Model with name m is not ready.",
        ),
        (ServerNotReady(), grpc.StatusCode.UNAVAILABLE, "Server is not ready."),
        (ServerNotLive(), grpc.StatusCode.UNAVAILABLE, "Server is not live."),
        (
            NotImplementedError("not supported"),
            grpc.StatusCode.UNIMPLEMENTED,
            "not supported",
        ),
    ],
)
async def test_exception_maps_to_grpc_status(exception, code, details):
    context = AsyncMock()
    await ExceptionToStatusInterceptor().handle_exception(
        exception, None, context, "/inference.GRPCInferenceService/ModelInfer"
    )
    context.abort.assert_awaited_once_with(code, details)


@pytest.mark.asyncio
async def test_failed_model_load_returns_unavailable():
    repository = ModelRepository()
    server = grpc.aio.server(interceptors=[ExceptionToStatusInterceptor()])
    grpc_predict_v2_pb2_grpc.add_GRPCInferenceServiceServicer_to_server(
        InferenceServicer(
            DataPlane(model_registry=repository),
            ModelRepositoryExtension(model_registry=repository),
        ),
        server,
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        async with grpc.aio.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = grpc_predict_v2_pb2_grpc.GRPCInferenceServiceStub(channel)
            with pytest.raises(grpc.aio.AioRpcError) as exc_info:
                await stub.RepositoryModelLoad(
                    grpc_predict_v2_pb2.RepositoryModelLoadRequest(model_name="m")
                )
    finally:
        await server.stop(None)

    assert exc_info.value.code() == grpc.StatusCode.UNAVAILABLE
    assert exc_info.value.details() == "Model with name m is not ready."
