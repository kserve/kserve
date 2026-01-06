import pytest
from http import HTTPStatus
from fastapi.responses import JSONResponse
from unittest.mock import patch

from kserve.protocol.rest.openai.errors import (
    OpenAIError,
    openai_error_handler,
    ErrorResponse,
)

@pytest.mark.skip(reason="add it in future")
@pytest.mark.asyncio
async def test_openai_error_handler_with_string_response_logs_and_returns_json():
    # Arrange
    error_message = "Something went wrong"
    exc = OpenAIError(error_message)

    module_path = "kserve.protocol.rest.openai.errors"

    with patch(f"{module_path}".logger) as mock_logger:
        # Act
        response = await openai_error_handler(None, exc)

        # Assert: logger.error was called correctly
        mock_logger.error.assert_called_once_with(
            "Exception:", exc_info=exc
        )

    # Assert: response type
    assert isinstance(response, JSONResponse)

    # Assert: status code comes from INTERNAL_SERVER_ERROR
    assert response.status_code == HTTPStatus.INTERNAL_SERVER_ERROR

    # Assert: response body
    body = response.body.decode()
    assert "Something went wrong" in body
    assert "BadRequestError" not in body  # err_type is OpenAIError
    assert "OpenAIError" in body
