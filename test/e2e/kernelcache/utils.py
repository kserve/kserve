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


def wait_for_kernelcache_nodes(expected_count: int, timeout: int = 120) -> list:
    """Poll until at least expected_count KernelCacheNode CRs exist (cluster-scoped)."""
    deadline = time.monotonic() + timeout
    api = _custom_api()
    while time.monotonic() < deadline:
        try:
            items = api.list_cluster_custom_object(
                KSERVE_GROUP, KSERVE_V1ALPHA1_VERSION, KSERVE_PLURAL_KERNELCACHENODE
            ).get("items", [])
            _logger.info("KernelCacheNode count: %d / %d", len(items), expected_count)
            if len(items) >= expected_count:
                return items
        except ApiException as e:
            if e.status != 404:
                raise
        time.sleep(5)
    raise TimeoutError(
        f"Expected {expected_count} KernelCacheNode CRs within {timeout}s"
    )


def get_kcc_for_isvc(namespace: str, isvc_name: str) -> dict | None:
    """Return the KernelCacheCapture owned by the ISVC, or None if not yet created."""
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
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        kcc = get_kcc_for_isvc(namespace, isvc_name)
        if kcc is not None:
            _logger.info(
                "KernelCacheCapture %s created for ISVC %s/%s",
                kcc["metadata"]["name"],
                namespace,
                isvc_name,
            )
            return kcc
        time.sleep(5)
    raise TimeoutError(
        f"KernelCacheCapture for ISVC {namespace}/{isvc_name} not created within {timeout}s"
    )


def wait_for_capture_complete(
    namespace: str, kcc_name: str, timeout: int = 600
) -> dict:
    """Poll until KernelCacheCapture.status.phase == 'Complete'."""
    deadline = time.monotonic() + timeout
    api = _custom_api()
    while time.monotonic() < deadline:
        try:
            kcc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                namespace,
                KSERVE_PLURAL_KERNELCACHECAPTURE,
                kcc_name,
            )
            phase = kcc.get("status", {}).get("phase", "")
            _logger.info(
                "KernelCacheCapture %s/%s phase: %s", namespace, kcc_name, phase
            )
            if phase == "Complete":
                return kcc
            if phase == "Failed":
                raise AssertionError(
                    f"KernelCacheCapture {namespace}/{kcc_name} reached Failed phase"
                )
        except ApiException as e:
            if e.status != 404:
                raise
        time.sleep(10)
    raise TimeoutError(
        f"KernelCacheCapture {namespace}/{kcc_name} did not reach Complete within {timeout}s"
    )


def wait_for_kernelcache_verified(
    namespace: str, kcc_name: str, timeout: int = 300
) -> dict:
    """Poll until the KernelCache for kcc_name has verification.state == 'Succeeded'.

    Reads kc_name from KCC.status.kernelCacheRef — the controller sets this
    together with phase=Complete, so no separate wait is needed.
    """
    deadline = time.monotonic() + timeout
    api = _custom_api()
    while time.monotonic() < deadline:
        try:
            kcc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                namespace,
                KSERVE_PLURAL_KERNELCACHECAPTURE,
                kcc_name,
            )
            kc_ref = kcc.get("status", {}).get("kernelCacheRef", {})
            kc_name = kc_ref.get("name", "")
            kc_namespace = kc_ref.get("namespace", namespace)
            if not kc_name:
                _logger.info(
                    "KernelCacheCapture %s/%s: waiting for kernelCacheRef",
                    namespace,
                    kcc_name,
                )
                time.sleep(5)
                continue

            kc = api.get_namespaced_custom_object(
                KSERVE_GROUP,
                KSERVE_V1ALPHA1_VERSION,
                kc_namespace,
                KSERVE_PLURAL_KERNELCACHE,
                kc_name,
            )
            verification = kc.get("status", {}).get("verification", {})
            state = verification.get("state", "")
            verified = verification.get("verified", False)
            _logger.info(
                "KernelCache %s/%s verification state=%s verified=%s",
                kc_namespace,
                kc_name,
                state,
                verified,
            )
            if state == "Succeeded" and verified:
                return kc
            if state == "Failed":
                raise AssertionError(
                    f"KernelCache {kc_namespace}/{kc_name} verification Failed: "
                    f"{verification.get('message', '')}"
                )
        except ApiException as e:
            if e.status != 404:
                raise
        time.sleep(5)
    raise TimeoutError(
        f"KernelCache for KCC {namespace}/{kcc_name} did not verify within {timeout}s"
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
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if pod_has_mcv_sidecar(namespace, isvc_name):
            _logger.info("MCV sidecar found in ISVC %s/%s pod", namespace, isvc_name)
            return
        time.sleep(5)
    raise AssertionError(
        f"No pod for ISVC {namespace}/{isvc_name} had 'mcv' sidecar within {timeout}s"
    )
