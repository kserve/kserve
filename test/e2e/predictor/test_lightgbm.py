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

import pytest
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1ResourceRequirements

from kserve import (KServeClient, V1beta1InferenceService,
                    V1beta1InferenceServiceSpec, V1beta1LightGBMSpec,
                    V1beta1ModelFormat, V1beta1ModelSpec, V1beta1PredictorSpec,
                    constants)

from ..common.utils import KSERVE_TEST_NAMESPACE, predict, predict_grpc


@pytest.mark.fast
def test_lightgbm_kserve():
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v3.json")
    assert res["predictions"][0][0] > 0.5
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.fast
def test_lightgbm_runtime_kserve():
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v3.json")
    assert res["predictions"][0][0] > 0.5
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.fast
def test_lightgbm_v2_runtime_mlserver():
    service_name = "isvc-lightgbm-v2-runtime"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="lightgbm",
            ),
            runtime="kserve-mlserver",
            storage_uri="gs://kfserving-examples/models/lightgbm/v2/iris",
            protocol_version="v2",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
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

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json",
                  protocol_version="v2")
    assert res["outputs"][0]["data"] == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.fast
def test_lightgbm_v2_kserve():
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
        kind=constants.KSERVE_KIND,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(
        service_name, namespace=KSERVE_TEST_NAMESPACE)

    res = predict(service_name, "./data/iris_input_v2.json",
                  protocol_version="v2")
    assert res["outputs"][0]["data"] == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06]

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.grpc
def test_lightgbm_v2_grpc():
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

    kserve_client = KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    json_file = open("./data/iris_input_v2_grpc.json")
    payload = json.load(json_file)["inputs"]

    response = predict_grpc(service_name=service_name, payload=payload, model_name=model_name)
    prediction = list(response.outputs[0].contents.fp64_contents)
    assert prediction == [
        8.796664107010673e-06,
        0.9992300031041593,
        0.0007612002317336916,
        4.974786820804187e-06,
        0.9999919650711493,
        3.0601420299625077e-06]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
