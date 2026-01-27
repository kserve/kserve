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
import json
import os
import time
import uuid
from contextlib import contextmanager

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    constants,
)
from kserve.logging import trace_logger as logger

from ..common.utils import KSERVE_TEST_NAMESPACE


def get_isvc_data(kserve_client: KServeClient, name: str, namespace: str):
    """Get ISVC resource data for debugging."""
    try:
        return kserve_client.get(name, namespace=namespace)
    except Exception as e:
        return {"error": f"Failed to get ISVC {name}: {e}"}


def get_deployments_for_isvc(name: str, namespace: str) -> list[dict]:
    """Get deployments matching the ISVC."""
    apps_api = client.AppsV1Api()
    try:
        deployments = apps_api.list_namespaced_deployment(
            namespace=namespace,
            label_selector=f"serving.kserve.io/inferenceservice={name}",
        )
        return [dep.to_dict() for dep in deployments.items]
    except Exception as e:
        return [{"error": f"Failed to list deployments for ISVC {name}: {e}"}]


def get_pods_for_isvc(name: str, namespace: str) -> list[dict]:
    """Get pods matching the ISVC."""
    core_api = client.CoreV1Api()
    try:
        pods = core_api.list_namespaced_pod(
            namespace=namespace,
            label_selector=f"serving.kserve.io/inferenceservice={name}",
        )
        return [pod.to_dict() for pod in pods.items]
    except Exception as e:
        return [{"error": f"Failed to list pods for ISVC {name}: {e}"}]


def get_controller_logs_for_isvc(name: str, namespace: str) -> list[dict]:
    """Get controller log entries for a specific ISVC."""
    try:
        logs = get_controller_logs(since_seconds=300)  # Last 5 minutes
        entries = []
        for line in logs.strip().split("\n"):
            if not line:
                continue
            try:
                entry = json.loads(line)
                if entry.get("isvc") == name and entry.get("namespace") == namespace:
                    entries.append(entry)
            except json.JSONDecodeError:
                if name in line and namespace in line:
                    entries.append({"raw": line})
        return entries
    except Exception as e:
        return [{"error": f"Failed to get controller logs: {e}"}]


def dump_debug_info(
    kserve_client: KServeClient, isvc_names: list[str], namespace: str
) -> None:
    """Dump debug info for the given ISVCs, their deployments, pods, and logs as compact JSON."""
    for isvc_name in isvc_names:
        debug_data = {
            "isvc": get_isvc_data(kserve_client, isvc_name, namespace),
            "deployments": get_deployments_for_isvc(isvc_name, namespace),
            "pods": get_pods_for_isvc(isvc_name, namespace),
            "controller_logs": get_controller_logs_for_isvc(isvc_name, namespace),
        }
        logger.info(
            "DEBUG DUMP %s/%s:\n%s",
            namespace,
            isvc_name,
            json.dumps(debug_data, separators=(",", ":"), default=str),
        )


@contextmanager
def managed_isvc(kserve_client: KServeClient, isvc: V1beta1InferenceService):
    """
    Context manager that handles ISVC lifecycle: creation, error dumping, and cleanup.

    Usage:
        with managed_isvc(kserve_client, isvc):
            # ISVC is already created
            # ... test logic ...
            # On any exception: dumps debug info for the ISVC
            # On exit: deletes the ISVC
    """
    assert isvc.metadata is not None, "ISVC must have metadata"
    assert isvc.metadata.name is not None, "ISVC must have a name"
    assert isvc.metadata.namespace is not None, "ISVC must have a namespace"
    name = isvc.metadata.name
    namespace = isvc.metadata.namespace
    error_occurred = False
    try:
        kserve_client.create(isvc)
        yield
    except Exception:
        error_occurred = True
        dump_debug_info(kserve_client, [name], namespace)
        raise
    finally:
        try:
            kserve_client.delete(name, namespace)
        except Exception as e:
            if not error_occurred:
                logger.warning("Failed to delete ISVC %s: %s", name, e)


def get_isvc_resource_version(
    kserve_client: KServeClient, name: str, namespace: str = KSERVE_TEST_NAMESPACE
) -> str:
    isvc = kserve_client.get(name, namespace=namespace)
    metadata = isvc.get("metadata") if isinstance(isvc, dict) else {}
    if isinstance(metadata, dict):
        return str(metadata.get("resourceVersion", ""))
    return ""


def get_isvc_model_status(
    kserve_client: KServeClient, name: str, namespace: str = KSERVE_TEST_NAMESPACE
) -> dict:
    isvc = kserve_client.get(name, namespace=namespace)
    status = isvc.get("status") if isinstance(isvc, dict) else {}
    if isinstance(status, dict):
        model_status = status.get("modelStatus")
        return model_status if isinstance(model_status, dict) else {}
    return {}


def get_isvc_conditions(
    kserve_client: KServeClient, name: str, namespace: str = KSERVE_TEST_NAMESPACE
) -> list:
    isvc = kserve_client.get(name, namespace=namespace)
    status = isvc.get("status") if isinstance(isvc, dict) else {}
    if isinstance(status, dict):
        conditions = status.get("conditions")
        return conditions if isinstance(conditions, list) else []
    return []


