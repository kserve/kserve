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

import base64
import json
import os

import pytest
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    constants
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort
from ..common.utils import predict_grpc
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.grpc
def test_custom_model_grpc():
    service_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-model-grpc:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                ports=[
                    V1ContainerPort(
                        container_port=8081,
                        name="h2c",
                        protocol="TCP"
                    )]
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/custom_model_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [base64.b64decode(data["instances"][0]["image"]["b64"])]
            }
        }
    ]
    response = predict_grpc(service_name=service_name, payload=payload)
    fields = response.outputs[0].contents.ListFields()
    _, field_value = fields[0]
    points = list(field_value)
    assert points == [
        14.975619316101074,
        14.036808967590332,
        13.966033935546875,
        12.252279281616211,
        12.086268424987793
    ]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
