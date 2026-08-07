# Copyright 2025 The KServe Authors.
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

from http import HTTPStatus

import pytest
from starlette.datastructures import Headers

from kserve.model import Model
from kserve.model_repository import ModelRepository
from kserve.protocol.rest.timeseries.dataplane import TimeSeriesDataPlane
from kserve.protocol.rest.timeseries.types import (
    ErrorResponse,
    ForecastOptions,
    ForecastRequest,
    Frequency,
    TimeSeriesInput,
    TimeSeriesType,
)


class NotATimeSeriesModel(Model):
    """A model that does not implement the Time Series API."""

    def __init__(self, name):
        super().__init__(name)
        self.ready = True


def _forecast_request(model_name: str) -> ForecastRequest:
    return ForecastRequest(
        model=model_name,
        inputs=[
            TimeSeriesInput(
                type=TimeSeriesType.UNIVARIATE,
                name="series-1",
                series=[1.0, 2.0, 3.0],
                frequency=Frequency.DAY,
            )
        ],
        options=ForecastOptions(horizon=3),
    )


@pytest.mark.asyncio
async def test_forecast_unsupported_model_returns_501():
    """A model that is not a HuggingFaceTimeSeriesModel must yield a clean
    501 ErrorResponse instead of crashing with a TypeError."""
    model_name = "not-a-timeseries-model"
    registry = ModelRepository()
    registry.update(NotATimeSeriesModel(model_name))
    dataplane = TimeSeriesDataPlane(model_registry=registry)

    result = await dataplane.forecast(
        request_body=_forecast_request(model_name),
        raw_request=None,
        headers=Headers({}),
        response=None,
    )

    assert isinstance(result, ErrorResponse)
    assert result.error.code == str(HTTPStatus.NOT_IMPLEMENTED.value)
    assert result.error.type == "not_implemented"
    assert "does not support Time Series API" in result.error.message
