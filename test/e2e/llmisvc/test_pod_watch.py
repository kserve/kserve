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
E2E tests for LLMISVC pod watch functionality.

These tests verify that:
1. Init container status changes trigger fast reconciliation of the owning LLMInferenceService
2. Init container failures on one LLMISVC don't cause unwanted modifications to unrelated LLMISVCs
   (event storm prevention)
"""

import asyncio
import json
import os
import time
import uuid
from contextlib import contextmanager

import pytest
from kubernetes import client

from kserve import KServeClient, V1alpha1LLMInferenceService, constants

from .fixtures import (
    KSERVE_TEST_NAMESPACE,
    inject_k8s_proxy,
)
from .logging import logger
from .test_llm_inference_service import (
    create_llmisvc,
    delete_llmisvc,
    get_llmisvc,
    wait_for_llm_isvc_ready,
)

# Labels used by LLMISVC controller
LLMISVC_POD_LABEL_NAME = "app.kubernetes.io/name"
LLMISVC_POD_LABEL_PART_OF = "app.kubernetes.io/part-of"
LLMISVC_POD_LABEL_PART_OF_VALUE = "llminferenceservice"

KSERVE_PLURAL_LLMINFERENCESERVICECONFIG = "llminferenceserviceconfigs"


def get_llmisvc_resource_version(
    kserve_client: KServeClient, name: str, namespace: str = KSERVE_TEST_NAMESPACE
) -> str:
    """Get the resourceVersion of an LLMInferenceService."""
    llmisvc = get_llmisvc(kserve_client, name, namespace)
    metadata = llmisvc.get("metadata") if isinstance(llmisvc, dict) else {}
    if isinstance(metadata, dict):
        return str(metadata.get("resourceVersion", ""))
    return ""


def get_llmisvc_conditions(
    kserve_client: KServeClient, name: str, namespace: str = KSERVE_TEST_NAMESPACE
) -> list:
    """Get conditions from LLMInferenceService status."""
    llmisvc = get_llmisvc(kserve_client, name, namespace)
    status = llmisvc.get("status") if isinstance(llmisvc, dict) else {}
    if isinstance(status, dict):
        conditions = status.get("conditions")
        return conditions if isinstance(conditions, list) else []
    return []


def get_condition_by_type(conditions: list, condition_type: str) -> dict | None:
    """Find a condition by type in a list of conditions."""
    for condition in conditions:
        if condition.get("type") == condition_type:
            return condition
    return None


def get_pods_for_llmisvc(name: str, namespace: str) -> list[dict]:
    """Get pods matching an LLMInferenceService by labels."""
    core_api = client.CoreV1Api()
    try:
        pods = core_api.list_namespaced_pod(
            namespace=namespace,
            label_selector=f"{LLMISVC_POD_LABEL_PART_OF}={LLMISVC_POD_LABEL_PART_OF_VALUE},{LLMISVC_POD_LABEL_NAME}={name}",
        )
        return [pod.to_dict() for pod in pods.items]
    except Exception as e:
        return [{"error": f"Failed to list pods for LLMISVC {name}: {e}"}]


def get_deployments_for_llmisvc(name: str, namespace: str) -> list[dict]:
    """Get deployments matching an LLMInferenceService."""
    apps_api = client.AppsV1Api()
    try:
        deployments = apps_api.list_namespaced_deployment(
            namespace=namespace,
            label_selector=f"{LLMISVC_POD_LABEL_PART_OF}={LLMISVC_POD_LABEL_PART_OF_VALUE},{LLMISVC_POD_LABEL_NAME}={name}",
        )
        return [dep.to_dict() for dep in deployments.items]
    except Exception as e:
        return [{"error": f"Failed to list deployments for LLMISVC {name}: {e}"}]


def dump_debug_info(
    kserve_client: KServeClient, llmisvc_names: list[str], namespace: str
) -> None:
    """Dump debug info for the given LLMISVCs, their deployments, and pods."""
    for name in llmisvc_names:
        try:
            llmisvc = get_llmisvc(kserve_client, name, namespace)
        except Exception as e:
            llmisvc = {"error": str(e)}
        debug_data = {
            "llmisvc": llmisvc,
            "deployments": get_deployments_for_llmisvc(name, namespace),
            "pods": get_pods_for_llmisvc(name, namespace),
        }
        logger.info(
            "DEBUG DUMP %s/%s:\n%s",
            namespace,
            name,
            json.dumps(debug_data, separators=(",", ":"), default=str),
        )


@contextmanager
def managed_llmisvc(kserve_client: KServeClient, llmisvc: V1alpha1LLMInferenceService):
    """
    Context manager that handles LLMInferenceService lifecycle: creation, error dumping, and cleanup.

    Usage:
        with managed_llmisvc(kserve_client, llmisvc):
            # LLMISVC is already created
            # ... test logic ...
            # On any exception: dumps debug info for the LLMISVC
            # On exit: deletes the LLMISVC
    """
    assert llmisvc.metadata is not None, "LLMISVC must have metadata"
    assert llmisvc.metadata.name is not None, "LLMISVC must have a name"
    assert llmisvc.metadata.namespace is not None, "LLMISVC must have a namespace"
    name = llmisvc.metadata.name
    namespace = llmisvc.metadata.namespace
    error_occurred = False
    try:
        create_llmisvc(kserve_client, llmisvc)
        yield
    except Exception:
        error_occurred = True
        dump_debug_info(kserve_client, [name], namespace)
        raise
    finally:
        try:
            delete_llmisvc(kserve_client, llmisvc)
        except Exception as e:
            if not error_occurred:
                logger.warning("Failed to delete LLMISVC %s: %s", name, e)


def create_invalid_s3_secret(namespace: str, secret_name: str):
    """Create a secret with invalid S3 credentials."""
    core_api = client.CoreV1Api()
    secret = client.V1Secret(
        api_version="v1",
        kind="Secret",
        metadata=client.V1ObjectMeta(
            name=secret_name,
            namespace=namespace,
            annotations={
                "serving.kserve.io/s3-endpoint": "s3.amazonaws.com",
                "serving.kserve.io/s3-region": "us-east-1",
                "serving.kserve.io/s3-usehttps": "1",
                "serving.kserve.io/s3-verifyssl": "1",
            },
        ),
        type="Opaque",
        string_data={
            "AWS_ACCESS_KEY_ID": "INVALID_ACCESS_KEY_ID_12345",
            "AWS_SECRET_ACCESS_KEY": "INVALID_SECRET_ACCESS_KEY_67890",
        },
    )

    try:
        core_api.delete_namespaced_secret(secret_name, namespace)
    except client.ApiException:
        pass

    return core_api.create_namespaced_secret(namespace, secret)


def create_service_account_with_secret(namespace: str, sa_name: str, secret_name: str):
    """Create a service account referencing a secret."""
    core_api = client.CoreV1Api()

    sa = client.V1ServiceAccount(
        api_version="v1",
        kind="ServiceAccount",
        metadata=client.V1ObjectMeta(name=sa_name, namespace=namespace),
        secrets=[client.V1ObjectReference(name=secret_name)],
    )

    try:
        core_api.delete_namespaced_service_account(sa_name, namespace)
    except client.ApiException:
        pass

    return core_api.create_namespaced_service_account(namespace, sa)


def delete_secret(namespace: str, secret_name: str):
    """Delete a secret if it exists."""
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_secret(secret_name, namespace)
    except client.ApiException:
        pass


def delete_service_account(namespace: str, sa_name: str):
    """Delete a service account if it exists."""
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_service_account(sa_name, namespace)
    except client.ApiException:
        pass


def wait_for_llmisvc_condition_false(
    kserve_client: KServeClient,
    name: str,
    condition_type: str,
    namespace: str = KSERVE_TEST_NAMESPACE,
    timeout_seconds: int = 180,
    poll_interval: float = 5.0,
) -> dict | None:
    """Wait for a condition to become False on an LLMInferenceService."""
    start_time = time.time()
    while time.time() - start_time < timeout_seconds:
        try:
            conditions = get_llmisvc_conditions(kserve_client, name, namespace)
            condition = get_condition_by_type(conditions, condition_type)
            if condition and condition.get("status") == "False":
                logger.info(
                    "LLMISVC %s condition %s is False: %s",
                    name,
                    condition_type,
                    condition,
                )
                return condition
        except Exception as e:
            logger.warning("Error checking LLMISVC conditions: %s", e)
        time.sleep(poll_interval)
    return None


def create_llmisvc_config(
    kserve_client: KServeClient, name: str, namespace: str, spec: dict
):
    """Create an LLMInferenceServiceConfig."""
    config = {
        "apiVersion": "serving.kserve.io/v1alpha1",
        "kind": "LLMInferenceServiceConfig",
        "metadata": {
            "name": name,
            "namespace": namespace,
        },
        "spec": spec,
    }
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

    return kserve_client.api_instance.create_namespaced_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICECONFIG,
        config,
    )


def delete_llmisvc_config(kserve_client: KServeClient, name: str, namespace: str):
    """Delete an LLMInferenceServiceConfig."""
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


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(scope="session")
async def test_event_storm_prevention_init_container_isolation():
    """
    Test that init container status changes on one LLMISVC don't cause unwanted modifications
    to unrelated LLMISVCs (event storm prevention).

    The controller may reconcile an LLMISVC for legitimate reasons (e.g.,
    HTTPRoute status updates) without making any changes. This is acceptable.
    The real concern is if the secondary LLMISVC's events cause the primary
    LLMISVC to be MODIFIED (resourceVersion change).

    Test flow:
    1. Creates a "primary" LLMISVC with a valid model (hf://facebook/opt-125m)
    2. Waits for the primary LLMISVC to become ready
    3. Records baseline resourceVersion
    4. Creates a "secondary" LLMISVC with invalid S3 credentials that will fail
    5. Waits for the secondary LLMISVC to show failure in conditions
    6. Verifies the primary LLMISVC's resourceVersion is unchanged
    """
    inject_k8s_proxy()

    suffix = str(uuid.uuid4())[:6]
    primary_name = f"llmisvc-primary-{suffix}"
    secondary_name = f"llmisvc-secondary-{suffix}"
    invalid_sa_name = f"invalid-s3-sa-{suffix}"
    invalid_secret_name = f"invalid-s3-secret-{suffix}"

    # Config names
    model_config_name = f"model-config-{suffix}"
    workload_config_name = f"workload-config-{suffix}"
    router_config_name = f"router-config-{suffix}"
    invalid_model_config_name = f"invalid-model-config-{suffix}"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    try:
        # Create configs for valid primary LLMISVC
        create_llmisvc_config(
            kserve_client,
            model_config_name,
            KSERVE_TEST_NAMESPACE,
            {"model": {"uri": "hf://facebook/opt-125m", "name": "facebook/opt-125m"}},
        )

        create_llmisvc_config(
            kserve_client,
            workload_config_name,
            KSERVE_TEST_NAMESPACE,
            {
                "template": {
                    "containers": [
                        {
                            "name": "main",
                            "image": "quay.io/pierdipi/vllm-cpu:latest",
                            "resources": {
                                "limits": {"cpu": "2", "memory": "7Gi"},
                                "requests": {"cpu": "200m", "memory": "2Gi"},
                            },
                        }
                    ]
                }
            },
        )

        create_llmisvc_config(
            kserve_client,
            router_config_name,
            KSERVE_TEST_NAMESPACE,
            {"router": {"route": {}, "gateway": {}}},
        )

        # Create primary LLMISVC with valid config
        primary_llmisvc = V1alpha1LLMInferenceService(
            api_version="serving.kserve.io/v1alpha1",
            kind="LLMInferenceService",
            metadata=client.V1ObjectMeta(
                name=primary_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec={
                "baseRefs": [
                    {"name": model_config_name},
                    {"name": workload_config_name},
                    {"name": router_config_name},
                ]
            },
        )

        with managed_llmisvc(kserve_client, primary_llmisvc):
            # Step 1: Wait for primary LLMISVC to be ready
            logger.info("Created primary LLMISVC: %s", primary_name)
            wait_for_llm_isvc_ready(kserve_client, primary_llmisvc, timeout_seconds=900)
            logger.info("Primary LLMISVC is ready")

            # Record baseline resourceVersion
            primary_rv_before = get_llmisvc_resource_version(
                kserve_client, primary_name, KSERVE_TEST_NAMESPACE
            )
            logger.info("Baseline recorded - resourceVersion: %s", primary_rv_before)

            # Step 2: Create invalid S3 credentials
            logger.info("Creating invalid S3 secret and service account")
            create_invalid_s3_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
            create_service_account_with_secret(
                KSERVE_TEST_NAMESPACE, invalid_sa_name, invalid_secret_name
            )

            # Create config with invalid S3 model
            create_llmisvc_config(
                kserve_client,
                invalid_model_config_name,
                KSERVE_TEST_NAMESPACE,
                {
                    "model": {
                        "uri": "s3://nonexistent-bucket-12345/invalid/path/model",
                        "name": "invalid-model",
                    }
                },
            )

            # Step 3: Create secondary LLMISVC with invalid S3 credentials
            secondary_llmisvc = V1alpha1LLMInferenceService(
                api_version="serving.kserve.io/v1alpha1",
                kind="LLMInferenceService",
                metadata=client.V1ObjectMeta(
                    name=secondary_name, namespace=KSERVE_TEST_NAMESPACE
                ),
                spec={
                    "baseRefs": [
                        {"name": invalid_model_config_name},
                        {"name": workload_config_name},
                        {"name": router_config_name},
                    ]
                },
            )

            with managed_llmisvc(kserve_client, secondary_llmisvc):
                # Step 4: Wait for secondary LLMISVC to report failure
                logger.info(
                    "Created secondary LLMISVC %s, waiting for failure...",
                    secondary_name,
                )

                # Wait for WorkloadsReady condition to become False
                failure_condition = wait_for_llmisvc_condition_false(
                    kserve_client,
                    secondary_name,
                    "WorkloadsReady",
                    timeout_seconds=180,
                )
                if failure_condition:
                    logger.info("Secondary LLMISVC failure detected: %s", failure_condition)

                # Give time for any potential event storms to propagate
                await asyncio.sleep(10)

                # Step 5: Verify primary LLMISVC was not modified
                primary_rv_after = get_llmisvc_resource_version(
                    kserve_client, primary_name, KSERVE_TEST_NAMESPACE
                )
                logger.info(
                    "Primary LLMISVC resourceVersion: before=%s, after=%s",
                    primary_rv_before,
                    primary_rv_after,
                )

                assert primary_rv_before == primary_rv_after, (
                    f"Primary LLMISVC '{primary_name}' was modified during secondary LLMISVC failure. "
                    f"ResourceVersion changed from {primary_rv_before} to {primary_rv_after}. "
                    "This indicates potential event storm - init container status changes "
                    "on secondary LLMISVC may have triggered modification of unrelated primary LLMISVC."
                )

                logger.info(
                    "Event storm prevention validated: Primary LLMISVC was not modified "
                    "during secondary LLMISVC init container failures"
                )

    finally:
        # Cleanup non-LLMISVC resources
        delete_service_account(KSERVE_TEST_NAMESPACE, invalid_sa_name)
        delete_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        delete_llmisvc_config(kserve_client, model_config_name, KSERVE_TEST_NAMESPACE)
        delete_llmisvc_config(kserve_client, workload_config_name, KSERVE_TEST_NAMESPACE)
        delete_llmisvc_config(kserve_client, router_config_name, KSERVE_TEST_NAMESPACE)
        delete_llmisvc_config(
            kserve_client, invalid_model_config_name, KSERVE_TEST_NAMESPACE
        )


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(scope="session")
async def test_quick_reconciliation_on_init_container_failure():
    """
    Test that when an init container fails (e.g., invalid storage credentials),
    the owning LLMInferenceService quickly reconciles and reflects the failure in its status.

    This test:
    1. Creates an LLMISVC with invalid S3 credentials
    2. Monitors the LLMISVC conditions for failure detection
    3. Validates that failure is reflected in conditions within a reasonable timeframe
    4. Verifies the condition message contains relevant error information
    """
    inject_k8s_proxy()

    suffix = str(uuid.uuid4())[:6]
    llmisvc_name = f"llmisvc-init-fail-{suffix}"
    invalid_sa_name = f"fail-s3-sa-{suffix}"
    invalid_secret_name = f"fail-s3-secret-{suffix}"

    # Config names
    invalid_model_config_name = f"fail-model-config-{suffix}"
    workload_config_name = f"fail-workload-config-{suffix}"
    router_config_name = f"fail-router-config-{suffix}"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    try:
        # Create invalid S3 credentials
        logger.info("Creating invalid S3 secret and service account")
        create_invalid_s3_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        create_service_account_with_secret(
            KSERVE_TEST_NAMESPACE, invalid_sa_name, invalid_secret_name
        )

        # Create configs
        create_llmisvc_config(
            kserve_client,
            invalid_model_config_name,
            KSERVE_TEST_NAMESPACE,
            {
                "model": {
                    "uri": "s3://nonexistent-bucket-xyz123/invalid/model",
                    "name": "invalid-model",
                }
            },
        )

        create_llmisvc_config(
            kserve_client,
            workload_config_name,
            KSERVE_TEST_NAMESPACE,
            {
                "template": {
                    "containers": [
                        {
                            "name": "main",
                            "image": "quay.io/pierdipi/vllm-cpu:latest",
                            "resources": {
                                "limits": {"cpu": "2", "memory": "7Gi"},
                                "requests": {"cpu": "200m", "memory": "2Gi"},
                            },
                        }
                    ]
                }
            },
        )

        create_llmisvc_config(
            kserve_client,
            router_config_name,
            KSERVE_TEST_NAMESPACE,
            {"router": {"route": {}, "gateway": {}}},
        )

        # Create LLMISVC with invalid S3 credentials
        llmisvc = V1alpha1LLMInferenceService(
            api_version="serving.kserve.io/v1alpha1",
            kind="LLMInferenceService",
            metadata=client.V1ObjectMeta(
                name=llmisvc_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec={
                "baseRefs": [
                    {"name": invalid_model_config_name},
                    {"name": workload_config_name},
                    {"name": router_config_name},
                ]
            },
        )

        creation_time = time.time()
        with managed_llmisvc(kserve_client, llmisvc):
            # Wait for failure to be reflected in conditions
            logger.info("Created LLMISVC %s, waiting for failure...", llmisvc_name)

            # The pod watch should trigger quick reconciliation when init container fails
            failure_condition = wait_for_llmisvc_condition_false(
                kserve_client,
                llmisvc_name,
                "WorkloadsReady",
                timeout_seconds=180,
                poll_interval=5.0,
            )

            failure_detection_time = time.time()
            time_to_failure = failure_detection_time - creation_time

            # Validate failure was detected
            assert failure_condition is not None, (
                f"LLMISVC {llmisvc_name} did not report WorkloadsReady=False within timeout. "
                f"The init container failure should trigger quick reconciliation and status update."
            )

            logger.info(
                "Failure detected in %.2f seconds: %s",
                time_to_failure,
                failure_condition,
            )

            # Check Ready condition as well
            conditions = get_llmisvc_conditions(
                kserve_client, llmisvc_name, KSERVE_TEST_NAMESPACE
            )
            ready_condition = get_condition_by_type(conditions, "Ready")

            if ready_condition:
                logger.info("Ready condition: %s", ready_condition)
                # The service should not be ready due to workload failure
                assert (
                    ready_condition.get("status") != "True"
                ), "LLMISVC should not be Ready when init container fails"

            # Validate reasonable time to failure detection
            assert time_to_failure < 180, (
                f"Failure detection took too long ({time_to_failure:.2f}s). "
                f"Pod watch should trigger quick reconciliation on init container failure."
            )

            logger.info(
                "Quick reconciliation validated: Failure detected in %.2f seconds",
                time_to_failure,
            )

    finally:
        # Cleanup non-LLMISVC resources
        delete_service_account(KSERVE_TEST_NAMESPACE, invalid_sa_name)
        delete_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        delete_llmisvc_config(
            kserve_client, invalid_model_config_name, KSERVE_TEST_NAMESPACE
        )
        delete_llmisvc_config(kserve_client, workload_config_name, KSERVE_TEST_NAMESPACE)
        delete_llmisvc_config(kserve_client, router_config_name, KSERVE_TEST_NAMESPACE)
