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
import logging

from kubernetes import client

from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1CatBoostSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
)
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc

logging.basicConfig(level=logging.INFO)
MODEL_NAME = "catboost"
PREDICTOR = "catboost"


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_catboost_kserve():
    service_name = "isvc-catboost"
    protocol_version = "v1"

    predictor_spec = V1beta1PredictorSpec(
        min_replicas=1,
        catboost=V1beta1CatBoostSpec(
            storage_uri="gs://kfserving-examples/models/catboost/iris",
            protocol_version=protocol_version,
            resources=client.V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor_spec),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE, timeout=720)

    res = await predict_isvc(
        service_name=service_name,
        input_json={
            "instances": [
                [6.8, 2.8, 4.8, 1.4],
                [6.0, 3.4, 4.5, 1.6],
            ]
        },
    )

    assert res["predictions"] is not None

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.runtime
@pytest.mark.asyncio(scope="session")
async def test_catboost_runtime_kserve():
    service_name = "isvc-catboost-runtime"
    protocol_version = "v1"

    predictor_spec = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="catboost",
            ),
            runtime="kserve-catboostserver",
            storage_uri="gs://kfserving-examples/models/catboost/iris",
            protocol_version=protocol_version,
            resources=client.V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "256Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor_spec),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE, timeout=720)

    res = await predict_isvc(
        service_name=service_name,
        input_json={
            "instances": [
                [6.8, 2.8, 4.8, 1.4],
                [6.0, 3.4, 4.5, 1.6],
            ]
        },
    )

    assert res["predictions"] is not None

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
