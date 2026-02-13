import pytest
import requests

from ..common import http_retry

pytestmark = pytest.mark.predictor


def _make_response(status_code: int) -> requests.Response:
    response = requests.Response()
    response.status_code = status_code
    response.url = "http://test.local/v1/completions"
    response._content = b"{}"
    return response


def test_post_with_retry_retries_retryable_status_codes(monkeypatch):
    responses = [_make_response(404), _make_response(503), _make_response(200)]
    sleep_calls = []

    def _fake_post(*args, **kwargs):
        return responses.pop(0)

    monkeypatch.setattr(http_retry.requests, "post", _fake_post)
    monkeypatch.setattr(
        http_retry.time,
        "sleep",
        lambda seconds: sleep_calls.append(seconds),
    )

    response = http_retry.post_with_retry(
        "http://test.local/v1/completions",
        json_data={"prompt": "KServe is"},
        total_retries=4,
        backoff_factor=1.0,
    )

    assert response.status_code == 200
    assert sleep_calls == [1.0, 2.0]
    assert responses == []


def test_post_with_retry_retries_on_request_exception(monkeypatch):
    call_sequence = [
        requests.exceptions.ConnectionError("temporary connection issue"),
        _make_response(200),
    ]
    sleep_calls = []

    def _fake_post(*args, **kwargs):
        call_result = call_sequence.pop(0)
        if isinstance(call_result, Exception):
            raise call_result
        return call_result

    monkeypatch.setattr(http_retry.requests, "post", _fake_post)
    monkeypatch.setattr(
        http_retry.time,
        "sleep",
        lambda seconds: sleep_calls.append(seconds),
    )

    response = http_retry.post_with_retry(
        "http://test.local/v1/completions",
        json_data={"prompt": "KServe is"},
        total_retries=4,
        backoff_factor=1.0,
    )

    assert response.status_code == 200
    assert sleep_calls == [1.0]
    assert call_sequence == []


def test_post_with_retry_returns_last_response_when_retries_exhausted(monkeypatch):
    responses = [_make_response(404), _make_response(404), _make_response(404)]
    sleep_calls = []

    def _fake_post(*args, **kwargs):
        return responses.pop(0)

    monkeypatch.setattr(http_retry.requests, "post", _fake_post)
    monkeypatch.setattr(
        http_retry.time,
        "sleep",
        lambda seconds: sleep_calls.append(seconds),
    )

    response = http_retry.post_with_retry(
        "http://test.local/v1/completions",
        json_data={"prompt": "KServe is"},
        total_retries=2,
        backoff_factor=1.0,
    )

    assert response.status_code == 404
    assert sleep_calls == [1.0, 2.0]
    assert responses == []
