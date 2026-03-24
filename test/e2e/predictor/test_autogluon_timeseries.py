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

"""E2E tests for AutoGluon TimeSeriesPredictor on kserve-autogluonserver.

``storage_uri`` must point at the AutoGluon TimeSeries predictor save directory
(the path passed to ``TimeSeriesPredictor.load``).

Set ``AUTOGLUON_TIMESERIES_STORAGE_URI`` to enable; otherwise tests are skipped.
"""

import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc

AUTOGLUON_TS_STORAGE_URI = os.getenv("AUTOGLUON_TIMESERIES_STORAGE_URI", "")

pytestmark = pytest.mark.skipif(
    not AUTOGLUON_TS_STORAGE_URI,
    reason="AUTOGLUON_TIMESERIES_STORAGE_URI not set (time series e2e)",
)

AUTOGLUON_TS_RESOURCES = V1ResourceRequirements(
    requests={"cpu": "100m", "memory": "2Gi"},
    limits={"cpu": "2", "memory": "4Gi"},
)


def _create_isvc(service_name: str, predictor: V1beta1PredictorSpec):
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )


def _create_ts_predictor(service_name: str):
    model = V1beta1ModelSpec(
        model_format=V1beta1ModelFormat(name="autogluon-timeseries"),
        runtime="kserve-autogluonserver",
        storage_uri=AUTOGLUON_TS_STORAGE_URI,
        resources=AUTOGLUON_TS_RESOURCES,
    )
    return V1beta1PredictorSpec(min_replicas=1, model=model)


async def _deploy_and_predict_v1(service_name: str, rest_v1_client, input_path: str):
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    predictor = _create_ts_predictor(service_name)
    isvc = _create_isvc(service_name, predictor)
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
        return await predict_isvc(rest_v1_client, service_name, input_path)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


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
