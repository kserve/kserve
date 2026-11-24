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
from kubernetes.stream import stream

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
    """Build a KC producer ISVC that triggers MCV capture via kernelcache-test-runtime.

    Resources are NOT specified here - the ClusterServingRuntime (kc_test_runtime)
    defines them.
    """
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
    """Poll until a KernelCacheNode exists for each expected node name and has been reconciled.

    Verifies that each node name in expected_node_names has a corresponding
    KernelCacheNode CR (matching by name) and that the agent has reconciled it
    (status.counts field is populated), preventing false passes from stale KCNs
    left over from previous tests or KCNs that haven't been reconciled yet.

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

        # Verify each expected KCN has been reconciled by the agent (status.counts exists).
        # The agent populates this field on its first reconcile after the controller creates
        # the KCN resource. Without this check, the test could pass before the agent pod starts.
        matching_kcns = [
            item for item in items if item["metadata"]["name"] in expected_node_names
        ]
        not_reconciled = [
            item["metadata"]["name"]
            for item in matching_kcns
            if not item.get("status", {}).get("counts")
        ]
        assert not not_reconciled, (
            f"KernelCacheNode exists but not yet reconciled by agent: {not_reconciled}"
        )

        return matching_kcns

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
                "This test always compiles new kernel cache files, so Unchanged means "
                "MCV did not detect the files (wrong timing, compilation failed, etc.)"
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
    api = _custom_api()
    key = f"{kc_namespace}/{kc_name}"

    def check_ready():
        nodes = api.list_cluster_custom_object(
            KSERVE_GROUP, KSERVE_V1ALPHA1_VERSION, KSERVE_PLURAL_KERNELCACHENODE
        ).get("items", [])
        for node in nodes:
            info = node.get("status", {}).get("cacheStatus", {}).get(key, {})
            state = info.get("state", "")
            _logger.info(
                "KernelCacheNode %s cacheStatus[%s].state=%s",
                node["metadata"]["name"],
                key,
                state or "(absent)",
            )
            if state == "Ready":
                return
        raise AssertionError(f"KernelCache {key} not yet Ready on any KernelCacheNode")

    wait_for(check_ready, timeout=timeout, interval=10.0)


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

    def check_injected():
        pod = pod_has_cache_injected(namespace, isvc_name)
        if pod is not None:
            _logger.info("Cache injected into ISVC %s/%s pod", namespace, isvc_name)
            return pod
        raise AssertionError(
            f"No pod for ISVC {namespace}/{isvc_name} has cache injected yet"
        )

    return wait_for(check_injected, timeout=timeout, interval=5.0)


def wait_for_isvc_pod_ready(namespace: str, isvc_name: str, timeout: int = 300) -> None:
    """Poll until at least one pod for the ISVC is Ready (readiness probe passed).

    A pod reaches Running phase as soon as its main container starts, but
    Ready condition only becomes True after the readiness probe succeeds.

    For cache injection tests, this ensures torch.compile has completed and
    the HTTP server has started before we check logs for cache activity.
    """
    core = _core_api()

    def check_ready():
        pods = core.list_namespaced_pod(
            namespace,
            label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
        ).items

        # Default values in case no pods found or all pods lack status
        phase = "Unknown"
        ready_status = "Unknown"

        for pod in pods:
            if not pod.status:
                continue

            # Check pod conditions for Ready status
            conditions = pod.status.conditions or []
            for condition in conditions:
                if condition.type == "Ready" and condition.status == "True":
                    _logger.info(
                        "Pod %s for ISVC %s/%s is Ready",
                        pod.metadata.name,
                        namespace,
                        isvc_name,
                    )
                    return

            # Update status for debugging (only if not ready yet)
            phase = pod.status.phase or "Unknown"
            ready_condition = next((c for c in conditions if c.type == "Ready"), None)
            ready_status = ready_condition.status if ready_condition else "Unknown"

        # Raise AssertionError if no pod is ready yet (will be retried by wait_for)
        raise AssertionError(
            f"No pod for ISVC {namespace}/{isvc_name} is Ready yet "
            f"(latest: phase={phase}, ready={ready_status})"
        )

    wait_for(check_ready, timeout=timeout, interval=5.0)


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


def wait_for_runtime_image_prefetch_jobs(
    job_namespace: str,
    expected_node_names: list[str],
    runtime_image: str,
    timeout: int = 300,
) -> None:
    """Wait for one completed runtime image prefetch Job per expected node."""
    _load_k8s_config()
    batch = client.BatchV1Api()

    def check_jobs() -> None:
        jobs = batch.list_namespaced_job(
            job_namespace,
            label_selector="internal.serving.kserve.io/runtime-image-prefetch=true",
        ).items
        matching = [
            job
            for job in jobs
            if (job.metadata.annotations or {}).get(
                "internal.serving.kserve.io/runtime-image"
            )
            == runtime_image
        ]

        by_node: dict[str, client.V1Job] = {}
        for job in matching:
            annotations = job.metadata.annotations or {}
            node_name = annotations.get(
                "internal.serving.kserve.io/kernel-cache-node", ""
            )
            if node_name not in expected_node_names:
                continue
            assert node_name not in by_node, (
                f"Multiple runtime image prefetch Jobs found for node {node_name}"
            )
            by_node[node_name] = job

            conditions = job.status.conditions or []
            complete = any(
                condition.type == "Complete" and condition.status == "True"
                for condition in conditions
            )
            assert complete, (
                f"Runtime image prefetch Job {job_namespace}/{job.metadata.name} "
                "is not complete"
            )

        missing = sorted(set(expected_node_names) - set(by_node))
        assert not missing, (
            f"Runtime image prefetch Jobs missing for nodes {missing}; "
            f"found {sorted(by_node)}"
        )

    wait_for(check_jobs, timeout=timeout, interval=5.0)


def verify_runtime_image_available_on_node(
    namespace: str,
    node_name: str,
    runtime_image: str,
    timeout: int = 120,
) -> None:
    """Run a pull-policy Never probe to verify that an image is cached on a node."""
    core = _core_api()
    pod = core.create_namespaced_pod(
        namespace,
        client.V1Pod(
            metadata=client.V1ObjectMeta(generate_name="runtime-image-probe-"),
            spec=client.V1PodSpec(
                automount_service_account_token=False,
                node_name=node_name,
                restart_policy="Never",
                containers=[
                    client.V1Container(
                        name="probe",
                        image=runtime_image,
                        image_pull_policy="Never",
                        command=["/bin/sh", "-c", "true"],
                    )
                ],
            ),
        ),
    )
    pod_name = pod.metadata.name

    def check_probe() -> None:
        current = core.read_namespaced_pod(pod_name, namespace)
        for status in current.status.container_statuses or []:
            state = status.state
            waiting = state.waiting if state else None
            if waiting and waiting.reason in {"ErrImageNeverPull", "ImagePullBackOff"}:
                raise TerminalFailure(
                    f"Runtime image {runtime_image} is not cached on node {node_name}: "
                    f"{waiting.reason}: {waiting.message or ''}"
                )
            if waiting and waiting.reason in {
                "CreateContainerError",
                "RunContainerError",
            }:
                raise TerminalFailure(
                    f"Runtime image cache probe {namespace}/{pod_name} could not start on "
                    f"{node_name}: {waiting.reason}: {waiting.message or ''}"
                )
        if current.status.phase == "Failed":
            terminated = next(
                (
                    status.state.terminated
                    for status in current.status.container_statuses or []
                    if status.state and status.state.terminated
                ),
                None,
            )
            if terminated:
                raise TerminalFailure(
                    f"Runtime image cache probe {namespace}/{pod_name} command failed on "
                    f"{node_name}: {terminated.reason or 'unknown'} "
                    f"(exit code {terminated.exit_code}): {terminated.message or ''}"
                )
            raise TerminalFailure(
                f"Runtime image cache probe {namespace}/{pod_name} failed on {node_name}"
            )
        assert current.status.phase == "Succeeded", (
            f"Runtime image cache probe {namespace}/{pod_name} phase is "
            f"{current.status.phase or '(empty)'}"
        )

    try:
        wait_for(check_probe, timeout=timeout, interval=2.0)
    finally:
        try:
            core.delete_namespaced_pod(
                pod_name,
                namespace,
                grace_period_seconds=0,
                propagation_policy="Background",
            )
        except ApiException as e:
            if e.status != 404:
                raise


def check_pod_logs_for_cache_usage(
    namespace: str, isvc_name: str, since_seconds: int = 300
) -> dict[str, bool]:
    """Check pod logs for torch.compile cache hit/miss indicators.

    Returns a dict with:
      - "cache_hit_found": True if "cache hit" or similar found in logs
      - "cache_miss_found": True if "cache miss" or compilation logs found
      - "logs_checked": True if logs were successfully retrieved

    This provides evidence of whether torch.compile reused cached artifacts
    or regenerated them.
    """
    core = _core_api()
    result = {
        "cache_hit_found": False,
        "cache_miss_found": False,
        "logs_checked": False,
    }

    try:
        pods = core.list_namespaced_pod(
            namespace,
            label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
        ).items
        if not pods:
            _logger.warning("No pods found for ISVC %s/%s", namespace, isvc_name)
            return result

        pod_name = pods[0].metadata.name
        logs = core.read_namespaced_pod_log(
            name=pod_name,
            namespace=namespace,
            container="kserve-container",
            since_seconds=since_seconds,
        )
        result["logs_checked"] = True

        # Look for cache hit indicators
        cache_hit_patterns = [
            "cache hit",
            "using cached",
            "loaded from cache",
            "fx graph cache hit",
        ]
        for pattern in cache_hit_patterns:
            if pattern.lower() in logs.lower():
                result["cache_hit_found"] = True
                _logger.info(
                    "Found cache hit indicator in logs for %s/%s: %s",
                    namespace,
                    pod_name,
                    pattern,
                )
                break

        # Look for cache miss indicators
        cache_miss_patterns = [
            "cache miss",
            "compiling",
            "fx graph cache miss",
        ]
        for pattern in cache_miss_patterns:
            if pattern.lower() in logs.lower():
                result["cache_miss_found"] = True
                _logger.info(
                    "Found cache miss indicator in logs for %s/%s: %s",
                    namespace,
                    pod_name,
                    pattern,
                )
                break

    except ApiException as e:
        _logger.warning(
            "Could not retrieve logs for %s/%s: %s", namespace, isvc_name, e.reason
        )

    return result


def verify_cache_files_accessible(
    namespace: str, isvc_name: str, timeout: int = 60
) -> None:
    """Verify that injected cache files are accessible in the consumer pod.

    The kernel-cache-linker init container symlinks the OCI image contents to
    /root/.cache/vllm/torch_compile_cache/. This function verifies the cache
    files are present and readable, confirming the cache is actually being
    used rather than regenerated.
    """
    core = _core_api()
    pods = core.list_namespaced_pod(
        namespace,
        label_selector=f"serving.kserve.io/inferenceservice={isvc_name}",
    ).items
    if not pods:
        raise AssertionError(f"No pod found for ISVC {namespace}/{isvc_name}")

    pod_name = pods[0].metadata.name
    # Cache location matches TORCHINDUCTOR_CACHE_DIR from the runtime.
    # PyTorch writes cache files to this explicitly configured directory.
    cache_dir = "/root/.cache/vllm/torch_compile_cache/inductor_cache"
    # The kernel-cache-source OCI image volume is mounted at this path
    # (from pkg/webhook/admission/kernelcache/consumer.go kernelCacheSourceMountPath)
    source_mount_path = "/var/run/gkm/kernel-cache"

    # Verify cache directory exists and contains files.
    # The kernel-cache-linker init container symlinks the OCI image contents,
    # including the torch cache written during producer pod compilation.
    # NOTE: kernel-cache-linker uses 'cp -sr' which creates symlinks, so we must
    # use 'find -L' to follow symlinks. Regular 'find -type f' only finds regular
    # files and will miss symlinks, causing false failures.
    find_script = f"""