def create_invalid_s3_secret(namespace: str, secret_name: str):
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
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_secret(secret_name, namespace)
    except client.ApiException:
        pass


def delete_service_account(namespace: str, sa_name: str):
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_service_account(sa_name, namespace)
    except client.ApiException:
        pass


def wait_for_isvc_failure_status(
    kserve_client: KServeClient,
    name: str,
    namespace: str = KSERVE_TEST_NAMESPACE,
    timeout_seconds: int = 120,
    poll_interval: float = 2.0,
) -> dict | None:
    start_time = time.time()
    while time.time() - start_time < timeout_seconds:
        try:
            model_status = get_isvc_model_status(kserve_client, name, namespace)
            last_failure = model_status.get("lastFailureInfo")
            if last_failure is not None:
                logger.info(
                    "ISVC %s reported failure: %s",
                    name,
                    last_failure,
                )
                return model_status
        except Exception as e:
            logger.warning("Error checking ISVC status: %s", e)
        time.sleep(poll_interval)
    return None


def get_controller_logs(since_seconds: int) -> str:
    core_api = client.CoreV1Api()
    pods = core_api.list_namespaced_pod(
        namespace="kserve",
        label_selector="control-plane=kserve-controller-manager",
    )
    if not pods.items:
        raise RuntimeError(
            "No controller manager pod found in kserve namespace. "
            "Cannot perform log analysis for reconciliation detection."
        )
    pod = pods.items[0]
    try:
        return core_api.read_namespaced_pod_log(
            name=pod.metadata.name,
            namespace="kserve",
            container="manager",
            since_seconds=since_seconds,
        )
    except client.ApiException as e:
        raise RuntimeError(
            f"Failed to read controller logs from pod {pod.metadata.name}: {e}"
        ) from e


