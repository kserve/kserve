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

from __future__ import annotations

import os
import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import (
    KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
    KSERVE_TEST_NAMESPACE,
    LLMINFERENCESERVICE_CONFIGS,
    _create_or_update_llmisvc_config,
    inject_k8s_proxy,
)
from ..common.utils import KSERVE_NAMESPACE
from .logging import log_execution
from .test_llm_inference_service import (
    create_llmisvc,
    delete_llmisvc,
    wait_for,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
CONFIG_FINALIZER = "serving.kserve.io/llmisvcconfig-finalizer"
API_VERSION = "v1alpha2"
WELL_KNOWN_CONFIG_SUFFIX = "-config-llm-template"


def _kserve_client() -> KServeClient:
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


def _create_config(kserve_client, name, namespace=KSERVE_TEST_NAMESPACE):
    """Create a minimal LLMInferenceServiceConfig with model fields."""
    config_body = {
        "apiVersion": f"serving.kserve.io/{API_VERSION}",
        "kind": "LLMInferenceServiceConfig",
        "metadata": {
            "name": name,
            "namespace": namespace,
        },
        "spec": {
            "model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"},
        },
    }
    return _create_or_update_llmisvc_config(kserve_client, config_body, namespace)


def _get_config(kserve_client, name, namespace=KSERVE_TEST_NAMESPACE):
    """Get an LLMInferenceServiceConfig by name."""
    return kserve_client.api_instance.get_namespaced_custom_object(
        constants.KSERVE_GROUP,
        API_VERSION,
        namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
        name,
    )


def _delete_config(kserve_client, name, namespace=KSERVE_TEST_NAMESPACE):
    """Delete an LLMInferenceServiceConfig by name."""
    return kserve_client.api_instance.delete_namespaced_custom_object(
        constants.KSERVE_GROUP,
        API_VERSION,
        namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
        name,
    )


def _config_has_finalizer(config_obj, finalizer=CONFIG_FINALIZER):
    """Check if a config object has the expected finalizer."""
    finalizers = config_obj.get("metadata", {}).get("finalizers", [])
    return finalizer in finalizers


def _config_has_deletion_timestamp(config_obj):
    """Check if a config object has a deletionTimestamp set."""
    return config_obj.get("metadata", {}).get("deletionTimestamp") is not None


def _create_llmisvc_with_config_ref(
    kserve_client, service_name, config_name, namespace=KSERVE_TEST_NAMESPACE
):
    """Create an LLMInferenceService referencing a config via baseRefs."""
    from kserve import V1alpha1LLMInferenceService

    # We need configs that together form a valid LLMInferenceService.
    # Create additional configs for workload and router alongside the config under test.
    workload_config_name = f"{service_name}-workload-cfg"
    router_config_name = f"{service_name}-router-cfg"

    _create_or_update_llmisvc_config(
        kserve_client,
        {
            "apiVersion": f"serving.kserve.io/{API_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {"name": workload_config_name, "namespace": namespace},
            "spec": LLMINFERENCESERVICE_CONFIGS["workload-single-cpu"],
        },
        namespace,
    )

    _create_or_update_llmisvc_config(
        kserve_client,
        {
            "apiVersion": f"serving.kserve.io/{API_VERSION}",
            "kind": "LLMInferenceServiceConfig",
            "metadata": {"name": router_config_name, "namespace": namespace},
            "spec": LLMINFERENCESERVICE_CONFIGS["router-managed"],
        },
        namespace,
    )

    llm_svc = V1alpha1LLMInferenceService(
        api_version=f"serving.kserve.io/{API_VERSION}",
        kind="LLMInferenceService",
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=namespace,
        ),
        spec={
            "baseRefs": [
                {"name": config_name},
                {"name": workload_config_name},
                {"name": router_config_name},
            ],
        },
    )

    create_llmisvc(kserve_client, llm_svc)
    return llm_svc, [workload_config_name, router_config_name]


def _cleanup_config_silent(kserve_client, name, namespace=KSERVE_TEST_NAMESPACE):
    """Delete a config, ignoring 404."""
    try:
        _delete_config(kserve_client, name, namespace)
    except client.rest.ApiException as e:
        if e.status != 404:
            print(f"Warning: Failed to cleanup config {name}: {e}")
    except Exception as e:
        print(f"Warning: Failed to cleanup config {name}: {e}")


def _cleanup_llmisvc_silent(kserve_client, llm_svc):
    """Delete an LLMInferenceService, ignoring errors."""
    try:
        delete_llmisvc(kserve_client, llm_svc)
    except Exception as e:
        print(f"Warning: Failed to cleanup service {llm_svc.metadata.name}: {e}")


