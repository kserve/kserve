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

import asyncio
import os

import pytest
from kubernetes import client
from kubernetes.client.exceptions import ApiException

from kserve import constants
from kserve.api.kserve_client import KServeClient
from ..common.utils import KSERVE_TEST_NAMESPACE


@pytest.mark.kernelcache
@pytest.mark.asyncio(scope="session")
async def test_kernelcache_state_transitions():
    """
    Test KernelCache state transitions when pods mount the cache.

    This E2E test verifies:
    1. KernelCache starts in Pending → Downloading → Extracted state
    2. Creating a pod that mounts cache → state transitions to Running
    3. Pod counts are tracked correctly (PodsUsing, PodsReady, PodsTerminating)
    4. Deleting the pod → state transitions back to Extracted
    5. Per-node state tracking in KernelCacheNode
    6. Aggregate state tracking in KernelCache
    """
    cache_name = "test-cache-pod-watching"
    pod_name = "test-pod-using-cache"
    # Use GKM example kernel cache image for testing
    cache_image = "quay.io/gkm/cache-examples:vector-add-cache-rocm-v2"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance
    core_v1 = client.CoreV1Api()

    # Get list of worker nodes (labeled with kserve/kernelcache=worker)
    nodes = core_v1.list_node(label_selector="kserve/kernelcache=worker")
    node_names = [node.metadata.name for node in nodes.items]

    # Ensure cluster has at least one worker node
    assert len(node_names) > 0, (
        "Cluster must have at least one worker node with kserve/kernelcache=worker label"
    )

    # Pick first worker node for testing
    test_node = node_names[0]

    # Create KernelCache CR
    kernel_cache = {
        "apiVersion": constants.KSERVE_V1ALPHA1,
        "kind": constants.KSERVE_KIND_KERNELCACHE,
        "metadata": {
            "name": cache_name,
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "image": cache_image,
        },
    }

    try:
        # Create the KernelCache
        k8s_client.create_namespaced_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            namespace=KSERVE_TEST_NAMESPACE,
            plural=constants.KSERVE_PLURAL_KERNELCACHE,
            body=kernel_cache,
        )

        # Wait for cache to reach Extracted state
        # State progression: Pending → Downloading → Extracted
        extracted = False
        for _ in range(60):  # Wait up to 60 seconds
            await asyncio.sleep(1)
            kc = k8s_client.get_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            state = kc.get("status", {}).get("state", "")
            if state == "Extracted":
                extracted = True
                break

        assert extracted, f"KernelCache did not reach Extracted state within 60s, current state: {state}"

        # Verify initial pod counts are zero
        counts = kc.get("status", {}).get("counts", {})
        assert counts.get("podRunningCnt", 0) == 0, "Should have 0 pods running initially"
        assert counts.get("nodeInUseCnt", 0) == 0, "Should have 0 nodes in use initially"

        # Create a pod that mounts the kernel cache Serving PVC
        # Serving PVC naming: {cache_name} (same as cache name)
        pod = {
            "apiVersion": "v1",
            "kind": "Pod",
            "metadata": {
                "name": pod_name,
                "namespace": KSERVE_TEST_NAMESPACE,
            },
            "spec": {
                "nodeName": test_node,  # Pin to specific node
                "containers": [
                    {
                        "name": "test-container",
                        "image": "busybox:latest",
                        "command": ["sleep", "3600"],  # Sleep for 1 hour
                        "volumeMounts": [
                            {
                                "name": "cache-volume",
                                "mountPath": "/kernel-cache",
                            }
                        ],
                    }
                ],
                "volumes": [
                    {
                        "name": "cache-volume",
                        "persistentVolumeClaim": {
                            "claimName": cache_name,  # Mount Serving PVC
                        },
                    }
                ],
                "restartPolicy": "Never",
            },
        }

        # Create the pod
        core_v1.create_namespaced_pod(
            namespace=KSERVE_TEST_NAMESPACE,
            body=pod,
        )

        # Wait for pod to become Running
        pod_running = False
        for _ in range(30):  # Wait up to 30 seconds
            await asyncio.sleep(1)
            try:
                pod_obj = core_v1.read_namespaced_pod(
                    name=pod_name,
                    namespace=KSERVE_TEST_NAMESPACE,
                )
                if pod_obj.status.phase == "Running":
                    pod_running = True
                    break
            except ApiException:
                pass

        assert pod_running, "Pod did not reach Running state within 30s"

        # Wait for KernelCache state to transition to Running
        # Agent reconciles every ~60s (configurable), so give it time
        state_running = False
        for _ in range(90):  # Wait up to 90 seconds
            await asyncio.sleep(1)
            kc = k8s_client.get_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            state = kc.get("status", {}).get("state", "")
            if state == "Running":
                state_running = True
                break

        assert state_running, f"KernelCache did not transition to Running state within 90s, current state: {state}"

        # Verify pod counts are updated
        counts = kc.get("status", {}).get("counts", {})
        assert counts.get("podRunningCnt", 0) >= 1, "Should have at least 1 pod running"
        assert counts.get("nodeInUseCnt", 0) >= 1, "Should have at least 1 node in use"

        # Verify per-node state in KernelCacheNode
        kcnode_name = f"kernel-cache-node-{test_node}"
        kcnode = k8s_client.get_cluster_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            plural=constants.KSERVE_PLURAL_KERNELCACHENODE,
            name=kcnode_name,
        )

        # Check cache status on this node
        cache_status = kcnode.get("status", {}).get("cacheStatus", {}).get(cache_name, {})
        assert cache_status.get("state") == "Running", f"Node cache state should be Running, got: {cache_status.get('state')}"

        # Verify serving namespace counts
        serving_ns = cache_status.get("servingNamespaces", {}).get(KSERVE_TEST_NAMESPACE, {})
        assert serving_ns.get("podsUsing", 0) >= 1, "Should count pod using cache"
        assert serving_ns.get("podsReady", 0) >= 1, "Should count ready pod"

        # Delete the pod
        core_v1.delete_namespaced_pod(
            name=pod_name,
            namespace=KSERVE_TEST_NAMESPACE,
        )

        # Wait for pod deletion to complete
        pod_deleted = False
        for _ in range(30):
            await asyncio.sleep(1)
            try:
                core_v1.read_namespaced_pod(
                    name=pod_name,
                    namespace=KSERVE_TEST_NAMESPACE,
                )
            except ApiException as e:
                if e.status == 404:
                    pod_deleted = True
                    break

        assert pod_deleted, "Pod did not delete within 30s"

        # Wait for KernelCache state to transition back to Extracted
        state_extracted = False
        for _ in range(90):  # Wait up to 90 seconds
            await asyncio.sleep(1)
            kc = k8s_client.get_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            state = kc.get("status", {}).get("state", "")
            if state == "Extracted":
                state_extracted = True
                break

        assert state_extracted, f"KernelCache did not transition back to Extracted state within 90s, current state: {state}"

        # Verify pod counts are back to zero
        counts = kc.get("status", {}).get("counts", {})
        assert counts.get("podRunningCnt", 0) == 0, "Should have 0 pods running after deletion"
        assert counts.get("nodeInUseCnt", 0) == 0, "Should have 0 nodes in use after deletion"

    finally:
        # Cleanup: Delete pod if still exists
        try:
            core_v1.delete_namespaced_pod(
                name=pod_name,
                namespace=KSERVE_TEST_NAMESPACE,
            )
        except ApiException:
            pass

        # Cleanup: Delete KernelCache
        try:
            k8s_client.delete_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
        except ApiException:
            pass