@pytest.mark.predictor
@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_event_storm_prevention_init_container_isolation(rest_v1_client):
    """
    Test that init container status changes on one ISVC don't cause unwanted modifications
    to unrelated ISVCs (event storm prevention).

    The controller may reconcile an ISVC for legitimate reasons (e.g.,
    HTTPRoute status updates from Istio, deployment status changes) without making any
    changes. This is acceptable. The real concern is if the secondary ISVC's events
    cause the primary ISVC to be MODIFIED (resourceVersion change).

    Test flow:
    1. Creates a "primary" ISVC that will successfully load a model from GCS
    2. Waits for the primary ISVC to become ready
    3. Records baseline resourceVersion
    4. Creates a "secondary" ISVC with invalid S3 credentials that will fail
    5. Waits for the secondary ISVC to show failure status
    6. Verifies the primary ISVC's resourceVersion is unchanged
    """
    suffix = str(uuid.uuid4())[:6]
    primary_name = f"isvc-primary-{suffix}"
    secondary_name = f"isvc-secondary-{suffix}"
    invalid_sa_name = f"invalid-s3-sa-{suffix}"
    invalid_secret_name = f"invalid-s3-secret-{suffix}"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    # Create primary ISVC with a valid GCS storage URI (no credentials needed)
    primary_predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    primary_isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=primary_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=primary_predictor),
    )

    with managed_isvc(kserve_client, primary_isvc):
        # Step 1: Wait for primary ISVC to be ready (created by managed_isvc)
        logger.info("Created primary ISVC: %s", primary_name)
        kserve_client.wait_isvc_ready(primary_name, namespace=KSERVE_TEST_NAMESPACE)
        logger.info("Primary ISVC is ready")

        # Record baseline resourceVersion
        primary_rv_before = get_isvc_resource_version(kserve_client, primary_name)
        logger.info("Baseline recorded - resourceVersion: %s", primary_rv_before)

        # Step 2: Create invalid S3 credentials
        logger.info("Creating invalid S3 secret and service account")
        create_invalid_s3_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        create_service_account_with_secret(
            KSERVE_TEST_NAMESPACE, invalid_sa_name, invalid_secret_name
        )

        # Step 3: Create secondary ISVC with invalid S3 credentials
        secondary_predictor = V1beta1PredictorSpec(
            min_replicas=1,
            service_account_name=invalid_sa_name,
            sklearn=V1beta1SKLearnSpec(
                storage_uri="s3://nonexistent-bucket-12345/invalid/path/model",
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )

        secondary_isvc = V1beta1InferenceService(
            api_version=constants.KSERVE_V1BETA1,
            kind=constants.KSERVE_KIND_INFERENCESERVICE,
            metadata=client.V1ObjectMeta(
                name=secondary_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec=V1beta1InferenceServiceSpec(predictor=secondary_predictor),
        )

        with managed_isvc(kserve_client, secondary_isvc):
            # Step 4: Wait for secondary ISVC to report failure (created by managed_isvc)
            logger.info(
                "Created secondary ISVC %s, waiting for failure status...",
                secondary_name,
            )
            secondary_failure = wait_for_isvc_failure_status(
                kserve_client, secondary_name, timeout_seconds=180
            )
            if secondary_failure:
                logger.info("Secondary ISVC failure detected: %s", secondary_failure)

            # Give time for any potential event storms to propagate
            await asyncio.sleep(10)

            # Step 5: Verify primary ISVC was not modified
            # The controller may reconcile the primary ISVC for legitimate reasons
            # (e.g., HTTPRoute status updates from Istio), but no-op reconciliations
            # are fine. Only fail if resourceVersion changed (actual modification).
            primary_rv_after = get_isvc_resource_version(kserve_client, primary_name)
            logger.info(
                "Primary ISVC resourceVersion: before=%s, after=%s",
                primary_rv_before,
                primary_rv_after,
            )

            assert primary_rv_before == primary_rv_after, (
                f"Primary ISVC '{primary_name}' was modified during secondary ISVC failure. "
                f"ResourceVersion changed from {primary_rv_before} to {primary_rv_after}. "
                "This indicates potential event storm - init container status changes "
                "on secondary ISVC may have triggered modification of unrelated primary ISVC."
            )

            logger.info(
                "Event storm prevention validated: Primary ISVC was not modified "
                "during secondary ISVC init container failures"
            )


@pytest.mark.predictor
@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_quick_reconciliation_on_init_container_failure():
    """
    Test that when an init container fails (e.g., invalid storage credentials),
    the owning InferenceService quickly reconciles and reflects the failure in its status.

    This test:
    1. Creates an ISVC with invalid S3 credentials
    2. Monitors the ISVC status for failure detection
    3. Validates that failure status is populated within a reasonable timeframe
    4. Verifies the failure message contains relevant error information
    """
    suffix = str(uuid.uuid4())[:6]
    isvc_name = f"isvc-init-fail-{suffix}"
    invalid_sa_name = f"fail-s3-sa-{suffix}"
    invalid_secret_name = f"fail-s3-secret-{suffix}"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    try:
        # Create invalid S3 credentials
        logger.info("Creating invalid S3 secret and service account")
        create_invalid_s3_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        create_service_account_with_secret(
            KSERVE_TEST_NAMESPACE, invalid_sa_name, invalid_secret_name
        )

        # Create ISVC with invalid S3 credentials
        predictor = V1beta1PredictorSpec(
            min_replicas=1,
            service_account_name=invalid_sa_name,
            sklearn=V1beta1SKLearnSpec(
                storage_uri="s3://nonexistent-bucket-xyz123/invalid/model",
                resources=V1ResourceRequirements(
                    requests={"cpu": "50m", "memory": "128Mi"},
                    limits={"cpu": "100m", "memory": "256Mi"},
                ),
            ),
        )

        isvc = V1beta1InferenceService(
            api_version=constants.KSERVE_V1BETA1,
            kind=constants.KSERVE_KIND_INFERENCESERVICE,
            metadata=client.V1ObjectMeta(
                name=isvc_name, namespace=KSERVE_TEST_NAMESPACE
            ),
            spec=V1beta1InferenceServiceSpec(predictor=predictor),
        )

        creation_time = time.time()
        with managed_isvc(kserve_client, isvc):
            # Wait for failure status to be populated
            logger.info("Created ISVC %s, waiting for failure status...", isvc_name)
            failure_status = wait_for_isvc_failure_status(
                kserve_client, isvc_name, timeout_seconds=180, poll_interval=5.0
            )

            failure_detection_time = time.time()
            time_to_failure = failure_detection_time - creation_time

            # Validate failure was detected
            assert failure_status is not None, (
                f"ISVC {isvc_name} did not report failure status within timeout. "
                f"The init container failure should trigger quick reconciliation and status update."
            )

            logger.info(
                "Failure status detected in %.2f seconds: %s",
                time_to_failure,
                failure_status,
            )

            # Validate failure info contains expected fields
            last_failure = failure_status.get("lastFailureInfo", {})
            assert (
                last_failure.get("reason") is not None
            ), "lastFailureInfo.reason should be populated"

            # The transition status should indicate blocked by failed load
            transition_status = failure_status.get("transitionStatus")
            logger.info("Transition status: %s", transition_status)

            # Check conditions for failure indication
            conditions = get_isvc_conditions(kserve_client, isvc_name)
            ready_condition = next(
                (c for c in conditions if c.get("type") == "Ready"), None
            )

            if ready_condition:
                logger.info("Ready condition: %s", ready_condition)
                # The service should not be ready due to init container failure
                assert (
                    ready_condition.get("status") != "True"
                ), "ISVC should not be Ready when init container fails"

            # Validate reasonable time to failure detection
            # The pod watch should trigger reconciliation quickly when init container status changes
            assert time_to_failure < 180, (
                f"Failure detection took too long ({time_to_failure:.2f}s). "
                f"Pod watch should trigger quick reconciliation on init container failure."
            )

            logger.info(
                "Quick reconciliation validated: Failure detected in %.2f seconds",
                time_to_failure,
            )

    finally:
        # Cleanup non-ISVC resources (ISVCs are cleaned up by managed_isvc)
        delete_service_account(KSERVE_TEST_NAMESPACE, invalid_sa_name)
        delete_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
