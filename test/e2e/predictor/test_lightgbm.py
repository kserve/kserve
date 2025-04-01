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

import numpy
import pytest
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1LightGBMSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc, predict_grpc


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_lightgbm_kserve(rest_v1_client):
    service_name = "isvc-lightgbm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        lightgbm=V1beta1LightGBMSpec(
            storage_uri="gs://kfserving-examples/models/lightgbm/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
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

    res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input_v3.json")
    assert numpy.argmax(res["predictions"][0]) == 0
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_lightgbm_runtime_kserve(rest_v1_client):
    service_name = "isvc-lightgbm-runtime"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="lightgbm",
            ),
            storage_uri="gs://kfserving-examples/models/lightgbm/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
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

    res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input_v3.json")
    assert numpy.argmax(res["predictions"][0]) == 0

    res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input_v4.json")
    assert numpy.argmax(res["predictions"][0]) == 0

    res = await predict_isvc(rest_v1_client, service_name, "./data/iris_input_v5.json")
    assert numpy.argmax(res["predictions"][0]) == 0
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_lightgbm_v2_runtime_mlserver(rest_v2_client):
    service_name = "isvc-lightgbm-v2-runtime"
    protocol_version = "v2"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="lightgbm",
            ),
            runtime="kserve-mlserver",
            storage_uri="gs://kfserving-examples/models/lightgbm/v2/iris",
            protocol_version=protocol_version,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
            readiness_probe=client.V1Probe(
                http_get=client.V1HTTPGetAction(
                    path=f"/v2/models/{service_name}/ready", port=8080
                ),
                initial_delay_seconds=30,
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
        "./data/iris_input_v2.json",
    )
    assert res.outputs[0].data == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06,
    ]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.path_based_routing
@pytest.mark.asyncio(scope="session")
async def test_lightgbm_v2_kserve(rest_v2_client):
    service_name = "isvc-lightgbm-v2-kserve"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="lightgbm",
            ),
            runtime="kserve-lgbserver",
            storage_uri="gs://kfserving-examples/models/lightgbm/v2/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
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
        "./data/iris_input_v2.json",
    )
    assert res.outputs[0].data == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06,
    ]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip(reason="Not testable in ODH at the moment")
@pytest.mark.grpc
@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_lightgbm_v2_grpc(rest_v2_client):
    service_name = "isvc-lightgbm-v2-grpc"
    model_name = "lightgbm"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="lightgbm",
            ),
            runtime="kserve-lgbserver",
            storage_uri="gs://kfserving-examples/models/lightgbm/v2/iris",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
            ports=[V1ContainerPort(container_port=8081, name="h2c", protocol="TCP")],
            args=["--model_name", model_name],
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

    json_file = open("./data/iris_input_v2_grpc.json")
    payload = json.load(json_file)["inputs"]

    response = await predict_grpc(
        service_name=service_name,
        payload=payload,
        model_name=model_name,
    )
    prediction = response.outputs[0].data
    assert prediction == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06,
    ]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
