from typing import Dict, List, Union

import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

DEFAULT_RETRY_STATUS_CODES = (404, 429, 502, 503, 504)
DEFAULT_RETRY_TOTAL = 8
DEFAULT_RETRY_BACKOFF_FACTOR = 2.0
# Per-request timeout (seconds). A stalled server that accepts the connection but
# never responds must not block a single request indefinitely; without this a hung
# predictor consumes the entire CI job budget instead of failing the test.
DEFAULT_TIMEOUT_SECONDS = 300.0


def _retry_session(
    allowed_methods,
    total_retries=DEFAULT_RETRY_TOTAL,
    backoff_factor=DEFAULT_RETRY_BACKOFF_FACTOR,
    retry_status_codes=DEFAULT_RETRY_STATUS_CODES,
) -> requests.Session:
    retry = Retry(
        total=total_retries,
        backoff_factor=backoff_factor,
        status_forcelist=retry_status_codes,
        allowed_methods=allowed_methods,
        raise_on_status=False,
    )
    session = requests.Session()
    session.mount("http://", HTTPAdapter(max_retries=retry))
    session.mount("https://", HTTPAdapter(max_retries=retry))
    return session


def get_with_retry(
    url: str,
    *,
    headers: Dict = None,
    timeout: float = DEFAULT_TIMEOUT_SECONDS,
    total_retries: int = DEFAULT_RETRY_TOTAL,
    backoff_factor: float = DEFAULT_RETRY_BACKOFF_FACTOR,
    retry_status_codes=DEFAULT_RETRY_STATUS_CODES,
) -> requests.Response:
    """
    Send GET request with retries for transient HTTP and network failures.
    """
    with _retry_session(
        ["GET"], total_retries, backoff_factor, retry_status_codes
    ) as session:
        return session.get(url, headers=headers, timeout=timeout)


def post_with_retry(
    url: str,
    *,
    headers: Dict = None,
    json_data: Union[Dict, List] = None,
    data: Union[str, bytes] = None,
    stream: bool = False,
    timeout: float = DEFAULT_TIMEOUT_SECONDS,
    total_retries: int = DEFAULT_RETRY_TOTAL,
    backoff_factor: float = DEFAULT_RETRY_BACKOFF_FACTOR,
    retry_status_codes=DEFAULT_RETRY_STATUS_CODES,
) -> requests.Response:
    """
    Send POST request with retries for transient HTTP and network failures.
    """
    if json_data is not None and data is not None:
        raise ValueError("Only one of json_data or data can be provided.")

    with _retry_session(
        ["POST"], total_retries, backoff_factor, retry_status_codes
    ) as session:
        return session.post(
            url,
            json=json_data,
            data=data,
            headers=headers,
            stream=stream,
            timeout=timeout,
        )
