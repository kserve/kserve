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
import uuid

import numpy as np
import pytest
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TransformerSpec,
    constants,
    InferRequest,
    InferInput,
)
from kubernetes.client import V1ResourceRequirements
from kubernetes import client
from kubernetes.client import V1Container, V1ContainerPort, V1EnvVar
from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    is_model_ready,
    predict_isvc,
    predict_grpc,
)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.grpc
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_custom_model_grpc():
    service_name = "custom-model-grpc"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")
                ],
                args=["--model_name", model_name],
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

    json_file = open("./data/custom_model_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [
                    base64.b64decode(data["instances"][0]["image"]["b64"])
                ]
            },
        }
    ]
    response = await predict_grpc(
        service_name=service_name, payload=payload, model_name=model_name
    )
    fields = response.outputs[0].data
    points = ["%.3f" % (point) for point in fields]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.grpc
@pytest.mark.transformer
@pytest.mark.asyncio(scope="session")
async def test_predictor_grpc_with_transformer_grpc():
    service_name = "model-grpc-trans-grpc"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")
                ],
                args=["--model_name", model_name],
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
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")
                ],
                args=["--model_name", model_name, "--predictor_protocol", "grpc-v2"],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/custom_model_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [
                    base64.b64decode(data["instances"][0]["image"]["b64"])
                ]
            },
        }
    ]
    response = await predict_grpc(
        service_name=service_name, payload=payload, model_name=model_name
    )
    fields = response.outputs[0].data
    points = ["%.3f" % (point) for point in list(fields)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.grpc
@pytest.mark.transformer
@pytest.mark.asyncio(scope="session")
async def test_predictor_grpc_with_transformer_http(rest_v2_client):
    service_name = "model-grpc-trans-http"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")
                ],
                args=["--model_name", model_name],
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
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=[
                    "--model_name",
                    model_name,
                    "--predictor_protocol",
                    "grpc-v2",
                    "--enable_predictor_health_check",
                ],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    is_ready = await is_model_ready(rest_v2_client, service_name, model_name) is True
    assert is_ready is True
    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/custom_model_input_v2.json",
        model_name=model_name,
    )
    points = ["%.3f" % point for point in list(res.outputs[0].data)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]

    with open("./data/custom_model_input_v2.json") as json_data:
        data = json.load(json_data)
    infer_input = InferInput(
        name=data["inputs"][0]["name"],
        datatype=data["inputs"][0]["datatype"],
        shape=data["inputs"][0]["shape"],
    )
    infer_input.set_data_from_numpy(
        np.array(data["inputs"][0]["data"], dtype=np.object_)
    )
    infer_request = InferRequest(
        model_name=model_name,
        infer_inputs=[infer_input],
        parameters={"binary_data_output": True},
    )
    res = await predict_isvc(
        rest_v2_client,
        service_name,
        infer_request,
        model_name=model_name,
    )
    points = ["%.3f" % point for point in list(res.outputs[0].data)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.transformer
@pytest.mark.asyncio(scope="session")
async def test_predictor_rest_with_transformer_rest(rest_v2_client):
    service_name = "model-rest-trans-rest"
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("CUSTOM_MODEL_GRPC_IMG_TAG"),
                command=["python", "-m", "custom_model.model"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "512Mi"},
                    limits={"cpu": "100m", "memory": "2Gi"},
                ),
                args=["--model_name", model_name],
                env=[V1EnvVar(name="PROTOCOL", value="v2")],
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
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=[
                    "--model_name",
                    model_name,
                    "--predictor_protocol",
                    "v2",
                    "--enable_predictor_health_check",
                ],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    is_ready = await is_model_ready(rest_v2_client, service_name, model_name) is True
    assert is_ready is True
    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/custom_model_input_v2.json",
        model_name=model_name,
    )
    points = ["%.3f" % point for point in list(res.outputs[0].data)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]

    with open("./data/custom_model_input_v2.json") as json_data:
        data = json.load(json_data)
    infer_input = InferInput(
        name=data["inputs"][0]["name"],
        datatype=data["inputs"][0]["datatype"],
        shape=data["inputs"][0]["shape"],
    )
    infer_input.set_data_from_numpy(
        np.array(data["inputs"][0]["data"], dtype=np.object_)
    )
    infer_request = InferRequest(
        model_name=model_name,
        infer_inputs=[infer_input],
        parameters={"binary_data_output": True},
    )
    res = await predict_isvc(
        rest_v2_client,
        service_name,
        infer_request,
        model_name=model_name,
    )
    points = ["%.3f" % point for point in list(res.outputs[0].data)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_predictor_grpc_with_transformer_grpc_raw(network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "model-grpc-trans-grpc-raw-" + suffix
    model_name = "custom-model"

    predictor = V1beta1PredictorSpec(
        containers=[
            V1Container(
                name="kserve-container",
                image="kserve/custom-model-grpc:" + os.environ.get("GITHUB_SHA"),
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(
                        container_port=8081, name="h2c-port", protocol="TCP"
                    )
                ],
                args=["--model_name", model_name],
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
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                ports=[
                    V1ContainerPort(
                        container_port=8081, name="grpc-port", protocol="TCP"
                    )
                ],
                args=["--model_name", model_name, "--predictor_protocol", "grpc-v2"],
            )
        ]
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "RawDeployment"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/custom_model_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [
                    base64.b64decode(data["instances"][0]["image"]["b64"])
                ]
            },
        }
    ]
    response = await predict_grpc(
        service_name=service_name,
        payload=payload,
        model_name=model_name,
        network_layer=network_layer,
    )
    fields = response.outputs[0].data
    points = ["%.3f" % (point) for point in list(fields)]
    assert points == ["14.976", "14.037", "13.966", "12.252", "12.086"]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
