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

"""
E2E tests for LLMISVC autoscaling.

These tests verify behavioral outcomes of the autoscaling pipeline:
- Scaling resources (VariantAutoscaling, HPA, KEDA ScaledObject) exist by name
- Pods actually scale up under load
- Lifecycle operations (cleanup, stop, update) work end-to-end

HPA and KEDA tests are isolated into separate CI jobs because their infra
requirements conflict: Prometheus Adapter and KEDA both register the
v1beta1.external.metrics.k8s.io APIService. Tests are tagged with
autoscaling_hpa / autoscaling_keda markers for filtering.

Actuator-switch tests (HPA<->KEDA) are covered by integration tests in
pkg/controller/v1alpha2/llmisvc/scaling_int_test.go and are not repeated here.

Scale-down is not asserted because it depends on WVA saturation metrics
(KV cache, queue depth) from the inference server; the llm-d-inference-sim
simulator does not emit the decreasing metrics WVA needs to lower
desired_replicas after load stops.

Spec-field assertions are intentionally omitted -- those are covered by
integration tests in pkg/controller/v1alpha2/llmisvc/scaling_int_test.go.
"""

import concurrent.futures
import logging
import os
import time

import pytest
import requests
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import (
    KSERVE_TEST_NAMESPACE,
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    delete_llmisvc,
    get_llm_service_url,
    wait_for,
    wait_for_llm_isvc_ready,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
STOP_ANNOTATION_KEY = "serving.kserve.io/stop"

logger = logging.getLogger(__name__)

# --- Resource existence helpers ---

VA_GROUP = "llmd.ai"
VA_VERSION = "v1alpha1"
VA_PLURAL = "variantautoscalings"

KEDA_GROUP = "keda.sh"
KEDA_VERSION = "v1alpha1"
KEDA_PLURAL = "scaledobjects"

HPA_GROUP = "autoscaling"
HPA_VERSION = "v2"
HPA_PLURAL = "horizontalpodautoscalers"


def _get_custom_resource(group, version, plural, name, namespace):
    """Fetch a custom resource, return the object or None if 404."""
    api = client.CustomObjectsApi()
    try:
        return api.get_namespaced_custom_object(group, version, namespace, plural, name)
    except client.rest.ApiException as e:
        if e.status == 404:
            return None
        raise


def resource_exists(group, version, plural, name, namespace):
    return _get_custom_resource(group, version, plural, name, namespace) is not None


def wait_for_resource(group, version, plural, name, namespace, timeout=120):
    """Poll until the resource exists."""

    def _check():
        assert resource_exists(group, version, plural, name, namespace), (
            f"{plural}/{name} not found in {namespace}"
        )

    wait_for(_check, timeout=timeout, interval=2.0)


def wait_for_resource_deleted(group, version, plural, name, namespace, timeout=120):
    """Poll until the resource returns 404."""

    def _check():
        assert not resource_exists(group, version, plural, name, namespace), (
            f"{plural}/{name} still exists in {namespace}"
        )

    wait_for(_check, timeout=timeout, interval=2.0)


# --- Scaling resource name helpers (mirrors controller naming) ---


def va_name(service_name, prefill=False):
    suffix = "-kserve-prefill-va" if prefill else "-kserve-va"
    return _child_name(service_name, suffix)


def hpa_name(service_name, prefill=False):
    suffix = "-kserve-prefill-hpa" if prefill else "-kserve-hpa"
    return _child_name(service_name, suffix)


def scaled_object_name(service_name, prefill=False):
    suffix = "-kserve-prefill-keda" if prefill else "-kserve-keda"
    return _child_name(service_name, suffix)


def _child_name(parent, suffix):
    """Replicate knative.dev/pkg/kmeta.ChildName truncation to 63 chars."""
    result = parent + suffix
    if len(result) > 63:
        result = result[:63]
    return result


# --- Pod count helpers ---


def get_pod_count(service_name, namespace=KSERVE_TEST_NAMESPACE):
    """Count Running or Pending pods for this LLMISVC workload."""
    v1 = client.CoreV1Api()
    pods = v1.list_namespaced_pod(
        namespace=namespace,
        label_selector=f"app.kubernetes.io/name={service_name}",
    )
    count = 0
    for pod in pods.items:
        if pod.status.phase in ("Running", "Pending"):
            count += 1
    return count


def wait_for_pod_count(
    service_name, min_count, namespace=KSERVE_TEST_NAMESPACE, timeout=300
):
    """Poll until the pod count reaches at least min_count."""

    def _check():
        current = get_pod_count(service_name, namespace)
        assert current >= min_count, (
            f"Pod count for {service_name}: {current}, expected >= {min_count}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


def wait_for_pod_count_exact(
    service_name, count, namespace=KSERVE_TEST_NAMESPACE, timeout=300
):
    """Poll until the pod count is exactly count."""

    def _check():
        current = get_pod_count(service_name, namespace)
        assert current == count, (
            f"Pod count for {service_name}: {current}, expected {count}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


# --- Load generation ---


def send_load(service_url, model_name, concurrency=5, duration_seconds=30):
    """Send concurrent requests to the service to trigger scale-up."""
    endpoint = service_url + "/v1/completions"
    payload = {
        "model": model_name,
        "prompt": "Explain autoscaling in Kubernetes in great detail. " * 10,
        "max_tokens": 200,
    }
    headers = {"Content-Type": "application/json"}

    deadline = time.time() + duration_seconds

    def _worker():
        while time.time() < deadline:
            try:
                requests.post(endpoint, json=payload, headers=headers, timeout=30)
            except Exception:
                pass
            time.sleep(0.1)

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = [pool.submit(_worker) for _ in range(concurrency)]
        concurrent.futures.wait(futures)


# --- Patching helpers ---


def patch_llmisvc(kserve_client, llm_isvc, patch_body):
    """Apply a JSON merge patch to the LLMInferenceService."""
    result = kserve_client.api_instance.patch_namespaced_custom_object(
        constants.KSERVE_GROUP,
        llm_isvc.api_version.split("/")[1],
        llm_isvc.metadata.namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        llm_isvc.metadata.name,
        patch_body,
    )
    logger.info(f"Patched LLMISVC {llm_isvc.metadata.name}")
    return result


# --- Assertion helpers for scaling resources ---


def assert_scaling_resources_exist(service_name, actuator="hpa", prefill=False):
    """Assert that VA + actuator exist by name."""
    ns = KSERVE_TEST_NAMESPACE
    wait_for_resource(
        VA_GROUP, VA_VERSION, VA_PLURAL, va_name(service_name, prefill), ns
    )
    if actuator == "hpa":
        wait_for_resource(
            HPA_GROUP, HPA_VERSION, HPA_PLURAL, hpa_name(service_name, prefill), ns
        )
    elif actuator == "keda":
        wait_for_resource(
            KEDA_GROUP,
            KEDA_VERSION,
            KEDA_PLURAL,
            scaled_object_name(service_name, prefill),
            ns,
        )


def assert_scaling_resources_deleted(
    service_name, actuator="hpa", prefill=False, timeout=120
):
    """Assert that VA + actuator are gone (404)."""
    ns = KSERVE_TEST_NAMESPACE
    wait_for_resource_deleted(
        VA_GROUP, VA_VERSION, VA_PLURAL, va_name(service_name, prefill), ns, timeout
    )
    if actuator == "hpa":
        wait_for_resource_deleted(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name, prefill),
            ns,
            timeout,
        )
    elif actuator == "keda":
        wait_for_resource_deleted(
            KEDA_GROUP,
            KEDA_VERSION,
            KEDA_PLURAL,
            scaled_object_name(service_name, prefill),
            ns,
            timeout,
        )


# --- Common test lifecycle ---


def _create_and_wait(kserve_client, test_case):
    """Create LLMISVC and wait for it to be ready."""
    create_llmisvc(kserve_client, test_case.llm_service)
    wait_for_llm_isvc_ready(
        kserve_client, test_case.llm_service, test_case.wait_timeout
    )


def _cleanup(kserve_client, test_case):
    """Delete LLMISVC unless SKIP_RESOURCE_DELETION is set."""
    if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in ("false", "0", "f"):
        try:
            delete_llmisvc(kserve_client, test_case.llm_service)
        except Exception as e:
            logger.warning(f"Failed to cleanup service: {e}")


def _new_kserve_client():
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


# =============================================================================
# Test 1: HPA + Deployment
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-hpa-deploy",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_hpa_deployment(test_case: TestCase):
    """HPA + Deployment: VA and HPA exist; pods scale up under load."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="hpa")
        assert not resource_exists(
            KEDA_GROUP,
            KEDA_VERSION,
            KEDA_PLURAL,
            scaled_object_name(service_name),
            KSERVE_TEST_NAMESPACE,
        )

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url, test_case.model_name, concurrency=10, duration_seconds=60
        )
        wait_for_pod_count(service_name, min_count=2, timeout=300)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 2: KEDA + Deployment
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-keda-deploy",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_keda_deployment(test_case: TestCase):
    """KEDA + Deployment: VA and ScaledObject exist; no HPA; pods scale up under load."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="keda")
        assert not resource_exists(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name),
            KSERVE_TEST_NAMESPACE,
        )

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url, test_case.model_name, concurrency=10, duration_seconds=60
        )
        wait_for_pod_count(service_name, min_count=2, timeout=300)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 3: HPA + LWS (multi-node)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-lws",
                    "scaling-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-hpa-lws",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_multi_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_hpa_lws(test_case: TestCase):
    """HPA + LWS: VA and HPA exist; pods scale under load."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="hpa")

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url, test_case.model_name, concurrency=10, duration_seconds=60
        )
        wait_for_pod_count(service_name, min_count=2, timeout=300)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 4: KEDA + LWS (multi-node)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-lws",
                    "scaling-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-keda-lws",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_multi_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_keda_lws(test_case: TestCase):
    """KEDA + LWS: VA and ScaledObject exist; pods scale under load."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="keda")

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url, test_case.model_name, concurrency=10, duration_seconds=60
        )
        wait_for_pod_count(service_name, min_count=2, timeout=300)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 5: Prefill + HPA (P/D disaggregated)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-pd",
                    "scaling-hpa",
                    "scaling-prefill-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-prefill-hpa",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_prefill_hpa(test_case: TestCase):
    """P/D + HPA: separate VA and HPA for both decode and prefill workloads."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="hpa", prefill=False)
        assert_scaling_resources_exist(service_name, actuator="hpa", prefill=True)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 6: Prefill + KEDA (P/D disaggregated)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-pd",
                    "scaling-keda",
                    "scaling-prefill-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-prefill-keda",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_prefill_keda(test_case: TestCase):
    """P/D + KEDA: separate VA and ScaledObject for both decode and prefill workloads."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)

        assert_scaling_resources_exist(service_name, actuator="keda", prefill=False)
        assert_scaling_resources_exist(service_name, actuator="keda", prefill=True)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 7: Cleanup -- remove scaling, verify resources deleted (HPA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-cleanup-hpa",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_cleanup_hpa(test_case: TestCase):
    """Removing scaling config should delete VA and HPA."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="hpa")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "spec": {
                    "scaling": None,
                    "replicas": 1,
                }
            },
        )

        assert_scaling_resources_deleted(service_name, actuator="hpa")
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 7b: Cleanup -- remove scaling, verify resources deleted (KEDA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-cleanup-keda",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_cleanup_keda(test_case: TestCase):
    """Removing scaling config should delete VA and ScaledObject."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="keda")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "spec": {
                    "scaling": None,
                    "replicas": 1,
                }
            },
        )

        assert_scaling_resources_deleted(service_name, actuator="keda")
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 8: Stop -- set stop annotation, verify resources deleted (HPA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-stop-hpa",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_stop_hpa(test_case: TestCase):
    """Setting stop annotation should delete VA and HPA."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="hpa")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "metadata": {
                    "annotations": {
                        STOP_ANNOTATION_KEY: "true",
                    }
                }
            },
        )

        assert_scaling_resources_deleted(service_name, actuator="hpa")
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 8b: Stop -- set stop annotation, verify resources deleted (KEDA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-stop-keda",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_stop_keda(test_case: TestCase):
    """Setting stop annotation should delete VA and ScaledObject."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="keda")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "metadata": {
                    "annotations": {
                        STOP_ANNOTATION_KEY: "true",
                    }
                }
            },
        )

        assert_scaling_resources_deleted(service_name, actuator="keda")
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 9: Update -- patch maxReplicas, verify scaling reflects new limit (HPA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_hpa
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-hpa",
                ],
                prompt="KServe is a",
                service_name="autoscale-update-hpa",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_update_hpa(test_case: TestCase):
    """Patching maxReplicas should update the HPA; VA and HPA still exist."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="hpa")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "spec": {
                    "scaling": {
                        "maxReplicas": 5,
                    }
                }
            },
        )

        time.sleep(5)
        assert_scaling_resources_exist(service_name, actuator="hpa")

        hpa = _get_custom_resource(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name),
            KSERVE_TEST_NAMESPACE,
        )
        assert hpa is not None
        assert hpa["spec"]["maxReplicas"] == 5, (
            f"Expected maxReplicas=5 after update, got {hpa['spec']['maxReplicas']}"
        )
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Test 9b: Update -- patch maxReplicas, verify scaling reflects new limit (KEDA)
# =============================================================================


@pytest.mark.llminferenceservice
@pytest.mark.autoscaling
@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-update-keda",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_update_keda(test_case: TestCase):
    """Patching maxReplicas should update the ScaledObject; VA and ScaledObject still exist."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name

    try:
        _create_and_wait(kserve_client, test_case)
        assert_scaling_resources_exist(service_name, actuator="keda")

        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "spec": {
                    "scaling": {
                        "maxReplicas": 5,
                    }
                }
            },
        )

        time.sleep(5)
        assert_scaling_resources_exist(service_name, actuator="keda")

        so = _get_custom_resource(
            KEDA_GROUP,
            KEDA_VERSION,
            KEDA_PLURAL,
            scaled_object_name(service_name),
            KSERVE_TEST_NAMESPACE,
        )
        assert so is not None
        assert so["spec"]["maxReplicaCount"] == 5, (
            f"Expected maxReplicaCount=5 after update, got {so['spec']['maxReplicaCount']}"
        )
    finally:
        _cleanup(kserve_client, test_case)
