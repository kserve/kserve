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

import logging
import os
import time
from typing import Any, Callable

from kubernetes import client, config as k8s_config
from kubernetes.client.exceptions import ApiException

from kserve.constants.constants import (
    KSERVE_GROUP,
    KSERVE_V1ALPHA1_VERSION,
    KSERVE_PLURAL_KERNELCACHE,
    KSERVE_PLURAL_KERNELCACHECAPTURE,
    KSERVE_PLURAL_KERNELCACHENODE,
)

_logger = logging.getLogger(__name__)


def _load_k8s_config() -> None:
    try:
        k8s_config.load_incluster_config()
    except k8s_config.ConfigException:
        k8s_config.load_kube_config(
            config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
        )


def _custom_api() -> client.CustomObjectsApi:
    _load_k8s_config()
    return client.CustomObjectsApi()


def _core_api() -> client.CoreV1Api:
    _load_k8s_config()
    return client.CoreV1Api()


def wait_for(
    assertion_fn: Callable[[], Any], timeout: float = 120.0, interval: float = 5.0
) -> Any:
    """Generic polling helper: repeatedly call assertion_fn until it succeeds or times out.

    The assertion function should raise AssertionError when the condition is not yet met.
    Follows the pattern established in test/e2e/llmisvc/test_llm_inference_service.py.

    Args:
        assertion_fn: Callable that checks a condition and raises AssertionError if not met
        timeout: Maximum time to wait in seconds
        interval: Polling interval in seconds

    Returns:
        Whatever assertion_fn returns on success

    Raises:
        AssertionError: If timeout is reached before assertion_fn succeeds
    """
    deadline = time.time() + timeout
    last_msg = None
    while True:
        try:
            return assertion_fn()
        except AssertionError as e:
            msg = str(e)
            if time.time() >= deadline:
                _logger.error("Timed out waiting: %s", e)
                raise
            if msg != last_msg:
                _logger.info("Waiting: %s", msg)
                last_msg = msg
            time.sleep(interval)


def wait_for_kernelcache_nodes(
    expected_node_names: list[str], timeout: int = 120
) -> list:
    """Poll until a KernelCacheNode exists for each expected node name.

    Verifies that each node name in expected_node_names has a corresponding
    KernelCacheNode CR (matching by name), preventing false passes from stale
    KCNs left over from previous tests.

    Args:
        expected_node_names: List of Kubernetes node names that should have KCNs
        timeout: Maximum time to wait in seconds

    Returns:
        List of KernelCacheNode CRs that match the expected node names
    """
    api = _custom_api()

    def check_nodes():
        try:
            items = api.list_cluster_custom_object(
                KSERVE_GROUP, KSERVE_V1ALPHA1_VERSION, KSERVE_PLURAL_KERNELCACHENODE
            ).get("items", [])
        except ApiException as e:
            if e.status == 404:
                items = []
            else:
                raise

        kcn_names = {item["metadata"]["name"] for item in items}
        missing = [name for name in expected_node_names if name not in kcn_names]
        assert not missing, (
            f"KernelCacheNode missing for nodes: {missing} "
            f"(have: {sorted(kcn_names)}, expect: {sorted(expected_node_names)})"
        )
        return [
            item for item in items if item["metadata"]["name"] in expected_node_names
        ]

    return wait_for(check_nodes, timeout=timeout, interval=5.0)


def get_kcc_for_isvc(namespace: str, isvc_name: str) -> dict | None:
    """Return the KernelCacheCapture for the ISVC, or None if not yet created."""
    api = _custom_api()
    items = api.list_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        namespace,
        KSERVE_PLURAL_KERNELCACHECAPTURE,
    ).get("items", [])
    for item in items:
        source_ref = item.get("spec", {}).get("sourceRef", {})
        if (
            source_ref.get("name") == isvc_name
            and source_ref.get("kind") == "InferenceService"
        ):
            return item
    return None


def wait_for_kcc_created(namespace: str, isvc_name: str, timeout: int = 120) -> dict:
    """Poll until a KernelCacheCapture for the ISVC is created by the controller."""

    def check_kcc():
        kcc = get_kcc_for_isvc(namespace, isvc_name)
        assert kcc is not None, (
            f"KernelCacheCapture for ISVC {namespace}/{isvc_name} not yet created"
        )
        return kcc

    return wait_for(check_kcc, timeout=timeout, interval=5.0)


