import pytest
from unittest.mock import MagicMock, patch
import urllib3

from kserve.rest import RESTClientObject, RESTResponse
from kserve.exceptions import ApiException, ApiValueError

@pytest.fixture
def mock_config():
    cfg = MagicMock()
    cfg.verify_ssl = True
    cfg.ssl_ca_cert = None
    cfg.assert_hostname = None
    cfg.retries = None
    cfg.connection_pool_maxsize = None
    cfg.cert_file = None
    cfg.key_file = None
    cfg.proxy = None
    cfg.proxy_headers = None
    return cfg

def test_rest_response_wraps_urllib3_response():
    resp = MagicMock()
    resp.status = 200
    resp.reason = "OK"
    resp.data = b"hello"
    resp.getheaders.return_value = {"Content-Type": "application/json"}
    resp.getheader.return_value = "application/json"

    r = RESTResponse(resp)

    assert r.status == 200
    assert r.reason == "OK"
    assert r.data == b"hello"
    assert r.getheaders() == {"Content-Type": "application/json"}
    assert r.getheader("Content-Type") == "application/json"

def test_get_request_success(mock_config):
    client = RESTClientObject(mock_config)

    mock_http_resp = MagicMock()
    mock_http_resp.status = 200
    mock_http_resp.reason = "OK"
    mock_http_resp.data = b"response"

    with patch.object(
        client.pool_manager,
        "request",
        return_value=mock_http_resp,
    ) as mock_request:
        resp = client.GET("http://test")

    assert isinstance(resp, RESTResponse)
    assert resp.status == 200
    mock_request.assert_called_once()

def test_post_json_body(mock_config):
    client = RESTClientObject(mock_config)

    mock_http_resp = MagicMock()
    mock_http_resp.status = 200
    mock_http_resp.reason = "OK"
    mock_http_resp.data = b"{}"

    with patch.object(
        client.pool_manager,
        "request",
        return_value=mock_http_resp,
    ) as mock_request:
        client.POST(
            "http://test",
            body={"a": 1},
            headers={"Content-Type": "application/json"},
        )

    args, kwargs = mock_request.call_args
    assert kwargs["body"] == '{"a": 1}'

def test_post_params_and_body_raises_error(mock_config):
    client = RESTClientObject(mock_config)

    with pytest.raises(ApiValueError):
        client.POST(
            "http://test",
            body={"a": 1},
            post_params={"x": "y"},
        )

def test_non_2xx_response_raises_api_exception(mock_config):
    client = RESTClientObject(mock_config)

    mock_http_resp = MagicMock()
    mock_http_resp.status = 500
    mock_http_resp.reason = "Internal Error"
    mock_http_resp.data = b"error"

    with patch.object(
        client.pool_manager,
        "request",
        return_value=mock_http_resp,
    ):
        with pytest.raises(ApiException):
            client.GET("http://test")

def test_ssl_error_raises_api_exception(mock_config):
    client = RESTClientObject(mock_config)

    with patch.object(
        client.pool_manager,
        "request",
        side_effect=urllib3.exceptions.SSLError("bad ssl"),
    ):
        with pytest.raises(ApiException) as exc:
            client.GET("https://secure")

    assert "SSLError" in str(exc.value)

def test_form_urlencoded_request(mock_config):
    client = RESTClientObject(mock_config)

    mock_http_resp = MagicMock()
    mock_http_resp.status = 200
    mock_http_resp.reason = "OK"
    mock_http_resp.data = b"ok"

    with patch.object(
        client.pool_manager,
        "request",
        return_value=mock_http_resp,
    ) as mock_request:
        client.POST(
            "http://test",
            headers={"Content-Type": "application/x-www-form-urlencoded"},
            post_params={"a": "1"},
        )

    args, kwargs = mock_request.call_args
    assert kwargs["fields"] == {"a": "1"}
