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
async def test_kernelcache_deletion_with_finalizer():
    """
    Test KernelCache deletion with finalizer cleanup.

    This test verifies:
    1. KernelCache deletion triggers finalizer
    2. Finalizer cleans up associated resources (Jobs, PVCs, PVs)
    3. KernelCache is removed after cleanup completes
    """
    cache_name = "test-kernel-cache-delete"
    cache_image = "quay.io/gkm/cache-examples:vector-add-cache-rocm-v2"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance
    core_v1 = client.CoreV1Api()
    batch_v1 = client.BatchV1Api()

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

        # Wait for extraction Job to be created
        await asyncio.sleep(15)

        # Verify resources exist before deletion
        jobs = batch_v1.list_namespaced_job(
            namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
            label_selector=f"cache={cache_name},cache-namespace={KSERVE_TEST_NAMESPACE}",
        )
        assert len(jobs.items) > 0, "Extraction Job should exist before deletion"

        # Wait for job to complete
        for _ in range(24):  # 24 * 5s = 2 minutes
            job = batch_v1.read_namespaced_job(
                name=jobs.items[0].metadata.name,
                namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
            )
            if job.status.succeeded and job.status.succeeded > 0:
                break
            await asyncio.sleep(5)

        # Wait for cache to be Ready
        await asyncio.sleep(10)

        # Delete the KernelCache
        k8s_client.delete_namespaced_custom_object(
            group=constants.KSERVE_GROUP,
            version=constants.KSERVE_V1ALPHA1_VERSION,
            namespace=KSERVE_TEST_NAMESPACE,
            plural=constants.KSERVE_PLURAL_KERNELCACHE,
            name=cache_name,
        )

        # Wait for finalizer to clean up resources (up to 1 minute)
        await asyncio.sleep(30)

        # Verify extraction Job is cleaned up
        jobs = batch_v1.list_namespaced_job(
            namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
            label_selector=f"cache={cache_name},cache-namespace={KSERVE_TEST_NAMESPACE}",
        )
        assert len(jobs.items) == 0, "Extraction Job should be deleted by finalizer"

        # Verify ServingPVC is cleaned up (name matches cache name)
        pvcs = core_v1.list_namespaced_persistent_volume_claim(
            namespace=KSERVE_TEST_NAMESPACE,
        )
        serving_pvc_exists = any(pvc.metadata.name == cache_name for pvc in pvcs.items)
        assert not serving_pvc_exists, "ServingPVC should be deleted by finalizer"

        # Verify download PVC is cleaned up (format: <namespace>-<cache-name>-download)
        download_pvcs = core_v1.list_namespaced_persistent_volume_claim(
            namespace=constants.INFERENCESERVICE_SYSTEM_NAMESPACE,
        )
        expected_download_pvc = f"{KSERVE_TEST_NAMESPACE}-{cache_name}-download"
        download_pvc_exists = any(
            pvc.metadata.name == expected_download_pvc
            for pvc in download_pvcs.items
        )
        assert not download_pvc_exists, "Download PVC should be deleted by finalizer"

        # Verify KernelCache CR is removed
        try:
            k8s_client.get_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            pytest.fail("KernelCache should be deleted after finalizer cleanup")
        except ApiException as e:
            assert e.status == 404, "KernelCache should return 404 after deletion"

    finally:
        # Ensure cleanup even if test fails
        try:
            k8s_client.delete_namespaced_custom_object(
                group=constants.KSERVE_GROUP,
                version=constants.KSERVE_V1ALPHA1_VERSION,
                namespace=KSERVE_TEST_NAMESPACE,
                plural=constants.KSERVE_PLURAL_KERNELCACHE,
                name=cache_name,
            )
            await asyncio.sleep(30)
        except ApiException:
            pass


@pytest.mark.kernelcache
@pytest.mark.asyncio(scope="session")
async def test_kernelcache_deletion_validation():
    """
    Test KernelCache deletion validation when cache is in use.

    This test verifies:
    1. Cache with ServingStatus.TotalPods > 0 cannot be deleted
    2. Validating webhook blocks deletion
    """
    # Note: This test requires a pod to be using the cache
    # For Phase 1, ServingStatus is not yet populated by agent
    # This test is a placeholder for future Phase 2 implementation
    pytest.skip("ServingStatus not yet implemented in Phase 1")
