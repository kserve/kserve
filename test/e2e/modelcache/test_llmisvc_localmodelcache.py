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
import time

import pytest
from kubernetes import client
from kubernetes.client import (
    V1ResourceRequirements,
    V1PersistentVolumeSpec,
    V1LocalVolumeSource,
    V1VolumeNodeAffinity,
    V1PersistentVolumeClaimSpec,
)

from kserve import constants
from kserve.api.kserve_client import KServeClient
from kserve.models.v1alpha1_local_model_node_group import V1alpha1LocalModelNodeGroup
from kserve.models.v1alpha1_local_model_node_group_spec import (
    V1alpha1LocalModelNodeGroupSpec,
)
from kserve.models.v1alpha1_local_model_cache import V1alpha1LocalModelCache
from kserve.models.v1alpha1_local_model_cache_spec import V1alpha1LocalModelCacheSpec
from ..common.utils import KSERVE_TEST_NAMESPACE

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_llmisvc_localmodelcache():
    """
    Test that LLMInferenceService is tracked in LocalModelCache status
    and that local model labels are propagated via the defaulter webhook.
    Uses raw k8s client for LLMInferenceService CRUD.
    """
    storage_uri = "hf://Qwen/Qwen2-0.5B-Instruct"
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
        metadata=client.V1ObjectMeta(
            name="llmisvc-nodegroup",
        ),
        spec=V1alpha1LocalModelNodeGroupSpec(
            storage_limit="1Gi",
            persistent_volume_spec=pv_spec,
            persistent_volume_claim_spec=pvc_spec,
        ),
    )

    model_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(
            name="llmisvc-cache",
        ),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="251Mi",
            node_groups=[node_group.metadata.name],
            source_model_uri=storage_uri,
        ),
    )

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance

    # Create node group and model cache
    kserve_client.create_local_model_node_group(node_group)
    kserve_client.create_local_model_cache(model_cache)
    kserve_client.wait_local_model_cache_ready(
        model_cache.metadata.name, nodes=nodes
    )

    # Create LLMInferenceService using raw k8s client
    # The defaulter webhook should match this to the model cache and set labels
    llmisvc_name = "llmisvc-cache-test"
    llmisvc_body = {
        "apiVersion": "serving.kserve.io/v1alpha2",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": llmisvc_name,
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "model": {
                "uri": storage_uri,
            },
        },
    }

    k8s_client.create_namespaced_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA2_VERSION,
        KSERVE_TEST_NAMESPACE,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        llmisvc_body,
    )

    try:
        # Verify the defaulter webhook added local model labels
        llmisvc = k8s_client.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llmisvc_name,
        )
        labels = llmisvc.get("metadata", {}).get("labels", {})
        assert labels.get("internal.serving.kserve.io/localmodel") == model_cache.metadata.name, \
            f"Expected local model label to be set to {model_cache.metadata.name}, got {labels}"

        # Verify the model cache status tracks the LLMInferenceService
        # Poll for up to 30 seconds
        deadline = time.time() + 30
        tracked = False
        while time.time() < deadline:
            cache_obj = k8s_client.get_cluster_custom_object(
                constants.KSERVE_GROUP,
                constants.KSERVE_V1ALPHA1_VERSION,
                constants.KSERVE_PLURAL_LOCALMODELCACHE,
                model_cache.metadata.name,
            )
            llm_svcs = cache_obj.get("status", {}).get("llmInferenceServices", [])
            for svc in llm_svcs:
                if svc.get("name") == llmisvc_name and svc.get("namespace") == KSERVE_TEST_NAMESPACE:
                    tracked = True
                    break
            if tracked:
                break
            await asyncio.sleep(2)

        assert tracked, (
            f"LocalModelCache status should track LLMInferenceService {llmisvc_name}. "
            f"Got llmInferenceServices: {llm_svcs}"
        )

    finally:
        # Cleanup
        k8s_client.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            llmisvc_name,
        )
        await asyncio.sleep(10)
        kserve_client.delete_local_model_cache(model_cache.metadata.name)
        kserve_client.delete_local_model_node_group(node_group.metadata.name)
