import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from grpc import StatusCode
from grpc_interceptor.exceptions import GrpcException

from kserve.protocol.grpc.interceptors import (
    LoggingInterceptor,
    ExceptionToStatusInterceptor,
)

from kserve.errors import (
    InferenceError,
    InvalidInput,
    UnsupportedProtocol,
    ServerNotReady,
    ServerNotLive,
    ModelNotFound,
    ModelNotReady,
)

@pytest.mark.asyncio
async def test_logging_interceptor_logs_and_continues():
    interceptor = LoggingInterceptor()

    handler_call_details = MagicMock()
    handler_call_details.method = "/test.Service/Method"

    continuation = AsyncMock()
    continuation.return_value = "handler"

    with patch("kserve.protocol.grpc.interceptors.logger") as mock_logger:
        result = await interceptor.intercept_service(
            continuation, handler_call_details
        )

        mock_logger.info.assert_called_once_with(
            "grpc method: /test.Service/Method"
        )
        continuation.assert_called_once_with(handler_call_details)
        assert result == "handler"

@pytest.fixture
def interceptor():
    return ExceptionToStatusInterceptor()


@pytest.fixture
def context():
    ctx = AsyncMock()
    ctx.abort = AsyncMock()
    return ctx

@pytest.mark.parametrize(
    "exception_cls",
    [InferenceError, InvalidInput, UnsupportedProtocol],
)
@pytest.mark.asyncio
async def test_invalid_argument_errors(interceptor, context, exception_cls):
    ex = exception_cls("bad request")

    await interceptor.handle_exception(
        ex, None, context, "TestMethod"
    )

    context.abort.assert_called_once_with(
        StatusCode.INVALID_ARGUMENT, ex.reason
    )

@pytest.mark.asyncio
async def test_model_not_found(interceptor, context):
    ex = ModelNotFound("model missing")

    await interceptor.handle_exception(
        ex, None, context, "TestMethod"
    )

    context.abort.assert_called_once_with(
        StatusCode.NOT_FOUND, ex.reason
    )

@pytest.mark.asyncio
async def test_grpc_exception_delegates_to_super(interceptor, context):
    ex = GrpcException(
        status_code=StatusCode.PERMISSION_DENIED,
        details="denied",
    )

    with patch(
        "grpc_interceptor.AsyncExceptionToStatusInterceptor.handle_exception",
        new=AsyncMock(),
    ) as mock_super:

        await interceptor.handle_exception(
            ex, None, context, "TestMethod"
        )

        mock_super.assert_called_once_with(
            ex, None, context, "TestMethod"
        )

@pytest.mark.asyncio
async def test_unknown_exception_results_in_internal_error(interceptor, context):
    ex = RuntimeError("boom")

    await interceptor.handle_exception(
        ex, None, context, "TestMethod"
    )

    context.abort.assert_called_once_with(
        StatusCode.INTERNAL, "Internal server error"
    )
