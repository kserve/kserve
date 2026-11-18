# Copyright 2026 The KServe Authors.
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

"""E2E tests for path-based routing (pathTemplate) with Gateway API in Standard mode.

These tests run as part of the ``raw`` suite but only do work when the cluster is
configured with ``enableGatewayApi: true`` and a ``pathTemplate`` (see
config/overlays/test/configmap/inferenceservice-path-template.yaml); otherwise they skip.
"""

import json
import os
import time
import uuid

import httpx
import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    constants,
    KServeClient,
    V1beta1ARTExplainerSpec,
    V1beta1ExplainerSpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
)
from kserve.logging import trace_logger as logger

from ..common.utils import (
    KSERVE_TEST_NAMESPACE,
    get_ingress_config,
    get_isvc_endpoint,
    get_path_based_endpoint,
    render_path_template,
)

# Time to wait for the gateway to program a newly created route.
ROUTE_READY_TIMEOUT_SECONDS = 180
POLL_INTERVAL_SECONDS = 3


@pytest.fixture
def path_ingress_config(network_layer):
    """Skip unless path-based routing with Gateway API is enabled."""
    if network_layer not in ("envoy-gatewayapi", "istio-gatewayapi", "gateway-api"):
        pytest.skip("path-based routing tests require a Gateway API network layer")
    ingress_config = get_ingress_config()
    if not ingress_config.get("enableGatewayApi") or not ingress_config.get(
        "pathTemplate"
    ):
        pytest.skip("pathTemplate with enableGatewayApi is not configured")
    return ingress_config


def _kserve_client() -> KServeClient:
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def _sklearn_isvc(service_name: str) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                sklearn=V1beta1SKLearnSpec(
                    storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "50m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
            ),
        ),
    )


def _request_until(
    method: str,
    url: str,
    host: str,
    expected_status: int,
    json_body=None,
    timeout_seconds: int = ROUTE_READY_TIMEOUT_SECONDS,
) -> httpx.Response:
    """Send a request until it returns expected_status, to allow the gateway to program routes."""
    deadline = time.monotonic() + timeout_seconds
    response = None
    with httpx.Client(timeout=60) as http:
        while True:
            try:
                response = http.request(
                    method, url, headers={"Host": host}, json=json_body
                )
                logger.info(
                    "%s %s (Host: %s) -> %s %s",
                    method,
                    url,
                    host,
                    response.status_code,
                    response.text[:200],
                )
                if response.status_code == expected_status:
                    return response
            except httpx.HTTPError as e:
                logger.info("%s %s (Host: %s) failed: %s", method, url, host, e)
            if time.monotonic() > deadline:
                status = response.status_code if response is not None else "no response"
                raise AssertionError(
                    f"{method} {url} (Host: {host}) did not return {expected_status} "
                    f"within {timeout_seconds}s, last status: {status}"
                )
            time.sleep(POLL_INTERVAL_SECONDS)


def _request_once(method: str, url: str, host: str) -> httpx.Response:
    with httpx.Client(timeout=60) as http:
        response = http.request(method, url, headers={"Host": host})
    logger.info(
        "%s %s (Host: %s) -> %s server=%s",
        method,
        url,
        host,
        response.status_code,
        response.headers.get("server"),
    )
    return response


def _assert_gateway_not_found(response: httpx.Response):
    """The gateway itself must answer 404: the request must not reach a model server."""
    assert response.status_code == 404
    assert (
        response.headers.get("server", "").lower() != "uvicorn"
    ), "request was forwarded to a model server instead of being rejected by the gateway"


