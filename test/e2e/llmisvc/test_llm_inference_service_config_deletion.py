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
from .logging import log_execution
from .test_llm_inference_service import (
    create_llmisvc,
    delete_llmisvc,
    wait_for,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
CONFIG_FINALIZER = "serving.kserve.io/llmisvcconfig-finalizer"
API_VERSION = "v1alpha1"


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


def _create_llmisvc_with_config_ref(kserve_client, service_name, config_name, namespace=KSERVE_TEST_NAMESPACE):
    """Create an LLMInferenceService referencing a config via baseRefs."""
    from kserve import V1alpha1LLMInferenceService

    # We need configs that together form a valid LLMInferenceService.
    # Create additional configs for workload and router alongside the config under test.
    workload_config_name = f"{service_name}-workload-cfg"
    router_config_name = f"{service_name}-router-cfg"

    _create_or_update_llmisvc_config(kserve_client, {
        "apiVersion": f"serving.kserve.io/{API_VERSION}",
        "kind": "LLMInferenceServiceConfig",
        "metadata": {"name": workload_config_name, "namespace": namespace},
        "spec": LLMINFERENCESERVICE_CONFIGS["workload-single-cpu"],
    }, namespace)

    _create_or_update_llmisvc_config(kserve_client, {
        "apiVersion": f"serving.kserve.io/{API_VERSION}",
        "kind": "LLMInferenceServiceConfig",
        "metadata": {"name": router_config_name, "namespace": namespace},
        "spec": LLMINFERENCESERVICE_CONFIGS["router-managed"],
    }, namespace)

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


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_config_finalizer_added():
    """Test that a finalizer is added to a new LLMInferenceServiceConfig."""
    inject_k8s_proxy()
    kserve_client = _kserve_client()
    config_name = "e2e-finalizer-add-test"

    try:
        _create_config(kserve_client, config_name)

        def assert_finalizer_present():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_finalizer(cfg), (
                f"Expected finalizer {CONFIG_FINALIZER} on config {config_name}, "
                f"got finalizers: {cfg.get('metadata', {}).get('finalizers', [])}"
            )
            return True

        wait_for(assert_finalizer_present, timeout=60, interval=2.0)
        print(f"Finalizer {CONFIG_FINALIZER} present on config {config_name}")

    finally:
        _cleanup_config_silent(kserve_client, config_name)


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
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
            kserve_client, service_name, config_name,
        )

        # Attempt to delete the config
        print(f"Attempting to delete config {config_name} (should be blocked)")
        _delete_config(kserve_client, config_name)

        # The config should still exist with a deletionTimestamp but the finalizer
        # should prevent actual removal
        def assert_deletion_blocked():
            cfg = _get_config(kserve_client, config_name)
            assert _config_has_deletion_timestamp(cfg), (
                "Config should have a deletionTimestamp after delete was called"
            )
            assert _config_has_finalizer(cfg), (
                "Finalizer should still be present while config is referenced"
            )
            return True

        wait_for(assert_deletion_blocked, timeout=60, interval=2.0)
        print(f"Config {config_name} deletion is correctly blocked by finalizer")

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
@pytest.mark.asyncio(loop_scope="session")
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
@pytest.mark.asyncio(loop_scope="session")
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
            kserve_client, service_name, config_name,
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
        print(f"Config {config_name} was deleted after service {service_name} was removed")

    finally:
        if llm_svc is not None:
            _cleanup_llmisvc_silent(kserve_client, llm_svc)
        _cleanup_config_silent(kserve_client, config_name)
        for cfg_name in extra_configs:
            _cleanup_config_silent(kserve_client, cfg_name)


def _config_is_gone(kserve_client, config_name, namespace=KSERVE_TEST_NAMESPACE):
    """Assert that a config no longer exists (404)."""
    try:
        _get_config(kserve_client, config_name, namespace)
        raise AssertionError(f"Config {config_name} still exists, expected 404")
    except client.rest.ApiException as e:
        if e.status == 404:
            return True
        raise AssertionError(f"Unexpected error checking config {config_name}: {e}") from e
