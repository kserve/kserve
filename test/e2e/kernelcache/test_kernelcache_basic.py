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
from kubernetes import client as k8s_client
from kserve import KServeClient
from kserve.constants.constants import (
    KSERVE_KIND_INFERENCESERVICE,
    KSERVE_V1BETA1,
)
from kserve.models.v1beta1_inference_service import V1beta1InferenceService
from kserve.models.v1beta1_inference_service_spec import V1beta1InferenceServiceSpec
from kserve.models.v1beta1_model_format import V1beta1ModelFormat
from kserve.models.v1beta1_model_spec import V1beta1ModelSpec
from kserve.models.v1beta1_predictor_spec import V1beta1PredictorSpec

from .utils import (
    _load_k8s_config,
    wait_for_capture_complete,
    wait_for_kcc_created,
    wait_for_kernelcache_nodes,
    wait_for_kernelcache_verified,
    wait_for_mcv_sidecar,
)

# storageUri for the test ISVC. The storage initializer init container must
# succeed before MCV (injected as a regular sidecar) and kserve-container start.
# Defaults to s3://example-models/facebook/opt-125m which setup-kserve.sh pre-seeds
# into the in-cluster SeaweedFS, avoiding slow HuggingFace downloads in CI. For
# local runs override with KERNELCACHE_MODEL_URI if SeaweedFS is not available.
_STORAGE_URI: str = os.environ.get(
    "KERNELCACHE_MODEL_URI",
    "s3://example-models/facebook/opt-125m",
)

_NODE_LABEL_KEY: str = os.environ.get(
    "KERNELCACHE_NODE_LABEL_KEY", "kernelcache.example.com/group"
)
_NODE_LABEL_VALUE: str = os.environ.get("KERNELCACHE_NODE_LABEL_VALUE", "workers")


@pytest.mark.kernelcache
def test_kernelcache_basic(
    kc_node_group,
    kc_test_runtime,
    kserve_client: KServeClient,
    test_namespace: str,
):
    """Core KernelCache lifecycle: node creation → capture → verification.

    Phase 1 — KernelCacheNode creation:
      [TEST VERIFIES] Controller creates one KernelCacheNode CR per worker node.

    Phase 2 — Cache capture:
      [TEST CREATES] ISVC using kernelcache-test-runtime (producer pod).
      [TEST VERIFIES] Controller auto-creates KernelCacheCapture for the ISVC.
      [TEST VERIFIES] Webhook injects MCV sidecar into ISVC pod.
      [TEST VERIFIES] KernelCacheCapture.status.phase reaches 'Complete'.
      [TEST VERIFIES] KernelCache CR exists with verification.state=Succeeded.
    """
    # ── Phase 1: KernelCacheNode creation ─────────────────────────────────────
    _load_k8s_config()
    core = k8s_client.CoreV1Api()
    label_selector = f"{_NODE_LABEL_KEY}={_NODE_LABEL_VALUE}"
    worker_nodes = core.list_node(label_selector=label_selector).items
    assert len(worker_nodes) > 0, (
        f"No worker nodes labelled {label_selector}. "
        "Ensure worker nodes are labelled (e.g. via setup-kernelcache.sh in CI)."
    )

    # Controller creates one KernelCacheNode CR per matching node after the
    # KernelCacheNodeGroup (created by the kc_node_group fixture) is reconciled.
    # This test verifies that each worker node has a corresponding KCN (by name).
    worker_node_names = [node.metadata.name for node in worker_nodes]
    wait_for_kernelcache_nodes(worker_node_names, timeout=120)

    # ── Phase 2: cache capture ────────────────────────────────────────────────
    isvc_name = "kc-basic-test"
    isvc = V1beta1InferenceService(
        api_version=KSERVE_V1BETA1,
        kind=KSERVE_KIND_INFERENCESERVICE,
        metadata=k8s_client.V1ObjectMeta(
            name=isvc_name,
            namespace=test_namespace,
            # Standard mode bypasses Knative (which strips the HTTP readinessProbe
            # from the pod spec when injecting queue-proxy, breaking the MCV webhook).
            annotations={"serving.kserve.io/deploymentMode": "Standard"},
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                model=V1beta1ModelSpec(
                    model_format=V1beta1ModelFormat(name="test-cache"),
                    # storageUri causes the storage annotation to be set on the pod,
                    # which is required by the MCV sidecar injection webhook.
                    storage_uri=_STORAGE_URI,
                    resources=k8s_client.V1ResourceRequirements(
                        requests={"cpu": "100m", "memory": "256Mi"},
                        limits={"cpu": "500m", "memory": "512Mi"},
                    ),
                ),
            )
        ),
    )
    kserve_client.create(isvc)

    # Controller auto-creates KernelCacheCapture once the pod is admitted.
    kcc = wait_for_kcc_created(test_namespace, isvc_name, timeout=120)
    kcc_name = kcc["metadata"]["name"]

    # Verify the webhook injected the MCV sidecar container into the pod.
    wait_for_mcv_sidecar(test_namespace, isvc_name, timeout=120)

    # MCV captures the delta (dummy.cubin — 10 KB) and pushes to the in-cluster
    # registry. With a tiny artifact and local registry this should finish fast.
    wait_for_capture_complete(test_namespace, kcc_name, timeout=300)

    # Controller creates a KernelCache CR and runs cert-mode signing + verification.
    wait_for_kernelcache_verified(test_namespace, kcc_name, timeout=300)
