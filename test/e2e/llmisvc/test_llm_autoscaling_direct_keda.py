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
    _get_custom_resource,
    assert_scaled_object_active,
    assert_scaling_ready_absent,
    assert_scaling_ready_condition,
    assert_scaling_resources_deleted,
    hpa_name,
    resource_exists,
    scaled_object_name,
    wait_for_resource,
)
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    delete_llmisvc,
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


def assert_direct_keda_scaled_object(service_name, *, namespace, trigger_type="cpu"):
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
    """Direct KEDA + Deployment: ScaledObject uses user triggers and skips WVA annotations."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    try:
        _create_and_wait(kserve_client, test_case)

        assert_direct_keda_scaled_object(service_name, namespace=ns, trigger_type="cpu")
        assert not resource_exists(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name),
            ns,
        )
        assert_scaled_object_active(service_name, namespace=ns)
        assert_scaling_ready_condition(service_name, namespace=ns)
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
