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

"""
E2E tests for AutoGluon TimeSeriesPredictor on kserve-autogluonserver.

The InferenceService ``storage_uri`` must refer to an AutoGluon TimeSeries
predictor save directory (the path you would pass to ``TimeSeriesPredictor.load``).

By default, tests use a sample model artifact in a **public** GCS bucket. Override
``AUTOGLUON_TIMESERIES_STORAGE_URI`` if you need a different ``storage_uri``.
"""

import os

import pytest
from kubernetes.client import V1ResourceRequirements

from kserve import (
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
)
from .autogluon_helpers import deploy_and_predict

# Public sample TimeSeriesPredictor artifact for e2e (override via env if needed).
_AUTOGLUON_TS_DEFAULT_STORAGE_URI = (
    "gs://test-project-frog-ml-artifacts/timeseries-artifacts/predictor/"
)
AUTOGLUON_TS_STORAGE_URI = os.getenv(
    "AUTOGLUON_TIMESERIES_STORAGE_URI",
    _AUTOGLUON_TS_DEFAULT_STORAGE_URI,
)

AUTOGLUON_TS_RESOURCES = V1ResourceRequirements(
    requests={"cpu": "100m", "memory": "2Gi"},
    limits={"cpu": "2", "memory": "4Gi"},
)


def _create_ts_predictor(service_name: str, storage_uri: str = None):
    model = V1beta1ModelSpec(
        model_format=V1beta1ModelFormat(name="autogluon"),
        runtime="kserve-autogluonserver",
        storage_uri=storage_uri or AUTOGLUON_TS_STORAGE_URI,
        resources=AUTOGLUON_TS_RESOURCES,
    )
    return V1beta1PredictorSpec(min_replicas=1, model=model)


async def _deploy_and_predict_v1(
    service_name: str, rest_v1_client, input_path: str, storage_uri: str = None
):
    predictor = _create_ts_predictor(service_name, storage_uri=storage_uri)
    return await deploy_and_predict(service_name, predictor, rest_v1_client, input_path)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_timeseries_runtime_kserve_v1(rest_v1_client):
    service_name = "isvc-autogluon-ts-v1"
    response = await _deploy_and_predict_v1(
        service_name,
        rest_v1_client,
        "./data/autogluon_timeseries_input.json",
    )
    assert "predictions" in response
    assert len(response["predictions"]) > 0


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_timeseries_runtime_kserve_v1_storage_uri_without_trailing_slash(
    rest_v1_client,
):
    service_name = "isvc-autogluon-ts-v1-noslash"
    response = await _deploy_and_predict_v1(
        service_name,
        rest_v1_client,
        "./data/autogluon_timeseries_input_long.json",
        storage_uri=AUTOGLUON_TS_STORAGE_URI.rstrip("/"),
    )
    assert "predictions" in response
    assert len(response["predictions"]) > 0
