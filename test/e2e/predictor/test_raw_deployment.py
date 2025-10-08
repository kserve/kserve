# Copyright 2022 The KServe Authors.
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
from kubernetes import client
from kubernetes.client import (
    V1ResourceRequirements,
    V1Container,
    V1ContainerPort,
)
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
)
import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_grpc
from ..common.utils import predict_isvc

api_version = constants.KSERVE_V1BETA1


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_raw_deployment_kserve(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "raw-sklearn-" + suffix
    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"
    labels = dict()
    labels["networking.kserve.io/visibility"] = "exposed"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/iris_input.json",
        network_layer=network_layer,
    )
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_raw_deployment_runtime_kserve(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "raw-sklearn-runtime-" + suffix
    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"
    labels = dict()
    labels["networking.kserve.io/visibility"] = "exposed"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels=labels,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = await predict_isvc(
        rest_v1_client,
        service_name,
        "./data/iris_input.json",
        network_layer=network_layer,
    )
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.grpc
@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
@pytest.mark.skip(
    "The custom-model-grpc image fails in OpenShift with a permission denied error"
)
async def test_isvc_with_multiple_container_port(network_layer):
    service_name = "raw-multiport-custom-model"
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
                    V1ContainerPort(
                        container_port=8081, name="grpc-port", protocol="TCP"
                    ),
                    V1ContainerPort(
                        container_port=8080, name="http-port", protocol="TCP"
                    ),
                ],
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
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    with open("./data/custom_model_input.json") as json_file:
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
    expected_output = ["14.976", "14.037", "13.966", "12.252", "12.086"]
    grpc_response = await predict_grpc(
        service_name=service_name,
        payload=payload,
        model_name=model_name,
        network_layer=network_layer,
    )
    fields = grpc_response.outputs[0].data
    grpc_output = ["%.3f" % value for value in fields]
    assert grpc_output == expected_output
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
