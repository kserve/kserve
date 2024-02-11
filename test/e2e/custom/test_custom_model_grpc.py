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
    V1beta1TransformerSpec,
    constants
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort
from ..common.utils import KSERVE_TEST_NAMESPACE, predict, predict_grpc


@pytest.mark.grpc
@pytest.mark.predictor
def test_custom_model_grpc():
    service_name = "custom-model-grpc"
    model_name = "custom-model"

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
                    )],
                args=["--model_name", model_name]
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
    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name)
    fields = response.outputs[0].contents.ListFields()
    _, field_value = fields[0]
    points = ['%.3f' % (point) for point in list(field_value)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.grpc
@pytest.mark.transformer
def test_predictor_grpc_with_transformer_grpc():
    service_name = "model-grpc-trans-grpc"
    model_name = "custom-model"

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
                    )],
                args=["--model_name", model_name]
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-image-transformer-grpc:"
                      + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                ports=[
                    V1ContainerPort(
                        container_port=8081,
                        name="h2c",
                        protocol="TCP"
                    )],
                args=["--model_name", model_name, "--predictor_protocol", "grpc-v2"]
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor, transformer=transformer),
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
    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name)
    fields = response.outputs[0].contents.ListFields()
    _, field_value = fields[0]
    points = ['%.3f' % (point) for point in list(field_value)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.grpc
@pytest.mark.transformer
def test_predictor_grpc_with_transformer_http():
    service_name = "model-grpc-trans-http"
    model_name = "custom-model"

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
                    )],
                args=["--model_name", model_name]
            )
        ]
    )

    transformer = V1beta1TransformerSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"}),
                args=["--model_name", model_name, "--predictor_protocol", "grpc-v2"]
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/custom_model_input_v2.json",
                  protocol_version="v2", model_name=model_name)
    points = ['%.3f' % (point) for point in list(res["outputs"][0]["data"])]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
