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

AUTOGLUON_STORAGE_URI = os.getenv(
    "AUTOGLUON_STORAGE_URI", "gs://test-project-frog-ml-artifacts/predictor/"
)
AUTOGLUON_RESOURCES = V1ResourceRequirements(
    requests={"cpu": "100m", "memory": "1Gi"},
    limits={"cpu": "1", "memory": "2Gi"},
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


def _create_predictor(
    service_name: str, protocol_version: str = None, storage_uri: str = None
):
    model = V1beta1ModelSpec(
        model_format=V1beta1ModelFormat(name="autogluon"),
        runtime="kserve-autogluonserver",
        storage_uri=storage_uri or AUTOGLUON_STORAGE_URI,
        resources=AUTOGLUON_RESOURCES,
    )
    if protocol_version:
        model.protocol_version = protocol_version
        model.readiness_probe = client.V1Probe(
            http_get=client.V1HTTPGetAction(
                path=f"/v2/models/{service_name}/ready", port=8080
            ),
            initial_delay_seconds=30,
        )
    return V1beta1PredictorSpec(min_replicas=1, model=model)


async def _deploy_and_predict(
    service_name: str,
    predictor: V1beta1PredictorSpec,
    rest_client,
    input_path: str,
):
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    isvc = _create_isvc(service_name, predictor)
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
        return await predict_isvc(rest_client, service_name, input_path)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v1(rest_v1_client):
    service_name = "isvc-autogluon-v1"
    predictor = _create_predictor(service_name)
    response = await _deploy_and_predict(
        service_name,
        predictor,
        rest_v1_client,
        "./data/autogluon_titanic_input.json",
    )
    assert "predictions" in response
    assert len(response["predictions"]) > 0


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v2(rest_v2_client):
    service_name = "isvc-autogluon-v2"
    predictor = _create_predictor(service_name, protocol_version="v2")
    response = await _deploy_and_predict(
        service_name,
        predictor,
        rest_v2_client,
        "./data/autogluon_titanic_input_v2.json",
    )
    assert len(response.outputs) > 0
    assert len(response.outputs[0].data) > 0


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v2_input_variants(rest_v2_client):
    service_name = "isvc-autogluon-v2-variants"
    predictor = _create_predictor(service_name, protocol_version="v2")
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(_create_isvc(service_name, predictor))
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
        for input_path in [
            "./data/autogluon_titanic_input_v2.json",
            "./data/autogluon_titanic_input_v2_binary.json",
            "./data/autogluon_titanic_input_v2_all_binary.json",
        ]:
            response = await predict_isvc(rest_v2_client, service_name, input_path)
            assert len(response.outputs) > 0
            assert len(response.outputs[0].data) > 0
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_autogluon_runtime_kserve_v2_storage_uri_without_trailing_slash(
    rest_v2_client,
):
    service_name = "isvc-autogluon-v2-noslash"
    storage_uri = AUTOGLUON_STORAGE_URI.rstrip("/")
    predictor = _create_predictor(
        service_name, protocol_version="v2", storage_uri=storage_uri
    )
    response = await _deploy_and_predict(
        service_name,
        predictor,
        rest_v2_client,
        "./data/autogluon_titanic_input_v2.json",
    )
    assert len(response.outputs) > 0
    assert len(response.outputs[0].data) > 0
