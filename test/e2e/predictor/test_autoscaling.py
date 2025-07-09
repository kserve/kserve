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
import uuid

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
    V1beta1AutoScalingSpec,
    V1beta1ResourceMetricSource,
    V1beta1MetricTarget,
    V1beta1ExtMetricAuthentication,
    V1beta1ExternalMetricSource,
    V1beta1ExternalMetrics,
    V1beta1MetricsSpec,
    V1beta1PodMetricSource,
    V1beta1PodMetrics,
    V1beta1AuthenticationRef,
)


from ..common.utils import KSERVE_TEST_NAMESPACE
from ..common.utils import predict_isvc
import time
import asyncio

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

    annotations = {"autoscaling.knative.dev/class": "hpa.autoscaling.knative.dev"}

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
async def test_sklearn_scale_raw(rest_v1_client, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "isvc-sklearn-scale-raw-" + suffix
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

    annotations = {"serving.kserve.io/deploymentMode": "Standard"}

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
    res = await predict_isvc(
        rest_v1_client, service_name, INPUT, network_layer=network_layer
    )
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_rolling_update():
    suffix = str(uuid.uuid4())[1:6]
    service_name = "isvc-sklearn-rolling-update-" + suffix
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

    annotations = {"serving.kserve.io/deploymentMode": "Standard"}

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

    updated_annotations = {
        "serving.kserve.io/deploymentMode": "Standard",
        "serving.kserve.io/customAnnotation": "TestAnnotation",
    }

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


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_env_update():
    suffix = str(uuid.uuid4())[1:6]
    service_name = "isvc-sklearn-rolling-update-" + suffix
    min_replicas = 4
    envs = [
        {
            "name": "TEST_ENV",
            "value": "TEST_ENV_VALUE",
        },
        {
            "name": "TEST_ENV_2",
            "value": "TEST_ENV_VALUE_2",
        },
        {
            "name": "TEST_ENV_3",
            "value": "TEST_ENV_VALUE_3",
        },
    ]
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
            env=envs,
        ),
    )

    annotations = {"serving.kserve.io/deploymentMode": "Standard"}

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels={"serving.kserve.io/test": "env-update"},
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    predictor.sklearn.env = [
        {
            "name": "TEST_ENV",
            "value": "TEST_ENV_VALUE",
        },
        {
            "name": "TEST_ENV_2",
            "value": "TEST_ENV_VALUE_2",
        },
    ]

    updated_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=annotations,
            labels={"serving.kserve.io/test": "env-update"},
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
        label_selector="serving.kserve.io/test=env-update",
    )
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # Check if the deployment replicas still remain the same as min_replicas
    assert deployment.items[0].spec.replicas == min_replicas
    # Check if the environment variables have been updated correctly
    container_envs = deployment.items[0].spec.template.spec.containers[0].env
    env_names = [env.name for env in container_envs]
    assert "TEST_ENV" in env_names
    assert "TEST_ENV_2" in env_names
    assert "TEST_ENV_3" not in env_names
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_keda_scale_resource_memory(rest_v1_client, network_layer):
    """
    Test KEDA autoscaling with new InferenceService (auto_scaling) spec
    """
    service_name = "isvc-sklearn-keda-scale-new-spec"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        max_replicas=5,
        auto_scaling=V1beta1AutoScalingSpec(
            metrics=[
                V1beta1MetricsSpec(
                    type="Resource",
                    resource=V1beta1ResourceMetricSource(
                        name="memory",
                        target=V1beta1MetricTarget(
                            type="Utilization", average_utilization=50
                        ),
                    ),
                )
            ]
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = {
        "serving.kserve.io/deploymentMode": "Standard",
        "serving.kserve.io/autoscalerClass": "keda",
    }

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

    scaledobject_resp = api_instance.list_namespaced_custom_object(
        group="keda.sh",
        version="v1alpha1",
        namespace=KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        plural="scaledobjects",
    )

    trigger_metadata = scaledobject_resp["items"][0]["spec"]["triggers"][0]["metadata"]
    assert (
        scaledobject_resp["items"][0]["spec"]["triggers"][0]["metricType"]
        == "Utilization"
    )
    assert scaledobject_resp["items"][0]["spec"]["triggers"][0]["type"] == "memory"
    res = await predict_isvc(
        rest_v1_client, service_name, INPUT, network_layer=network_layer
    )
    assert trigger_metadata["value"] == "50"
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_sklearn_keda_scale_new_spec_external(rest_v1_client, network_layer):
    """
    Test KEDA autoscaling with new InferenceService (auto_scaling) spec
    """
    service_name = "isvc-sklearn-keda-scale-new-spec-2"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        max_replicas=5,
        auto_scaling=V1beta1AutoScalingSpec(
            metrics=[
                V1beta1MetricsSpec(
                    type="External",
                    external=V1beta1ExternalMetricSource(
                        metric=V1beta1ExternalMetrics(
                            backend="prometheus",
                            server_address="http://prometheus:9090",
                            query="http_requests_per_second",
                        ),
                        target=V1beta1MetricTarget(type="Value", value=50),
                        authentication_ref=V1beta1ExtMetricAuthentication(
                            auth_modes="basic",
                            authentication_ref=V1beta1AuthenticationRef(
                                name="prometheus-auth",
                            ),
                        ),
                    ),
                )
            ]
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = {
        "serving.kserve.io/deploymentMode": "Standard",
        "serving.kserve.io/autoscalerClass": "keda",
    }

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

    scaledobject_resp = api_instance.list_namespaced_custom_object(
        group="keda.sh",
        version="v1alpha1",
        namespace=KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        plural="scaledobjects",
    )

    trigger_metadata = scaledobject_resp["items"][0]["spec"]["triggers"][0]["metadata"]
    authentication_ref = scaledobject_resp["items"][0]["spec"]["triggers"][0][
        "authenticationRef"
    ]
    trigger_type = scaledobject_resp["items"][0]["spec"]["triggers"][0]["type"]
    assert trigger_type == "prometheus"
    assert trigger_metadata["query"] == "http_requests_per_second"
    assert trigger_metadata["serverAddress"] == "http://prometheus:9090"
    assert trigger_metadata["threshold"] == "50.000000"
    assert trigger_metadata["authModes"] == "basic"
    assert authentication_ref["name"] == "prometheus-auth"
    res = await predict_isvc(
        rest_v1_client, service_name, INPUT, network_layer=network_layer
    )
    assert res["predictions"] == [1, 1]
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_scaling_sklearn_with_keda_otel_add_on(rest_v1_client, network_layer):
    """
    Test KEDA-Otel-Add-On autoscaling with InferenceService (auto_scaling) spec,
    including scale up and scale down behavior by generating load.
    """

    service_name = "isvc-sklearn-keda-otel-add-on"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        max_replicas=5,
        auto_scaling=V1beta1AutoScalingSpec(
            metrics=[
                V1beta1MetricsSpec(
                    type="PodMetric",
                    podmetric=V1beta1PodMetricSource(
                        metric=V1beta1PodMetrics(
                            backend="opentelemetry",
                            metric_names=["http_requests_total"],
                            query="http_requests_total",
                        ),
                        target=V1beta1MetricTarget(type="Value", value=50),
                    ),
                )
            ]
        ),
        sklearn=V1beta1SKLearnSpec(
            storage_uri=MODEL,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    annotations = {
        "serving.kserve.io/deploymentMode": "Standard",
        "serving.kserve.io/autoscalerClass": "keda",
        "sidecar.opentelemetry.io/inject": f"{service_name}-predictor",
    }

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

    # Check Otel Collector config
    otelp_collector_resp = api_instance.list_namespaced_custom_object(
        group="opentelemetry.io",
        version="v1beta1",
        namespace=KSERVE_TEST_NAMESPACE,
        plural="opentelemetrycollectors",
    )

    otel_receiver = otelp_collector_resp["items"][0]["spec"]["config"]["receivers"]
    otel_exporter = otelp_collector_resp["items"][0]["spec"]["config"]["exporters"]
    assert (
        otel_receiver["prometheus"]["config"]["scrape_configs"][0]["job_name"]
        == "otel-collector"
    )
    assert (
        otel_receiver["prometheus"]["config"]["scrape_configs"][0]["static_configs"][0][
            "targets"
        ][0]
        == "localhost:8080"
    )
    assert otel_exporter["otlp"]["endpoint"] == "keda-otel-scaler.keda.svc:4317"

    # Check KEDA ScaledObject
    scaledobject_resp = api_instance.list_namespaced_custom_object(
        group="keda.sh",
        version="v1alpha1",
        namespace=KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        plural="scaledobjects",
    )

    trigger_metadata = scaledobject_resp["items"][0]["spec"]["triggers"][0]["metadata"]
    trigger_type = scaledobject_resp["items"][0]["spec"]["triggers"][0]["type"]
    assert trigger_type == "external"
    assert trigger_metadata["metricQuery"] == 'sum(http_requests_per_second{namespace="kserve-ci-e2e-test", deployment="isvc-sklearn-keda-otel-add-on-predictor"})'
    assert trigger_metadata["scalerAddress"] == "keda-otel-scaler.keda.svc:4318"
    assert trigger_metadata["targetValue"] == "50.000000"

    # Generate load to trigger scale up
    async def send_load(num_requests, concurrency=5):
        sem = asyncio.Semaphore(concurrency)

        async def send_one():
            async with sem:
                res = await predict_isvc(
                    rest_v1_client, service_name, INPUT, network_layer=network_layer
                )
                assert res["predictions"] == [1, 1]

        await asyncio.gather(*(send_one() for _ in range(num_requests)))

    # Initial pod count should be min_replicas
    def get_pod_count():
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        running_pods = [p for p in pods.items if p.status.phase == "Running"]
        print(f"[DEBUG] Current running pod count: {len(running_pods)}")
        return len(running_pods)

    # Wait for pod count to reach expected value, with timeout
    def wait_for_pod_count(expected, timeout=180):
        start = time.time()
        while time.time() - start < timeout:
            count = get_pod_count()
            print(f"[DEBUG] Waiting for pod count >= {expected}, current: {count}")
            if count >= expected:
                print(f"[DEBUG] Pod count reached expected: {count}")
                return True
            time.sleep(5)
        print(f"[DEBUG] Timeout reached. Final pod count: {count}")
        return False

    # Check initial pod count
    initial_count = get_pod_count()
    print(f"[DEBUG] Initial pod count: {initial_count}")
    assert initial_count == 1

    # Send enough load to trigger scale up
    print("[DEBUG] Sending load to trigger scale up...")
    await send_load(100, concurrency=10)
    scaled_up = wait_for_pod_count(2, timeout=900)
    print(f"[DEBUG] Scaled up: {scaled_up}")
    assert scaled_up, "Failed to scale up pods"

    # Wait for scale down (after load stops)
    def wait_for_scale_down(expected=1, timeout=300):
        start = time.time()
        while time.time() - start < timeout:
            count = get_pod_count()
            if count <= expected:
                return True
            time.sleep(10)
        return False

    scaled_down = wait_for_scale_down(expected=1, timeout=500)
    assert scaled_down, "Failed to scale down pods"

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
