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

"""E2E coverage for LLMInferenceService storageContainerName.

Does not wait for the workload to become Ready. The named ClusterStorageContainer
uses a sentinel image so we only assert controller wiring and CEL admission.
"""

from __future__ import annotations

import os

import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .fixtures import generate_k8s_safe_suffix, inject_k8s_proxy
from .logging import log_execution
from .test_llm_inference_service import wait_for

KSERVE_PLURAL_LLMINFERENCESERVICE = "llminferenceservices"
CLUSTER_STORAGE_CONTAINERS = "clusterstoragecontainers"
STORAGE_INITIALIZER_CONTAINER = "storage-initializer"
CSC_IMAGE = "example.com/custom-storage-initializer:e2e"


def _kserve_client() -> KServeClient:
    inject_k8s_proxy()
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


def _llmisvc_body(name: str, namespace: str, version: str, storage_initializer: dict):
    return {
        "apiVersion": f"{constants.KSERVE_GROUP}/{version}",
        "kind": "LLMInferenceService",
        "metadata": {"name": name, "namespace": namespace},
        "spec": {
            "model": {"uri": "custom://models/llama", "name": "foo"},
            "storageInitializer": storage_initializer,
            "template": {
                "containers": [
                    {
                        "name": "main",
                        "image": "busybox:1.36",
                        "command": ["sleep", "3600"],
                    }
                ]
            },
        },
    }


def _create_llmisvc(kserve_client: KServeClient, body: dict, version: str):
    return kserve_client.api_instance.create_namespaced_custom_object(
        constants.KSERVE_GROUP,
        version,
        body["metadata"]["namespace"],
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        body,
    )


def _delete_llmisvc(
    kserve_client: KServeClient, name: str, namespace: str, version: str
):
    try:
        kserve_client.api_instance.delete_namespaced_custom_object(
            constants.KSERVE_GROUP,
            version,
            namespace,
            KSERVE_PLURAL_LLMINFERENCESERVICE,
            name,
        )
    except client.rest.ApiException as e:
        if e.status != 404:
            raise


def _create_cluster_storage_container(kserve_client: KServeClient, name: str):
    body = {
        "apiVersion": f"{constants.KSERVE_GROUP}/{constants.KSERVE_V1ALPHA1_VERSION}",
        "kind": "ClusterStorageContainer",
        "metadata": {"name": name},
        "spec": {
            "container": {
                "name": STORAGE_INITIALIZER_CONTAINER,
                "image": CSC_IMAGE,
                "env": [{"name": "CUSTOM_STORAGE", "value": "enabled"}],
            },
            "supportedUriFormats": [{"prefix": "custom://"}],
            "workloadType": "initContainer",
        },
    }
    return kserve_client.api_instance.create_cluster_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        CLUSTER_STORAGE_CONTAINERS,
        body,
    )


def _delete_cluster_storage_container(kserve_client: KServeClient, name: str):
    try:
        kserve_client.api_instance.delete_cluster_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            CLUSTER_STORAGE_CONTAINERS,
            name,
        )
    except client.rest.ApiException as e:
        if e.status != 404:
            raise


@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
@pytest.mark.parametrize(
    "version",
    [constants.KSERVE_V1ALPHA2_VERSION, constants.KSERVE_V1ALPHA1_VERSION],
)
def test_storage_container_name_rejected_when_disabled(test_namespace, version):
    """CEL must reject enabled=false together with storageContainerName."""
    kserve_client = _kserve_client()
    name = generate_k8s_safe_suffix("e2e-si-disabled", [test_namespace, version])
    body = _llmisvc_body(
        name,
        test_namespace,
        version,
        {"enabled": False, "storageContainerName": "my-csc"},
    )

    with pytest.raises(client.rest.ApiException) as exc_info:
        _create_llmisvc(kserve_client, body, version)

    assert exc_info.value.status == 422, (
        f"Expected 422, got {exc_info.value.status}: {exc_info.value.body}"
    )
    assert "storageContainerName cannot be set when enabled is false" in str(
        exc_info.value.body
    )


@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@log_execution
def test_named_cluster_storage_container_merged_into_init_container(test_namespace):
    """Controller must merge the named ClusterStorageContainer onto storage-initializer."""
    kserve_client = _kserve_client()
    apps = client.AppsV1Api()
    csc_name = generate_k8s_safe_suffix("e2e-csc", [test_namespace])
    svc_name = generate_k8s_safe_suffix("e2e-si-named", [test_namespace])
    version = constants.KSERVE_V1ALPHA2_VERSION

    _create_cluster_storage_container(kserve_client, csc_name)
    created = False
    try:
        body = _llmisvc_body(
            svc_name,
            test_namespace,
            version,
            {"storageContainerName": csc_name},
        )
        _create_llmisvc(kserve_client, body, version)
        created = True

        def assert_named_init_container():
            try:
                deployment = apps.read_namespaced_deployment(
                    f"{svc_name}-kserve", test_namespace
                )
            except client.rest.ApiException as e:
                if e.status == 404:
                    raise AssertionError("deployment not created yet") from e
                raise
            inits = deployment.spec.template.spec.init_containers or []
            named = [c for c in inits if c.name == STORAGE_INITIALIZER_CONTAINER]
            assert named, (
                f"storage-initializer missing; init containers: {[c.name for c in inits]}"
            )
            init = named[0]
            assert init.image == CSC_IMAGE, f"unexpected image {init.image}"
            env = {e.name: e.value for e in (init.env or [])}
            assert env.get("CUSTOM_STORAGE") == "enabled"

        wait_for(assert_named_init_container, timeout=120.0, interval=2.0)
    finally:
        if created:
            _delete_llmisvc(kserve_client, svc_name, test_namespace, version)
        _delete_cluster_storage_container(kserve_client, csc_name)
