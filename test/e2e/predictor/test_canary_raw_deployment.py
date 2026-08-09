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

import logging
import os
import time

import pytest
from kserve import (
    KServeClient,
    V1beta1CanarySpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)
from kubernetes import client
from kubernetes.client import V1ResourceRequirements
from kubernetes.client.rest import ApiException

from ..common.http_retry import post_with_retry
from ..common.utils import KSERVE_TEST_NAMESPACE, get_isvc_endpoint

logger = logging.getLogger(__name__)

STABLE_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.0/model"
CANARY_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.3/mixedtype"

RESOURCES = V1ResourceRequirements(
    requests={"cpu": "50m", "memory": "128Mi"},
    limits={"cpu": "100m", "memory": "256Mi"},
)


def _kserve_client():
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def _apps_v1():
    return client.AppsV1Api()


def _make_predictor(storage_uri, name=None, min_replicas=2):
    spec = V1beta1PredictorSpec(
        min_replicas=min_replicas,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=storage_uri,
            resources=RESOURCES,
        ),
    )
    if name:
        spec.name = name
    return spec


def _make_isvc(service_name, canaries=None):
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={
                "serving.kserve.io/deploymentMode": "Standard",
                "serving.kserve.io/autoscalerClass": "none",
            },
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=_make_predictor(STABLE_MODEL_URI),
            canary=canaries,
        ),
    )


def _safe_delete(kserve, service_name):
    try:
        kserve.delete(service_name, KSERVE_TEST_NAMESPACE)
    except Exception:
        logger.exception("Failed to delete %s during cleanup", service_name)


