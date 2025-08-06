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

import pytest
import dateutil.parser

from transformers import AutoConfig

from huggingfaceserver.task import infer_task_from_model_architecture
from huggingfaceserver.task import MLTask
from huggingfaceserver.time_series_model import HuggingFaceTimeSeriesModel
from kserve.protocol.rest.timeseries.types import (
    ForecastRequest,
    ForecastResponse,
    TimeSeriesType,
    TimeSeriesInput,
    ForecastOptions,
    Frequency,
    FREQUENCY_MAP,
)


def verify_forecast_response(response, request):
    """
    Checks numerical properties for ForecastResponse.
    Raises AssertionError if any check fails.
    """
    input_names = [inp.name for inp in request.inputs]
    output_names = []
    for output in response.outputs:
        for content in output.content:
            output_names.append(content.name)
    assert set(input_names) == set(
        output_names
    ), f"Output names {output_names} do not match input names {input_names}"

    expected_horizon = request.options.horizon
    expected_quantiles = request.options.quantiles

    for output in response.outputs:
        for content in output.content:
            # Type should match
            corresponding_input = next(
                inp for inp in request.inputs if inp.name == content.name
            )
            assert (
                content.type == corresponding_input.type
            ), f"Type mismatch for {content.name}: {content.type} vs {corresponding_input.type}"

            # Horizon should match
            if isinstance(content.mean_forecast[0], list):  # multivariate
                horizon_len = len(content.mean_forecast[0])
            else:
                horizon_len = len(content.mean_forecast)
            assert (
                horizon_len == expected_horizon
            ), f"Horizon mismatch for {content.name}: got {horizon_len}, expected {expected_horizon}"

            # Time stamp should be advanced by the length of the input series
            input_start = dateutil.parser.isoparse(corresponding_input.start_timestamp)
            freq = corresponding_input.frequency
            steps = len(corresponding_input.series)
            expected_output_start = input_start + FREQUENCY_MAP[freq](steps)
            actual_output_start = dateutil.parser.isoparse(content.start_timestamp)
            assert actual_output_start == expected_output_start, (
                f"For series '{content.name}', expected start_timestamp {expected_output_start.isoformat()} "
                f"but got {actual_output_start.isoformat()}"
            )

            # Quantiles structure and monotonicity
            if expected_quantiles is not None:
                assert (
                    content.quantiles is not None
                ), f"Quantiles missing for {content.name}"
                for q in expected_quantiles:
                    qstr = str(q)
                    assert (
                        qstr in content.quantiles
                    ), f"Quantile {qstr} missing for {content.name}"
                    q_values = content.quantiles[qstr]
                    if isinstance(q_values[0], list):  # multivariate
                        q_horizon = len(q_values[0])
                    else:
                        q_horizon = len(q_values)
                    assert (
                        q_horizon == expected_horizon
                    ), f"Quantile {qstr} horizon mismatch for {content.name}"

                # Quantile monotonicity for each time step
                quantile_keys = [str(q) for q in sorted(expected_quantiles)]
                for t in range(expected_horizon):
                    prev = None
                    for q in quantile_keys:
                        val = content.quantiles[q][t]
                        if prev is not None:
                            assert (
                                val >= prev
                            ), f"Quantiles not monotonic at step {t} for {content.name}: {val} < {prev} (q={q})"
                        prev = val


@pytest.fixture(scope="module")
def load_timesfm():
    config = AutoConfig.from_pretrained("google/timesfm-2.0-500m-pytorch")
    model = HuggingFaceTimeSeriesModel(
        model_config=config,
        model_name="timesfm",
        model_id_or_path="google/timesfm-2.0-500m-pytorch",
    )
    model.load()
    yield model
    model.stop()


def test_timesseries_support(load_timesfm):
    model = load_timesfm
    model_task = infer_task_from_model_architecture(model.model_config)
    assert model_task == MLTask.time_series_forecast


@pytest.mark.asyncio
async def test_timesfm_forecast(load_timesfm):
    model = load_timesfm

    input_sequence_1 = TimeSeriesInput(
        type=TimeSeriesType.UNIVARIATE,
        name="stock_price",
        series=[120, 122, 125, 127, 130, 133, 135],
        frequency=Frequency.DAY,
        start_timestamp="2025-06-05T13:10:00Z",
    )
    input_sequence_2 = TimeSeriesInput(
        type=TimeSeriesType.UNIVARIATE,
        name="humidity",
        series=[33, 34, 35, 36, 37],
        frequency="H",
        start_timestamp="2025-06-05T13:10:00Z",
    )

    request = ForecastRequest(
        model="timesfm",
        inputs=[input_sequence_1, input_sequence_2],
        options=ForecastOptions(horizon=8, quantiles=[0.1, 0.5, 0.9]),
    )

    response = await model.forecast(request)
    print(type(response))
    print(response)
    assert isinstance(response, ForecastResponse)
    verify_forecast_response(response, request)
