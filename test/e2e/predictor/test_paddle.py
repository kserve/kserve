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
import logging
import os

import numpy as np
import pytest
from kubernetes.client import V1ContainerPort, V1ObjectMeta, V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PaddleServerSpec,
    V1beta1PredictorSpec,
    constants,
)

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc, predict_grpc


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_paddle(rest_v1_client):
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        paddle=V1beta1PaddleServerSpec(
            storage_uri="gs://kfserving-examples/models/paddle/resnet",
            resources=V1ResourceRequirements(
                requests={"cpu": "200m", "memory": "256Mi"},
                limits={"cpu": "200m", "memory": "1Gi"},
            ),
        ),
    )

    service_name = "isvc-paddle"
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = await predict_isvc(rest_v1_client, service_name, "./data/jay.json")
    assert np.argmax(res["predictions"][0]) == 17

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_paddle_runtime(rest_v1_client):
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="paddle",
            ),
            storage_uri="gs://kfserving-examples/models/paddle/resnet",
            resources=V1ResourceRequirements(
                requests={"cpu": "200m", "memory": "256Mi"},
                limits={"cpu": "200m", "memory": "1Gi"},
            ),
        ),
    )

    service_name = "isvc-paddle-runtime"
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = await predict_isvc(rest_v1_client, service_name, "./data/jay.json")
    assert np.argmax(res["predictions"][0]) == 17

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_paddle_v2_kserve(rest_v2_client):
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="paddle",
            ),
            runtime="kserve-paddleserver",
            storage_uri="gs://kfserving-examples/models/paddle/resnet",
            resources=V1ResourceRequirements(
                requests={"cpu": "200m", "memory": "256Mi"},
                limits={"cpu": "200m", "memory": "1Gi"},
            ),
        ),
    )

    service_name = "isvc-paddle-v2-kserve"
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            logging.info(pod)
        raise e

    res = await predict_isvc(
        rest_v2_client,
        service_name,
        "./data/jay-v2.json",
    )
    assert np.argmax(res.outputs[0].data) == 17

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
@pytest.mark.skip("GRPC tests are failing in ODH at the moment")
async def test_paddle_v2_grpc():
    service_name = "isvc-paddle-v2-grpc"
    model_name = "paddle"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="paddle",
            ),
            runtime="kserve-paddleserver",
            storage_uri="gs://kfserving-examples/models/paddle/resnet",
            resources=V1ResourceRequirements(
                requests={"cpu": "200m", "memory": "256Mi"},
                limits={"cpu": "200m", "memory": "1Gi"},
            ),
            ports=[V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")],
            args=["--model_name", model_name],
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=V1ObjectMeta(name=service_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
    except RuntimeError as e:
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
        )
        for pod in pods.items:
            logging.info(pod)
        raise e

    json_file = open("./data/jay-v2-grpc.json")
    payload = json.load(json_file)["inputs"]
    response = await predict_grpc(
        service_name=service_name, payload=payload, model_name=model_name
    )
    prediction = response.outputs[0].data
    assert np.argmax(prediction) == 17

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