def _wait_for_deployment(apps_v1, name, namespace, timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            dep = apps_v1.read_namespaced_deployment(name, namespace)
            if dep.status.available_replicas and dep.status.available_replicas > 0:
                return dep
        except ApiException as e:
            if e.status != 404:
                raise
        time.sleep(5)
    raise TimeoutError(f"Deployment {name} not ready within {timeout}s")


def _wait_for_deployment_gone(apps_v1, name, namespace, timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            apps_v1.read_namespaced_deployment(name, namespace)
        except ApiException as e:
            if e.status == 404:
                return
            raise
        time.sleep(5)
    raise TimeoutError(f"Deployment {name} still exists after {timeout}s")


def _wait_for_condition(
    kserve, service_name, condition_type, expected_reason, timeout=60
):
    deadline = time.time() + timeout
    while time.time() < deadline:
        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        conditions = got.get("status", {}).get("conditions", [])
        cond = next((c for c in conditions if c["type"] == condition_type), None)
        if cond is not None and cond.get("reason") == expected_reason:
            return cond
        time.sleep(5)
    raise TimeoutError(
        f"Condition {condition_type} reason={expected_reason} not seen within {timeout}s"
    )


def _get_httproute(name, namespace):
    api = client.CustomObjectsApi()
    return api.get_namespaced_custom_object(
        "gateway.networking.k8s.io", "v1", namespace, "httproutes", name
    )


def _get_httproute_backend_refs(name, namespace):
    """Return a list of (service_name, weight) tuples from the first rule's backendRefs."""
    route = _get_httproute(name, namespace)
    rules = route.get("spec", {}).get("rules", [])
    if not rules:
        return []
    return [(ref["name"], ref.get("weight")) for ref in rules[0].get("backendRefs", [])]


def _is_gatewayapi(network_layer):
    return network_layer is not None and "gatewayapi" in network_layer


def _get_pod_uids(apps_v1, deployment_name, namespace):
    core_v1 = client.CoreV1Api()
    dep = apps_v1.read_namespaced_deployment(deployment_name, namespace)
    selector = dep.spec.selector.match_labels
    label_selector = ",".join(f"{k}={v}" for k, v in selector.items())
    pods = core_v1.list_namespaced_pod(namespace, label_selector=label_selector)
    return {pod.metadata.uid for pod in pods.items}


def _verify_traffic_split(
    kserve,
    service_name,
    namespace,
    expected_pcts,
    network_layer,
    num_requests=100,
    tolerance=10,
):
    """
    Verify traffic split by sending requests and counting responses by status code.

    In this test setup:
    - Stable predictor uses STABLE_MODEL_URI which returns 200 (successful)
    - Canary predictor uses CANARY_MODEL_URI which returns 500 (bad model path)

    Args:
        kserve: KServeClient instance
        service_name: Name of the InferenceService
        namespace: Kubernetes namespace
        expected_pcts: Dict mapping status code to expected percentage, e.g., {200: 80, 500: 20}
        network_layer: Network layer type (e.g., "istio-gatewayapi")
        num_requests: Total number of requests to send
        tolerance: Allowed deviation in percentage points

    Returns:
        dict: {status_code: (count, percentage)}
    """
    isvc = kserve.get(service_name, namespace=namespace)
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, network_layer)

    url = f"{scheme}://{cluster_ip}{path}/v1/models/{service_name}:predict"
    headers = {"Host": host, "Content-Type": "application/json"}

    # Simple prediction input for sklearn model
    input_data = {"instances": [[1, 2, 3, 4]]}

    status_counts = {}

    logger.info(
        f"Sending {num_requests} requests to verify traffic split: {expected_pcts}"
    )

    for i in range(num_requests):
        try:
            # Don't retry on 500 since we expect it from canary (bad model path)
            # Only retry on network errors and 502/503/504 gateway errors
            response = post_with_retry(
                url,
                headers=headers,
                json_data=input_data,
                retry_status_codes=(502, 503, 504),
            )
            status_counts[response.status_code] = (
                status_counts.get(response.status_code, 0) + 1
            )
        except Exception as e:
            logger.warning(f"Request {i + 1} failed: {e}")

    total = sum(status_counts.values())
    results = {}

    logger.info("Traffic split results:")
    for status_code, count in sorted(status_counts.items()):
        pct = (count / total * 100) if total > 0 else 0
        results[status_code] = (count, pct)
        logger.info(f"  {status_code}: {count}/{total} ({pct:.1f}%)")

    # Verify each expected status code is within tolerance
    for status_code, expected_pct in expected_pcts.items():
        assert status_code in results, (
            f"Expected status code {status_code} not seen in responses"
        )
        _, actual_pct = results[status_code]
        assert abs(actual_pct - expected_pct) <= tolerance, (
            f"Status {status_code} traffic {actual_pct:.1f}% outside expected {expected_pct}% ±{tolerance}%"
        )

    return results


@pytest.mark.predictor
@pytest.mark.raw
def test_canary_create(network_layer):
    service_name = "isvc-canary-create"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        stable_dep = _wait_for_deployment(
            apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
        )
        canary_dep = _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        assert stable_dep is not None
        assert canary_dep is not None

        canary_condition = _wait_for_condition(
            kserve, service_name, "CanaryPredictorReady", "AllCanariesReady"
        )
        assert canary_condition["status"] == "True"

        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        canary_status = got.get("status", {}).get("canaryStatuses", [])
        assert len(canary_status) > 0, "canary status should be populated"
        assert canary_status[0]["name"] == "v2"

        if _is_gatewayapi(network_layer):
            backends = _get_httproute_backend_refs(
                f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
            )
            assert len(backends) == 2, (
                f"Expected 2 backends (stable + canary), got {backends}"
            )
            names = {name for name, _ in backends}
            assert f"{service_name}-predictor" in names
            assert f"{service_name}-v2-predictor" in names
            weights = {name: weight for name, weight in backends}
            assert weights[f"{service_name}-predictor"] == 80
            assert weights[f"{service_name}-v2-predictor"] == 20

            # Verify actual traffic split by sending requests
            # Stable (STABLE_MODEL_URI) returns 200, canary (CANARY_MODEL_URI) returns 500
            _verify_traffic_split(
                kserve,
                service_name,
                KSERVE_TEST_NAMESPACE,
                expected_pcts={200: 80, 500: 20},
                network_layer=network_layer,
                num_requests=100,
                tolerance=10,
            )
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
@pytest.mark.raw
def test_canary_promote(network_layer):
    service_name = "isvc-canary-promote"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        canary_pod_uids = _get_pod_uids(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        assert len(canary_pod_uids) > 0

        promoted = _make_isvc(service_name)
        promoted.spec.predictor = _make_predictor(CANARY_MODEL_URI, name="v2")
        promoted.spec.canary = []

        patch_resp = kserve.patch(
            service_name, promoted, namespace=KSERVE_TEST_NAMESPACE
        )
        kserve.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            expected_generation=patch_resp["metadata"]["generation"],
        )

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        _wait_for_deployment_gone(
            apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
        )

        post_promote_uids = _get_pod_uids(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        assert canary_pod_uids.issubset(post_promote_uids), (
            f"Canary pods were restarted during promotion: canary={canary_pod_uids}, post_promote={post_promote_uids}"
        )

        if _is_gatewayapi(network_layer):
            backends = _get_httproute_backend_refs(
                f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
            )
            assert len(backends) == 1, (
                f"Expected 1 backend after promotion, got {backends}"
            )
            assert backends[0][0] == f"{service_name}-v2-predictor"

            # Verify 100% traffic goes to promoted canary (v2)
            # The promoted v2 uses CANARY_MODEL_URI which returns 500
            _verify_traffic_split(
                kserve,
                service_name,
                KSERVE_TEST_NAMESPACE,
                expected_pcts={500: 100},
                network_layer=network_layer,
                num_requests=50,
                tolerance=10,
            )
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
@pytest.mark.raw
def test_canary_rollback(network_layer):
    service_name = "isvc-canary-rollback"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        rolled_back = _make_isvc(service_name)
        rolled_back.spec.canary = []

        patch_resp = kserve.patch(
            service_name, rolled_back, namespace=KSERVE_TEST_NAMESPACE
        )
        kserve.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            expected_generation=patch_resp["metadata"]["generation"],
        )

        _wait_for_deployment_gone(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        _wait_for_deployment(apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE)

        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        conditions = got.get("status", {}).get("conditions", [])
        canary_condition = next(
            (c for c in conditions if c["type"] == "CanaryPredictorReady"), None
        )
        assert canary_condition is None, (
            "CanaryPredictorReady should be cleared after rollback"
        )

        if _is_gatewayapi(network_layer):
            backends = _get_httproute_backend_refs(
                f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
            )
            assert len(backends) == 1, (
                f"Expected 1 backend after rollback, got {backends}"
            )
            assert backends[0][0] == f"{service_name}-predictor"

            # Verify 100% traffic goes to stable after rollback
            # Stable uses STABLE_MODEL_URI which returns 200
            _verify_traffic_split(
                kserve,
                service_name,
                KSERVE_TEST_NAMESPACE,
                expected_pcts={200: 100},
                network_layer=network_layer,
                num_requests=50,
                tolerance=10,
            )
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
@pytest.mark.raw
def test_canary_force_stop():
    service_name = "isvc-canary-stop"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        stop_patch = V1beta1InferenceService(
            api_version=constants.KSERVE_V1BETA1,
            kind=constants.KSERVE_KIND_INFERENCESERVICE,
            metadata=client.V1ObjectMeta(
                name=service_name,
                namespace=KSERVE_TEST_NAMESPACE,
                annotations={
                    "serving.kserve.io/stop": "true",
                },
            ),
            spec=isvc.spec,
        )

        kserve.patch(service_name, stop_patch, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment_gone(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        _wait_for_condition(kserve, service_name, "CanaryPredictorReady", "Stopped")
    finally:
        _safe_delete(kserve, service_name)
