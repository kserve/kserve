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

import json
import os
import time
import uuid

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
from ..common.utils import KSERVE_TEST_NAMESPACE


KSERVE_V1ALPHA2_VERSION = "v1alpha2"
LLMISVC_PLURAL = "llminferenceservices"
LOCALMODEL_LABEL = "internal.serving.kserve.io/localmodel"
LOCALMODEL_LORA_ANNOTATION = "internal.serving.kserve.io/localmodel-lora"

ADAPTER_NAME = "lora-adapter"
NODES = ["minikube-m02", "minikube-m03"]


def _node_group_pv_pvc_specs():
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
                                values=NODES,
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
    return pv_spec, pvc_spec


def _wait_for_llmisvc_deployment(
    apps_api: client.AppsV1Api, llmisvc_name: str, namespace: str, timeout: int = 60
) -> dict:
    deployment_name = f"{llmisvc_name}-kserve"
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            deployment = apps_api.read_namespaced_deployment(deployment_name, namespace)
            return deployment.to_dict()
        except ApiException as exc:
            if exc.status != 404:
                raise
        time.sleep(2)
    pytest.fail(
        f"Deployment {deployment_name} was not created in namespace {namespace} within {timeout}s"
    )


def _pod_spec(deployment: dict) -> dict:
    return deployment.get("spec", {}).get("template", {}).get("spec", {}) or {}


def _storage_initializer_args(deployment: dict) -> list[str]:
    init_containers = _pod_spec(deployment).get("init_containers") or []
    for container in init_containers:
        if container.get("name") == "storage-initializer":
            return container.get("args") or []
    return []


def _volume_names(deployment: dict) -> set[str]:
    volumes = _pod_spec(deployment).get("volumes") or []
    return {volume.get("name") for volume in volumes}


def _pvc_claim_names(deployment: dict) -> set[str]:
    volumes = _pod_spec(deployment).get("volumes") or []
    return {
        volume.get("persistent_volume_claim", {}).get("claim_name")
        for volume in volumes
        if volume.get("persistent_volume_claim")
    }


def _wait_until_deleted(get_fn, timeout: int = 60) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            get_fn()
        except ApiException as exc:
            if exc.status == 404:
                return True
            raise
        time.sleep(2)
    return False


def _cleanup_test_resources(
    kserve_client: KServeClient,
    k8s_client,
    *,
    llmisvc_name: str,
    base_cache_name: str,
    adapter_cache_name: str,
    node_group_name: str,
) -> None:
    try:
        k8s_client.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc_name,
        )
    except Exception:
        pass

    _wait_until_deleted(
        lambda: k8s_client.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc_name,
        ),
        timeout=120,
    )

    for cache_name in (adapter_cache_name, base_cache_name):
        try:
            kserve_client.delete_local_model_cache(cache_name)
        except Exception:
            pass
        _wait_until_deleted(
            lambda cache_name=cache_name: k8s_client.get_cluster_custom_object(
                constants.KSERVE_GROUP,
                constants.KSERVE_V1ALPHA1_VERSION,
                "localmodelcaches",
                cache_name,
            ),
            timeout=120,
        )

    try:
        kserve_client.delete_local_model_node_group(node_group_name)
    except Exception:
        pass
    _wait_until_deleted(
        lambda: k8s_client.get_cluster_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            constants.KSERVE_PLURAL_LOCALMODELNODEGROUP,
            node_group_name,
        ),
        timeout=120,
    )


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_llmisvc_localmodelcache_lora_adapters():
    """Test LocalModelCache integration for LLMISVC LoRA adapters."""
    suffix = uuid.uuid4().hex[:8]
    node_group_name = f"llmisvc-lora-ng-{suffix}"
    base_cache_name = f"llmisvc-lora-base-{suffix}"
    adapter_cache_name = f"llmisvc-lora-adapter-{suffix}"
    llmisvc_name = f"llmisvc-lora-e2e-{suffix}"
    base_uri = f"hf://test-org/lora-e2e-base-{suffix}"
    adapter_uri = f"hf://test-org/lora-e2e-adapter-{suffix}"

    pv_spec, pvc_spec = _node_group_pv_pvc_specs()

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

    base_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(name=base_cache_name),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="100Mi",
            node_groups=[node_group_name],
            source_model_uri=base_uri,
        ),
    )

    adapter_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(name=adapter_cache_name),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="100Mi",
            node_groups=[node_group_name],
            source_model_uri=adapter_uri,
        ),
    )

    llmisvc = {
        "apiVersion": f"serving.kserve.io/{KSERVE_V1ALPHA2_VERSION}",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": llmisvc_name,
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "model": {
                "uri": base_uri,
                "lora": {
                    "adapters": [
                        {
                            "name": ADAPTER_NAME,
                            "uri": adapter_uri,
                        }
                    ]
                },
            },
        },
    }

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance
    apps_api = client.AppsV1Api()

    try:
        kserve_client.create_local_model_node_group(node_group)
        kserve_client.create_local_model_cache(base_cache)
        kserve_client.create_local_model_cache(adapter_cache)

        k8s_client.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc,
        )

        time.sleep(5)

        llmisvc_result = k8s_client.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc_name,
        )

        labels = llmisvc_result.get("metadata", {}).get("labels", {})
        assert labels.get(LOCALMODEL_LABEL) == base_cache_name

        annotations = llmisvc_result.get("metadata", {}).get("annotations", {})
        lora_raw = annotations.get(LOCALMODEL_LORA_ANNOTATION)
        assert lora_raw, f"Expected {LOCALMODEL_LORA_ANNOTATION} annotation"
        lora_entries = json.loads(lora_raw)
        assert lora_entries[ADAPTER_NAME]["cache"] == adapter_cache_name
        assert lora_entries[ADAPTER_NAME]["sourceUri"] == adapter_uri

        deployment = _wait_for_llmisvc_deployment(
            apps_api, llmisvc_name, KSERVE_TEST_NAMESPACE
        )
        volume_names = _volume_names(deployment)
        assert f"lora-pvc-{ADAPTER_NAME}" in volume_names
        assert f"{base_cache_name}-{node_group_name}" in _pvc_claim_names(deployment)

        init_args = _storage_initializer_args(deployment)
        assert base_uri not in init_args
        assert adapter_uri not in init_args

        for cache_name in (base_cache_name, adapter_cache_name):
            max_retries = 10
            for _ in range(max_retries):
                cache_result = k8s_client.get_cluster_custom_object(
                    constants.KSERVE_GROUP,
                    constants.KSERVE_V1ALPHA1_VERSION,
                    "localmodelcaches",
                    cache_name,
                )
                llm_svcs = cache_result.get("status", {}).get(
                    "llmInferenceServices", []
                )
                if any(
                    svc.get("name") == llmisvc_name
                    and svc.get("namespace") == KSERVE_TEST_NAMESPACE
                    for svc in llm_svcs
                ):
                    break
                time.sleep(2)
            else:
                pytest.fail(
                    f"Cache {cache_name} did not track {llmisvc_name} after {max_retries} retries"
                )

        with pytest.raises(ApiException) as exc:
            k8s_client.delete_cluster_custom_object(
                constants.KSERVE_GROUP,
                constants.KSERVE_V1ALPHA1_VERSION,
                "localmodelcaches",
                adapter_cache_name,
            )
        assert exc.value.status == 403

    finally:
        _cleanup_test_resources(
            kserve_client,
            k8s_client,
            llmisvc_name=llmisvc_name,
            base_cache_name=base_cache_name,
            adapter_cache_name=adapter_cache_name,
            node_group_name=node_group_name,
        )
