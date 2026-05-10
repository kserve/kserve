# Copyright 2025 The KServe Authors.
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

import asyncio
import os
import uuid

import pytest
import requests
from portforward import AsyncPortForwarder
from kubernetes import client
from kubernetes.client import (
    V1Container,
    V1ContainerPort,
    V1EnvVar,
    V1ResourceRequirements,
)

from kserve import (
    constants,
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1TransformerSpec,
    V1beta1TorchServeSpec,
)
from kserve.logging import logger

from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    INFERENCESERVICE_CONTAINER,
    TRANSFORMER_CONTAINER,
    STORAGE_URI_ENV,
    predict_isvc,
)

METRICS_PORT = 8080
METRICS_PATH = "metrics"

EXPECTED_METRICS = [
    "request_preprocess_seconds",
    "request_postprocess_seconds",
]

STANDARD_MODE_ANNOTATIONS = {"serving.kserve.io/deploymentMode": "Standard"}


async def scrape_metrics(
    kserve_client, service_name, pod_filter=None, port=METRICS_PORT
):
    """Port-forward to a pod and scrape its /metrics endpoint.

    Args:
        kserve_client: KServeClient instance.
        service_name: InferenceService name used to find pods.
        pod_filter: Optional substring to match in pod name (e.g. "transformer").
        port: Container port exposing /metrics.

    Returns:
        The response text from the /metrics endpoint.
    """
    await asyncio.sleep(5)
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
    )

    pod_name = None
    for pod in pods.items:
        name = pod.metadata.name
        if pod_filter and pod_filter not in name:
            continue
        pod_name = name
        break

    assert pod_name is not None, f"No pod found for service {service_name}" + (
        f" matching filter '{pod_filter}'" if pod_filter else ""
    )

    url = f"http://localhost:{port}/{METRICS_PATH}"
    port_forwarder = AsyncPortForwarder(KSERVE_TEST_NAMESPACE, pod_name, port, port)
    try:
        await port_forwarder.forward()
        logger.info("Scraping metrics from pod %s at %s", pod_name, url)
        response = requests.get(url)
    finally:
        await port_forwarder.stop()

    assert response.status_code == 200, (
        f"Metrics endpoint returned {response.status_code}"
    )
    return response.text


def assert_metrics_present(metrics_text, expected_metrics, model_name):
    """Assert that expected Prometheus metrics appear in the scraped output."""
    for metric in expected_metrics:
        assert metric in metrics_text, (
            f"Expected metric '{metric}' not found in metrics output"
        )
    assert model_name in metrics_text, (
        f"Expected model_name label '{model_name}' not found in metrics output"
    )


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_transformer_metrics_isolated(rest_v1_client, network_layer):
    """Test that an isolated transformer in Standard mode exposes Prometheus metrics."""
    suffix = str(uuid.uuid4())[:5]
    service_name = f"raw-trans-metrics-iso-{suffix}"
    model_name = "mnist"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        pytorch=V1beta1TorchServeSpec(
            storage_uri="gs://kfserving-examples/models/torchserve/image_classifier/v1",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
            ),
        ),
    )

    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[
            V1Container(
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                name="kserve-container",
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
                args=["--model_name", model_name],
                env=[
                    V1EnvVar(
                        name="STORAGE_URI",
                        value="gs://kfserving-examples/models/torchserve/image_classifier/v1",
                    )
                ],
            )
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=STANDARD_MODE_ANNOTATIONS,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor, transformer=transformer),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        res = await predict_isvc(
            rest_v1_client,
            service_name,
            "./data/transformer.json",
            model_name=model_name,
            network_layer=network_layer,
        )
        assert res["predictions"][0] == 2

        metrics_text = await scrape_metrics(
            kserve_client, service_name, pod_filter="transformer"
        )
        assert_metrics_present(metrics_text, EXPECTED_METRICS, model_name)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_transformer_metrics_collocated(rest_v1_client, network_layer):
    """Test that a collocated transformer in Standard mode exposes Prometheus metrics."""
    suffix = str(uuid.uuid4())[:5]
    service_name = f"raw-trans-metrics-col-{suffix}"
    model_name = "mnist"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name=INFERENCESERVICE_CONTAINER,
                image="pytorch/torchserve:0.9.0-cpu",
                env=[
                    V1EnvVar(
                        name=STORAGE_URI_ENV,
                        value="gs://kfserving-examples/models/torchserve/image_classifier/v1",
                    ),
                    V1EnvVar(name="TS_SERVICE_ENVELOPE", value="kserve"),
                ],
                args=[
                    "torchserve",
                    "--start",
                    "--model-store=/mnt/models/model-store",
                    "--ts-config=/mnt/models/config/config.properties",
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "1", "memory": "1Gi"},
                ),
            ),
            V1Container(
                name=TRANSFORMER_CONTAINER,
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                args=[
                    f"--model_name={model_name}",
                    "--http_port=8080",
                    "--grpc_port=8081",
                    "--predictor_host=localhost:8085",
                    "--enable_predictor_health_check",
                ],
                ports=[
                    V1ContainerPort(name="http", container_port=8080, protocol="TCP"),
                    V1ContainerPort(name="grpc", container_port=8081, protocol="TCP"),
                ],
                resources=V1ResourceRequirements(
                    requests={"cpu": "10m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "1Gi"},
                ),
            ),
        ],
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=STANDARD_MODE_ANNOTATIONS,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        res = await predict_isvc(
            rest_v1_client,
            service_name,
            "./data/transformer.json",
            model_name=model_name,
            network_layer=network_layer,
        )
        assert res["predictions"][0] == 2

        metrics_text = await scrape_metrics(
            kserve_client, service_name, port=METRICS_PORT
        )
        assert_metrics_present(metrics_text, EXPECTED_METRICS, model_name)
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
