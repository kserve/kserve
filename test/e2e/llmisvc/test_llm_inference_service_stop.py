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
from kserve import KServeClient, V1alpha1LLMInferenceService, constants
from kubernetes import client

from .fixtures import (  # noqa: F401, F811
    generate_test_id,
    inject_k8s_proxy,
    test_case,
)
from .logging import log_execution
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    delete_llmisvc,
    get_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
)

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
STOP_ANNOTATION_KEY = "serving.kserve.io/stop"


@pytest.mark.llminferenceservice
@pytest.mark.asyncio(loop_scope="session")
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                service_name="stop-feature-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_stop_feature(test_case: TestCase):  # noqa: F811
    """Test that stopping an LLMInferenceService sets the Ready condition to False with reason Stopped."""
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name
    test_failed = False

    # Disable auth for this test
    if not test_case.llm_service.metadata.annotations:
        test_case.llm_service.metadata.annotations = {}
    test_case.llm_service.metadata.annotations[
        "security.opendatahub.io/enable-auth"
    ] = "false"

    try:
        # Create the service
        print(f"Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, test_case.llm_service)

        # Wait for the service to be ready
        print(f"Waiting for LLMInferenceService {service_name} to be ready")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        print(f"✅ LLMInferenceService {service_name} is ready")

        # Stop the service by adding the stop annotation
        print(f"Stopping LLMInferenceService {service_name}")
        stop_llmisvc(kserve_client, test_case.llm_service)

        # Wait for the service to be marked as stopped
        print(f"Waiting for LLMInferenceService {service_name} to be stopped")
        wait_for_llm_isvc_stopped(
            kserve_client, test_case.llm_service, timeout_seconds=120
        )
        print(f"✅ LLMInferenceService {service_name} is stopped")

        # Verify the workload resources are deleted
        print(f"Verifying workload resources are deleted for {service_name}")
        verify_workload_resources_deleted(
            kserve_client, test_case.llm_service, timeout_seconds=120
        )
        print(f"✅ Workload resources deleted for {service_name}")

        # Restart the service by removing the stop annotation
        print(f"Restarting LLMInferenceService {service_name}")
        restart_llmisvc(kserve_client, test_case.llm_service)

        # Wait for the service to be ready again
        print(f"Waiting for LLMInferenceService {service_name} to be ready again")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        print(f"✅ LLMInferenceService {service_name} is ready again after restart")

    except Exception as e:
        test_failed = True
        print(f"❌ ERROR: Stop feature test failed for {service_name}: {e}")
        raise
    finally:
        try:
            skip_all_deletion = os.getenv(
                "SKIP_RESOURCE_DELETION", "False"
            ).lower() in (
                "true",
                "1",
                "t",
            )
            skip_deletion_on_failure = os.getenv(
                "SKIP_DELETION_ON_FAILURE", "False"
            ).lower() in (
                "true",
                "1",
                "t",
            )

            should_skip_deletion = skip_all_deletion or (
                skip_deletion_on_failure and test_failed
            )

            if not should_skip_deletion:
                delete_llmisvc(kserve_client, test_case.llm_service)
            elif test_failed and skip_deletion_on_failure:
                print(
                    f"⏭️  Skipping deletion of {service_name} due to test failure (SKIP_DELETION_ON_FAILURE=True)"
                )
        except Exception as e:
            print(f"⚠️ Warning: Failed to cleanup service {service_name}: {e}")


@log_execution
def stop_llmisvc(kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService):
    """Add the stop annotation to the LLMInferenceService."""
    try:
        # Get the current service
        current = kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            llm_isvc.metadata.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc.metadata.name,
        )

        # Add the stop annotation
        if "metadata" not in current:
            current["metadata"] = {}
        if "annotations" not in current["metadata"]:
            current["metadata"]["annotations"] = {}
        current["metadata"]["annotations"][STOP_ANNOTATION_KEY] = "true"

        # Patch the service
        result = kserve_client.api_instance.patch_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            llm_isvc.metadata.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc.metadata.name,
            current,
        )
        print(f"✅ LLM inference service {llm_isvc.metadata.name} stopped successfully")
        return result
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception when calling CustomObjectsApi->"
            f"patch_namespaced_custom_object for LLMInferenceService: {e}"
        ) from e


