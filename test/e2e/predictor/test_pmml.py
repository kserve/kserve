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

import json
import os

from kserve import KServeClient
from kserve import V1beta1InferenceService
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1PMMLSpec
from kserve import V1beta1PredictorSpec
from kserve import V1beta1ModelSpec, V1beta1ModelFormat
from kserve import constants
from kubernetes import client
from kubernetes.client import V1ResourceRequirements, V1ContainerPort
import pytest

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_grpc
from ..common.utils import predict


@pytest.mark.predictor
def test_pmml_kserve():
    service_name = 'isvc-pmml'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pmml=V1beta1PMMLSpec(
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, './data/pmml_input.json')
    assert (res["predictions"] == [{'Species': 'setosa',
                                    'Probability_setosa': 1.0,
                                    'Probability_versicolor': 0.0,
                                    'Probability_virginica': 0.0,
                                    'Node_Id': '2'}])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_pmml_runtime_kserve():
    service_name = 'isvc-pmml-runtime'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pmml",
            ),
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                        name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    res = predict(service_name, './data/pmml_input.json')
    assert (res["predictions"] == [{'Species': 'setosa',
                                    'Probability_setosa': 1.0,
                                    'Probability_versicolor': 0.0,
                                    'Probability_virginica': 0.0,
                                    'Node_Id': '2'}])
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_pmml_v2_kserve():
    service_name = 'isvc-pmml-v2-kserve'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pmml",
            ),
            runtime="kserve-pmmlserver",
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
            )
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
    res = predict(service_name, './data/pmml-input-v2.json',
                  protocol_version="v2")
    assert res["outputs"] == [
        {'name': 'Species', 'shape': [1], 'datatype': 'BYTES', 'data': ['setosa'], 'parameters': None},
        {'name': 'Probability_setosa', 'shape': [1], 'datatype': 'FP64', 'data': [1.0], 'parameters': None},
        {'name': 'Probability_versicolor', 'shape': [1], 'datatype': 'FP64', 'data': [0.0], 'parameters': None},
        {'name': 'Probability_virginica', 'shape': [1], 'datatype': 'FP64', 'data': [0.0], 'parameters': None},
        {'name': 'Node_Id', 'shape': [1], 'datatype': 'BYTES', 'data': ['2'], 'parameters': None}]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
def test_pmml_v2_grpc():
    service_name = "isvc-pmml-v2-grpc"
    model_name = "pmml"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pmml",
            ),
            runtime="kserve-pmmlserver",
            storage_uri='gs://kfserving-examples/models/pmml',
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '256Mi'}
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

    json_file = open("./data/pmml_input_v2_grpc.json")
    payload = json.load(json_file)["inputs"]

    response = predict_grpc(service_name=service_name,
                            payload=payload, model_name=model_name)
    assert response.outputs[0].contents.bytes_contents[0] == b'setosa'
    assert response.outputs[1].contents.fp64_contents[0] == 1.0
    assert response.outputs[2].contents.fp64_contents[0] == 0.0
    assert response.outputs[3].contents.fp64_contents[0] == 0.0
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
