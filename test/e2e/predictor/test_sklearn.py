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

import json
import os

import pytest
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1ResourceRequirements

from kserve import (KServeClient, V1beta1InferenceService,
                    V1beta1InferenceServiceSpec, V1beta1ModelFormat,
                    V1beta1ModelSpec, V1beta1PredictorSpec, V1beta1SKLearnSpec,
                    constants)

import kserve.protocol.grpc.grpc_predict_v2_pb2 as inference_pb2

from ..common.utils import KSERVE_TEST_NAMESPACE, predict, predict_grpc


@pytest.mark.slow
def test_sklearn_kserve():
    service_name = "isvc-sklearn"
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2_mlserver():
    service_name = "isvc-sklearn-v2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://seldon-models/sklearn/mms/lr_model",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json", protocol_version="v2")
    assert res["outputs"][0]["data"] == [1, 1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_runtime_kserve():
    service_name = "isvc-sklearn-runtime"
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, "./data/iris_input.json")
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2_runtime_mlserver():
    service_name = "isvc-sklearn-v2-runtime"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            runtime="kserve-mlserver",
            storage_uri="gs://seldon-models/sklearn/mms/lr_model",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json", protocol_version="v2")
    assert res["outputs"][0]["data"] == [1, 1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2():
    service_name = "isvc-sklearn-v2"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            runtime="kserve-sklearnserver",
            storage_uri="gs://seldon-models/sklearn/mms/lr_model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json", protocol_version="v2")
    assert res["outputs"][0]["data"] == [1, 1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2_grpc():
    service_name = "isvc-sklearn-v2-grpc"
    model_name = "sklearn"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            runtime="kserve-sklearnserver",
            storage_uri="gs://seldon-models/sklearn/mms/lr_model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
            ports=[
                V1ContainerPort(
                    container_port=8081,
                    name="h2c",
                    protocol="TCP"
                )],
            args=["--model_name", model_name]
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/iris_input_v2_grpc.json")
    payload = json.load(json_file)["inputs"]

    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name)
    prediction = list(response.outputs[0].contents.int64_contents)
    assert prediction == [1, 1]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2_mixed():
    service_name = "isvc-sklearn-v2-mixed"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            runtime="kserve-sklearnserver",
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/mixedtype",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    response = predict(service_name, "./data/sklearn_mixed_v2.json", protocol_version="v2")
    assert response["outputs"][0]["data"] == [12.202832815138274]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.slow
def test_sklearn_v2_mixed_grpc():
    service_name = "isvc-sklearn-v2-mixed-grpc"
    model_name = "sklearn"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="sklearn",
            ),
            runtime="kserve-sklearnserver",
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/mixedtype",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
            ports=[
                V1ContainerPort(
                    container_port=8081,
                    name="h2c",
                    protocol="TCP"
                )],
            args=["--model_name", model_name]
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/sklearn_mixed.json")
    data = json.load(json_file)
    payload = []
    for key, val in data.items():
        if isinstance(val, int):
            payload.append(
                {
                    "name": key,
                    "shape": [1],
                    "datatype": "INT32",
                    "contents": {
                        "int_contents": [val]
                    }
                }
            )
        elif isinstance(val, str):
            payload.append(
                {
                    "name": key,
                    "shape": [1],
                    "datatype": "BYTES",
                    "contents": {
                        "bytes_contents": [bytes(val, 'utf-8')]
                    }
                }
            )
    parameters = {"content_type": inference_pb2.InferParameter(string_param="pd")}
    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name, parameters=parameters)
    prediction = list(response.outputs[0].contents.fp64_contents)
    assert prediction == [12.202832815138274]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
