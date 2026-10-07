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

import pytest
from kubernetes import client
from kubernetes.client.exceptions import ApiException

from kserve import KServeClient
from kserve.constants.constants import (
    KSERVE_GROUP,
    KSERVE_V1ALPHA1_VERSION,
    KSERVE_PLURAL_KERNELCACHENODEGROUP,
)

from ..common.namespace import (
    create_namespace,
    delete_namespace,
    provision_secrets,
    skip_resource_deletion,
)

from .utils import (
    _load_k8s_config,
    make_producer_isvc,
    wait_for_capture_complete,
    wait_for_kcc_created,
    wait_for_kernelcache_verified,
    wait_for_mcv_sidecar,
    wait_for_resource_deleted,
)

_STORAGE_URI: str = os.environ.get(
    "KERNELCACHE_MODEL_URI",
    "s3://example-models/facebook/opt-125m",
)

_KC_NODE_GROUP_NAME = os.environ.get("KERNELCACHE_NODE_GROUP", "kc-test-group")
_KC_NODE_LABEL_KEY = os.environ.get(
    "KERNELCACHE_NODE_LABEL_KEY", "kernelcache.example.com/group"
)
_KC_NODE_LABEL_VALUE = os.environ.get("KERNELCACHE_NODE_LABEL_VALUE", "workers")
_KC_JOBS_NS = os.environ.get("KERNELCACHE_JOBS_NS", "kserve-kernelcache-jobs")
_KSERVE_NAMESPACE = os.environ.get("KSERVE_NAMESPACE", "kserve")

KC_TEST_RUNTIME_NAME = "kernelcache-test-runtime"


@pytest.fixture(scope="session")
def kc_node_label_selector() -> str:
    return f"{_KC_NODE_LABEL_KEY}={_KC_NODE_LABEL_VALUE}"


@pytest.fixture(scope="session")
def kernelcache_registry_url() -> str:
    """Read the KernelCache OCI registry endpoint from env.

    In CI this is set to the in-cluster registry (e.g. Minikube registry addon
    at registry.kube-system.svc.cluster.local:80). For local runs it can be
    overridden to point to a local registry like localhost:5000.
    """
    registry = os.environ.get("KERNELCACHE_REGISTRY_ENDPOINT")
    if not registry:
        pytest.fail(
            "KERNELCACHE_REGISTRY_ENDPOINT must be set (e.g. 'localhost:5000' or "
            "'registry.kube-system.svc.cluster.local:80')"
        )
    return registry


@pytest.fixture(scope="session")
def kc_config(kernelcache_registry_url):
    """Patch inferenceservice-config ConfigMap with KernelCache settings.

    Re-applies the config on every test session to guard against stale values
    from prior local runs. The settings match what setup-kernelcache.sh applies
    in CI but read the registry endpoint and MCV image from the environment.
    """
    _load_k8s_config()
    core = client.CoreV1Api()

    # MCV image: in CI this is built and loaded locally, read from env.
    # For local runs it can be overridden or fall back to a public image.
    mcv_image = os.environ.get(
        "KERNELCACHE_MCV_IMAGE",
        "kserve/kserve-mcv:latest-minimal",  # fallback for local dev
    )

    config_data = {
        "enabled": True,
        "defaultSidecarInjection": True,
        "defaultMountType": "oci",
        "defaultNodeGroup": _KC_NODE_GROUP_NAME,
        "jobNamespace": _KC_JOBS_NS,
        "mcvImage": mcv_image,
        "prefetchImage": "registry.access.redhat.com/ubi9/ubi-minimal:latest",
        "registry": {
            "endpoint": kernelcache_registry_url,
            "insecure": True,
            "auth": {"type": "none"},
        },
        "artifactSecurity": {
            "mode": "cert",
            "failurePolicy": "reject",
            "cert": {
                "signingProfileRef": "kernelcache-signer",
                "trustBundle": "kserve/kernelcache-root-ca",
                "subjectRegexp": "spiffe://kserve/kernelcache-signer",
            },
        },
        "abandonedCapturePolicy": "retain",
        "jobTTLSecondsAfterFinished": 600,
        "mcvCaptureReadinessTimeoutSeconds": 600,
        "reconcileIntervalSeconds": 300,
    }

    # Patch the ConfigMap with the KernelCache config JSON.
    patch = [
        {
            "op": "replace",
            "path": "/data/kernelcache",
            "value": json.dumps(config_data),
        }
    ]

    try:
        core.patch_namespaced_config_map(
            name="inferenceservice-config",
            namespace=_KSERVE_NAMESPACE,
            body=patch,
            _content_type="application/json-patch+json",
        )
    except ApiException as e:
        pytest.fail(
            f"Failed to patch inferenceservice-config ConfigMap: {e.status} {e.reason}"
        )

    yield config_data