def wait_for_capture_complete(
    namespace: str, kcc_name: str, timeout: int = 600
) -> dict:
    """Poll until KernelCacheCapture.status.phase == 'Complete'.

    Assumes the KCC exists when called (e.g. via wait_for_kcc_created). Fails
    immediately (rather than waiting for timeout) if:
      - The KCC is deleted (404)
      - Phase reaches "Failed"
      - Phase reaches "Unchanged" (this test always writes a new file, so
        Unchanged indicates MCV didn't detect it — a test setup problem)
    """
    api = _custom_api()

    def check_phase():
        try:
            kcc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                namespace,
                KSERVE_PLURAL_KERNELCACHECAPTURE,
                kcc_name,
            )
        except ApiException as e:
            if e.status == 404:
                raise AssertionError(
                    f"KernelCacheCapture {namespace}/{kcc_name} was deleted unexpectedly"
                ) from e
            raise

        phase = kcc.get("status", {}).get("phase", "")
        if phase == "Failed":
            raise AssertionError(
                f"KernelCacheCapture {namespace}/{kcc_name} reached Failed phase"
            )
        if phase == "Unchanged":
            raise AssertionError(
                f"KernelCacheCapture {namespace}/{kcc_name} reached Unchanged phase. "
                "This test always writes a new dummy cache file, so Unchanged means "
                "MCV did not detect the file (wrong timing, file not written, etc.)"
            )
        assert phase == "Complete", (
            f"KernelCacheCapture {namespace}/{kcc_name} phase: {phase or '(empty)'}"
        )
        return kcc

    return wait_for(check_phase, timeout=timeout, interval=10.0)


def wait_for_kernelcache_verified(
    namespace: str, kcc_name: str, timeout: int = 300
) -> dict:
    """Poll until the KernelCache for kcc_name has verification.state == 'Succeeded'.

    Called after wait_for_capture_complete, so assumes the KCC exists and has
    phase=Complete (meaning the KC was created). If either resource disappears
    during polling (404), fails immediately rather than waiting for timeout.

    Reads kc_name from KCC.status.kernelCacheRef — the controller sets this
    together with phase=Complete, so no separate wait is needed.
    """
    api = _custom_api()

    def check_verification():
        try:
            kcc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                namespace,
                KSERVE_PLURAL_KERNELCACHECAPTURE,
                kcc_name,
            )
        except ApiException as e:
            if e.status == 404:
                raise AssertionError(
                    f"KernelCacheCapture {namespace}/{kcc_name} was deleted unexpectedly"
                ) from e
            raise

        kc_ref = kcc.get("status", {}).get("kernelCacheRef", {})
        kc_name = kc_ref.get("name", "")
        kc_namespace = kc_ref.get("namespace", namespace)
        assert kc_name, (
            f"KernelCacheCapture {namespace}/{kcc_name}: waiting for kernelCacheRef"
        )

        try:
            kc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                kc_namespace,
                KSERVE_PLURAL_KERNELCACHE,
                kc_name,
            )
        except ApiException as e:
            if e.status == 404:
                raise AssertionError(
                    f"KernelCache {kc_namespace}/{kc_name} was deleted unexpectedly"
                ) from e
            raise

        verification = kc.get("status", {}).get("verification", {})
        state = verification.get("state", "")
        verified = verification.get("verified", False)
        if state == "Failed":
            raise AssertionError(
                f"KernelCache {kc_namespace}/{kc_name} verification Failed: "
                f"{verification.get('message', '')}"
            )
        assert state == "Succeeded" and verified, (
            f"KernelCache {kc_namespace}/{kc_name} verification state={state} verified={verified}"
        )
        return kc

    return wait_for(check_verification, timeout=timeout, interval=5.0)


def pod_has_mcv_sidecar(namespace: str, isvc_name: str) -> bool:
    """Return True if any pod for the ISVC has a container named 'mcv'."""
    core = _core_api()
    pods = core.list_namespaced_pod(
        namespace,
        label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
    ).items
    for pod in pods:
        for container in pod.spec.containers:
            if container.name == "mcv":
                return True
    return False


def wait_for_mcv_sidecar(namespace: str, isvc_name: str, timeout: int = 120) -> None:
    """Poll until at least one pod for the ISVC has the 'mcv' sidecar container."""

    def check_mcv():
        assert pod_has_mcv_sidecar(namespace, isvc_name), (
            f"No pod for ISVC {namespace}/{isvc_name} has 'mcv' sidecar yet"
        )

    wait_for(check_mcv, timeout=timeout, interval=5.0)
