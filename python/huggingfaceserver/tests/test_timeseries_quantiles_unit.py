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

"""Small TimesFM quantile regression tests; no pretrained weights are downloaded."""

from types import SimpleNamespace

import pytest
import torch

from huggingfaceserver.time_series_model import HuggingFaceTimeSeriesModel
from kserve.protocol.rest.timeseries.types import (
    ErrorResponse,
    ForecastOptions,
    ForecastRequest,
    ForecastResponse,
    Frequency,
    TimeSeriesInput,
    TimeSeriesType,
)


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "options_kwargs, expected_keys, expected_error",
    [
        ({}, set(), None),  # omitted quantiles
        ({"quantiles": None}, set(), None),  # explicit JSON null
        ({"quantiles": []}, set(), None),  # explicit empty array
        ({"quantiles": [0.1, 0.9]}, {"0.1", "0.9"}, None),
        ({"quantiles": [0.25]}, None, "invalid_quantile"),
    ],
    ids=["omitted", "explicit-none", "empty-list", "valid-subset", "invalid"],
)
async def test_timesfm_quantiles_without_pretrained_weights(
    options_kwargs, expected_keys, expected_error
):
    """Exercise the actual KServe forecasting method with a deterministic fake model."""
    config = SimpleNamespace(
        horizon_length=128, patch_length=16, quantiles=[0.1, 0.5, 0.9]
    )
    model = HuggingFaceTimeSeriesModel(
        model_name="timesfm",
        model_id_or_path="not-loaded",
        model_config=config,
        dtype=torch.float32,
    )

    # TimesFM output layout: point forecast at index 0, quantiles at indices 1+.
    predictions = torch.zeros((1, 8, 4), dtype=torch.float32)
    predictions[:, :, 0] = 100.0
    predictions[:, :, 1] = 90.0
    predictions[:, :, 2] = 100.0
    predictions[:, :, 3] = 110.0

    class FakeTimesFM:
        calls = 0

        def __call__(self, inputs, frequencies, return_dict):
            self.calls += 1
            assert return_dict is True
            assert len(inputs) == 1
            assert frequencies.tolist() == [0]
            return SimpleNamespace(full_predictions=predictions)

    fake = FakeTimesFM()
    model._model = fake

    request = ForecastRequest(
        model="timesfm",
        inputs=[
            TimeSeriesInput(
                type=TimeSeriesType.UNIVARIATE,
                name="temperature",
                series=[23.0, 24.0, 25.0],
                frequency=Frequency.HOUR,
                start_timestamp="2026-10-29T10:00:00Z",
            )
        ],
        options=ForecastOptions(horizon=8, **options_kwargs),
    )

    response = await model.forecast(request)

    if expected_error:
        assert isinstance(response, ErrorResponse)
        assert response.error.type == expected_error
        assert fake.calls == 0  # Validation occurs before model inference.
        return

    assert isinstance(response, ForecastResponse)
    assert fake.calls == 1
    assert request.options.quantiles == options_kwargs.get("quantiles")
    assert len(response.outputs) == 1
    result = response.outputs[0].content[0]
    assert result.mean_forecast == [100.0] * 8
    assert set(result.quantiles or {}) == expected_keys
    if "0.1" in expected_keys:
        assert result.quantiles["0.1"] == [90.0] * 8
    if "0.9" in expected_keys:
        assert result.quantiles["0.9"] == [110.0] * 8
