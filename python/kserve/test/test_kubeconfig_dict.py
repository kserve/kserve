# Copyright 2021 The KServe Authors.
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

from unittest.mock import patch

from kubernetes import client

from kserve import V1beta1PredictorSpec
from kserve import V1beta1TFServingSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kserve import KServeClient


KUBECONFIG_DICT = {
    "apiVersion": "v1",
    "kind": "Config",
    "metadata": {"name": "example-cluster"},
    "clusters": [
        {
            "name": "example-cluster",
            "cluster": {
                "certificate-authority": "./ca.pem",
                "server": "http://127.0.0.1:8080",
            },
        }
    ],
    "contexts": [
        {
            "name": "example-cluster-context",
            "context": {
                "cluster": "example-cluster",
                "namespace": "example",
                "user": "example-cluster-admin",
            },
        }
    ],
    "users": [
        {
            "name": "example-cluster-admin",
            "user": {
                "client-certificate": "./admin.pem",
                "client-key": "./admin-key.pem",
            },
        }
    ],
    "current-context": "example-cluster-context",
}

kserve_client = KServeClient(config_dict=KUBECONFIG_DICT)

mocked_unit_result = """
{
    "api_version": "serving.kserve.io/v1beta1",
    "kind": "InferenceService",
    "metadata": {
        "name": "flower-sample",
        "namespace": "kubeflow"
    },
    "spec": {
        "predictor": {
            "tensorflow": {
                "storage_uri": "gs://kfserving-samples/models/tensorflow/flowers"
            }
        }
    }
}
 """


def generate_inferenceservice():
    tf_spec = V1beta1TFServingSpec(
        storage_uri="gs://kfserving-samples/models/tensorflow/flowers"
    )
    predictor_spec = V1beta1PredictorSpec(tensorflow=tf_spec)

    isvc = V1beta1InferenceService(
        api_version="serving.kserve.io/v1beta1",
        kind="InferenceService",
        metadata=client.V1ObjectMeta(name="flower-sample"),
        spec=V1beta1InferenceServiceSpec(predictor=predictor_spec),
    )
    return isvc


def test_inferenceservice_client_create():
    """Unit test for kserve create api"""
    with patch(
        "kserve.api.kserve_client.KServeClient.create", return_value=mocked_unit_result
    ):
        isvc = generate_inferenceservice()
        assert mocked_unit_result == kserve_client.create(isvc, namespace="kubeflow")


def test_inferenceservice_client_get():
    """Unit test for kserve get api"""
    with patch(
        "kserve.api.kserve_client.KServeClient.get", return_value=mocked_unit_result
    ):
        assert mocked_unit_result == kserve_client.get(
            "flower-sample", namespace="kubeflow"
        )
