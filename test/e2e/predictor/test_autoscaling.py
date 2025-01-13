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

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
)
from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common.utils import predict_isvc

TARGET = "autoscaling.knative.dev/target"
METRIC = "autoscaling.knative.dev/metric"
MODEL = "gs://kfserving-examples/models/sklearn/1.0/model"
INPUT = "./data/iris_input.json"


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_sklearn_kserve_concurrency(rest_v1_client):
    service_name = "isvc-sklearn-scale-concurrency"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        scale_metric="concurrency",
        scale_target=2,
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
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
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
    )

    isvc_annotations = pods.items[0].metadata.annotations

    res = await predict_isvc(rest_v1_client, service_name, INPUT)
    assert res["predictions"] == [1, 1]
    assert isvc_annotations[METRIC] == "concurrency"
    assert isvc_annotations[TARGET] == "2"
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.predictor
@pytest.mark.asyncio(scope="session")
async def test_sklearn_kserve_rps(rest_v1_client):
    service_name = "isvc-sklearn-scale-rps"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        scale_metric="rps",
        scale_target=5,
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
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
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
    )

    annotations = pods.items[0].metadata.annotations

    assert annotations[METRIC] == "rps"
    assert annotations[TARGET] == "5"
    res = await predict_isvc(rest_v1_client, service_name, INPUT)
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.skip()
@pytest.mark.asyncio(scope="session")
async def test_sklearn_kserve_cpu(rest_v1_client):
    service_name = "isvc-sklearn-scale-cpu"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        scale_metric="cpu",
        scale_target=50,
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = dict()
    annotations["autoscaling.knative.dev/class"] = "hpa.autoscaling.knative.dev"

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE, annotations=annotations
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/inferenceservice={}".format(service_name),
    )

    isvc_annotations = pods.items[0].metadata.annotations

    assert isvc_annotations[METRIC] == "cpu"
    assert isvc_annotations[TARGET] == "50"
    res = await predict_isvc(rest_v1_client, service_name, INPUT)
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_scale_raw(rest_v1_client):
    service_name = "isvc-sklearn-scale-raw"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        scale_metric="cpu",
        scale_target=50,
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE, annotations=annotations
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    api_instance = kserve_client.api_instance
    hpa_resp = api_instance.list_namespaced_custom_object(
        group="autoscaling",
        version="v1",
        namespace=KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice=" f"{service_name}",
        plural="horizontalpodautoscalers",
    )

    assert hpa_resp["items"][0]["spec"]["targetCPUUtilizationPercentage"] == 50
    res = await predict_isvc(rest_v1_client, service_name, INPUT)
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_rolling_update():
    service_name = "isvc-sklearn-rolling-update"
    min_replicas = 4
    predictor = V1beta1PredictorSpec(
        min_replicas=min_replicas,
        scale_metric="cpu",
        scale_target=50,
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = dict()
    annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels={"serving.kserve.io/test": "rolling-update"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    updated_annotations = dict()
    updated_annotations["serving.kserve.io/deploymentMode"] = "RawDeployment"
    updated_annotations["serving.kserve.io/customAnnotation"] = "TestAnnotation"

    updated_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=updated_annotations,
            labels={"serving.kserve.io/test": "rolling-update"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    kserve_client.patch(service_name, updated_isvc)
    deployment = kserve_client.app_api.list_namespaced_deployment(
        namespace=KSERVE_TEST_NAMESPACE,
        label_selector="serving.kserve.io/test=rolling-update",
    )
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # Check if the deployment replicas still remain the same as min_replicas
    assert deployment.items[0].spec.replicas == min_replicas
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
