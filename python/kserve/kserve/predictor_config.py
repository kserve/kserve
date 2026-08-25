# Copyright 2021 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Configuration classes for KServe."""

from .constants.constants import PredictorProtocol


class PredictorConfig:
    def __init__(
        self,
        predictor_host: str,
        predictor_protocol: str = PredictorProtocol.REST_V1.value,
        predictor_use_ssl: bool = False,
        predictor_request_timeout_seconds: int = 600,
        predictor_request_retries: int = 0,
        predictor_health_check: bool = False,
    ):
        """The configuration for the http call to the predictor

        Args:
            predictor_host: The host name of the predictor
            predictor_protocol: The inference protocol used for predictor http call
            predictor_use_ssl: Enable using ssl for http connection to the predictor
            predictor_request_timeout_seconds: The request timeout seconds for the predictor http call. Default is 600 seconds.
            predictor_request_retries: The number of retries if the predictor request fails. Default is 0.
            predictor_health_check: Enable predictor health check
        """
        self._predictor_host = predictor_host
        self._predictor_protocol = predictor_protocol
        self._predictor_use_ssl = predictor_use_ssl
        self._predictor_request_timeout_seconds = predictor_request_timeout_seconds
        self._predictor_request_retries = predictor_request_retries
        self._predictor_health_check = predictor_health_check

    @property
    def predictor_host(self) -> str:
        """Get the predictor host."""
        return self._predictor_host

    @property
    def predictor_protocol(self) -> str:
        """Get the predictor protocol."""
        return self._predictor_protocol

    @property
    def predictor_use_ssl(self) -> bool:
        """Get the predictor use ssl flag."""
        return self._predictor_use_ssl

    @property
    def predictor_request_timeout_seconds(self) -> int:
        """Get the predictor request timeout in seconds."""
        return self._predictor_request_timeout_seconds

    @property
    def predictor_request_retries(self) -> int:
        """Get the predictor request retries."""
        return self._predictor_request_retries

    @property
    def predictor_health_check(self) -> bool:
        """Get the predictor health check flag."""
        return self._predictor_health_check

    @property
    def predictor_base_url(self) -> str:
        """
        Get the base url for the predictor.

        Returns:
            str: The base url for the predictor
        """
        protocol = "https" if self._predictor_use_ssl else "http"
        return f"{protocol}://{self._predictor_host}"

    @property
    def protocol(self) -> str:
        """Alias for predictor_protocol for backward compatibility."""
        return self._predictor_protocol

    @property
    def timeout(self) -> int:
        """Alias for predictor_request_timeout_seconds for backward compatibility."""
        return self._predictor_request_timeout_seconds

    @property
    def retries(self) -> int:
        """Alias for predictor_request_retries for backward compatibility."""
        return self._predictor_request_retries

    @property
    def use_ssl(self) -> bool:
        """Alias for predictor_use_ssl for backward compatibility."""
        return self._predictor_use_ssl