@log_execution
def restart_llmisvc(kserve_client: KServeClient, llm_isvc: V1alpha1LLMInferenceService):
    """Remove the stop annotation from the LLMInferenceService."""
    try:
        # Get the current service
        current = kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            llm_isvc.metadata.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc.metadata.name,
        )

        # Set the stop annotation to 'false'
        if "metadata" not in current:
            current["metadata"] = {}
        if "annotations" not in current["metadata"]:
            current["metadata"]["annotations"] = {}
        current["metadata"]["annotations"][STOP_ANNOTATION_KEY] = "false"

        # Patch the service
        result = kserve_client.api_instance.patch_namespaced_custom_object(
            constants.KSERVE_GROUP,
            llm_isvc.api_version.split("/")[1],
            llm_isvc.metadata.namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llm_isvc.metadata.name,
            current,
        )
        print(
            f"✅ LLM inference service {llm_isvc.metadata.name} restarted successfully"
        )
        return result
    except client.rest.ApiException as e:
        raise RuntimeError(
            f"❌ Exception when calling CustomObjectsApi->"
            f"patch_namespaced_custom_object for LLMInferenceService: {e}"
        ) from e


@log_execution
def wait_for_llm_isvc_stopped(
    kserve_client: KServeClient,
    given: V1alpha1LLMInferenceService,
    timeout_seconds: int = 120,
) -> bool:
    """Wait for the LLMInferenceService to be marked as stopped."""

    def assert_llm_isvc_stopped():
        out = get_llmisvc(
            kserve_client,
            given.metadata.name,
            given.metadata.namespace,
            given.api_version.split("/")[1],
        )

        if "status" not in out:
            raise AssertionError("No status found in LLM inference service")

        status = out["status"]
        if "conditions" not in status:
            raise AssertionError("No conditions found in status")

        conditions = status["conditions"]

        # Find the Ready condition
        ready_condition = None
        workload_ready_condition = None
        router_ready_condition = None
        main_workload_ready_condition = None

        for condition in conditions:
            cond_type = condition.get("type")
            if cond_type == "Ready":
                ready_condition = condition
            elif cond_type == "WorkloadsReady":
                workload_ready_condition = condition
            elif cond_type == "RouterReady":
                router_ready_condition = condition
            elif cond_type == "MainWorkloadReady":
                main_workload_ready_condition = condition

        # Verify Ready condition is False
        if ready_condition is None:
            raise AssertionError("Ready condition not found")

        if ready_condition.get("status") != "False":
            raise AssertionError(
                f"Ready condition status is not False: {ready_condition.get('status')}"
            )

        # Verify at least one of the workload conditions has reason "Stopped"
        stopped_conditions = []
        for cond in [
            workload_ready_condition,
            router_ready_condition,
            main_workload_ready_condition,
        ]:
            if cond and cond.get("reason") == "Stopped":
                stopped_conditions.append(cond.get("type"))

        if not stopped_conditions:
            raise AssertionError(
                f"None of the workload conditions have reason 'Stopped'. Conditions: {conditions}"
            )

        print(
            f"✅ Service is stopped. Conditions with reason 'Stopped': {stopped_conditions}"
        )
        return True

    return wait_for(assert_llm_isvc_stopped, timeout=timeout_seconds, interval=2.0)


@log_execution
def verify_workload_resources_deleted(
    kserve_client: KServeClient,
    llm_isvc: V1alpha1LLMInferenceService,
    timeout_seconds: int = 120,
) -> bool:
    """Verify that workload resources (deployments) are deleted when service is stopped."""

    def assert_deployment_deleted():
        core_v1 = client.CoreV1Api()
        apps_v1 = client.AppsV1Api()

        namespace = llm_isvc.metadata.namespace
        service_name = llm_isvc.metadata.name

        # Check if the main deployment is deleted
        deployment_name = f"{service_name}-kserve"
        try:
            apps_v1.read_namespaced_deployment(deployment_name, namespace)
            raise AssertionError(
                f"Deployment {deployment_name} still exists but should be deleted"
            )
        except client.rest.ApiException as e:
            if e.status != 404:
                raise AssertionError(
                    f"Unexpected error checking deployment {deployment_name}: {e}"
                )
            # 404 is expected - deployment is deleted
            print(f"✅ Deployment {deployment_name} is deleted")

        # Check if the workload service is deleted
        workload_service_name = f"{service_name}-kserve-workload-svc"
        try:
            core_v1.read_namespaced_service(workload_service_name, namespace)
            raise AssertionError(
                f"Service {workload_service_name} still exists but should be deleted"
            )
        except client.rest.ApiException as e:
            if e.status != 404:
                raise AssertionError(
                    f"Unexpected error checking service {workload_service_name}: {e}"
                )
            # 404 is expected - service is deleted
            print(f"✅ Service {workload_service_name} is deleted")

        return True

    return wait_for(assert_deployment_deleted, timeout=timeout_seconds, interval=2.0)
