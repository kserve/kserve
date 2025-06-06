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
from ..common.utils import KSERVE_TEST_NAMESPACE, generate


@pytest.mark.modelcache
@pytest.mark.asyncio(scope="session")
async def test_vllm_modelcache():
    service_name = "qwen-chat-modelcache-worker1"
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
            name="qwen-nodegroup",
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
            name="qwen-model",
        ),
        spec=V1alpha1LocalModelCacheSpec(
            model_size="251Mi",
            node_groups=[node_group.metadata.name],
            source_model_uri=storage_uri,
        ),
    )

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(
                name="huggingface",
            ),
            args=[
                "--model_name",
                "hf-qwen-chat",
                "--max_model_len",
                "512",
                "--dtype",
                "bfloat16",
            ],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "6Gi"},
                limits={"cpu": "2", "memory": "6Gi"},
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
    kserve_client.create_local_model_node_group(node_group)
    kserve_client.create_local_model_cache(model_cache)
    kserve_client.wait_local_model_cache_ready(model_cache.metadata.name, nodes=nodes)
    kserve_client.create(isvc)
    kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)
    k8s_client = kserve_client.api_instance

    # Test the model is cached on the correct nodes
    worker_node_1_cache = k8s_client.get_cluster_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELNODE,
        "minikube-m02",
    )
    worker_node_2_cache = k8s_client.get_cluster_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        constants.KSERVE_PLURAL_LOCALMODELNODE,
        "minikube-m03",
    )
    assert (
        worker_node_1_cache["status"]["modelStatus"][model_cache.metadata.name]
        == "ModelDownloaded"
    )
    assert (
        worker_node_2_cache["status"]["modelStatus"][model_cache.metadata.name]
        == "ModelDownloaded"
    )

    # Test the model is not cached on the controller node
    with pytest.raises(ApiException):
        k8s_client.get_cluster_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            constants.KSERVE_PLURAL_LOCALMODELNODE,
            "minikube",
        )

    res = generate(service_name, "./data/qwen_input_chat.json")
    assert res["choices"][0]["message"]["content"] == "The result of 2 + 2 is 4."
    kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
    # Wait for the isvc to be deleted to avoid modelcache still in use error when deleting the model cache
    await asyncio.sleep(30)
    kserve_client.delete_local_model_cache(model_cache.metadata.name)
    kserve_client.delete_local_model_node_group(node_group.metadata.name)