@pytest.fixture(scope="session")
def kc_node_group(kc_config):
    """Create a KernelCacheNodeGroup for the session; delete at teardown.

    Deletes any existing node group with the same name before creating to ensure
    fresh state (handles interrupted prior runs and avoids testing against stale specs).
    """
    _load_k8s_config()
    api = client.CustomObjectsApi()

    # Delete any existing node group to ensure fresh state.
    try:
        api.delete_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            KSERVE_PLURAL_KERNELCACHENODEGROUP,
            _KC_NODE_GROUP_NAME,
        )
        # Wait for deletion to complete before creating a new resource with the same name.
        # Kubernetes deletion is asynchronous — the delete call returns immediately, but
        # the resource may still be finalizing. Creating immediately could fail with 409.
        wait_for_resource_deleted(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            KSERVE_PLURAL_KERNELCACHENODEGROUP,
            _KC_NODE_GROUP_NAME,
            timeout=30,
        )
    except ApiException as e:
        if e.status != 404:
            raise

    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_V1ALPHA1_VERSION}",
        "kind": "KernelCacheNodeGroup",
        "metadata": {"name": _KC_NODE_GROUP_NAME},
        "spec": {
            "nodeSelector": {_KC_NODE_LABEL_KEY: _KC_NODE_LABEL_VALUE},
        },
    }
    api.create_cluster_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        KSERVE_PLURAL_KERNELCACHENODEGROUP,
        body,
    )

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
def kc_test_runtime(kc_config):
    """Create the kernelcache-test-runtime ClusterServingRuntime; delete at teardown.

    Deletes any existing runtime with the same name before creating to ensure
    fresh state (handles interrupted prior runs and avoids testing against stale specs).

    The container writes a file that DetectVLLMCache recognises (path containing
    'inductor_cache') and runs a minimal HTTP server so the readiness probe
    required by the MCV sidecar injection webhook passes.
    """
    _load_k8s_config()
    api = client.CustomObjectsApi()

    # Delete any existing runtime to ensure fresh state.
    try:
        api.delete_cluster_custom_object(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            "clusterservingruntimes",
            KC_TEST_RUNTIME_NAME,
        )
        # Wait for deletion to complete before creating a new resource with the same name.
        wait_for_resource_deleted(
            KSERVE_GROUP,
            KSERVE_V1ALPHA1_VERSION,
            "clusterservingruntimes",
            KC_TEST_RUNTIME_NAME,
            timeout=30,
        )
    except ApiException as e:
        if e.status != 404:
            raise

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
                    # python:3.11-slim provides Unix tools (sh, dd, mkdir) for creating
                    # dummy cache files and python3 for running a simple HTTP server.
                    # The HTTP server satisfies the MCV webhook's requirement that pods
                    # have a readiness probe — without one, the webhook skips injection.
                    "image": "python:3.11-slim",
                    "command": ["/bin/sh", "-c"],
                    "args": [
                        # Ordering matters for MCV capture:
                        # 1. Sleep so MCV starts and takes an empty baseline snapshot
                        #    before we write the cache files. 30s is a conservative
                        #    buffer for MCV image pull + container start on cold clusters.
                        #    This could likely be reduced after testing, but erring on the
                        #    side of reliability for now.
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
    api.create_cluster_custom_object(
        KSERVE_GROUP,
        KSERVE_V1ALPHA1_VERSION,
        "clusterservingruntimes",
        body,
    )

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


@pytest.fixture(scope="session")
def kc_session_namespace(kc_node_group, kc_test_runtime):
    """Create a fixed namespace for the shared session KC; torn down at session end.

    Depends on kc_node_group and kc_test_runtime so the KCNG and
    ClusterServingRuntime are ready before any session-level capture begins.
    """
    _load_k8s_config()
    core = client.CoreV1Api()
    ns_name = "kc-e2e-session"

    # create_namespace copies Istio-injection and pod-security labels from the
    # seed namespace — without them the Istio proxy is not injected and MCV
    # cannot reach the KCC controller over mTLS, leaving the phase stuck empty.
    create_namespace(core, ns_name)
    # provision_secrets copies S3/seaweedfs credentials so the storage
    # initializer can pull the model from the in-cluster S3 store.
    provision_secrets(core, ns_name)

    yield ns_name
    if skip_resource_deletion():
        return
    delete_namespace(core, ns_name)


@pytest.fixture(scope="session")
def kc_session(kc_session_namespace):
    """Perform the full KC capture once for the entire test session.

    Creates a producer ISVC, waits for MCV sidecar injection, capture
    completion, and KC verification. All downstream tests that need an
    already-verified KernelCache (injection, deletion) depend on this
    fixture instead of repeating the expensive capture themselves.

    Yields a dict:
      namespace    – the session namespace (kc-e2e-session)
      isvc_name    – producer ISVC name
      kcc_name     – KernelCacheCapture name
      kc           – KernelCache CR dict (status.verification.verified == True)
      kc_name      – KernelCache CR name
      kc_namespace – KernelCache CR namespace
    """
    namespace = kc_session_namespace
    isvc_name = "kc-session-producer"

    kserve = KServeClient()
    isvc = make_producer_isvc(isvc_name, namespace, _STORAGE_URI)
    kserve.create(isvc)

    kcc = wait_for_kcc_created(namespace, isvc_name, timeout=120)
    kcc_name = kcc["metadata"]["name"]

    wait_for_mcv_sidecar(namespace, isvc_name, timeout=120)
    wait_for_capture_complete(namespace, kcc_name)  # uses 600s default
    kc = wait_for_kernelcache_verified(namespace, kcc_name, timeout=300)

    yield {
        "namespace": namespace,
        "isvc_name": isvc_name,
        "kcc_name": kcc_name,
        "kc": kc,
        "kc_name": kc["metadata"]["name"],
        "kc_namespace": kc["metadata"]["namespace"],
    }
    # kc_session_namespace teardown deletes the namespace, cleaning up all resources.
