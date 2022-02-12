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

from ..common.utils import predict, grpc_stub
from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common import inference_pb2


def test_torchserve_kserve():
    service_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            protocol_version="v1",
            resources=V1ResourceRequirements(
                requests={
                    "cpu": "100m",
                    "memory": "1Gi"
                },
                limits={
                    "cpu": "1",
                    "memory": "1Gi"
                },
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/torchserve_input.json")
    assert (res.get("predictions")[0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


def test_torchserve_v2_kserve():
    service_name = "mnist-v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v2",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={
                    "cpu": "100m",
                    "memory": "1Gi"
                },
                limits={
                    "cpu": "1",
                    "memory": "1Gi"
                },
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/torchserve_input_v2.json", model_name="mnist")
    assert (res.get("outputs")[0]["data"] == [1])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip()
def test_torchserve_grpc():
    service_name = "mnist-grpc"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            ports=[V1ContainerPort(container_port=7070, name="h2c", protocol="TCP")],
            resources=V1ResourceRequirements(
                requests={
                    "cpu": "100m",
                    "memory": "1Gi"
                },
                limits={
                    "cpu": "1",
                    "memory": "1Gi"
                },
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    with open("./data/torchserve_input.json", 'rb') as f:
        data = f.read()

    input_data = {'data': data}
    stub = grpc_stub(service_name, KSERVE_TEST_NAMESPACE)
    response = stub.Predictions(
        inference_pb2.PredictionsRequest(model_name='mnist', input=input_data))

    prediction = response.prediction.decode('utf-8')
    json_output = json.loads(prediction)
    print(json_output)
    assert (json_output["predictions"][0][0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


def test_torchserve_runtime_kserve():
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/torchserve_input.json", model_name="mnist")
    assert (res.get("predictions")[0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