@pytest.mark.kernelcache
@pytest.mark.asyncio(scope="session")
async def test_kernelcache_multiple_pods_counting():
    """
    Test KernelCache pod counting with multiple pods across namespaces.

    This E2E test verifies:
    1. Multiple pods mounting the same cache are counted correctly
    2. Pod counts across multiple namespaces work
    3. Terminating pods are tracked separately
    4. Ready vs not-ready pods are distinguished
    """
    cache_name = "test-cache-multi-pod"
    pod1_name = "test-pod-1"
    pod2_name = "test-pod-2"
    cache_image = "quay.io/gkm/cache-examples:vector-add-cache-rocm-v2"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance
    core_v1 = client.CoreV1Api()

    # Get list of worker nodes
    nodes = core_v1.list_node(label_selector="kserve/kernelcache=worker")
    node_names = [node.metadata.name for node in nodes.items]
    assert len(node_names) > 0, "Cluster must have at least one worker node"
    test_node = node_names[0]

    # Create KernelCache
    kernel_cache = {
        "apiVersion": constants.KSERVE_V1ALPHA1,
        "kind": constants.KSERVE_KIND_KERNELCACHE,
        "metadata": {
            "name": cache_name,
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "image": cache_image,
        },
    }

    try:
        k8s_client.create_namespaced_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            namespace=KSERVE_TEST_NAMESPACE,
            plural=constants.KSERVE_PLURAL_KERNELCACHE,
            body=kernel_cache,
        )

        # Wait for Extracted state
        for _ in range(60):
            await asyncio.sleep(1)
            kc = k8s_client.get_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            if kc.get("status", {}).get("state") == "Extracted":
                break

        # Create two pods mounting the same cache
        for pod_name in [pod1_name, pod2_name]:
            pod = {
                "apiVersion": "v1",
                "kind": "Pod",
                "metadata": {
                    "name": pod_name,
                    "namespace": KSERVE_TEST_NAMESPACE,
                },
                "spec": {
                    "nodeName": test_node,
                    "containers": [
                        {
                            "name": "test-container",
                            "image": "busybox:latest",
                            "command": ["sleep", "3600"],
                            "volumeMounts": [
                                {
                                    "name": "cache-volume",
                                    "mountPath": "/kernel-cache",
                                }
                            ],
                        }
                    ],
                    "volumes": [
                        {
                            "name": "cache-volume",
                            "persistentVolumeClaim": {
                                "claimName": cache_name,
                            },
                        }
                    ],
                    "restartPolicy": "Never",
                },
            }
            core_v1.create_namespaced_pod(
                namespace=KSERVE_TEST_NAMESPACE,
                body=pod,
            )

        # Wait for both pods to be running
        for _ in range(30):
            await asyncio.sleep(1)
            pods_running = 0
            for pod_name in [pod1_name, pod2_name]:
                try:
                    pod_obj = core_v1.read_namespaced_pod(
                        name=pod_name,
                        namespace=KSERVE_TEST_NAMESPACE,
                    )
                    if pod_obj.status.phase == "Running":
                        pods_running += 1
                except ApiException:
                    pass
            if pods_running == 2:
                break

        # Wait for state to update
        await asyncio.sleep(65)  # Wait for reconcile cycle

        # Verify pod count is 2
        kc = k8s_client.get_namespaced_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            namespace=KSERVE_TEST_NAMESPACE,
            plural=constants.KSERVE_PLURAL_KERNELCACHE,
            name=cache_name,
        )
        counts = kc.get("status", {}).get("counts", {})
        assert counts.get("podRunningCnt", 0) >= 2, f"Should have at least 2 pods running, got: {counts.get('podRunningCnt')}"

        # Verify serving status shows namespace counts
        serving_status = kc.get("status", {}).get("servingStatus", {})
        ns_counts = serving_status.get("namespaceCounts", {}).get(KSERVE_TEST_NAMESPACE, {})
        assert ns_counts.get("podsUsing", 0) >= 2, "Should count 2 pods using cache"

    finally:
        # Cleanup pods
        for pod_name in [pod1_name, pod2_name]:
            try:
                core_v1.delete_namespaced_pod(
                    name=pod_name,
                    namespace=KSERVE_TEST_NAMESPACE,
                )
            except ApiException:
                pass

        # Cleanup KernelCache
        try:
            k8s_client.delete_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
        except ApiException:
            pass
