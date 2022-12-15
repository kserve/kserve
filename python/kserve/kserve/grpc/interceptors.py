import logging
from typing import Awaitable, Callable

from grpc import HandlerCallDetails, RpcMethodHandler
from grpc.aio import ServerInterceptor


class LoggingInterceptor(ServerInterceptor):

    async def intercept_service(
        self,
        continuation: Callable[[HandlerCallDetails], Awaitable[RpcMethodHandler]],
        handler_call_details: HandlerCallDetails,
    ) -> RpcMethodHandler:
        logging.info(f"grpc method: {handler_call_details.method}")
        return await continuation(handler_call_details)
