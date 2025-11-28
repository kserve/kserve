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
from ..common.utils import predict_isvc


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_triton(rest_v2_client):
    service_name = "isvc-triton"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        triton=V1beta1TritonSpec(
            storage_uri="gs://kfserving-examples/models/torchscript",
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            labels={
                constants.KSERVE_LABEL_NETWORKING_VISIBILITY: constants.KSERVE_LABEL_NETWORKING_VISIBILITY_EXPOSED,
            },
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=800
        )
    except RuntimeError as e:
        services = kserve_client.core_api.list_namespaced_service(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for svc in services.items:
            print(svc)
        deployments = kserve_client.app_api.list_namespaced_deployment(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/"
            "inferenceservice={}".format(service_name),
        )
        for deployment in deployments.items:
            print(deployment)
        raise e
    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/cifar10_input_v2.json",
        model_name="cifar10",
    )
    assert np.argmax(res.outputs[0].data) == 3
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.transformer
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
@pytest.mark.skip(reason="Reactivate after https://github.com/opendatahub-io/odh-model-controller/pull/601 is merged")
async def test_triton_runtime_with_transformer(rest_v1_client):
    service_name = "isvc-triton-runtime"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="triton",
            ),
            storage_uri="gs://kfserving-examples/models/torchscript",
            ports=[V1ContainerPort(name="h2c", protocol="TCP", container_port=9000)],
            resources=V1ResourceRequirements(
                requests={"cpu": "10m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "512Mi"},
            ),
        ),
    )

    # Check if IMAGE_TRANSFORMER_IMG_TAG environment variable is set
    transformer_image = os.environ.get("IMAGE_TRANSFORMER_IMG_TAG")
    if not transformer_image:
        error_msg = "ERROR: IMAGE_TRANSFORMER_IMG_TAG environment variable is not set. This is required for the transformer container image."
        print(error_msg)
        raise ValueError(error_msg)

    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[
            V1Container(
                image=transformer_image,
                name="kserve-container",
                ports=[V1ContainerPort(container_port=8080, name="http1", protocol="TCP")],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "512Mi"},
                ),
                args=["--model_name", "cifar10", "--predictor_protocol", "grpc-v2"],
            )
        ],
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            labels={
                constants.KSERVE_LABEL_NETWORKING_VISIBILITY: constants.KSERVE_LABEL_NETWORKING_VISIBILITY_EXPOSED,
            },
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=800
        )

    except RuntimeError as e:
        services = kserve_client.core_api.list_namespaced_service(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for svc in services.items:
            print(svc)
        deployments = kserve_client.app_api.list_namespaced_deployment(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/"
            "inferenceservice={}".format(service_name),
        )
        for deployment in deployments.items:
            print(deployment)
        raise e

    res = await predict_isvc(
        rest_v1_client, service_name, "./data/image.json", model_name="cifar10"
    )
    assert np.argmax(res["predictions"][0]) == 5
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