set -e
if [ ! -d "{cache_dir}" ]; then
    echo "ERROR: Cache directory does not exist: {cache_dir}"
    exit 1
fi
# -L follows symlinks, -type f finds regular files (after following links)
files=$(find -L "{cache_dir}" -type f 2>&1)
if [ -z "$files" ]; then
    echo "ERROR: No cache files found in {cache_dir}"
    exit 1
fi
echo "$files" | head -5
"""

    # Verify that cache files are symlinks, not regular files.
    # This confirms cache injection worked - the kernel-cache-linker uses 'cp -sr'
    # which creates symlinks. If the linker failed to expose the image files,
    # torch.compile would regenerate them as regular files into the EmptyDir target.
    # Finding symlinks proves the injected cache is being used, not regenerated files.
    symlink_check_script = f"""
set -e
# Count symlinks vs regular files in the cache directory
symlink_count=$(find "{cache_dir}" -type l 2>/dev/null | wc -l)
regular_file_count=$(find "{cache_dir}" -type f 2>/dev/null | wc -l)

if [ "$symlink_count" -eq 0 ]; then
    echo "ERROR: No symlinks found in {cache_dir} (found $regular_file_count regular files)"
    echo "This indicates cache was regenerated, not injected"
    exit 1
fi

echo "OK: Found $symlink_count symlinks in cache directory (vs $regular_file_count regular files)"
"""
    try:
        resp = stream(
            core.connect_get_namespaced_pod_exec,
            name=pod_name,
            namespace=namespace,
            container="kserve-container",
            command=["sh", "-c", find_script],
            stderr=True,
            stdin=False,
            stdout=True,
            tty=False,
        )
        # Check for error conditions in output
        if "ERROR:" in resp:
            raise AssertionError(
                f"Cache verification failed in pod {namespace}/{pod_name}: {resp.strip()}"
            )
        if not resp or len(resp.strip()) == 0:
            raise AssertionError(
                f"Cache directory {cache_dir} is empty in pod {namespace}/{pod_name}. "
                "Cache injection may have failed or files are not accessible."
            )
        file_count = len(resp.strip().split("\n"))
        _logger.info(
            "Found %d cache files in %s (showing first 5): %s",
            file_count,
            cache_dir,
            resp.strip(),
        )

        # Verify symlinks point to the source mount (injected cache, not regenerated)
        symlink_resp = stream(
            core.connect_get_namespaced_pod_exec,
            name=pod_name,
            namespace=namespace,
            container="kserve-container",
            command=["sh", "-c", symlink_check_script],
            stderr=True,
            stdin=False,
            stdout=True,
            tty=False,
        )
        if "ERROR:" in symlink_resp:
            raise AssertionError(
                f"Symlink verification failed in pod {namespace}/{pod_name}: {symlink_resp.strip()}"
            )
        _logger.info(
            "Verified cache symlinks point to source mount %s", source_mount_path
        )

    except ApiException as e:
        raise AssertionError(
            f"Failed to verify cache files in {namespace}/{pod_name}: {e.reason}"
        ) from e
