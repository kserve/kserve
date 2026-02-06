import pytest
from fastapi.testclient import TestClient
from fastapi import FastAPI
from unittest.mock import AsyncMock, MagicMock

from kserve.protocol.rest.v1_endpoints import register_v1_endpoints


@pytest.fixture
def mock_dataplane():
    dp = MagicMock()

    # model_ready returns True
    dp.model_ready = AsyncMock(return_value=True)

    # decode returns (infer_request, req_attributes)
    dp.decode.return_value = ({"decoded": True}, {})

    # explain returns (response_bytes, response_headers)
    dp.explain = AsyncMock(return_value=(b"raw-bytes-response", {"x-test": "1"}))

    # encode also returns (bytes_response, encode_headers)
    dp.encode.return_value = (b"encoded-bytes-response", {"x-encode": "2"})

    return dp


def test_explain_returns_raw_response_for_bytes(mock_dataplane):
    app = FastAPI()
    register_v1_endpoints(app, mock_dataplane, model_repository_extension=None)

    client = TestClient(app)

    response = client.post("/v1/models/test-model:explain", json={"input": "data"})

    # Validate status code
    assert response.status_code == 200

    # Validate raw bytes response
    assert response.content == b"encoded-bytes-response"

    # Validate merged headers
    assert response.headers.get("x-test") == "1"
    assert response.headers.get("x-encode") == "2"

    # The Response() call does NOT set content-type, so ensure it's missing
    assert "content-type" not in response.headers
