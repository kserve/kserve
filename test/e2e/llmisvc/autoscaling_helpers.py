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

"""Shared helpers for LLMInferenceService autoscaling e2e tests."""

import concurrent.futures
import logging
import threading
import time

import requests
from kserve import constants
from kubernetes import client

from .test_llm_inference_service import wait_for

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
KEDA_GROUP = "keda.sh"
KEDA_VERSION = "v1alpha1"
KEDA_PLURAL = "scaledobjects"
HPA_GROUP = "autoscaling"
HPA_VERSION = "v2"
HPA_PLURAL = "horizontalpodautoscalers"
WORKLOAD_COMPONENT_MAIN = "llminferenceservice-workload"
WORKLOAD_COMPONENT_PREFILL = "llminferenceservice-workload-prefill"

logger = logging.getLogger(__name__)


def _get_custom_resource(group, version, plural, name, namespace):
    api = client.CustomObjectsApi()
    try:
        return api.get_namespaced_custom_object(group, version, namespace, plural, name)
    except client.rest.ApiException as e:
        if e.status == 404:
            return None
        raise


def resource_exists(group, version, plural, name, namespace):
    return _get_custom_resource(group, version, plural, name, namespace) is not None


def wait_for_resource(group, version, plural, name, namespace, timeout=120):
    def _check():
        assert resource_exists(group, version, plural, name, namespace), (
            f"{plural}/{name} not found in {namespace}"
        )

    wait_for(_check, timeout=timeout, interval=2.0)


def wait_for_resource_deleted(group, version, plural, name, namespace, timeout=120):
    def _check():
        assert not resource_exists(group, version, plural, name, namespace), (
            f"{plural}/{name} still exists in {namespace}"
        )

    wait_for(_check, timeout=timeout, interval=2.0)


def _child_name(parent, suffix):
    """Replicate knative.dev/pkg/kmeta.ChildName truncation to 63 chars."""
    return (parent + suffix)[:63]


def hpa_name(service_name, prefill=False):
    suffix = "-kserve-prefill-hpa" if prefill else "-kserve-hpa"
    return _child_name(service_name, suffix)


def scaled_object_name(service_name, prefill=False):
    suffix = "-kserve-prefill-keda" if prefill else "-kserve-keda"
    return _child_name(service_name, suffix)


def get_pod_count(service_name, namespace, component=None):
    v1 = client.CoreV1Api()
    label_selector = f"app.kubernetes.io/name={service_name}"
    if component:
        label_selector += f",app.kubernetes.io/component={component}"
    pods = v1.list_namespaced_pod(
        namespace=namespace,
        label_selector=label_selector,
    )
    return sum(pod.status.phase in ("Running", "Pending") for pod in pods.items)


