import logging
import time
from typing import Dict, List, Union

import requests

DEFAULT_RETRY_STATUS_CODES = (404, 429, 502, 503, 504)
DEFAULT_RETRY_TOTAL = 4
DEFAULT_RETRY_BACKOFF_FACTOR = 1.0

logger = logging.getLogger(__name__)


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

    attempt = 0
    while True:
        try:
            response = requests.post(
                url,
                json=json_data,
                data=data,
                headers=headers,
                stream=stream,
                timeout=timeout,
            )
        except requests.exceptions.RequestException as e:
            if attempt >= total_retries:
                raise

            sleep_seconds = backoff_factor * (2**attempt)
            logger.info(
                "POST %s failed with %s, retrying in %.1fs (%d/%d)",
                url,
                e.__class__.__name__,
                sleep_seconds,
                attempt + 1,
                total_retries,
            )
            time.sleep(sleep_seconds)
            attempt += 1
            continue

        if response.status_code in retry_status_codes and attempt < total_retries:
            sleep_seconds = backoff_factor * (2**attempt)
            logger.info(
                "POST %s returned %s, retrying in %.1fs (%d/%d)",
                url,
                response.status_code,
                sleep_seconds,
                attempt + 1,
                total_retries,
            )
            if getattr(response, "raw", None) is not None:
                response.close()
            time.sleep(sleep_seconds)
            attempt += 1
            continue

        return response
