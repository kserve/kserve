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
from kubernetes.client import V1Container, V1ResourceRequirements, V1ContainerPort

from kserve import (
    V1beta1PredictorSpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    KServeClient,
)
from kserve.constants import constants
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_custom_model_http_ray(rest_v1_client):
    service_name = "custom-model-http-ray"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                # Override the entrypoint to run the model using ray
                command=["python", "-m", "custom_model.model_remote"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "1", "memory": "1Gi"},
                    limits={"cpu": "1", "memory": "2Gi"},
                ),
                ports=[V1ContainerPort(container_port=8080, protocol="TCP")],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    response = await predict_isvc(
        rest_v1_client,
        service_name=service_name,
        input="./data/custom_model_input.json",
        model_name=model_name,
    )
    outputs = response["predictions"]
    points = ["%.3f" % (point) for point in outputs[0]]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
