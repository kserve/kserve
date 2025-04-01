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
import os
import json
import pytest
from kubernetes import client
from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TorchServeSpec,
    V1beta1ModelSpec,
    V1beta1ModelFormat,
)
from kubernetes.client import V1ResourceRequirements, V1ContainerPort

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc, predict_grpc

pytest.skip("ODH does not support torchserve at the moment", allow_module_level=True)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_torchserve_kserve(rest_v1_client):
    service_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            protocol_version="v1",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
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

    res = await predict_isvc(
        rest_v1_client, service_name, "./data/torchserve_input.json"
    )
    assert res["predictions"][0] == 2
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_torchserve_v2_kserve(rest_v2_client):
    service_name = "mnist-v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v2",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
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

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/torchserve_input_v2.json",
        model_name="mnist",
    )
    assert res.outputs[0].data == [1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.grpc
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_torchserve_grpc_v2():
    service_name = "mnist-grpc"
    model_name = "mnist"
    predictor = V1beta1PredictorSpec(
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v2",
            ports=[V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")],
            protocol_version="grpc-v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
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

    json_file = open("./data/torchserve_input.json")
    data = json.load(json_file)
    payload = [
        {
            "name": "input-0",
            "shape": [],
            "datatype": "BYTES",
            "contents": {
                "bytes_contents": [base64.b64decode(data["instances"][0]["data"])]
            },
        }
    ]
    response = await predict_grpc(
        service_name=service_name, payload=payload, model_name=model_name
    )
    fields = response.outputs[0].data
    assert fields == [2]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_torchserve_runtime_kserve(rest_v1_client):
    service_name = "mnist-runtime"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pytorch",
            ),
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            protocol_version="v1",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "4Gi"},
                limits={"cpu": "1", "memory": "4Gi"},
            ),
        ),
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

    res = await predict_isvc(
        rest_v1_client, service_name, "./data/torchserve_input.json", model_name="mnist"
    )
    assert res["predictions"][0] == 2
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
