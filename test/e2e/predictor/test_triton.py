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

import os

import numpy as np
import pytest
from kubernetes import client

from kserve import KServeClient, V1beta1TransformerSpec
from kubernetes.client import V1ResourceRequirements, V1Container, V1ContainerPort
from kserve import V1beta1InferenceService
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1ModelSpec, V1beta1ModelFormat
from kserve import V1beta1PredictorSpec
from kserve import V1beta1TritonSpec
from kserve import constants
from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common.utils import predict


@pytest.mark.fast
def test_triton():
    service_name = 'isvc-triton'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        triton=V1beta1TritonSpec(
            storage_uri='gs://kfserving-examples/models/torchscript',
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '512Mi'},
            ),
        )
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=800)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor"))
        deployments = kserve_client.app_api. \
            list_namespaced_deployment(KSERVE_TEST_NAMESPACE, label_selector='serving.kserve.io/'
                                                                             'inferenceservice={}'.
                                       format(service_name))
        for deployment in deployments.items:
            print(deployment)
        raise e
    res = predict(service_name, "./data/cifar10_input_v2.json", model_name='cifar10', protocol_version="v2")
    assert (np.argmax(res.get("outputs")[0]['data']) == 3)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.transformer
def test_triton_runtime_with_transformer():
    service_name = 'isvc-triton-runtime'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="triton",
            ),
            storage_uri='gs://kfserving-examples/models/torchscript',
            ports=[V1ContainerPort(name="h2c", protocol="TCP", container_port=9000)],
            resources=V1ResourceRequirements(
                requests={'cpu': '10m', 'memory': '128Mi'},
                limits={'cpu': '100m', 'memory': '512Mi'},
            ),
        )
    )

    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[V1Container(
                      image=os.environ.get("IMAGE_TRANSFORMER_IMG"),
                      name='kserve-container',
                      resources=V1ResourceRequirements(
                          requests={'cpu': '10m', 'memory': '128Mi'},
                          limits={'cpu': '100m', 'memory': '512Mi'}),
                      args=["--model_name", "cifar10", "--predictor_protocol", "grpc-v2"])]
    )
    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=800)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor"))
        deployments = kserve_client.app_api. \
            list_namespaced_deployment(KSERVE_TEST_NAMESPACE, label_selector='serving.kserve.io/'
                                       'inferenceservice={}'.
                                       format(service_name))
        for deployment in deployments.items:
            print(deployment)
        raise e
    res = predict(service_name, "./data/image.json", model_name='cifar10')
    assert (np.argmax(res.get("predictions")[0]) == 5)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
