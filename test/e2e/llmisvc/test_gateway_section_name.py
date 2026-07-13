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
E2E tests for Gateway SectionName propagation to HTTPRoute ParentRefs.

These tests verify that when a GatewayObjectReference includes a sectionName,
the managed HTTPRoute's ParentReference targets the specific listener, and
when sectionName is omitted the route attaches to all listeners (backward compat).
"""

import logging
import os

import pytest
from kserve import KServeClient, V1alpha1LLMInferenceService, constants
from kubernetes import client

from .fixtures import (
    KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
    KSERVE_TEST_NAMESPACE,
    LLMINFERENCESERVICE_CONFIGS,
    _create_or_update_llmisvc_config,
    create_router_resources,
    generate_k8s_safe_suffix,
    inject_k8s_proxy,
)
from .test_llm_inference_service import (
    KSERVE_PLURAL_LLMINFERENCESERVICE,
    _collect_diagnostics,
    wait_for,
)
from .test_resources import ROUTER_GATEWAYS

logger = logging.getLogger(__name__)

GATEWAY_NAME = "router-gateway-1"


def _get_kserve_client():
    inject_k8s_proxy()
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


def _create_llmisvc_configs(kserve_client, base_refs, service_name):
    """Create LLMInferenceServiceConfig resources and return unique names."""
    unique_base_refs = []
    for base_ref in base_refs:
        unique_config_name = generate_k8s_safe_suffix(base_ref, [service_name])
        unique_base_refs.append(unique_config_name)
        original_spec = LLMINFERENCESERVICE_CONFIGS[base_ref]
        config_body = {
            "apiVersion": "serving.kserve.io/v1alpha1",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {
                "name": unique_config_name,
                "namespace": KSERVE_TEST_NAMESPACE,
            },
            "spec": original_spec,
        }
        _create_or_update_llmisvc_config(kserve_client, config_body)
    return unique_base_refs


def _delete_llmisvc_config(kserve_client, name, namespace):
    """Delete an LLMInferenceServiceConfig, ignoring not-found errors."""
    try:
        kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
            name,
        )
    except client.ApiException:
        pass


def _get_managed_httproutes(kserve_client, service_name, namespace):
    """List HTTPRoutes owned by a given LLMInferenceService."""
    routes = kserve_client.api_instance.list_namespaced_custom_object(
        "gateway.networking.k8s.io",
        "v1",
        namespace,
        "httproutes",
        label_selector=f"app.kubernetes.io/name={service_name},app.kubernetes.io/part-of=llminferenceservice",
    )
    return routes.get("items", [])


def _find_gateway_parent_ref(routes, gateway_name):
    """Find the parentRef matching the given gateway name across all routes.

    Returns the parentRef dict if found, None otherwise.
    """
    for route in routes:
        for parent_ref in route.get("spec", {}).get("parentRefs", []):
            if parent_ref.get("name") == gateway_name:
                return parent_ref
    return None


def _cleanup_llmisvc(kserve_client, name, namespace, version="v1alpha1"):
    """Delete an LLMInferenceService, ignoring not-found errors."""
    try:
        kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            name,
        )
    except client.rest.ApiException as e:
        if e.status != 404:
            logger.warning(f"Failed to cleanup LLMInferenceService {name}: {e}")


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@pytest.mark.llmd_simulator
@pytest.mark.parametrize(
    "gateway_config_key, expected_section_name",
    [
        ("router-with-gateway-section-name", "http"),
        ("router-with-gateway-ref", None),
    ],
    ids=["with-section-name", "without-section-name"],
)
def test_gateway_section_name_propagation(gateway_config_key, expected_section_name):
    """When sectionName is set on a gateway ref, the managed HTTPRoute's
    ParentReference should include the matching sectionName. When omitted,
    the parentRef should not include sectionName (backward compatibility)."""
    kserve_client = _get_kserve_client()

    service_name = generate_k8s_safe_suffix("gw-section-name", [gateway_config_key])

    # Ensure the gateway exists (its listener is named "http")
    create_router_resources(gateways=[ROUTER_GATEWAYS[0]], kserve_client=kserve_client)

    base_refs = [
        gateway_config_key,
        "router-with-managed-route",
        "model-fb-opt-125m",
        "workload-llmd-simulator",
    ]
    created_config_names = []

    llm_service = None
    try:
        created_config_names = _create_llmisvc_configs(
            kserve_client, base_refs, service_name
        )

        llm_service = V1alpha1LLMInferenceService(
            api_version="serving.kserve.io/v1alpha1",
            kind="LLMInferenceService",
            metadata=client.V1ObjectMeta(
                name=service_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec={"baseRefs": [{"name": ref} for ref in created_config_names]},
        )

        kserve_client.api_instance.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            "v1alpha1",
            KSERVE_TEST_NAMESPACE,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_service,
        )

        def assert_managed_route_exists():
            routes = _get_managed_httproutes(
                kserve_client, service_name, KSERVE_TEST_NAMESPACE
            )
            assert len(routes) >= 1, (
                f"Expected at least 1 managed HTTPRoute, got {len(routes)}"
            )
            return routes

        routes = wait_for(assert_managed_route_exists, timeout=120, interval=2.0)

        gw_parent = _find_gateway_parent_ref(routes, GATEWAY_NAME)
        assert gw_parent is not None, (
            f"Expected parentRef for {GATEWAY_NAME} in managed routes, "
            f"got routes: {[r.get('metadata', {}).get('name') for r in routes]}"
        )

        if expected_section_name is not None:
            assert gw_parent.get("sectionName") == expected_section_name, (
                f"Expected sectionName '{expected_section_name}' on parentRef, got {gw_parent}"
            )
        else:
            assert "sectionName" not in gw_parent, (
                f"Expected no sectionName on parentRef, got {gw_parent}"
            )

    except Exception:
        if llm_service is not None:
            _collect_diagnostics(kserve_client, llm_service)
        raise

    finally:
        if os.getenv("SKIP_RESOURCE_DELETION", "False").lower() in (
            "false",
            "0",
            "f",
        ):
            _cleanup_llmisvc(kserve_client, service_name, KSERVE_TEST_NAMESPACE)
            for config_name in created_config_names:
                _delete_llmisvc_config(
                    kserve_client, config_name, KSERVE_TEST_NAMESPACE
                )
