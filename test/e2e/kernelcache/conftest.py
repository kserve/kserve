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

import os

import pytest
from kubernetes import client
from kubernetes.client.exceptions import ApiException

from kserve.constants.constants import (
    KSERVE_GROUP,
    KSERVE_V1ALPHA1_VERSION,
    KSERVE_PLURAL_KERNELCACHENODEGROUP,
)

from ..common.namespace import skip_resource_deletion

from .utils import _load_k8s_config

_KC_NODE_GROUP_NAME = os.environ.get("KERNELCACHE_NODE_GROUP", "kc-test-group")
_KC_NODE_LABEL_KEY = os.environ.get(
    "KERNELCACHE_NODE_LABEL_KEY", "kernelcache.example.com/group"
)
_KC_NODE_LABEL_VALUE = os.environ.get("KERNELCACHE_NODE_LABEL_VALUE", "workers")

KC_TEST_RUNTIME_NAME = "kernelcache-test-runtime"


@pytest.fixture(scope="session")
def kc_node_label_selector() -> str:
    return f"{_KC_NODE_LABEL_KEY}={_KC_NODE_LABEL_VALUE}"


@pytest.fixture(scope="session")
def kc_node_group():
    """Create a KernelCacheNodeGroup for the session; delete at teardown."""
    _load_k8s_config()
    api = client.CustomObjectsApi()
    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_V1ALPHA1_VERSION}",
        "kind": "KernelCacheNodeGroup",
        "metadata": {"name": _KC_NODE_GROUP_NAME},
        "spec": {
            "nodeSelector": {_KC_NODE_LABEL_KEY: _KC_NODE_LABEL_VALUE},
        },
    }
    try:
        api.create_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            KSERVE_PLURAL_KERNELCACHENODEGROUP,
            body,
        )
    except ApiException as e:
        if e.status != 409:
            raise

    yield _KC_NODE_GROUP_NAME

    if skip_resource_deletion():
        return
    try:
        api.delete_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            KSERVE_PLURAL_KERNELCACHENODEGROUP,
            _KC_NODE_GROUP_NAME,
        )
    except ApiException as e:
        if e.status != 404:
            raise


@pytest.fixture(scope="session")
def kc_test_runtime():
    """Create the kernelcache-test-runtime ClusterServingRuntime; delete at teardown.

    The container writes a file that DetectVLLMCache recognises (path containing
    'inductor_cache') and runs a minimal HTTP server so the readiness probe
    required by the MCV sidecar injection webhook passes.
    """
    _load_k8s_config()
    api = client.CustomObjectsApi()
    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_V1ALPHA1_VERSION}",
        "kind": "ClusterServingRuntime",
        "metadata": {
            "name": KC_TEST_RUNTIME_NAME,
        },
        "spec": {
            # spec.annotations flow to the pod template via the predictor reconciler
            # (predictor.go:181 sRuntimeAnnotations = sRuntime.Annotations), making
            # the annotation visible to the kernelcache pod mutator webhook which
            # checks pod.Annotations[KernelCacheSupportedAnnotationKey] == "true".
            "annotations": {
                "serving.kserve.io/kernelcache-supported": "true",
            },
            "supportedModelFormats": [
                {
                    "name": "test-cache",
                    "version": "1",
                    "autoSelect": True,
                    "priority": 1,
                }
            ],
            "containers": [
                {
                    "name": "kserve-container",
                    # python:3.11-slim provides sh, dd, mkdir, and python3 for the
                    # HTTP server needed by the MCV sidecar injection webhook.
                    "image": "python:3.11-slim",
                    "command": ["/bin/sh", "-c"],
                    "args": [
                        # Ordering matters for MCV capture:
                        # 1. Sleep so MCV starts and takes an empty baseline. 30s
                        #    provides enough margin for MCV image-layer unpacking
                        #    on cold (fresh-cluster) starts.
                        # 2. Write the dummy cache file (now in the delta, not baseline).
                        # 3. Start the HTTP server last — readiness probe passes only
                        #    after the file exists, so MCV captures it immediately.
                        "sleep 30\n"
                        "mkdir -p /root/.cache/vllm/torch_compile_cache/inductor_cache\n"
                        "dd if=/dev/urandom "
                        "of=/root/.cache/vllm/torch_compile_cache/inductor_cache/dummy.cubin "
                        "bs=1k count=10\n"
                        "python3 -m http.server 8080 &\n"
                        "sleep 3600\n"
                    ],
                    "readinessProbe": {
                        "httpGet": {"path": "/", "port": 8080},
                        "failureThreshold": 3,
                        "periodSeconds": 10,
                    },
                    "resources": {
                        "requests": {"cpu": "100m", "memory": "256Mi"},
                        "limits": {"cpu": "500m", "memory": "512Mi"},
                    },
                }
            ],
        },
    }
    try:
        api.create_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            "clusterservingruntimes",
            body,
        )
    except ApiException as e:
        if e.status != 409:
            raise

    yield KC_TEST_RUNTIME_NAME

    if skip_resource_deletion():
        return
    try:
        api.delete_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            "clusterservingruntimes",
            KC_TEST_RUNTIME_NAME,
        )
    except ApiException as e:
        if e.status != 404:
            raise
