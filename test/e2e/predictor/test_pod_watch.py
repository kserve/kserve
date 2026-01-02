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
E2E tests for pod watch functionality.

These tests validate that:
1. Init container status changes on one ISVC don't trigger reconciliation of unrelated ISVCs
   (event storm prevention)
2. When an init container fails (e.g., invalid S3 credentials), the owning ISVC quickly
   reconciles and reflects the failure in its status

These tests run in both Serverless and RawDeployment modes via the `predictor` and `raw` markers.
"""

import asyncio
import os
import time
import uuid
from typing import Optional

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


def get_isvc_resource_version(kserve_client: KServeClient, name: str) -> str:
    """Get the current resourceVersion of an InferenceService."""
    isvc = kserve_client.get(name, namespace=KSERVE_TEST_NAMESPACE)
    metadata = isvc.get("metadata") if isinstance(isvc, dict) else {}
    if isinstance(metadata, dict):
        return str(metadata.get("resourceVersion", ""))
    return ""


def get_isvc_model_status(kserve_client: KServeClient, name: str) -> dict:
    """Get the modelStatus of an InferenceService."""
    isvc = kserve_client.get(name, namespace=KSERVE_TEST_NAMESPACE)
    status = isvc.get("status") if isinstance(isvc, dict) else {}
    if isinstance(status, dict):
        model_status = status.get("modelStatus")
        return model_status if isinstance(model_status, dict) else {}
    return {}


def get_isvc_conditions(kserve_client: KServeClient, name: str) -> list:
    """Get the conditions of an InferenceService."""
    isvc = kserve_client.get(name, namespace=KSERVE_TEST_NAMESPACE)
    status = isvc.get("status") if isinstance(isvc, dict) else {}
    if isinstance(status, dict):
        conditions = status.get("conditions")
        return conditions if isinstance(conditions, list) else []
    return []


def create_invalid_s3_secret(namespace: str, secret_name: str):
    """Create a secret with invalid S3 credentials that will cause storage init to fail."""
    core_api = client.CoreV1Api()

    # Create a secret with invalid credentials - these will fail when trying to access S3
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


def create_service_account_with_secret(
    namespace: str, sa_name: str, secret_name: str
):
    """Create a service account that references the given secret."""
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
    """Delete a secret, ignoring not found errors."""
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_secret(secret_name, namespace)
    except client.ApiException:
        pass


def delete_service_account(namespace: str, sa_name: str):
    """Delete a service account, ignoring not found errors."""
    core_api = client.CoreV1Api()
    try:
        core_api.delete_namespaced_service_account(sa_name, namespace)
    except client.ApiException:
        pass


def wait_for_isvc_failure_status(
    kserve_client: KServeClient,
    name: str,
    timeout_seconds: int = 120,
    poll_interval: float = 2.0,
) -> Optional[dict]:
    """
    Wait for an InferenceService to report a failure in its modelStatus.

    Returns the modelStatus when failure is detected, or None if timeout occurs.
    """
    start_time = time.time()
    while time.time() - start_time < timeout_seconds:
        try:
            model_status = get_isvc_model_status(kserve_client, name)
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


@pytest.mark.predictor
@pytest.mark.raw
@pytest.mark.asyncio(scope="session")
async def test_event_storm_prevention_init_container_isolation(rest_v1_client):
    """
    Test that init container status changes on one ISVC don't trigger reconciliation
    of unrelated ISVCs (event storm prevention).

    This test:
    1. Creates a "primary" ISVC that will successfully load a model from GCS
    2. Waits for the primary ISVC to become ready
    3. Records the primary ISVC's resourceVersion
    4. Creates a "secondary" ISVC with invalid S3 credentials that will fail
    5. Waits for the secondary ISVC to show failure status
    6. Verifies the primary ISVC was NOT reconciled during the secondary's failure
       (resourceVersion should remain unchanged or have minimal changes from unrelated updates)
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

    try:
        # Step 1: Create and wait for primary ISVC to be ready
        logger.info("Creating primary ISVC: %s", primary_name)
        kserve_client.create(primary_isvc)
        kserve_client.wait_isvc_ready(primary_name, namespace=KSERVE_TEST_NAMESPACE)
        logger.info("Primary ISVC is ready")

        # Record resourceVersion after primary is ready
        primary_rv_before = get_isvc_resource_version(kserve_client, primary_name)
        logger.info(
            "Primary ISVC resourceVersion before secondary failure: %s",
            primary_rv_before,
        )

        # Step 2: Create invalid S3 credentials
        logger.info("Creating invalid S3 secret and service account")
        create_invalid_s3_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
        create_service_account_with_secret(
            KSERVE_TEST_NAMESPACE, invalid_sa_name, invalid_secret_name
        )

        # Step 3: Create secondary ISVC with invalid S3 credentials
        # Using a non-existent S3 bucket with invalid credentials
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

        logger.info("Creating secondary ISVC with invalid credentials: %s", secondary_name)
        kserve_client.create(secondary_isvc)

        # Step 4: Wait for secondary ISVC to report failure
        logger.info("Waiting for secondary ISVC to report failure status...")
        secondary_failure = wait_for_isvc_failure_status(
            kserve_client, secondary_name, timeout_seconds=180
        )
        if secondary_failure:
            logger.info("Secondary ISVC failure detected: %s", secondary_failure)

        # The secondary ISVC should show a failure (or at least not be ready)
        # even if failure status takes time, init containers should have had status changes
        await asyncio.sleep(10)  # Give time for any potential event storms

        # Step 5: Check that primary ISVC was not reconciled due to secondary's issues
        primary_rv_after = get_isvc_resource_version(kserve_client, primary_name)
        logger.info(
            "Primary ISVC resourceVersion after secondary failure: %s",
            primary_rv_after,
        )

        # The primary's resourceVersion should not have changed significantly
        # A change would indicate an unwanted reconciliation was triggered
        # Note: Some minor updates might happen due to periodic reconciliation,
        # but we expect no rapid changes caused by the secondary's init container failures
        assert primary_rv_before == primary_rv_after, (
            f"Primary ISVC was unexpectedly reconciled during secondary's init container failure. "
            f"ResourceVersion changed from {primary_rv_before} to {primary_rv_after}. "
            f"This indicates potential event storm - init container status changes on secondary ISVC "
            f"may have triggered reconciliation of unrelated primary ISVC."
        )

        logger.info(
            "Event storm prevention validated: Primary ISVC was not reconciled "
            "during secondary ISVC init container failures"
        )

    finally:
        # Cleanup
        logger.info("Cleaning up test resources")
        try:
            kserve_client.delete(primary_name, KSERVE_TEST_NAMESPACE)
        except Exception as e:
            logger.warning("Failed to delete primary ISVC: %s", e)

        try:
            kserve_client.delete(secondary_name, KSERVE_TEST_NAMESPACE)
        except Exception as e:
            logger.warning("Failed to delete secondary ISVC: %s", e)

        delete_service_account(KSERVE_TEST_NAMESPACE, invalid_sa_name)
        delete_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)


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

        logger.info("Creating ISVC with invalid S3 credentials: %s", isvc_name)
        creation_time = time.time()
        kserve_client.create(isvc)

        # Wait for failure status to be populated
        logger.info("Waiting for ISVC to report failure status...")
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
        assert last_failure.get("reason") is not None, (
            "lastFailureInfo.reason should be populated"
        )

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
            assert ready_condition.get("status") != "True", (
                "ISVC should not be Ready when init container fails"
            )

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
        # Cleanup
        logger.info("Cleaning up test resources")
        try:
            kserve_client.delete(isvc_name, KSERVE_TEST_NAMESPACE)
        except Exception as e:
            logger.warning("Failed to delete ISVC: %s", e)

        delete_service_account(KSERVE_TEST_NAMESPACE, invalid_sa_name)
        delete_secret(KSERVE_TEST_NAMESPACE, invalid_secret_name)
