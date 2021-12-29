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
from kubernetes import client
import numpy as np
from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec, V1beta1TransformerSpec
from kserve import V1beta1TritonSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kserve import V1beta1ModelSpec, V1beta1ModelFormat
from kubernetes.client import V1ResourceRequirements, V1Container, V1ContainerPort
from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common.utils import predict


def test_triton():
    service_name = 'isvc-triton'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        triton=V1beta1TritonSpec(
            storage_uri='gs://kfserving-examples/models/torchscript',
            ports=[V1ContainerPort(name="h2c", protocol="TCP", container_port=9000)],
            resources=V1ResourceRequirements(
                requests={'cpu': '100m', 'memory': '1Gi'},
                limits={'cpu': '1', 'memory': '1Gi'},
            ),
        )
    )
    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[V1Container(
                      image='809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/image-transformer:'
                            + os.environ.get("PULL_BASE_SHA"),
                      name='kserve-container',
                      resources=V1ResourceRequirements(
                          requests={'cpu': '100m', 'memory': '1Gi'},
                          limits={'cpu': '100m', 'memory': '1Gi'}),
                      args=["--model_name", "cifar10", "--protocol", "grpc-v2"])]
    )
    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor-default"))
        deployments = kserve_client.app_api. \
            list_namespaced_deployment(KSERVE_TEST_NAMESPACE, label_selector='serving.kserve.io/'
                                       'inferenceservice={}'.
                                       format(service_name))
        for deployment in deployments.items:
            print(deployment)
        raise e
    res = predict(service_name, "./data/image.json", model_name='cifar10')
    assert(np.argmax(res.get("predictions")[0]) == 5)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


def test_triton_runtime():
    service_name = 'isvc-triton-runtime'
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="pytorch",
            ),
            runtime="kserve-tritonserver",
            storage_uri='gs://kfserving-examples/models/torchscript',
            ports=[V1ContainerPort(name="h2c", protocol="TCP", container_port=9000)]
        )
    )

    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[V1Container(
                      image='809251082950.dkr.ecr.us-west-2.amazonaws.com/kserve/image-transformer:'
                            + os.environ.get("PULL_BASE_SHA"),
                      name='kserve-container',
                      resources=V1ResourceRequirements(
                          requests={'cpu': '100m', 'memory': '1Gi'},
                          limits={'cpu': '100m', 'memory': '1Gi'}),
                      args=["--model_name", "cifar10", "--protocol", "grpc-v2"])]
    )
    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor-default"))
        deployments = kserve_client.app_api. \
            list_namespaced_deployment(KSERVE_TEST_NAMESPACE, label_selector='serving.kserve.io/'
                                       'inferenceservice={}'.
                                       format(service_name))
        for deployment in deployments.items:
            print(deployment)
        raise e
    res = predict(service_name, "./data/image.json", model_name='cifar10')
    assert(np.argmax(res.get("predictions")[0]) == 5)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
