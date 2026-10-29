# Copyright 2026 The KServe Authors.
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
import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from kserve.protocol.rest.timeseries.endpoints import (
    register_time_series_endpoints,
)
from kserve.protocol.rest.timeseries.types import (
    Error,
    ErrorResponse,
)


@pytest.mark.parametrize(
    "error_type, expected_status",
    [
        ("invalid_horizon", 400),
        ("invalid_quantile", 400),
        ("invalid_frequency", 400),
        ("model_error", 500),
    ],
)
def test_forecast_error_without_code_returns_correct_status(
    error_type, expected_status
):
    class FakeDataPlane:
        async def forecast(self, request_body, raw_request, headers, response):
            return ErrorResponse(
                error=Error(
                    type=error_type,
                    message=f"Forecast error: {error_type}",
                )
            )

        async def models(self):
            return []

    app = FastAPI()
    register_time_series_endpoints(app, FakeDataPlane())

    payload = {
        "model": "timesfm",
        "inputs": [
            {
                "type": "univariate_time_series",
                "name": "temperature_sensor",
                "series": [23.0, 24.0, 25.0],
                "frequency": "H",
                "start_timestamp": "2026-10-29T10:00:00Z",
            }
        ],
        "options": {
            "horizon": 999999,
            "quantiles": [0.1, 0.5, 0.9],
        },
    }

    with TestClient(app, raise_server_exceptions=False) as client:
        result = client.post(
            "/v1/timeseries/forecast",
            json=payload,
        )

    assert result.status_code == expected_status, (
        f"Expected HTTP {expected_status} for {error_type}, "
        f"got HTTP {result.status_code}"
    )

    assert result.headers["content-type"].startswith("application/json"), (
        f"Expected JSON error response, got: {result.text}"
    )

    assert result.json()["error"]["type"] == error_type


def test_forecast_preserves_explicit_error_status():
    class FakeDataPlane:
        async def forecast(self, request_body, raw_request, headers, response):
            return ErrorResponse(
                error=Error(
                    code="422",
                    type="validation_error",
                    message="Invalid forecasting request",
                )
            )

        async def models(self):
            return []

    app = FastAPI()
    register_time_series_endpoints(app, FakeDataPlane())

    payload = {
        "model": "timesfm",
        "inputs": [
            {
                "type": "univariate_time_series",
                "name": "temperature_sensor",
                "series": [23.0, 24.0, 25.0],
                "frequency": "H",
                "start_timestamp": "2026-10-29T10:00:00Z",
            }
        ],
        "options": {
            "horizon": 10,
            "quantiles": [0.1, 0.5, 0.9],
        },
    }

    with TestClient(app, raise_server_exceptions=False) as client:
        result = client.post("/v1/timeseries/forecast", json=payload)

    assert result.status_code == 422
    assert result.json()["error"]["code"] == "422"
    assert result.json()["error"]["type"] == "validation_error"
