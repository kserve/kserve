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

"""
E2E test for KEDA fallback during a Prometheus outage.

Scales the *shared* cluster Prometheus instance to 0 replicas to simulate an
outage. Deliberately not tagged with autoscaling_wva/autoscaling_keda/
autoscaling_direct_keda so it runs in its own CI step, after every other test
depending on that instance. The finally block always restores Prometheus.
"""

import logging
import os
import time

import pytest
from kubernetes import client

from .fixtures import generate_test_id, inject_k8s_proxy
from .logging import log_execution
from .test_llm_autoscaling_direct_keda import _cleanup, _new_kserve_client
from .test_llm_autoscaling_wva import (
    assert_scaled_object_condition,
    get_pod_count,
)
from .test_llm_inference_service import (
    TestCase,
    create_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
)

logger = logging.getLogger(__name__)

PROMETHEUS_GROUP = "monitoring.coreos.com"
PROMETHEUS_VERSION = "v1"
PROMETHEUS_PLURAL = "prometheuses"

PROMETHEUS_NAMESPACE = os.environ.get("PROMETHEUS_NAMESPACE", "monitoring")
PROMETHEUS_RELEASE_NAME = os.environ.get("PROMETHEUS_RELEASE_NAME", "prometheus")
PROMETHEUS_CR_NAME = f"{PROMETHEUS_RELEASE_NAME}-kube-prometheus-prometheus"


def _create_and_wait(kserve_client, test_case):
    create_llmisvc(kserve_client, test_case.llm_service)
    wait_for_llm_isvc_ready(
        kserve_client, test_case.llm_service, test_case.wait_timeout
    )


def _get_prometheus_replicas():
    api = client.CustomObjectsApi()
    prom = api.get_namespaced_custom_object(
        PROMETHEUS_GROUP,
        PROMETHEUS_VERSION,
        PROMETHEUS_NAMESPACE,
        PROMETHEUS_PLURAL,
        PROMETHEUS_CR_NAME,
    )
    return prom.get("spec", {}).get("replicas", 1)


def _patch_prometheus_replicas(replicas):
    api = client.CustomObjectsApi()
    api.patch_namespaced_custom_object(
        PROMETHEUS_GROUP,
        PROMETHEUS_VERSION,
        PROMETHEUS_NAMESPACE,
        PROMETHEUS_PLURAL,
        PROMETHEUS_CR_NAME,
        {"spec": {"replicas": replicas}},
    )
    logger.info(f"Patched Prometheus {PROMETHEUS_CR_NAME} spec.replicas={replicas}")


def _wait_for_prometheus_pod_count(count, timeout=180):
    def _check():
        v1 = client.CoreV1Api()
        pods = v1.list_namespaced_pod(
            namespace=PROMETHEUS_NAMESPACE,
            label_selector="app.kubernetes.io/name=prometheus",
        )
        running = [p for p in pods.items if p.status.phase == "Running"]
        assert len(running) == count, (
            f"Prometheus running pod count: {len(running)}, expected {count}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "scheduler-prometheus-scrape",
                    "workload-llmd-simulator-no-replicas",
                    "scaling-direct-keda-fallback",
                ],
                prompt="KServe is a",
                service_name="autoscale-keda-fallback",
            ),
            marks=[
                pytest.mark.cluster_cpu,
                pytest.mark.cluster_single_node,
                pytest.mark.llmd_simulator,
            ],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_llm_autoscaling_keda_fallback_holds_replicas(test_case: TestCase):
    """Direct KEDA + fallback: during a Prometheus outage, KEDA holds the
    deployment at fallback.replicas instead of scaling to zero or erroring."""
    inject_k8s_proxy()
    kserve_client = _new_kserve_client()
    service_name = test_case.llm_service.metadata.name
    ns = test_case.namespace

    original_prometheus_replicas = _get_prometheus_replicas()

    try:
        _create_and_wait(kserve_client, test_case)

        baseline_pod_count = get_pod_count(service_name, ns)
        assert baseline_pod_count >= 1, (
            f"Expected at least 1 pod before outage, got {baseline_pod_count}"
        )

        try:
            logger.info(
                "Simulating Prometheus outage: scaling Prometheus to 0 replicas"
            )
            _patch_prometheus_replicas(0)
            _wait_for_prometheus_pod_count(0, timeout=120)

            # failureThreshold=3, pollingInterval=5s (see
            # fixtures.scaling-direct-keda-fallback)
            assert_scaled_object_condition(
                service_name,
                namespace=ns,
                condition_type="Fallback",
                expected_status="True",
                timeout=120,
            )

            # Sample repeatedly over a window: the replica count must hold
            # steady at the fallback count, not drift while metrics are down.
            deadline = time.time() + 40
            while time.time() < deadline:
                current = get_pod_count(service_name, ns)
                assert current == baseline_pod_count, (
                    f"Pod count drifted during Prometheus outage: "
                    f"{current}, expected {baseline_pod_count}"
                )
                time.sleep(5)
        finally:
            # Always restore Prometheus for subsequent tests/jobs, regardless
            # of whether the outage assertions above passed or failed.
            logger.info(
                f"Restoring Prometheus to spec.replicas={original_prometheus_replicas}"
            )
            _patch_prometheus_replicas(original_prometheus_replicas)
            _wait_for_prometheus_pod_count(original_prometheus_replicas, timeout=180)

        # Once Prometheus is back, the ScaledObject should recover to a
        # non-fallback state.
        assert_scaled_object_condition(
            service_name,
            namespace=ns,
            condition_type="Fallback",
            expected_status="False",
            timeout=120,
        )
    finally:
        _cleanup(kserve_client, test_case)