def _get_condition(config_obj, condition_type):
    """Extract a condition by type from a config object's status."""
    conditions = config_obj.get("status", {}).get("conditions", [])
    for cond in conditions:
        if cond.get("type") == condition_type:
            return cond
    return None


def _config_is_gone(kserve_client, config_name, namespace=KSERVE_TEST_NAMESPACE):
    """Assert that a config no longer exists (404)."""
    try:
        _get_config(kserve_client, config_name, namespace)
        raise AssertionError(f"Config {config_name} still exists, expected 404")
    except client.rest.ApiException as e:
        if e.status == 404:
            return True
        raise AssertionError(
            f"Unexpected error checking config {config_name}: {e}"
        ) from e


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_config_finalizer_added():
    """Test that a finalizer is added and Ready condition is set on a new LLMInferenceServiceConfig."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    config_name = "e2e-finalizer-add-test"

    try:
        _create_config(kserve_client, config_name)

        def assert_finalizer_and_ready():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_finalizer(cfg), (
                f"Expected finalizer {CONFIG_FINALIZER} on config {config_name}, "
                f"got finalizers: {cfg.get('metadata', {}).get('finalizers', [])}"
            )
            # Verify Ready condition is True
            ready_cond = _get_condition(cfg, "Ready")
            assert ready_cond is not None, "Expected Ready condition to be set"
            assert ready_cond.get("status") == "True", (
                f"Expected Ready=True, got {ready_cond.get('status')}"
            )
            return True

        wait_for(assert_finalizer_and_ready, timeout=60, interval=2.0)
        print(f"Finalizer and Ready=True condition present on config {config_name}")

    finally:
        _cleanup_config_silent(kserve_client, config_name)


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_config_deletion_blocked_when_referenced():
    """Test that deleting a config referenced by an LLMInferenceService is blocked by the finalizer."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    config_name = "e2e-del-blocked-cfg"
    service_name = "e2e-del-blocked-svc"
    extra_configs = []
    llm_svc = None

    try:
        # Create the config under test (model config)
        _create_config(kserve_client, config_name)

        # Wait for the finalizer to be added
        def assert_finalizer_present():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_finalizer(cfg), "Finalizer not yet present"
            return True

        wait_for(assert_finalizer_present, timeout=60, interval=2.0)

        # Create an LLMInferenceService that references this config
        llm_svc, extra_configs = _create_llmisvc_with_config_ref(
            kserve_client,
            service_name,
            config_name,
        )

        # Attempt to delete the config
        print(f"Attempting to delete config {config_name} (should be blocked)")
        _delete_config(kserve_client, config_name)

        # The config should still exist with a deletionTimestamp but the finalizer
        # should prevent actual removal, and Ready condition should be False
        def assert_deletion_blocked():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_deletion_timestamp(cfg), (
                "Config should have a deletionTimestamp after delete was called"
            )
            assert _config_has_finalizer(cfg), (
                "Finalizer should still be present while config is referenced"
            )
            # Verify ConfigInUse condition is True with DeletionBlocked reason
            in_use_cond = _get_condition(cfg, "ConfigInUse")
            assert in_use_cond is not None, "Expected ConfigInUse condition to be set"
            assert in_use_cond.get("status") == "True", (
                f"Expected ConfigInUse=True when deletion is blocked, got {in_use_cond.get('status')}"
            )
            assert in_use_cond.get("reason") == "DeletionBlocked", (
                f"Expected reason=DeletionBlocked, got {in_use_cond.get('reason')}"
            )
            # Verify referencedBy lists the referencing service
            referenced_by = cfg.get("status", {}).get("referencedBy", [])
            assert len(referenced_by) > 0, (
                f"Expected referencedBy to list referencing services, got {referenced_by}"
            )
            svc_names = [ref.get("name") for ref in referenced_by]
            assert service_name in svc_names, (
                f"Expected {service_name} in referencedBy names, got {svc_names}"
            )
            return True

        wait_for(assert_deletion_blocked, timeout=60, interval=2.0)
        print(
            f"Config {config_name} deletion is correctly blocked (ConfigInUse=True, reason=DeletionBlocked)"
        )

    finally:
        # Clean up: delete the service first (unblocks the config), then configs
        if llm_svc is not None:
            _cleanup_llmisvc_silent(kserve_client, llm_svc)
        # Wait a bit for the finalizer to be removed after service deletion
        try:
            wait_for(
                lambda: _config_is_gone(kserve_client, config_name),
                timeout=120,
                interval=2.0,
            )
        except AssertionError:
            _cleanup_config_silent(kserve_client, config_name)
        for cfg_name in extra_configs:
            _cleanup_config_silent(kserve_client, cfg_name)


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_config_deletion_allowed_when_unreferenced():
    """Test that deleting a config not referenced by any LLMInferenceService succeeds."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    config_name = "e2e-del-unreferenced-cfg"

    try:
        _create_config(kserve_client, config_name)

        # Wait for the finalizer to be added
        def assert_finalizer_present():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_finalizer(cfg), "Finalizer not yet present"
            return True

        wait_for(assert_finalizer_present, timeout=60, interval=2.0)

        # Delete the config (no service references it)
        print(f"Deleting unreferenced config {config_name}")
        _delete_config(kserve_client, config_name)

        # Config should be fully deleted
        def assert_config_gone():
            return _config_is_gone(kserve_client, config_name)

        wait_for(assert_config_gone, timeout=60, interval=2.0)
        print(f"Config {config_name} was deleted successfully (not referenced)")

    finally:
        _cleanup_config_silent(kserve_client, config_name)


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_config_deletion_unblocked_after_service_deleted():
    """Test that a blocked config deletion completes after the referencing service is deleted."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    config_name = "e2e-del-unblock-cfg"
    service_name = "e2e-del-unblock-svc"
    extra_configs = []
    llm_svc = None

    try:
        # Create config and service
        _create_config(kserve_client, config_name)

        def assert_finalizer_present():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_finalizer(cfg), "Finalizer not yet present"
            return True

        wait_for(assert_finalizer_present, timeout=60, interval=2.0)

        llm_svc, extra_configs = _create_llmisvc_with_config_ref(
            kserve_client,
            service_name,
            config_name,
        )

        # Attempt to delete the config (should be blocked)
        print(f"Attempting to delete config {config_name} (should be blocked)")
        _delete_config(kserve_client, config_name)

        # Verify deletion is blocked
        def assert_deletion_blocked():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_deletion_timestamp(cfg), "Should have deletionTimestamp"
            assert _config_has_finalizer(cfg), "Finalizer should still be present"
            return True

        wait_for(assert_deletion_blocked, timeout=60, interval=2.0)
        print(f"Config {config_name} deletion is blocked as expected")

        # Now delete the referencing service
        print(f"Deleting referencing service {service_name}")
        delete_llmisvc(kserve_client, llm_svc)

        # The config should now be fully deleted (finalizer removed)
        print(f"Waiting for config {config_name} to be deleted after service removal")

        def assert_config_gone():
            return _config_is_gone(kserve_client, config_name)

        wait_for(assert_config_gone, timeout=120, interval=2.0)
        print(
            f"Config {config_name} was deleted after service {service_name} was removed"
        )

    finally:
        if llm_svc is not None:
            _cleanup_llmisvc_silent(kserve_client, llm_svc)
        _cleanup_config_silent(kserve_client, config_name)
        for cfg_name in extra_configs:
            _cleanup_config_silent(kserve_client, cfg_name)


