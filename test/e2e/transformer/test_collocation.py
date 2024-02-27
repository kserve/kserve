# Copyright 2023 The KServe Authors.
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

from kserve import KServeClient
from kserve import constants
from kserve import V1beta1PredictorSpec
from kserve import V1beta1InferenceServiceSpec
from kserve import V1beta1InferenceService
from kubernetes.client import V1ResourceRequirements
from kubernetes.client import V1Container
from kubernetes.client import V1EnvVar
from kubernetes.client import V1ContainerPort
import pytest
from ..common.utils import predict
from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    INFERENCESERVICE_CONTAINER,
    TRANSFORMER_CONTAINER,
    STORAGE_URI_ENV,
)


@pytest.mark.collocation
def test_transformer_collocation():
    service_name = "custom-model-transformer-collocation"
    model_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name=INFERENCESERVICE_CONTAINER,
                image="pytorch/torchserve:0.9.0-cpu",
                env=[
                    V1EnvVar(name=STORAGE_URI_ENV,
                             value="gs://kfserving-examples/models/torchserve/image_classifier/v1"),
                    V1EnvVar(name="TS_SERVICE_ENVELOPE", value="kserve")
                ],
                args=["torchserve", "--start", "--model-store=/mnt/models/model-store",
                      "--ts-config=/mnt/models/config/config.properties"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "1", "memory": "1Gi"},
                ),
            ),
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[f"--model_name={model_name}", "--http_port=8080", "--grpc_port=8081",
                      "--predictor_host=localhost:8085"],
                ports=[
                    V1ContainerPort(
                        container_port=8080,
                        protocol="TCP"
                    )
                ],
                resources=V1ResourceRequirements(
                    requests={'cpu': '10m', 'memory': '128Mi'},
                    limits={'cpu': '100m', 'memory': '1Gi'}),
            )
        ]
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor"))
        pods = kserve_client.core_api.list_namespaced_pod(KSERVE_TEST_NAMESPACE,
                                                          label_selector='serving.kserve.io/inferenceservice={}'
                                                          .format(service_name))
        for pod in pods.items:
            print(pod)
        raise e
    res = predict(service_name, "./data/transformer.json", model_name=model_name)
    assert (res.get("predictions")[0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
def test_raw_transformer_collocation():
    service_name = "raw-custom-model-collocation"
    model_name = "mnist"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name=INFERENCESERVICE_CONTAINER,
                image="pytorch/torchserve:0.9.0-cpu",
                env=[
                    V1EnvVar(name=STORAGE_URI_ENV,
                             value="gs://kfserving-examples/models/torchserve/image_classifier/v1"),
                    V1EnvVar(name="TS_SERVICE_ENVELOPE", value="kserve")
                ],
                args=["torchserve", "--start", "--model-store=/mnt/models/model-store",
                      "--ts-config=/mnt/models/config/config.properties"],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "1", "memory": "1Gi"},
                ),
            ),
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[f"--model_name={model_name}", "--http_port=8080", "--grpc_port=8081",
                      "--predictor_host=localhost:8085"],
                ports=[
                    V1ContainerPort(
                        name="http",
                        container_port=8080,
                        protocol="TCP"
                    ),
                    V1ContainerPort(
                        name="grpc",
                        container_port=8081,
                        protocol="TCP"
                    )
                ],
                resources=V1ResourceRequirements(
                    requests={'cpu': '10m', 'memory': '128Mi'},
                    limits={'cpu': '100m', 'memory': '1Gi'}),
            )
        ]
    )

    isvc = V1beta1InferenceService(api_version=constants.KSERVE_V1BETA1,
                                   kind=constants.KSERVE_KIND,
                                   metadata=client.V1ObjectMeta(
                                       name=service_name, namespace=KSERVE_TEST_NAMESPACE,
                                       annotations={"serving.kserve.io/deploymentMode": "RawDeployment"}
                                   ),
                                   spec=V1beta1InferenceServiceSpec(predictor=predictor))

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    except RuntimeError as e:
        print(kserve_client.api_instance.get_namespaced_custom_object("serving.knative.dev", "v1",
                                                                      KSERVE_TEST_NAMESPACE,
                                                                      "services", service_name + "-predictor"))
        pods = kserve_client.core_api.list_namespaced_pod(KSERVE_TEST_NAMESPACE,
                                                          label_selector='serving.kserve.io/inferenceservice={}'
                                                          .format(service_name))
        for pod in pods.items:
            print(pod)
        raise e
    res = predict(service_name, "./data/transformer.json", model_name=model_name)
    assert (res.get("predictions")[0] == 2)
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
