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

"""End-to-end coverage for latency predictor sidecar injection.

The controller injects the training and prediction sidecars when the scheduler
configuration declares predicted-latency-producer. The existing integration
tests cover that injection under envtest, which runs without a kubelet: they
assert the containers appear in the Deployment but never resolve their images
or start their processes. An unpublished image version and an argument the
binary no longer accepts both pass there.

This test schedules the pod on a real cluster, so the sidecars must pull and
start.
"""

import os

import pytest
from kserve import KServeClient
from kubernetes import client

from .fixtures import (
    generate_test_id,
    inject_k8s_proxy,
)
from .logging import log_execution, logger
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    maybe_delete_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
)

SIDECAR_CONTAINERS = ("training-server", "prediction-server")


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-with-latency-predictor",
                    "workload-llmd-simulator",
                ],
                service_name="latency-predictor-test",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
            id="latency-predictor-sidecars",
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_latency_predictor_sidecars_run(test_case: TestCase):
    """The injected sidecars pull, start and pass their probes."""
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name
    namespace = test_case.llm_service.metadata.namespace
    test_failed = False

    try:
        logger.info(f"Creating LLMInferenceService {service_name}")
        create_llmisvc(kserve_client, test_case.llm_service)

        selector = assert_sidecars_injected(service_name, namespace)
        assert_sidecars_ready(
            selector, namespace, timeout_seconds=test_case.wait_timeout
        )
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
    except Exception as e:
        test_failed = True
        logger.error(f"Latency predictor test failed for {service_name}: {e}")
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)


@log_execution
def assert_sidecars_injected(service_name: str, namespace: str) -> str:
    """Verify the controller added both sidecars to the scheduler Deployment.

    Returns the Deployment's pod selector so the readiness check matches the
    labels the controller applied rather than a copy maintained here.
    """
    apps_v1 = client.AppsV1Api()
    deployment_name = f"{service_name}-kserve-router-scheduler"

    def assert_containers_present():
        dep = apps_v1.read_namespaced_deployment(deployment_name, namespace)
        names = [c.name for c in dep.spec.template.spec.containers]
        missing = [c for c in SIDECAR_CONTAINERS if c not in names]
        assert not missing, (
            f"Deployment {deployment_name} is missing {missing}; "
            f"has {names}. Expected the predicted-latency-producer plugin to "
            f"make the controller append the latency predictor preset."
        )
        return ",".join(f"{k}={v}" for k, v in dep.spec.selector.match_labels.items())

    return wait_for(assert_containers_present, timeout=120, interval=2.0)


@log_execution
def assert_sidecars_ready(label_selector: str, namespace: str, timeout_seconds: int):
    """Verify every scheduler container reaches Ready, sidecars included.

    This assertion requires a kubelet. It fails when an image cannot be
    resolved and when a binary rejects the arguments the preset renders.
    """
    core_v1 = client.CoreV1Api()

    def assert_all_containers_ready():
        pods = core_v1.list_namespaced_pod(namespace, label_selector=label_selector)
        assert pods.items, f"No scheduler pods for {label_selector} in {namespace}"

        for pod in pods.items:
            statuses = pod.status.container_statuses or []
            # A pod that was never scheduled has no container statuses; the
            # reason appears in the pod conditions instead.
            assert statuses, (
                f"Pod {pod.metadata.name} has no container statuses "
                f"(phase={pod.status.phase}): {describe_pod_conditions(pod)}"
            )
            not_ready = [s for s in statuses if not s.ready]
            assert not not_ready, (
                f"Pod {pod.metadata.name} not ready: "
                f"{describe_container_states(not_ready)}"
            )
        return [p.metadata.name for p in pods.items]

    ready = wait_for(assert_all_containers_ready, timeout=timeout_seconds, interval=5.0)
    logger.info(f"Scheduler pods ready with sidecars: {ready}")


def describe_pod_conditions(pod) -> str:
    """Report why a pod has not started.

    Typically Unschedulable, when the sidecar requests do not fit on the node
    alongside the rest of the workload.
    """
    unmet = [c for c in (pod.status.conditions or []) if c.status != "True"]
    if not unmet:
        return "no unmet pod conditions reported"
    return "; ".join(
        " ".join(filter(None, [f"{c.type}={c.status}", c.reason, c.message]))
        for c in unmet
    )


def describe_container_states(statuses) -> str:
    """Report container states so a failure identifies its cause.

    ImagePullBackOff indicates an image that cannot be resolved.
    CrashLoopBackOff following an image bump usually indicates the binary
    rejected an argument the preset still passes.
    """
    parts = []
    for s in statuses:
        state = s.state
        if state.waiting:
            detail = f"waiting/{state.waiting.reason}: {state.waiting.message}"
        elif state.terminated:
            detail = (
                f"terminated/{state.terminated.reason} "
                f"exit={state.terminated.exit_code}"
            )
        else:
            detail = "running (probes not yet passing)"
        parts.append(f"{s.name} [{detail}] restarts={s.restart_count}")
    return "; ".join(parts)
