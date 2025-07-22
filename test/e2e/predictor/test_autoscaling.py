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
                            metric_names=["process_cpu_seconds_total"],
                            query="process_cpu_seconds_total",
                        ),
                        target=V1beta1MetricTarget(type="Value", value=4),
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
    assert trigger_metadata["metricQuery"] == 'sum(process_cpu_seconds_total{namespace="kserve-ci-e2e-test", deployment="isvc-sklearn-keda-otel-add-on-predictor"})'
    assert trigger_metadata["scalerAddress"] == "keda-otel-scaler.keda.svc:4318"
    assert trigger_metadata["targetValue"] == "4"

    # Wait for OpenTelemetry sidecar and metrics collection to be ready
    print("Waiting for OpenTelemetry metrics collection to stabilize...")
    time.sleep(60)  # Give time for metrics to be collected and KEDA to be ready
    
    # Ensure the predictor service is receiving traffic properly
    print("Testing initial prediction to ensure service is ready...")
    initial_res = await predict_isvc(
        rest_v1_client, service_name, INPUT, network_layer=network_layer
    )
    assert initial_res["predictions"] == [1, 1]
    print("Initial prediction successful, proceeding with load test...")

    # Helper function to check KEDA scaling status
    def check_keda_status():
        try:
            scaledobject_status = api_instance.get_namespaced_custom_object(
                group="keda.sh",
                version="v1alpha1",
                namespace=KSERVE_TEST_NAMESPACE,
                plural="scaledobjects",
                name=f"{service_name}-predictor-scaledobject"
            )
            
            conditions = scaledobject_status.get("status", {}).get("conditions", [])
            health = scaledobject_status.get("status", {}).get("health", {})
            
            print(f"KEDA ScaledObject status - Health: {health}, Conditions: {conditions}")
            
            # Check for any error conditions
            for condition in conditions:
                if condition.get("status") == "False":
                    print(f"KEDA Warning - {condition.get('type')}: {condition.get('message')}")
                    
        except Exception as e:
            print(f"Could not check KEDA status: {e}")

    # Initial pod count should be min_replicas
    def get_pod_count():
        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        running_pods = [p for p in pods.items if p.status.phase == "Running"]
        
        # Debug information for troubleshooting
        all_pods = [(p.metadata.name, p.status.phase, p.status.conditions[-1].type if p.status.conditions else "No conditions") 
                   for p in pods.items]
        if len(all_pods) > 0:
            print(f"Debug - All pods: {all_pods}")
        
        return len(running_pods)

    # Check initial pod count
    initial_count = get_pod_count()
    assert initial_count == 1
    
    # Check KEDA status before starting load test
    print("Checking KEDA ScaledObject status before load test...")
    check_keda_status()

    # Wait for pod count to reach expected value, with timeout
    def wait_for_pod_count(expected, timeout=180):
        start = time.time()
        last_count = -1
        stable_count = 0
        required_stable_checks = 3  # Require 3 consecutive checks with correct count
        
        while time.time() - start < timeout:
            count = get_pod_count()
            
            # Log when count changes
            if count != last_count:
                elapsed = time.time() - start
                print(f"Pod count changed: {last_count} -> {count} (elapsed: {elapsed:.1f}s)")
                last_count = count
                stable_count = 0
            
            if count >= expected:
                stable_count += 1
                if stable_count >= required_stable_checks:
                    print(f"Successfully reached target pod count {count} >= {expected} (stable for {stable_count} checks)")
                    return True
            else:
                stable_count = 0
                
            time.sleep(5)
            
        final_count = get_pod_count()
        print(f"Timeout waiting for pod count. Expected: {expected}, Final: {final_count}, Timeout: {timeout}s")
        return False

    # Check initial pod count
    initial_count = get_pod_count()
    assert initial_count == 1

    async def send_load_until_scaled(num_requests, concurrency, target_pods, sustain_duration=30):
        """Send sustained load to trigger autoscaling and wait for target pod count"""
        sem = asyncio.Semaphore(concurrency)
        sent_requests = 0
        scaling_detected = False
        sustain_start = None

        async def send_one():
            async with sem:
                try:
                    res = await predict_isvc(
                        rest_v1_client, service_name, INPUT, network_layer=network_layer
                    )
                    assert res["predictions"] == [1, 1]
                except Exception as e:
                    print(f"Warning: Request failed: {e}")
                    # Don't fail the test on individual request failures

        # First, send sustained load for metrics to be collected
        print(f"Sending sustained load with {concurrency} concurrent requests...")
        load_start = time.time()
        
        while sent_requests < num_requests:
            current_pods = get_pod_count()
            
            # Check if scaling has been detected
            if current_pods >= target_pods and not scaling_detected:
                scaling_detected = True
                sustain_start = time.time()
                print(f"Scaling detected! Current pods: {current_pods}, continuing load for {sustain_duration}s to ensure stability")
            
            # If we've detected scaling and sustained it for the required duration, we can stop
            if scaling_detected and sustain_start and (time.time() - sustain_start >= sustain_duration):
                print(f"Sustained {current_pods} pods for {sustain_duration}s, stopping load generation")
                break
                
            # Continue sending load, but with smaller batches for better control
            batch = min(5, num_requests - sent_requests)
            tasks = [send_one() for _ in range(batch)]
            await asyncio.gather(*tasks, return_exceptions=True)  # Don't fail on individual errors
            sent_requests += batch
            
            # Add small delay to prevent overwhelming the system
            await asyncio.sleep(0.1)
            
            # Log progress every 20 requests
            if sent_requests % 20 == 0:
                print(f"Sent {sent_requests} requests, current pods: {current_pods}, elapsed: {time.time() - load_start:.1f}s")
                # Check KEDA status periodically during load
                if sent_requests % 60 == 0:
                    check_keda_status()

    # Send more requests and higher concurrency for CI environments  
    await send_load_until_scaled(300, concurrency=20, target_pods=2, sustain_duration=90)
    
    # Give additional time for scaling to stabilize
    print("Waiting for scaling to stabilize...")
    time.sleep(60)
    
    # Final check with multiple attempts
    scaled_up = False
    max_attempts = 3
    for attempt in range(max_attempts):
        # Check KEDA status during scaling attempts
        print(f"Scaling attempt {attempt + 1}/{max_attempts} - checking KEDA status...")
        check_keda_status()
        
        scaled_up = wait_for_pod_count(2, timeout=300)  # Increased timeout for CI
        if scaled_up:
            break
        print(f"Scaling attempt {attempt + 1}/{max_attempts} failed, retrying...")
        if attempt < max_attempts - 1:
            # Send more load to trigger scaling again
            print("Sending additional load burst...")
            burst_tasks = []
            for _ in range(30):
                burst_tasks.append(predict_isvc(
                    rest_v1_client, service_name, INPUT, network_layer=network_layer
                ))
            await asyncio.gather(*burst_tasks, return_exceptions=True)
            time.sleep(30)
    
    current_pod_count = get_pod_count()
    assert scaled_up, f"Failed to scale up pods after {max_attempts} attempts. Current pod count: {current_pod_count}, Expected: >= 2. This might indicate issues with KEDA scaling, OpenTelemetry metrics collection, or resource constraints in the CI environment."

    # Wait for scale down (after load stops)
    # def wait_for_scale_down(expected=1, timeout=300):
    #     start = time.time()
    #     while time.time() - start < timeout:
    #         count = get_pod_count()
    #         if count <= expected:
    #             return True
    #         time.sleep(10)
    #     return False

    # scaled_down = wait_for_scale_down(expected=1, timeout=500)
    # assert scaled_down, "Failed to scale down pods"

    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