@pytest.mark.raw
@pytest.mark.path_based_routing
def test_raw_path_based_routing_gateway_api(path_ingress_config, network_layer):
    suffix = str(uuid.uuid4())[1:6]
    names = [f"raw-path-a-{suffix}", f"raw-path-b-{suffix}"]
    kserve_client = _kserve_client()
    with open("./data/iris_input.json") as f:
        iris_input = json.load(f)

    for name in names:
        kserve_client.create(_sklearn_isvc(name))
    try:
        for name in names:
            kserve_client.wait_isvc_ready(name, namespace=KSERVE_TEST_NAMESPACE)

        shared_host = path_ingress_config["ingressDomain"]
        for name in names:
            isvc = kserve_client.get(name, namespace=KSERVE_TEST_NAMESPACE)
            base_url, host = get_path_based_endpoint(
                isvc, network_layer, path_ingress_config
            )
            assert host == shared_host

            # The path prefix is stripped before reaching the model server, which only
            # serves /v1/..., and each path reaches its own InferenceService.
            response = _request_until("GET", f"{base_url}/v1/models", host, 200)
            assert response.json()["models"] == [name]

            response = _request_until(
                "POST",
                f"{base_url}/v1/models/{name}:predict",
                host,
                200,
                json_body=iris_input,
            )
            assert response.json()["predictions"] == [1, 1]

            # Host-based routing on the InferenceService's own hostname keeps working.
            scheme, cluster_ip, isvc_host, _ = get_isvc_endpoint(isvc, network_layer)
            response = _request_until(
                "GET", f"{scheme}://{cluster_ip}/v1/models", isvc_host, 200
            )
            assert response.json()["models"] == [name]

        # On the shared ingress domain, paths that match no InferenceService must not be
        # routed to an arbitrary model.
        isvc = kserve_client.get(names[0], namespace=KSERVE_TEST_NAMESPACE)
        scheme, cluster_ip, _, _ = get_isvc_endpoint(isvc, network_layer)
        gateway_url = f"{scheme}://{cluster_ip}"
        unknown_path = render_path_template(
            path_ingress_config, f"does-not-exist-{suffix}", KSERVE_TEST_NAMESPACE
        )
        _assert_gateway_not_found(
            _request_once("GET", f"{gateway_url}{unknown_path}/v1/models", shared_host)
        )
        _assert_gateway_not_found(
            _request_once("GET", f"{gateway_url}/v1/models", shared_host)
        )
    finally:
        for name in names:
            kserve_client.delete(name, KSERVE_TEST_NAMESPACE)


@pytest.mark.raw
@pytest.mark.path_based_routing
def test_raw_path_based_routing_explainer_gateway_api(
    path_ingress_config, network_layer
):
    suffix = str(uuid.uuid4())[1:6]
    service_name = "raw-path-art-" + suffix
    kserve_client = _kserve_client()
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                sklearn=V1beta1SKLearnSpec(
                    storage_uri="gs://kfserving-examples/models/sklearn/mnist/art",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
                timeout=300,
            ),
            explainer=V1beta1ExplainerSpec(
                min_replicas=1,
                art=V1beta1ARTExplainerSpec(
                    type="SquareAttack",
                    name="explainer",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "10m", "memory": "256Mi"},
                        limits={"cpu": "100m", "memory": "512Mi"},
                    ),
                    config={"nb_classes": "10"},
                ),
                timeout=300,
            ),
        ),
    )
    with open("./data/mnist_input_bw_flat.json") as f:
        predict_input = json.load(f)
    with open("./data/mnist_input_bw.json") as f:
        explain_input = json.load(f)

    kserve_client.create(isvc)
    try:
        kserve_client.wait_isvc_ready(
            service_name, namespace=KSERVE_TEST_NAMESPACE, timeout_seconds=720
        )
        isvc = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        base_url, host = get_path_based_endpoint(
            isvc, network_layer, path_ingress_config
        )

        response = _request_until(
            "POST",
            f"{base_url}/v1/models/{service_name}:predict",
            host,
            200,
            json_body=predict_input,
        )
        assert response.json()["predictions"] == [3]

        # The explain path is rewritten to /v1/models/<model>:explain for the explainer.
        response = _request_until(
            "POST",
            f"{base_url}/v1/models/{service_name}:explain",
            host,
            200,
            json_body=explain_input,
            timeout_seconds=600,
        )
        assert response.json()["explanations"]["adversarial_prediction"] != 3
    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
