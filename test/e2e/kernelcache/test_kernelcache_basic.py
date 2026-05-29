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
async def test_kernelcache_basic():
    """
    Test KernelCache basic functionality with stub GPU mode.

    This test verifies:
    1. KernelCache creation triggers extraction Job
    2. KernelCacheNode is created automatically for worker nodes
    3. GPU detection populates GPUInfo (stub mode with noGPU=true)
    4. Extraction Job completes successfully
    5. Cache becomes ready (cacheCopies.available == cacheCopies.total)
    6. ServingPVC is created for pod mounting
    """
    cache_name = "test-kernel-cache"
    # Use GKM example kernel cache image for testing
    cache_image = "quay.io/gkm/cache-examples:vector-add-cache-rocm-v2"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance
    core_v1 = client.CoreV1Api()
    batch_v1 = client.BatchV1Api()

    # Get list of worker nodes (labeled with kserve/kernelcache=worker)
    # Control-plane nodes don't run the agent, so no KernelCacheNode created for them
    nodes = core_v1.list_node(label_selector="kserve/kernelcache=worker")
    node_names = [node.metadata.name for node in nodes.items]

    # Ensure cluster has at least one worker node
    assert len(node_names) > 0, (
        "Cluster must have at least one worker node with kserve/kernelcache=worker label"
    )

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

        # Wait for KernelCacheNode to be created (controller creates one per node)
        await asyncio.sleep(10)

        # Verify KernelCacheNode is created for each node
        for node_name in node_names:
            kcnode_name = f"kernel-cache-node-{node_name}"
            try:
                kcnode = k8s_client.get_cluster_custom_object(
                    group=constants.KSERVE_GROUP,
                    version=constants.KSERVE_V1ALPHA1_VERSION,
                    plural=constants.KSERVE_PLURAL_KERNELCACHENODE,
                    name=kcnode_name,
                )

                # Verify GPU detection populated GPUInfo (stub mode)
                assert "status" in kcnode, (
                    f"KernelCacheNode {kcnode_name} should have status"
                )
                assert "gpuInfo" in kcnode["status"], (
                    f"KernelCacheNode {kcnode_name} should have gpuInfo"
                )

                # In stub mode (noGPU=true), we should see AMD MI210 GPUs
                gpu_info = kcnode["status"]["gpuInfo"]
                assert len(gpu_info) > 0, (
                    f"KernelCacheNode {kcnode_name} should detect GPUs in stub mode"
                )

                # Verify GPU info structure
                for gpu in gpu_info:
                    assert "gpuType" in gpu, "GPU info should have gpuType"
                    assert "ids" in gpu, "GPU info should have ids"
                    assert "driverVersion" in gpu, "GPU info should have driverVersion"

            except ApiException as e:
                if e.status == 404:
                    pytest.fail(
                        f"KernelCacheNode {kcnode_name} was not created for node {node_name}"
                    )
                raise

        # Wait for extraction Job to be created
        await asyncio.sleep(15)

        # Verify extraction Job exists
        jobs = batch_v1.list_namespaced_job(
            namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
            label_selector=f"cache={cache_name},cache-namespace={KSERVE_TEST_NAMESPACE}",
        )
        assert len(jobs.items) > 0, "Extraction Job should be created"
        extraction_job = jobs.items[0]

        # Wait for extraction Job to complete (up to 2 minutes)
        for _ in range(24):  # 24 * 5s = 2 minutes
            job = batch_v1.read_namespaced_job(
                name=extraction_job.metadata.name,
                namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
            )

            if job.status.succeeded and job.status.succeeded > 0:
                break

            if job.status.failed and job.status.failed > 0:
                pytest.fail(f"Extraction Job {extraction_job.metadata.name} failed")

            await asyncio.sleep(5)
        else:
            pytest.fail("Extraction Job did not complete within 2 minutes")

        # Wait for cache to become Ready
        await asyncio.sleep(10)

        # Verify KernelCache is ready (all copies available, no failures)
        kc = k8s_client.get_namespaced_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            namespace=KSERVE_TEST_NAMESPACE,
            plural=constants.KSERVE_PLURAL_KERNELCACHE,
            name=cache_name,
        )

        assert "status" in kc, "KernelCache should have status"
        assert "cacheCopies" in kc["status"], (
            "KernelCache status should have cacheCopies"
        )

        cache_copies = kc["status"]["cacheCopies"]
        assert cache_copies["available"] > 0, (
            "KernelCache should have at least one available copy"
        )
        assert cache_copies["failed"] == 0, (
            f"KernelCache should have no failed copies, got {cache_copies['failed']}"
        )
        assert cache_copies["available"] == cache_copies["total"], (
            f"KernelCache should have all copies available, got {cache_copies['available']}/{cache_copies['total']}"
        )

        # Verify ServingPVC was created (name matches cache name)
        pvcs = core_v1.list_namespaced_persistent_volume_claim(
            namespace=KSERVE_TEST_NAMESPACE,
        )
        serving_pvc_exists = any(pvc.metadata.name == cache_name for pvc in pvcs.items)
        assert serving_pvc_exists, f"ServingPVC {cache_name} should be created"

        # Verify download PVC was created (format: <namespace>-<cache-name>-download)
        download_pvcs = core_v1.list_namespaced_persistent_volume_claim(
            namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
        )
        expected_download_pvc = f"{KSERVE_TEST_NAMESPACE}-{cache_name}-download"
        download_pvc_exists = any(
            pvc.metadata.name == expected_download_pvc for pvc in download_pvcs.items
        )
        assert download_pvc_exists, (
            f"Download PVC {expected_download_pvc} should be created"
        )

    finally:
        # Cleanup
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

        # Wait for finalizer to clean up resources
        await asyncio.sleep(30)
