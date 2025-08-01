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

from typing import Any, Awaitable, Callable

from grpc import HandlerCallDetails, RpcMethodHandler, ServicerContext, StatusCode
from grpc.aio import ServerInterceptor
from grpc_interceptor import AsyncExceptionToStatusInterceptor
from grpc_interceptor.exceptions import GrpcException

from ...errors import (
    InferenceError,
    InvalidInput,
    ModelNotFound,
    ModelNotReady,
    UnsupportedProtocol,
    ServerNotReady,
    ServerNotLive,
)
from ...logging import logger


class LoggingInterceptor(ServerInterceptor):

    async def intercept_service(
        self,
        continuation: Callable[[HandlerCallDetails], Awaitable[RpcMethodHandler]],
        handler_call_details: HandlerCallDetails,
    ) -> RpcMethodHandler:
        logger.info(f"grpc method: {handler_call_details.method}")
        return await continuation(handler_call_details)


class ExceptionToStatusInterceptor(AsyncExceptionToStatusInterceptor):

    async def handle_exception(
        self,
        ex: Exception,
        request_or_iterator: Any,
        context: ServicerContext,
        method_name: str,
    ):
        if isinstance(ex, (InferenceError, InvalidInput, UnsupportedProtocol)):
            logger.error(
                "Error processing inference request: %s, grpc_method: %s",
                ex,
                method_name,
                exc_info=True,
            )
            await context.abort(StatusCode.INVALID_ARGUMENT, ex.reason)
        elif isinstance(ex, (ServerNotReady, ServerNotLive)):
            logger.error(
                "Unable to process the request: %s, grpc_method: %s",
                ex,
                method_name,
                exc_info=True,
            )
            await context.abort(StatusCode.UNAVAILABLE, ex.reason)
        elif isinstance(ex, NotImplementedError):
            logger.error(
                "Method not implemented: %s, grpc_method: %s",
                ex,
                method_name,
                exc_info=True,
            )
            await context.abort(StatusCode.UNIMPLEMENTED, ex.reason)
        elif isinstance(ex, ModelNotFound):
            logger.error(
                "Model not found: %s, grpc_method: %s", ex, method_name, exc_info=True
            )
            await context.abort(StatusCode.NOT_FOUND, ex.reason)
        elif isinstance(ex, ModelNotReady):
            logger.error(
                "Model not ready: %s, grpc_method: %s", ex, method_name, exc_info=True
            )
            await context.abort(StatusCode.UNAVAILABLE, ex.reason)
        elif isinstance(ex, GrpcException):
            await super().handle_exception(
                ex, request_or_iterator, context, method_name
            )
        else:
            logger.error(
                "Internal server error: %s, grpc_method: %s",
                ex,
                method_name,
                exc_info=True,
            )
            await context.abort(StatusCode.INTERNAL, "Internal server error")
