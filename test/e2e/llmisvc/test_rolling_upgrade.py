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

"""Test rolling upgrade coordination: workload rolls out before EPP is updated."""

from __future__ import annotations

import os
import pytest
from kserve import KServeClient
from kubernetes import client

from .fixtures import (
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_inference_service import (
    KSERVE_PLURAL_LLMINFERENCESERVICE,
    TestCase,
    create_llmisvc,
    get_llmisvc,
    maybe_delete_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)


@pytest.mark.llminferenceservice
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-llmd-simulator",
                    "model-fb-opt-125m",
                ],
                prompt="KServe is a",
                service_name="rolling-upgrade-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_rolling_upgrade_coordination(test_case: TestCase):
    """
    Verify the service recovers cleanly after a workload rolling update.

    Triggers a rolling update by patching a pod-template annotation on the
    LLMInferenceService. After the rollout completes the workload Deployment
    must be Available and the overall service must be Ready and able to serve
    inference requests — confirming the workload-before-EPP update ordering
    left the system in a healthy state.
    """
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name
    namespace = test_case.llm_service.metadata.namespace
    test_failed = False

    try:
        print(f"Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, test_case.llm_service)

        print(f"Waiting for {service_name} to be ready")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )

        apps_v1 = client.AppsV1Api()
        workload_name = f"{service_name}-kserve"

        initial = apps_v1.read_namespaced_deployment(workload_name, namespace)
        initial_generation = initial.metadata.generation
        print(f"Workload Deployment initial generation: {initial_generation}")

        print("Triggering workload rolling update via pod-template annotation patch")
        _patch_rolling_update_trigger(kserve_client, test_case.llm_service)

        print("Waiting for workload rollout to start (generation increment)")
        wait_for_rollout_started(
            apps_v1, workload_name, namespace, initial_generation, timeout_seconds=60
        )

        print("Waiting for workload rollout to complete (Deployment Available)")
        wait_for_deployment_available(
            apps_v1, workload_name, namespace, timeout_seconds=120
        )
        print("Workload rollout complete")

        print(f"Waiting for {service_name} to be Ready after rollout")
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )

        print("Verifying inference still works after rolling update")
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)
        print("✅ Rolling upgrade coordination verified: service healthy after rollout")

    except Exception as e:
        test_failed = True
        print(f"❌ Rolling upgrade coordination test failed for {service_name}: {e}")
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@log_execution
def _patch_rolling_update_trigger(
    kserve_client: KServeClient,
    llm_isvc,
):
    """Add a harmless env var to the main container to force a workload rolling update.

    spec.template is a PodSpec (no metadata field), so env var injection on the
    main container is the correct way to change the pod-template and trigger a
    new rollout without altering workload behaviour.
    """
    from kserve import constants

    current = get_llmisvc(
        kserve_client,
        llm_isvc.metadata.name,
        llm_isvc.metadata.namespace,
        llm_isvc.api_version.split("/")[1],
    )

    spec = current.setdefault("spec", {})
    template = spec.setdefault("template", {})
    containers = template.setdefault("containers", [])
    main = next((c for c in containers if c.get("name") == "main"), None)
    if main is None:
        main = {"name": "main"}
        containers.append(main)
    env = main.setdefault("env", [])
    trigger = next((e for e in env if e.get("name") == "TEST_ROLLING_UPDATE_TRIGGER"), None)
    if trigger:
        trigger["value"] = "v2"
    else:
        env.append({"name": "TEST_ROLLING_UPDATE_TRIGGER", "value": "v2"})

    kserve_client.api_instance.patch_namespaced_custom_object(
        constants.KSERVE_GROUP,
        llm_isvc.api_version.split("/")[1],
        llm_isvc.metadata.namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        llm_isvc.metadata.name,
        current,
    )


@log_execution
def wait_for_rollout_started(
    apps_v1: client.AppsV1Api,
    deployment_name: str,
    namespace: str,
    initial_generation: int,
    timeout_seconds: int = 60,
) -> None:
    """Block until the Deployment generation increments, signalling a new rollout."""

    def assert_generation_incremented():
        d = apps_v1.read_namespaced_deployment(deployment_name, namespace)
        assert d.metadata.generation > initial_generation, (
            f"Deployment {deployment_name} generation still at {d.metadata.generation}, "
            f"expected > {initial_generation}"
        )

    wait_for(assert_generation_incremented, timeout=timeout_seconds, interval=2.0)
    print(f"Rollout started for {deployment_name}")


@log_execution
def wait_for_deployment_available(
    apps_v1: client.AppsV1Api,
    deployment_name: str,
    namespace: str,
    timeout_seconds: int = 120,
) -> None:
    """Block until the Deployment's Available condition is True."""

    def assert_deployment_available():
        d = apps_v1.read_namespaced_deployment(deployment_name, namespace)
        conditions = d.status.conditions or []
        for cond in conditions:
            if cond.type == "Available" and cond.status == "True":
                return
        raise AssertionError(
            f"Deployment {deployment_name} not yet Available: "
            f"{[(c.type, c.status) for c in conditions]}"
        )

    wait_for(assert_deployment_available, timeout=timeout_seconds, interval=2.0)
