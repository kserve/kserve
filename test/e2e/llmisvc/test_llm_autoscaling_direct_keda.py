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

"""
E2E tests for LLMISVC direct KEDA autoscaling (spec.scaling.keda, no WVA).

These tests verify:
- ScaledObject is created with user-defined triggers
- WVA discovery annotations are NOT set
- Lifecycle cleanup works when scaling is removed

Tagged with autoscaling_keda so they run in the existing KEDA CI job.
"""

import logging
import os

import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import (
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_autoscaling_wva import (
    HPA_GROUP,
    HPA_PLURAL,
    HPA_VERSION,
    KEDA_GROUP,
    KEDA_PLURAL,
    KEDA_VERSION,
    WORKLOAD_COMPONENT_MAIN,
    _get_custom_resource,
    assert_scaled_object_condition,
    assert_scaled_object_ready,
    assert_scaling_ready_absent,
    assert_scaling_ready_condition,
    assert_scaling_resources_deleted,
    hpa_name,
    resource_exists,
    scaled_object_name,
    send_load,
    wait_for_pod_count,
    wait_for_pod_count_at_most,
    wait_for_resource,
)
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    delete_llmisvc,
    get_llm_service_url,
    wait_for,
    wait_for_llm_isvc_ready,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"

logger = logging.getLogger(__name__)


def _create_and_wait(kserve_client, test_case):
    create_llmisvc(kserve_client, test_case.llm_service)
    wait_for_llm_isvc_ready(
        kserve_client, test_case.llm_service, test_case.wait_timeout
    )


def _cleanup(kserve_client, test_case):
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


def patch_llmisvc(kserve_client, llm_isvc, patch_body):
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


def assert_direct_keda_scaled_object(
    service_name, *, namespace, trigger_type="prometheus"
):
    """Assert ScaledObject exists with user triggers and without WVA annotations."""
    name = scaled_object_name(service_name)
    wait_for_resource(KEDA_GROUP, KEDA_VERSION, KEDA_PLURAL, name, namespace)

    def _check():
        resource = _get_custom_resource(
            KEDA_GROUP, KEDA_VERSION, KEDA_PLURAL, name, namespace
        )
        assert resource is not None, f"ScaledObject {name} not found"
        annotations = resource.get("metadata", {}).get("annotations", {}) or {}
        assert "llm-d.ai/managed" not in annotations, (
            f"Direct KEDA ScaledObject should not have llm-d.ai/managed, got {annotations}"
        )
        assert "llm-d.ai/model-id" not in annotations, (
            f"Direct KEDA ScaledObject should not have llm-d.ai/model-id, got {annotations}"
        )
        triggers = resource.get("spec", {}).get("triggers", [])
        assert triggers, f"ScaledObject {name} has no triggers"
        assert triggers[0].get("type") == trigger_type, (
            f"Expected trigger type {trigger_type}, got {triggers[0].get('type')}"
        )

    wait_for(_check, timeout=60, interval=2.0)


@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-prometheus-scrape",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-direct-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-direct-keda",
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
def test_llm_autoscaling_direct_keda_deployment(test_case: TestCase):
    """Direct KEDA + Deployment: user-defined Prometheus trigger (EPP request
    rate) fires under load and drives an actual scale-up, not just object
    existence/Ready."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    try:
        _create_and_wait(kserve_client, test_case)

        assert_direct_keda_scaled_object(service_name, namespace=ns)
        assert not resource_exists(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name),
            ns,
        )
        assert_scaled_object_ready(service_name, namespace=ns)
        assert_scaling_ready_condition(service_name, namespace=ns)

        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url, test_case.model_name, concurrency=10, duration_seconds=60
        )

        wait_for_pod_count(
            service_name,
            min_count=2,
            namespace=ns,
            timeout=300,
            component=WORKLOAD_COMPONENT_MAIN,
        )
        assert_scaled_object_condition(
            service_name, namespace=ns, condition_type="Active", expected_status="True"
        )
    finally:
        _cleanup(kserve_client, test_case)


@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-direct-keda",
                ],
                prompt="KServe is a",
                service_name="autoscale-direct-cleanup",
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
def test_llm_autoscaling_direct_keda_cleanup(test_case: TestCase):
    """Removing direct KEDA scaling should delete the ScaledObject."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    try:
        _create_and_wait(kserve_client, test_case)
        assert_direct_keda_scaled_object(service_name, namespace=ns)
        assert_scaling_ready_condition(service_name, namespace=ns)

        base_refs = test_case.llm_service.spec["baseRefs"]
        non_scaling_refs = [ref for ref in base_refs if "scaling" not in ref["name"]]
        patch_llmisvc(
            kserve_client,
            test_case.llm_service,
            {
                "spec": {
                    "baseRefs": non_scaling_refs,
                    "replicas": 1,
                }
            },
        )

        assert_scaling_resources_deleted(service_name, actuator="keda", namespace=ns)
        assert_scaling_ready_absent(service_name, namespace=ns)
    finally:
        _cleanup(kserve_client, test_case)


# =============================================================================
# Idle scale-to-zero: direct KEDA + idleReplicaCount: 0
# =============================================================================
#
# Scoped to the direct-KEDA path only; see test_llm_autoscaling_wva.py for
# why WVA-mediated idle scale-down is out of scope (the simulator never
# emits decreasing WVA saturation metrics).


@pytest.mark.autoscaling_keda
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-prometheus-scrape",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-direct-keda-idle",
                ],
                prompt="KServe is a",
                service_name="autoscale-direct-keda-idle",
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
def test_llm_autoscaling_direct_keda_scale_to_zero(test_case: TestCase):
    """Direct KEDA + idleReplicaCount=0: deployment scales to zero when idle
    and back up once the EPP request-rate trigger becomes active again."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    try:
        _create_and_wait(kserve_client, test_case)
        assert_direct_keda_scaled_object(service_name, namespace=ns)

        # Scoped to the main workload component so the always-running
        # router/scheduler pod doesn't mask the workload reaching zero.
        wait_for_pod_count_at_most(
            service_name,
            max_count=0,
            namespace=ns,
            timeout=120,
            component=WORKLOAD_COMPONENT_MAIN,
        )
        assert_scaled_object_condition(
            service_name, namespace=ns, condition_type="Active", expected_status="False"
        )

        # KEDA has no request-buffering activator like Knative, so requests
        # sent while scaled to zero are expected to fail; they only need to
        # make EPP emit request-rate signal for KEDA's next poll to scale up.
        service_url = get_llm_service_url(kserve_client, test_case.llm_service)
        send_load(
            service_url,
            test_case.model_name,
            concurrency=10,
            duration_seconds=60,
            tolerate_failures=True,
        )

        wait_for_pod_count(
            service_name,
            min_count=1,
            namespace=ns,
            timeout=300,
            component=WORKLOAD_COMPONENT_MAIN,
        )
        assert_scaled_object_condition(
            service_name, namespace=ns, condition_type="Active", expected_status="True"
        )

        # Once the cooldown period elapses, KEDA scales back to
        # idleReplicaCount=0.
        wait_for_pod_count_at_most(
            service_name,
            max_count=0,
            namespace=ns,
            timeout=180,
            component=WORKLOAD_COMPONENT_MAIN,
        )
        assert_scaled_object_condition(
            service_name, namespace=ns, condition_type="Active", expected_status="False"
        )
    finally:
        _cleanup(kserve_client, test_case)
