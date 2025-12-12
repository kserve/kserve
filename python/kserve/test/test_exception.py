import pytest
from types import SimpleNamespace

# Import everything from your exception module
# Adjust path as needed (e.g., from mypackage.exceptions import ...)
from kserve.exceptions import (
    ApiTypeError,
    ApiValueError,
    ApiKeyError,
    ApiException,
    render_path,
)


def test_render_path():
    path = ["data", 0, "name"]
    assert render_path(path) == "['data'][0]['name']"


def test_api_type_error_message_with_path():
    error = ApiTypeError("Invalid type", path_to_item=["items", 2])
    assert "Invalid type at ['items'][2]" in str(error)
    assert error.path_to_item == ["items", 2]


def test_api_type_error_message_without_path():
    error = ApiTypeError("Invalid type")
    assert str(error) == "Invalid type"


def test_api_value_error():
    error = ApiValueError("Bad value", path_to_item=["meta", "id"])
    assert "Bad value at ['meta']['id']" in str(error)
    assert error.path_to_item == ["meta", "id"]


def test_api_key_error():
    error = ApiKeyError("Missing key", path_to_item=["user", "profile"])
    assert "Missing key at ['user']['profile']" in str(error)
    assert error.path_to_item == ["user", "profile"]


def test_api_exception_with_http_response():
    # Mock a minimal http response object
    http_resp = SimpleNamespace(
        status=404,
        reason="Not Found",
        data="Error body",
        getheaders=lambda: {"Content-Type": "application/json"},
    )

    ex = ApiException(http_resp=http_resp)
    msg = str(ex)

    assert "(404)" in msg
    assert "Reason: Not Found" in msg
    assert "HTTP response body: Error body" in msg
    assert "Content-Type" in msg


def test_api_exception_without_http_response():
    ex = ApiException(status=500, reason="Internal Error")
    msg = str(ex)

    assert "(500)" in msg
    assert "Reason: Internal Error" in msg
    assert "HTTP response body" not in msg  # body should be None