def wait_for_pod_count(service_name, min_count, namespace, timeout=300, component=None):
    def _check():
        current = get_pod_count(service_name, namespace, component=component)
        assert current >= min_count, (
            f"Pod count for {service_name}: {current}, expected >= {min_count}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


def wait_for_pod_count_at_most(
    service_name, max_count, namespace, timeout=300, component=None
):
    def _check():
        current = get_pod_count(service_name, namespace, component=component)
        assert current <= max_count, (
            f"Pod count for {service_name}: {current}, expected <= {max_count}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


def send_load(
    service_url, model_name, concurrency=5, duration_seconds=30, tolerate_failures=False
):
    endpoint = service_url + "/v1/completions"
    payload = {
        "model": model_name,
        "prompt": "Explain autoscaling in Kubernetes in great detail. " * 10,
        "max_tokens": 200,
    }
    headers = {"Content-Type": "application/json"}
    deadline = time.time() + duration_seconds
    lock = threading.Lock()
    counters = {"success": 0, "client_error": 0, "server_error": 0}

    def _worker():
        while time.time() < deadline:
            try:
                resp = requests.post(
                    endpoint, json=payload, headers=headers, timeout=30
                )
                with lock:
                    if resp.status_code == 200:
                        counters["success"] += 1
                    elif resp.status_code < 500:
                        counters["client_error"] += 1
                    else:
                        counters["server_error"] += 1
            except Exception:
                with lock:
                    counters["server_error"] += 1
            time.sleep(0.1)

    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as pool:
        futures = [pool.submit(_worker) for _ in range(concurrency)]
        concurrent.futures.wait(futures)

    logger.info(
        f"send_load complete: {counters['success']} ok, "
        f"{counters['client_error']} client errors, "
        f"{counters['server_error']} server errors to {endpoint}"
    )
    if not tolerate_failures:
        total_delivered = counters["success"] + counters["client_error"]
        assert total_delivered > 0, (
            f"send_load: all {counters['server_error']} requests failed to {endpoint}"
        )


def assert_scaling_resources_deleted(
    service_name, actuator="hpa", prefill=False, timeout=120, *, namespace
):
    if actuator == "hpa":
        wait_for_resource_deleted(
            HPA_GROUP,
            HPA_VERSION,
            HPA_PLURAL,
            hpa_name(service_name, prefill),
            namespace,
            timeout,
        )
    elif actuator == "keda":
        wait_for_resource_deleted(
            KEDA_GROUP,
            KEDA_VERSION,
            KEDA_PLURAL,
            scaled_object_name(service_name, prefill),
            namespace,
            timeout,
        )


def assert_scaled_object_ready(service_name, namespace):
    def _check():
        name = scaled_object_name(service_name)
        so = _get_custom_resource(
            KEDA_GROUP, KEDA_VERSION, KEDA_PLURAL, name, namespace
        )
        assert so is not None, f"ScaledObject {name} not found"
        conditions = so.get("status", {}).get("conditions", [])
        ready = next((c for c in conditions if c["type"] == "Ready"), None)
        assert ready and ready["status"] == "True", (
            f"ScaledObject Ready is not True: {conditions}"
        )

    wait_for(_check, timeout=60, interval=5.0)


def assert_scaled_object_condition(
    service_name,
    namespace,
    condition_type,
    expected_status="True",
    prefill=False,
    timeout=120,
):
    name = scaled_object_name(service_name, prefill)

    def _check():
        so = _get_custom_resource(
            KEDA_GROUP, KEDA_VERSION, KEDA_PLURAL, name, namespace
        )
        assert so is not None, f"ScaledObject {name} not found"
        conditions = so.get("status", {}).get("conditions", [])
        cond = next((c for c in conditions if c["type"] == condition_type), None)
        assert cond and cond["status"] == expected_status, (
            f"ScaledObject {name} condition {condition_type} is "
            f"{cond['status'] if cond else 'missing'}, expected {expected_status}: "
            f"{conditions}"
        )

    wait_for(_check, timeout=timeout, interval=5.0)


def _get_llmisvc_condition(name, namespace, condition_type):
    api = client.CustomObjectsApi()
    resource = api.get_namespaced_custom_object(
        constants.KSERVE_GROUP,
        "v1alpha2",
        namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        name,
    )
    return next(
        (
            condition
            for condition in resource.get("status", {}).get("conditions", [])
            if condition.get("type") == condition_type
        ),
        None,
    )


def assert_scaling_ready_condition(service_name, namespace):
    def _check():
        cond = _get_llmisvc_condition(service_name, namespace, "ScalingReady")
        assert cond is not None, f"ScalingReady condition not found on {service_name}"
        assert cond.get("status") == "True", (
            f"ScalingReady status is {cond.get('status')} "
            f"(reason={cond.get('reason')}), expected True"
        )

    wait_for(_check, timeout=180, interval=5.0)


def assert_scaling_ready_absent(service_name, namespace):
    cond = _get_llmisvc_condition(service_name, namespace, "ScalingReady")
    assert cond is None, (
        f"ScalingReady condition should be absent on {service_name}, got: {cond}"
    )