def _find_well_known_config(kserve_client, suffix, namespace=KSERVE_NAMESPACE):
    """Find a well-known config by name suffix in the given namespace."""
    configs = kserve_client.api_instance.list_namespaced_custom_object(
        constants.KSERVE_GROUP,
        API_VERSION,
        namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
    )
    for cfg in configs.get("items", []):
        name = cfg.get("metadata", {}).get("name", "")
        if name.endswith(suffix):
            return name
    return None


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_well_known_config_deletion_prevented_by_webhook():
    """Test that the webhook prevents deletion of well-known configs in the kserve namespace."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()

    config_name = _find_well_known_config(kserve_client, WELL_KNOWN_CONFIG_SUFFIX)
    assert config_name is not None, (
        f"No config ending with {WELL_KNOWN_CONFIG_SUFFIX!r} found in namespace {KSERVE_NAMESPACE}"
    )
    print(f"Found well-known config: {config_name} in namespace {KSERVE_NAMESPACE}")

    with pytest.raises(client.rest.ApiException) as exc_info:
        _delete_config(kserve_client, config_name, namespace=KSERVE_NAMESPACE)

    assert exc_info.value.status == 403, (
        f"Expected 403 Forbidden, got {exc_info.value.status}"
    )
    assert "cannot be deleted" in str(exc_info.value.body), (
        f"Expected 'cannot be deleted' in error body, got: {exc_info.value.body}"
    )
    print(
        f"Well-known config {config_name} deletion correctly prevented by webhook (403)"
    )

    cfg = _get_config(kserve_client, config_name, namespace=KSERVE_NAMESPACE)
    assert not _config_has_deletion_timestamp(cfg), (
        "Config should NOT have deletionTimestamp since the webhook rejected the request"
    )
    print(f"Well-known config {config_name} is intact (no deletionTimestamp)")


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_well_known_config_deletion_blocked_by_implicit_reference():
    """Test that a well-known config is blocked from deletion by the finalizer while any
    LLMInferenceService exists in the same namespace.

    Uses a well-known-named config in the test namespace to avoid the webhook that
    prevents deletion of well-known configs in the kserve namespace.
    """
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    service_name = "e2e-wk-cfg-implicit-svc"
    extra_configs = []
    llm_svc = None

    # Discover the well-known config name (handles custom prefixes)
    wk_config_name = _find_well_known_config(kserve_client, WELL_KNOWN_CONFIG_SUFFIX)
    assert wk_config_name is not None, (
        f"No config ending with {WELL_KNOWN_CONFIG_SUFFIX!r} found in namespace {KSERVE_NAMESPACE}"
    )
    print(f"Discovered well-known config name: {wk_config_name}")

    try:
        # Create a config with the well-known name in the test namespace.
        _create_config(kserve_client, wk_config_name)
        print(
            f"Created well-known config {wk_config_name} in namespace {KSERVE_TEST_NAMESPACE}"
        )

        # Create an LLMInferenceService that does NOT explicitly reference the
        # well-known config. The controller treats well-known configs as implicitly
        # referenced by all services in the same namespace.
        model_config_name = f"{service_name}-model-cfg"
        _create_config(kserve_client, model_config_name)
        extra_configs.append(model_config_name)

        llm_svc, svc_extra = _create_llmisvc_with_config_ref(
            kserve_client,
            service_name,
            model_config_name,
        )
        extra_configs.extend(svc_extra)

        # Wait for the well-known config to have a finalizer
        def assert_finalizer_present():
            cfg = _get_config(kserve_client, wk_config_name)
            assert _config_has_finalizer(cfg), (
                "Finalizer not yet present on well-known config"
            )
            return True

        wait_for(assert_finalizer_present, timeout=60, interval=2.0)

        # Attempt to delete the well-known config (webhook allows it in test
        # namespace, but the finalizer should block it)
        print(
            f"Attempting to delete well-known config {wk_config_name} (should be blocked by finalizer)"
        )
        _delete_config(kserve_client, wk_config_name)

        # The well-known config should be blocked from deletion
        def assert_deletion_blocked():
            cfg = _get_config(kserve_client, wk_config_name)
            assert _config_has_deletion_timestamp(cfg), (
                "Well-known config should have a deletionTimestamp after delete was called"
            )
            assert _config_has_finalizer(cfg), (
                "Finalizer should still be present while any service exists"
            )
            in_use_cond = _get_condition(cfg, "ConfigInUse")
            assert in_use_cond is not None, "Expected ConfigInUse condition to be set"
            assert in_use_cond.get("status") == "True", (
                f"Expected ConfigInUse=True, got {in_use_cond.get('status')}"
            )
            assert in_use_cond.get("reason") == "DeletionBlocked", (
                f"Expected reason=DeletionBlocked, got {in_use_cond.get('reason')}"
            )
            referenced_by = cfg.get("status", {}).get("referencedBy", [])
            assert len(referenced_by) > 0, (
                f"Expected referencedBy to list services, got {referenced_by}"
            )
            return True

        wait_for(assert_deletion_blocked, timeout=60, interval=2.0)
        print(
            f"Well-known config {wk_config_name} deletion is correctly blocked "
            f"(ConfigInUse=True, reason=DeletionBlocked)"
        )

        # Delete the service to unblock
        print(f"Deleting service {service_name} to unblock well-known config deletion")
        delete_llmisvc(kserve_client, llm_svc)
        llm_svc = None

        # The well-known config should now be deleted
        def assert_config_gone():
            return _config_is_gone(kserve_client, wk_config_name)

        wait_for(assert_config_gone, timeout=120, interval=2.0)
        print(f"Well-known config {wk_config_name} deleted after service removal")

    finally:
        if llm_svc is not None:
            _cleanup_llmisvc_silent(kserve_client, llm_svc)
        _cleanup_config_silent(kserve_client, wk_config_name)
        for cfg_name in extra_configs:
            _cleanup_config_silent(kserve_client, cfg_name)
