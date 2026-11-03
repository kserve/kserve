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

"""Test that the vLLM container preStop lifecycle hook fires during pod termination."""

from __future__ import annotations

import os
import time
import pytest
from kserve import KServeClient
from kubernetes import client

from .fixtures import (
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    delete_llmisvc,
    wait_for,
)

_PRESTOP_SLEEP_SECONDS = 15
_PRESTOP_TOLERANCE_SECONDS = 3


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
                service_name="prestop-hook-test",
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_prestop_hook(test_case: TestCase):
    """Verify the preStop lifecycle hook delays pod termination by at least 15 seconds."""
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

        workload_pod_name = wait_for_workload_pod_running(
            namespace, service_name, timeout_seconds=test_case.wait_timeout
        )
        print(f"Pod {workload_pod_name} is Running")

        elapsed = delete_pod_and_measure_termination(
            namespace, workload_pod_name, timeout_seconds=120
        )
        print(f"Pod terminated in {elapsed:.1f}s")

        assert elapsed >= _PRESTOP_SLEEP_SECONDS - _PRESTOP_TOLERANCE_SECONDS, (
            f"Pod {workload_pod_name} terminated in {elapsed:.1f}s, "
            f"expected >= {_PRESTOP_SLEEP_SECONDS}s — preStop hook may not have fired"
        )
        print(
            f"✅ preStop hook verified: pod took {elapsed:.1f}s to terminate "
            f"(expected >= {_PRESTOP_SLEEP_SECONDS}s)"
        )

    except Exception as e:
        test_failed = True
        print(f"❌ ERROR: preStop hook test failed for {service_name}: {e}")
        raise
    finally:
        try:
            skip_all_deletion = os.getenv(
                "SKIP_RESOURCE_DELETION", "False"
            ).lower() in ("true", "1", "t")
            skip_deletion_on_failure = os.getenv(
                "SKIP_DELETION_ON_FAILURE", "False"
            ).lower() in ("true", "1", "t")

            should_skip_deletion = skip_all_deletion or (
                skip_deletion_on_failure and test_failed
            )

            if not should_skip_deletion:
                delete_llmisvc(kserve_client, test_case.llm_service)
            elif test_failed and skip_deletion_on_failure:
                print(
                    f"⏭️  Skipping deletion of {service_name} due to test failure "
                    f"(SKIP_DELETION_ON_FAILURE=True)"
                )
        except Exception as e:
            print(f"⚠️ Warning: Failed to cleanup service {service_name}: {e}")


@log_execution
def wait_for_workload_pod_running(
    namespace: str,
    service_name: str,
    timeout_seconds: int = 900,
) -> str:
    """Wait for the main workload pod to reach Running state, return its name."""
    core_v1 = client.CoreV1Api()
    label_selector = (
        f"app.kubernetes.io/name={service_name},kserve.io/component=workload"
    )

    def assert_pod_running() -> str:
        pods = core_v1.list_namespaced_pod(namespace, label_selector=label_selector)
        running = [
            p
            for p in pods.items
            if p.status.phase == "Running"
            and all(cs.ready for cs in (p.status.container_statuses or []))
        ]
        running_names = [p.metadata.name for p in running]
        assert running_names, (
            f"No Running pods found for {label_selector} in {namespace}"
        )
        return running_names[0]

    return wait_for(assert_pod_running, timeout=timeout_seconds, interval=5.0)


@log_execution
def delete_pod_and_measure_termination(
    namespace: str,
    pod_name: str,
    timeout_seconds: int = 120,
) -> float:
    """Delete the pod and return the elapsed seconds until it is fully gone."""
    core_v1 = client.CoreV1Api()

    start = time.monotonic()
    core_v1.delete_namespaced_pod(pod_name, namespace)

    def assert_pod_gone():
        try:
            core_v1.read_namespaced_pod(pod_name, namespace)
            raise AssertionError(f"Pod {pod_name} still exists")
        except client.rest.ApiException as e:
            if e.status == 404:
                return
            raise

    wait_for(assert_pod_gone, timeout=timeout_seconds, interval=2.0)
    return time.monotonic() - start
