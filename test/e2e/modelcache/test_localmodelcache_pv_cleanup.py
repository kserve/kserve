# Copyright 2024 The KServe Authors.
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
import logging
import os

import pytest
from kubernetes import client
from kubernetes.client import (
    V1ResourceRequirements,
    V1PersistentVolumeSpec,
    V1LocalVolumeSource,
    V1VolumeNodeAffinity,
    V1PersistentVolumeClaimSpec,
)
from kubernetes.client.exceptions import ApiException

from kserve import constants
from kserve.api.kserve_client import KServeClient
from kserve.models.v1alpha1_local_model_node_group import V1alpha1LocalModelNodeGroup
from kserve.models.v1alpha1_local_model_node_group_spec import (
    V1alpha1LocalModelNodeGroupSpec,
)
from kserve.models.v1alpha1_local_model_cache import V1alpha1LocalModelCache
from kserve.models.v1alpha1_local_model_cache_spec import V1alpha1LocalModelCacheSpec
from kserve.models.v1beta1_inference_service import V1beta1InferenceService
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec
from kserve.models.v1beta1_predictor_spec import V1beta1PredictorSpec
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from ..common.utils import KSERVE_TEST_NAMESPACE

logger = logging.getLogger(__name__)

CLEANUP_POLL_INTERVAL = 5
CLEANUP_TIMEOUT = 120


def _pv_exists(core_api: client.CoreV1Api, name: str) -> bool:
    try:
        core_api.read_persistent_volume(name)
        return True
    except ApiException as e:
        if e.status == 404:
            return False
        raise


def _pvc_exists(core_api: client.CoreV1Api, name: str, namespace: str) -> bool:
    try:
        core_api.read_namespaced_persistent_volume_claim(name, namespace)
        return True
    except ApiException as e:
        if e.status == 404:
            return False
        raise


async def _wait_until_pv_deleted(core_api: client.CoreV1Api, name: str):
    for _ in range(CLEANUP_TIMEOUT // CLEANUP_POLL_INTERVAL):
        if not _pv_exists(core_api, name):
            return
        await asyncio.sleep(CLEANUP_POLL_INTERVAL)
    raise TimeoutError(f"PV {name} was not deleted within {CLEANUP_TIMEOUT}s")


async def _wait_until_pvc_deleted(
    core_api: client.CoreV1Api, name: str, namespace: str
):
    for _ in range(CLEANUP_TIMEOUT // CLEANUP_POLL_INTERVAL):
        if not _pvc_exists(core_api, name, namespace):
            return
        await asyncio.sleep(CLEANUP_POLL_INTERVAL)
    raise TimeoutError(
        f"PVC {name} in {namespace} was not deleted within {CLEANUP_TIMEOUT}s"
    )


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_localmodelcache_pv_pvc_cleanup(rest_v1_client, network_layer):
    """
    Verify that PVs and PVCs created by the localmodel controller are cleaned
    up after the LocalModelCache and its consuming InferenceService are deleted.

    This validates that the localmodel controller ClusterRole includes the
    'delete' verb for persistentvolumes and persistentvolumeclaims.
    """
    service_name = "sklearn-pv-cleanup"
    storage_uri = "gs://kfserving-examples/models/sklearn/1.0/model"
    node_group_name = "cleanup-nodegroup"
    model_cache_name = "cleanup-model"
    nodes = ["minikube-m02", "minikube-m03"]

    pv_spec = V1PersistentVolumeSpec(
        access_modes=["ReadWriteOnce"],
        storage_class_name="standard",
        capacity={"storage": "1Gi"},
        local=V1LocalVolumeSource(path="/models"),
        persistent_volume_reclaim_policy="Delete",
        node_affinity=V1VolumeNodeAffinity(
            required=client.V1NodeSelector(
                node_selector_terms=[
                    client.V1NodeSelectorTerm(
                        match_expressions=[
                            client.V1NodeSelectorRequirement(
                                key="kubernetes.io/hostname",
                                operator="In",
                                values=nodes,
                            )
                        ]
                    )
                ]
            )
        ),
    )
    pvc_spec = V1PersistentVolumeClaimSpec(
        access_modes=["ReadWriteOnce"],
        resources=V1ResourceRequirements(requests={"storage": "1Gi"}),
        storage_class_name="standard",
    )

    node_group = V1alpha1LocalModelNodeGroup(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELNODEGROUP,
        metadata=client.V1ObjectMeta(name=node_group_name),
        spec=V1alpha1LocalModelNodeGroupSpec(
            storage_limit="1Gi",
            persistent_volume_spec=pv_spec,
            persistent_volume_claim_spec=pvc_spec,
        ),
    )

    model_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(name=model_cache_name),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="10Mi",
            node_groups=[node_group_name],
            source_model_uri=storage_uri,
        ),
    )

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            runtime="kserve-sklearnserver",
            resources=V1ResourceRequirements(
                requests={"cpu": "100m", "memory": "256Mi"},
                limits={"cpu": "500m", "memory": "512Mi"},
            ),
            storage_uri=storage_uri,
        ),
        node_selector={"kubernetes.io/hostname": nodes[0]},
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    core_api = client.CoreV1Api()

    # --- Setup: create node group, model cache, and ISVC ---
    kserve_client.create_local_model_node_group(node_group)
    kserve_client.create_local_model_cache(model_cache)
    kserve_client.wait_local_model_cache_ready(model_cache_name, nodes=nodes)
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

    # Serving PV/PVC naming: {modelName}-{nodeGroup}-{namespace}
    serving_pv_name = f"{model_cache_name}-{node_group_name}-{KSERVE_TEST_NAMESPACE}"
    serving_pvc_name = f"{model_cache_name}-{node_group_name}"

    # --- Verify PVs/PVCs exist before deletion ---
    assert _pv_exists(core_api, serving_pv_name), (
        f"Serving PV {serving_pv_name} should exist while ISVC is running"
    )
    assert _pvc_exists(core_api, serving_pvc_name, KSERVE_TEST_NAMESPACE), (
        f"Serving PVC {serving_pvc_name} should exist while ISVC is running"
    )
    logger.info(
        "Verified serving PV %s and PVC %s exist", serving_pv_name, serving_pvc_name
    )

    # --- Delete ISVC first, then LocalModelCache ---
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    await asyncio.sleep(30)

    kserve_client.delete_local_model_cache(model_cache_name)

    # --- Verify PVs/PVCs are cleaned up ---
    # For cluster-scoped LocalModelCache, PVs have owner references and are
    # garbage-collected. The controller also needs 'delete' RBAC to explicitly
    # clean up PVs/PVCs in ReconcileForIsvcs when ISVCs are removed.
    await _wait_until_pv_deleted(core_api, serving_pv_name)
    logger.info("Serving PV %s successfully cleaned up", serving_pv_name)

    await _wait_until_pvc_deleted(core_api, serving_pvc_name, KSERVE_TEST_NAMESPACE)
    logger.info("Serving PVC %s successfully cleaned up", serving_pvc_name)

    # --- Cleanup ---
    kserve_client.delete_local_model_node_group(node_group_name)
