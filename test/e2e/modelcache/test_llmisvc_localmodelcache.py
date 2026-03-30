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


KSERVE_V1ALPHA2_VERSION = "v1alpha2"
LLMISVC_PLURAL = "llminferenceservices"


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_llmisvc_localmodelcache_labels():
    """Test that LLMInferenceService gets local model labels when a matching cache exists."""
    storage_uri = "hf://test-org/test-model-for-cache"
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
        metadata=client.V1ObjectMeta(name="llmisvc-test-nodegroup"),
        spec=V1alpha1LocalModelNodeGroupSpec(
            storage_limit="1Gi",
            persistent_volume_spec=pv_spec,
            persistent_volume_claim_spec=pvc_spec,
        ),
    )

    model_cache = V1alpha1LocalModelCache(
        api_version=constants.KSERVE_V1ALPHA1,
        kind=constants.KSERVE_KIND_LOCALMODELCACHE,
        metadata=client.V1ObjectMeta(name="llmisvc-test-cache"),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="100Mi",
            node_groups=[node_group.metadata.name],
            source_model_uri=storage_uri,
        ),
    )

    # Create an LLMInferenceService with matching model URI using raw k8s client
    llmisvc = {
        "apiVersion": f"serving.kserve.io/{KSERVE_V1ALPHA2_VERSION}",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": "llmisvc-cache-test",
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "model": {
                "uri": storage_uri,
            },
        },
    }

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    k8s_client = kserve_client.api_instance

    try:
        # Create prerequisites
        kserve_client.create_local_model_node_group(node_group)
        kserve_client.create_local_model_cache(model_cache)

        # Create LLMInferenceService - the defaulter webhook should set localmodel labels
        k8s_client.create_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc,
        )

        # Wait for webhook to set labels and for cache controller to update status
        time.sleep(5)

        # Verify the LLMInferenceService has localmodel labels
        llmisvc_result = k8s_client.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            KSERVE_V1ALPHA2_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            "llmisvc-cache-test",
        )

        labels = llmisvc_result.get("metadata", {}).get("labels", {})
        assert "internal.serving.kserve.io/localmodel" in labels, (
            f"Expected localmodel label, got labels: {labels}"
        )
        assert labels["internal.serving.kserve.io/localmodel"] == "llmisvc-test-cache"

        annotations = llmisvc_result.get("metadata", {}).get("annotations", {})
        assert "internal.serving.kserve.io/localmodel-sourceuri" in annotations
        assert (
            annotations["internal.serving.kserve.io/localmodel-sourceuri"]
            == storage_uri
        )

        # Verify the cache status tracks the LLMInferenceService
        # Allow time for the cache controller to reconcile
        max_retries = 10
        for _i in range(max_retries):
            cache_result = k8s_client.get_cluster_custom_object(
                constants.KSERVE_GROUP,
                constants.KSERVE_V1ALPHA1_VERSION,
                "localmodelcaches",
                "llmisvc-test-cache",
            )
            llm_svcs = cache_result.get("status", {}).get("llmInferenceServices", [])
            if len(llm_svcs) == 1:
                assert llm_svcs[0]["name"] == "llmisvc-cache-test"
                assert llm_svcs[0]["namespace"] == KSERVE_TEST_NAMESPACE
                break
            time.sleep(2)
        else:
            pytest.fail(
                f"Cache status did not track LLMInferenceService after {max_retries} retries. "
                f"Status: {cache_result.get('status', {})}"
            )

    finally:
        # Cleanup in reverse order
        try:
            k8s_client.delete_namespaced_custom_object(
                constants.KSERVE_GROUP,
                KSERVE_V1ALPHA2_VERSION,
                KSERVE_TEST_NAMESPACE,
                LLMISVC_PLURAL,
                "llmisvc-cache-test",
            )
        except Exception:
            pass
        try:
            kserve_client.delete_local_model_cache("llmisvc-test-cache")
        except Exception:
            pass
        try:
            kserve_client.delete_local_model_node_group("llmisvc-test-nodegroup")
        except Exception:
            pass
