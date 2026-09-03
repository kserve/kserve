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

import asyncio
import os

import pytest
from kubernetes import client
from kubernetes.client import (
    V1ResourceRequirements,
    V1PersistentVolumeClaimSpec,
)
from kubernetes.client.exceptions import ApiException

from kserve import constants
from kserve.api.kserve_client import KServeClient
from kserve.models.v1alpha1_local_model_namespace_cache import (
    V1alpha1LocalModelNamespaceCache,
)
from kserve.models.v1alpha1_local_model_namespace_cache_spec import (
    V1alpha1LocalModelNamespaceCacheSpec,
)
from kserve.models.v1beta1_inference_service import V1beta1InferenceService
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec
from kserve.models.v1beta1_predictor_spec import V1beta1PredictorSpec
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc

# The RWX StorageClass provisioned by test/scripts/gh-actions/setup-nfs-csi.sh.
RWX_STORAGE_CLASS = "nfs-csi"
INFERENCESERVICE_LABEL = "serving.kserve.io/inferenceservice"


def _cache_ready_reason(kserve_client, name, namespace):
    """Return (ready_status, reason) from the cache's Ready condition, or (None, None)."""
    cache = kserve_client.get_local_model_namespace_cache(name, namespace)
    for cond in cache.get("status", {}).get("conditions", []):
        if cond.get("type") == "Ready":
            return cond.get("status"), cond.get("reason")
    return None, None


async def _wait_shared_cache_ready(
    kserve_client, name, namespace, timeout_seconds=600, polling_interval=10
):
    """Wait for a shared-PVC cache to report Ready=True.

    The node-based readiness helper does not apply here: shared-PVC caches carry
    an empty status.nodeStatus and signal readiness only through the Ready
    condition.
    """
    for _ in range(round(timeout_seconds / polling_interval)):
        status, reason = _cache_ready_reason(kserve_client, name, namespace)
        if status == "True":
            return reason
        await asyncio.sleep(polling_interval)
    cache = kserve_client.get_local_model_namespace_cache(name, namespace)
    raise RuntimeError(
        f"Timeout waiting for shared-PVC cache to be Ready. Current state: {cache}"
    )


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_sklearn_modelnamespacecache_sharedpvc(rest_v1_client, network_layer):
    service_name = "sklearn-nscache-sharedpvc"
    storage_uri = "gs://kfserving-examples/models/sklearn/1.0/model"
    pvc_name = "shared-models-sharedpvc"
    cache_name = "sklearn-model-sharedpvc"

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    core_api = client.CoreV1Api()

    # Pre-create the shared RWX PVC (user-owned; the controller never mutates it).
    pvc = client.V1PersistentVolumeClaim(
        metadata=client.V1ObjectMeta(name=pvc_name, namespace=KSERVE_TEST_NAMESPACE),
        spec=V1PersistentVolumeClaimSpec(
            access_modes=["ReadWriteMany"],
            volume_mode="Filesystem",
            resources=V1ResourceRequirements(requests={"storage": "1Gi"}),
            storage_class_name=RWX_STORAGE_CLASS,
        ),
    )
    core_api.create_namespaced_persistent_volume_claim(KSERVE_TEST_NAMESPACE, pvc)

    # Shared-PVC cache: pvcRef instead of nodeGroups.
    model_cache = V1alpha1LocalModelNamespaceCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELNAMESPACECACHE,
        metadata=client.V1ObjectMeta(
            name=cache_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1alpha1LocalModelNamespaceCacheSpec(
            model_size="10Mi",
            pvc_ref=pvc_name,
            source_model_uri=storage_uri,
        ),
    )

    # Two replicas forced onto different nodes prove that a single imported copy
    # is mounted concurrently across nodes from shared RWX storage.
    predictor = V1beta1PredictorSpec(
        min_replicas=2,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            runtime="kserve-sklearnserver",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "500m", "memory": "512Mi"},
            ),
            storage_uri=storage_uri,
        ),
        affinity=client.V1Affinity(
            pod_anti_affinity=client.V1PodAntiAffinity(
                required_during_scheduling_ignored_during_execution=[
                    client.V1PodAffinityTerm(
                        topology_key="kubernetes.io/hostname",
                        label_selector=client.V1LabelSelector(
                            match_labels={INFERENCESERVICE_LABEL: service_name}
                        ),
                    )
                ]
            )
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create_local_model_namespace_cache(
            model_cache, namespace=KSERVE_TEST_NAMESPACE
        )
        reason = await _wait_shared_cache_ready(
            kserve_client, cache_name, KSERVE_TEST_NAMESPACE
        )
        assert reason == "ImportSucceeded"

        # Shared-PVC mode reports one available copy and no per-node status.
        cache = kserve_client.get_local_model_namespace_cache(
            cache_name, KSERVE_TEST_NAMESPACE
        )
        model_copies = cache.get("status", {}).get("modelCopies", {})
        assert model_copies.get("total") == 1
        assert model_copies.get("available") == 1
        assert not cache.get("status", {}).get("nodeStatus")

        # No node fan-out: no LocalModelNode references the shared-PVC cache.
        k8s_client = kserve_client.api_instance
        local_model_nodes = k8s_client.list_cluster_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            constants.KSERVE_PLURAL_LOCALMODELNODE,
        )
        status_key = f"{KSERVE_TEST_NAMESPACE}/{cache_name}"
        for node in local_model_nodes.get("items", []):
            assert status_key not in node.get("status", {}).get("modelStatus", {})

        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        # Serving pods: model already present on the shared volume, so no
        # storage-initializer transfer container, and replicas span >1 node.
        pods = core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"{INFERENCESERVICE_LABEL}={service_name}",
        )
        running = [p for p in pods.items if p.status.phase == "Running"]
        assert len(running) >= 2
        nodes_used = set()
        for pod in running:
            init_names = [c.name for c in (pod.spec.init_containers or [])]
            assert "storage-initializer" not in init_names
            nodes_used.add(pod.spec.node_name)
        assert len(nodes_used) >= 2, f"replicas did not span nodes: {nodes_used}"

        res = await predict_isvc(
            rest_v1_client,
            service_name,
            "./data/iris_input.json",
            network_layer=network_layer,
        )
        assert res["predictions"] == [1, 1]
    finally:
        try:
            kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
            # Let the isvc release the cache before deleting it.
            await asyncio.sleep(30)
        except ApiException:
            pass
        try:
            kserve_client.delete_local_model_namespace_cache(
                cache_name, namespace=KSERVE_TEST_NAMESPACE
            )
        except ApiException:
            pass

        # The user-owned PVC is never deleted by the controller: it survives cache
        # deletion. Clean it up explicitly.
        surviving = core_api.read_namespaced_persistent_volume_claim(
            pvc_name, KSERVE_TEST_NAMESPACE
        )
        assert surviving is not None
        core_api.delete_namespaced_persistent_volume_claim(
            pvc_name, KSERVE_TEST_NAMESPACE
        )
