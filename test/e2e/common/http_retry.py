from typing import Dict, List, Union

import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

DEFAULT_RETRY_STATUS_CODES = (404, 429, 502, 503, 504)
DEFAULT_RETRY_TOTAL = 4
DEFAULT_RETRY_BACKOFF_FACTOR = 1.0


def post_with_retry(
    url: str,
    *,
    headers: Dict = None,
    json_data: Union[Dict, List] = None,
    data: Union[str, bytes] = None,
    stream: bool = False,
    timeout: float = None,
    total_retries: int = DEFAULT_RETRY_TOTAL,
    backoff_factor: float = DEFAULT_RETRY_BACKOFF_FACTOR,
    retry_status_codes=DEFAULT_RETRY_STATUS_CODES,
) -> requests.Response:
    """
    Send POST request with retries for transient HTTP and network failures.
    """
    if json_data is not None and data is not None:
        raise ValueError("Only one of json_data or data can be provided.")

    retry = Retry(
        total=total_retries,
        backoff_factor=backoff_factor,
        status_forcelist=retry_status_codes,
        allowed_methods=["POST"],
        raise_on_status=False,
    )
    with requests.Session() as session:
        session.mount("http://", HTTPAdapter(max_retries=retry))
        session.mount("https://", HTTPAdapter(max_retries=retry))
        return session.post(
            url,
            json=json_data,
            data=data,
            headers=headers,
            stream=stream,
            timeout=timeout,
        )
