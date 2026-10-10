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
    KSERVE_KIND_INFERENCESERVICE,
    KSERVE_V1ALPHA1_VERSION,
    KSERVE_V1BETA1,
    KSERVE_PLURAL_KERNELCACHE,
    KSERVE_PLURAL_KERNELCACHECAPTURE,
    KSERVE_PLURAL_KERNELCACHENODE,
)
from kserve.models.v1beta1_inference_service import V1beta1InferenceService
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kserve.models.v1beta1_predictor_spec import V1beta1PredictorSpec

_logger = logging.getLogger(__name__)


class TerminalFailure(Exception):
    """Raised when a wait condition encounters a terminal failure state.

    Unlike AssertionError (which indicates "not ready yet"), TerminalFailure
    indicates the resource reached a failed/invalid state that will never recover.
    wait_for() does not retry TerminalFailure — it propagates immediately.
    """

    pass


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
    For terminal failures that should not be retried (e.g., resource deleted, phase=Failed),
    raise TerminalFailure instead — wait_for() propagates it immediately.

    Follows the pattern established in test/e2e/llmisvc/test_llm_inference_service.py.

    Args:
        assertion_fn: Callable that checks a condition and raises AssertionError if not met
        timeout: Maximum time to wait in seconds
        interval: Polling interval in seconds

    Returns:
        Whatever assertion_fn returns on success

    Raises:
        AssertionError: If timeout is reached before assertion_fn succeeds
        TerminalFailure: If assertion_fn raises TerminalFailure (propagated immediately)
    """
    deadline = time.time() + timeout
    last_msg = None
    while True:
        try:
            return assertion_fn()
        except TerminalFailure:
            # Terminal failures (resource deleted, phase=Failed, etc.) should not be
            # retried — propagate immediately so the test fails fast.
            raise
        except AssertionError as e:
            msg = str(e)
            if time.time() >= deadline:
                _logger.error("Timed out waiting: %s", e)
                raise
            if msg != last_msg:
                _logger.info("Waiting: %s", msg)
                last_msg = msg
            time.sleep(interval)


def make_producer_isvc(
    name: str, namespace: str, storage_uri: str
) -> V1beta1InferenceService:
    """Build a KC producer ISVC that triggers MCV capture via kernelcache-test-runtime."""
    return V1beta1InferenceService(
        api_version=KSERVE_V1BETA1,
        kind=KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=name,
            namespace=namespace,
            # Standard mode bypasses Knative (which strips the HTTP readinessProbe
            # from the pod spec when injecting queue-proxy, breaking the MCV webhook).
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                model=V1beta1ModelSpec(
                    model_format=V1beta1ModelFormat(name="test-cache"),
                    # storageUri causes the storage annotation to be set on the pod,
                    # which is required by the MCV sidecar injection webhook.
                    storage_uri=storage_uri,
                    resources=client.V1ResourceRequirements(
                        requests={"cpu": "100m", "memory": "256Mi"},
                        limits={"cpu": "500m", "memory": "512Mi"},
                    ),
                ),
            )
        ),
    )


def wait_for_resource_deleted(
    group: str,
    version: str,
    plural: str,
    name: str,
    namespace: str | None = None,
    timeout: int = 30,
) -> None:
    """Wait until a Kubernetes resource is fully deleted (404).

    Kubernetes deletion is asynchronous — the delete API call returns immediately,
    but the resource may still exist while finalizers run. This helper polls until
    GET returns 404, ensuring it's safe to recreate a resource with the same name.

    Args:
        group: API group (e.g., KSERVE_GROUP)
        version: API version (e.g., KSERVE_V1ALPHA1_VERSION)
        plural: Resource plural (e.g., KSERVE_PLURAL_KERNELCACHENODEGROUP)
        name: Resource name
        namespace: Namespace (None for cluster-scoped resources)
        timeout: Maximum time to wait in seconds
    """
    api = _custom_api()

    def check_deleted():
        try:
            if namespace:
                api.get_namespaced_custom_object(
                    group, version, namespace, plural, name
                )
            else:
                api.get_cluster_custom_object(group, version, plural, name)
            # Resource still exists — not ready yet
            raise AssertionError(f"Waiting for {plural}/{name} to be deleted")
        except ApiException as e:
            if e.status == 404:
                # Resource is gone — deletion complete
                return
            # Other errors (permissions, etc.) should propagate
            raise

    wait_for(check_deleted, timeout=timeout, interval=1.0)


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
                raise TerminalFailure(
                    f"KernelCacheCapture {namespace}/{kcc_name} was deleted unexpectedly"
                ) from e
            raise

        phase = kcc.get("status", {}).get("phase", "")
        if phase == "Failed":
            raise TerminalFailure(
                f"KernelCacheCapture {namespace}/{kcc_name} reached Failed phase"
            )
        if phase == "Unchanged":
            raise TerminalFailure(
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
                raise TerminalFailure(
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
                raise TerminalFailure(
                    f"KernelCache {kc_namespace}/{kc_name} was deleted unexpectedly"
                ) from e
            raise

        verification = kc.get("status", {}).get("verification", {})
        state = verification.get("state", "")
        verified = verification.get("verified", False)
        if state == "Failed":
            raise TerminalFailure(
                f"KernelCache {kc_namespace}/{kc_name} verification Failed: "
                f"{verification.get('message', '')}"
            )
        assert state == "Succeeded" and verified, (
            f"KernelCache {kc_namespace}/{kc_name} verification state={state} verified={verified}"
        )
        return kc

    return wait_for(check_verification, timeout=timeout, interval=5.0)


_KERNEL_CACHE_SOURCE_VOLUME = "kernel-cache-source"
_KERNEL_CACHE_LINKER_CONTAINER = "kernel-cache-linker"
_KERNEL_CACHE_USAGE_ANNOTATION = "internal.serving.kserve.io/kernelcache-usage"


def wait_for_kernelcachenode_cache_ready(
    kc_namespace: str, kc_name: str, timeout: int = 300
) -> None:
    """Poll until at least one KernelCacheNode reports the KC as Ready.

    The injection webhook reads KernelCacheNode.status.cacheStatus to find
    candidates; if no node has the KC in Ready state the webhook falls back
    to injecting MCV instead of mounting the cache.
    """
    deadline = time.monotonic() + timeout
    api = _custom_api()
    key = f"{kc_namespace}/{kc_name}"
    iteration = 0
    while time.monotonic() < deadline:
        nodes = api.list_cluster_custom_object(
            KSERVE_GROUP, KSERVE_V1ALPHA1_VERSION, KSERVE_PLURAL_KERNELCACHENODE
        ).get("items", [])

        # Log all caches on first iteration and every 6th iteration (every 60s)
        # to help diagnose if the cache exists but with a different key
        dump_all_caches = iteration % 6 == 0

        for node in nodes:
            node_name = node["metadata"]["name"]
            cache_status = node.get("status", {}).get("cacheStatus", {})
            info = cache_status.get(key, {})
            state = info.get("state", "")

            if dump_all_caches and cache_status:
                _logger.info(
                    "KernelCacheNode %s has %d cache(es): %s",
                    node_name,
                    len(cache_status),
                    ", ".join(
                        f"{k}={v.get('state', 'no-state')}"
                        for k, v in cache_status.items()
                    ),
                )

            _logger.info(
                "KernelCacheNode %s cacheStatus[%s].state=%s",
                node_name,
                key,
                state or "(absent)",
            )
            if state == "Ready":
                return

        iteration += 1
        time.sleep(10)

    # Gather final diagnostic info
    final_status = []
    for node in nodes:
        node_name = node["metadata"]["name"]
        cache_status = node.get("status", {}).get("cacheStatus", {})
        if cache_status:
            final_status.append(f"{node_name}: {len(cache_status)} cache(s)")
        else:
            final_status.append(f"{node_name}: no caches")

    raise TimeoutError(
        f"KernelCache {key} did not reach Ready state on any KernelCacheNode within {timeout}s. "
        f"Final KCN status: {'; '.join(final_status) if final_status else 'no nodes found'}. "
        f"Check kserve-kernelcachenode-agent DaemonSet and logs for errors."
    )


def pod_has_cache_injected(namespace: str, isvc_name: str) -> client.V1Pod | None:
    """Return the ISVC pod if it has the kernel-cache volume and linker init
    container injected, or None if not yet present or not injected."""
    core = _core_api()
    pods = core.list_namespaced_pod(
        namespace,
        label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
    ).items
    for pod in pods:
        has_source_volume = any(
            v.name == _KERNEL_CACHE_SOURCE_VOLUME and v.image is not None
            for v in (pod.spec.volumes or [])
        )
        has_linker = any(
            c.name == _KERNEL_CACHE_LINKER_CONTAINER
            for c in (pod.spec.init_containers or [])
        )
        if has_source_volume and has_linker:
            return pod
    return None


def wait_for_cache_injected(
    namespace: str, isvc_name: str, timeout: int = 120
) -> client.V1Pod:
    """Poll until the ISVC pod has the kernel-cache OCI volume and linker
    init container injected by the pod mutator webhook."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        pod = pod_has_cache_injected(namespace, isvc_name)
        if pod is not None:
            _logger.info("Cache injected into ISVC %s/%s pod", namespace, isvc_name)
            return pod
        time.sleep(5)
    raise AssertionError(
        f"No pod for ISVC {namespace}/{isvc_name} had cache injected within {timeout}s"
    )


def wait_for_isvc_pod_running(
    namespace: str, isvc_name: str, timeout: int = 300
) -> None:
    """Poll until at least one pod for the ISVC reaches Running phase."""
    deadline = time.monotonic() + timeout
    core = _core_api()
    while time.monotonic() < deadline:
        pods = core.list_namespaced_pod(
            namespace,
            label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
        ).items
        for pod in pods:
            phase = (pod.status.phase or "") if pod.status else ""
            _logger.info("Pod for ISVC %s/%s phase: %s", namespace, isvc_name, phase)
            if phase == "Running":
                return
        time.sleep(5)
    raise TimeoutError(
        f"No pod for ISVC {namespace}/{isvc_name} reached Running within {timeout}s"
    )


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
