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

"""
Spec-level smoke test for LoRA adapter support in LLMInferenceService.

This test deploys an LLMInferenceService with 2 Preload LoRA adapters and
verifies that the controller emits the correct Deployment init containers and
InferenceObjective objects. It does NOT wait for pods to be Ready or send
inference requests, because:

  - The adapter URIs are intentionally fake (s3://fake-*), so storage-initializer
    init containers will fail; that is acceptable for spec-level assertions.
  - Request-level routing (model=<adapter-name> → correct replica) requires a
    real GPU + downloaded adapter weights and is tracked as a follow-up in
    kserve#3750.

The test uses the llm-d inference simulator as the workload runtime so it can
run on CPU-only clusters without GPU resources.
"""

from __future__ import annotations

import logging
import os
import time

import pytest
from kubernetes import client

from .fixtures import (
    KSERVE_TEST_NAMESPACE,
    inject_k8s_proxy,
)

logger = logging.getLogger(__name__)

KSERVE_GROUP = "serving.kserve.io"
GIE_GROUP = "inference.networking.x-k8s.io"
GIE_VERSION = "v1alpha2"
LLMISVC_PLURAL = "llminferenceservices"
IO_PLURAL = "inferenceobjectives"

_LORA_SIMULATOR_SERVICE_BODY = {
    "apiVersion": f"{KSERVE_GROUP}/v1alpha2",
    "kind": "LLMInferenceService",
    "metadata": {
        "name": "lora-spec-smoke",
        "namespace": KSERVE_TEST_NAMESPACE,
    },
    "spec": {
        # Base model — publicly accessible, downloaded by the storage initializer.
        # Pods will not reach Ready because the adapter init containers below
        # use fake URIs, but the Deployment spec is what this test verifies.
        "model": {
            "uri": "hf://facebook/opt-125m",
            "name": "facebook/opt-125m",
            "lora": {
                "adapters": [
                    {
                        "name": "alpha",
                        # Intentionally fake — init container will fail, which is
                        # expected for this spec-level smoke test.
                        "uri": "s3://fake-lora-bucket/alpha",
                    },
                    {
                        "name": "beta",
                        "uri": "s3://fake-lora-bucket/beta",
                    },
                ]
            },
        },
        "replicas": 1,
        # Use the llm-d inference simulator: CPU-only, no model weights needed.
        "template": {
            "containers": [
                {
                    "name": "main",
                    "image": "ghcr.io/llm-d/llm-d-inference-sim:v0.8.2",
                    "command": ["/app/llm-d-inference-sim"],
                    "args": [
                        "--port",
                        "8000",
                        "--model",
                        "facebook/opt-125m",
                        "--mode",
                        "random",
                    ],
                    "resources": {
                        "limits": {"cpu": "1", "memory": "2Gi"},
                        "requests": {"cpu": "200m", "memory": "2Gi"},
                    },
                    "securityContext": {
                        "runAsNonRoot": True,
                        "runAsUser": 65532,
                        "runAsGroup": 65532,
                    },
                }
            ]
        },
        "router": {"scheduler": {}, "route": {}, "gateway": {}},
    },
}


def _wait_for(fn, timeout: float = 120.0, interval: float = 2.0):
    """Poll fn() until it returns a truthy value or timeout elapses."""
    deadline = time.monotonic() + timeout
    last_exc = None
    while time.monotonic() < deadline:
        try:
            result = fn()
            if result:
                return result
        except AssertionError as exc:
            last_exc = exc
            logger.debug("Waiting: %s", exc)
        time.sleep(interval)
    raise AssertionError(
        f"Condition not met within {timeout}s"
        + (f": {last_exc}" if last_exc else "")
    )


@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
def test_lora_spec_smoke():
    """
    Spec-level smoke: controller must emit 2 adapter-fetch init containers
    and 2 InferenceObjective objects for a service with 2 LoRA adapters.
    """
    inject_k8s_proxy()

    custom_api = client.CustomObjectsApi()
    apps_v1 = client.AppsV1Api()

    svc_name = _LORA_SIMULATOR_SERVICE_BODY["metadata"]["name"]
    namespace = _LORA_SIMULATOR_SERVICE_BODY["metadata"]["namespace"]

    try:
        custom_api.create_namespaced_custom_object(
            KSERVE_GROUP,
            "v1alpha2",
            namespace,
            LLMISVC_PLURAL,
            _LORA_SIMULATOR_SERVICE_BODY,
        )
        logger.info(f"Created LLMInferenceService {svc_name!r}")

        # --- Wait for Deployment to be created (spec check only, not readiness) ---
        def _deployment_exists():
            deps = apps_v1.list_namespaced_deployment(
                namespace,
                label_selector=(
                    f"app.kubernetes.io/name={svc_name},"
                    "app.kubernetes.io/part-of=llminferenceservice"
                ),
            )
            assert deps.items, f"No Deployment found for {svc_name!r} yet"
            return deps.items[0]

        dep = _wait_for(_deployment_exists, timeout=120)
        logger.info(f"Deployment {dep.metadata.name!r} found")

        # --- Assert adapter-fetch init containers ---
        init_names = [
            c.name for c in (dep.spec.template.spec.init_containers or [])
        ]
        assert "adapter-fetch-alpha" in init_names, (
            f"Missing adapter-fetch-alpha in init containers: {init_names}"
        )
        assert "adapter-fetch-beta" in init_names, (
            f"Missing adapter-fetch-beta in init containers: {init_names}"
        )
        logger.info(f"Init containers verified: {init_names}")

        # --- Assert lora-adapters emptyDir volume on the pod ---
        vol_names = [v.name for v in (dep.spec.template.spec.volumes or [])]
        assert "lora-adapters" in vol_names, (
            f"Missing lora-adapters volume: {vol_names}"
        )

        # --- Assert init-container args (uri → output-dir) ---
        init_by_name = {
            c.name: c for c in (dep.spec.template.spec.init_containers or [])
        }
        alpha_args = init_by_name["adapter-fetch-alpha"].args or []
        assert alpha_args[0] == "s3://fake-lora-bucket/alpha", (
            f"Unexpected init container source URI: {alpha_args}"
        )
        assert alpha_args[1] == "/mnt/loras/alpha", (
            f"Unexpected init container dest dir: {alpha_args}"
        )

        # --- Assert InferenceObjective objects (conditional on GIE CRD availability) ---
        try:
            svc_obj = custom_api.get_namespaced_custom_object(
                KSERVE_GROUP, "v1alpha2", namespace, LLMISVC_PLURAL, svc_name
            )
            svc_uid = svc_obj["metadata"]["uid"]

            def _ios_exist():
                ios = custom_api.list_namespaced_custom_object(
                    GIE_GROUP, GIE_VERSION, namespace, IO_PLURAL
                )
                owned = [
                    io
                    for io in ios.get("items", [])
                    if any(
                        ref.get("uid") == svc_uid
                        for ref in io.get("metadata", {}).get("ownerReferences", [])
                    )
                ]
                assert len(owned) == 2, (
                    f"Expected 2 InferenceObjectives owned by {svc_name!r}, "
                    f"got {len(owned)}"
                )
                return owned

            io_items = _wait_for(_ios_exist, timeout=60)
            io_names = {io["metadata"]["name"] for io in io_items}
            logger.info(f"InferenceObjective names: {io_names}")

            assert any("adapter-alpha" in n for n in io_names), (
                f"No IO with 'adapter-alpha' in names: {io_names}"
            )
            assert any("adapter-beta" in n for n in io_names), (
                f"No IO with 'adapter-beta' in names: {io_names}"
            )

            # Verify PoolRef points at the expected inference pool
            for io in io_items:
                pool_ref_name = io["spec"].get("poolRef", {}).get("name", "")
                assert pool_ref_name.endswith("-inference-pool"), (
                    f"Unexpected poolRef name {pool_ref_name!r} in IO {io['metadata']['name']!r}"
                )

        except client.rest.ApiException as exc:
            if exc.status == 404:
                logger.warning(
                    "GIE InferenceObjective CRD not available in this cluster; "
                    "skipping InferenceObjective assertions. "
                    "Install gateway-api-inference-extension to enable this check."
                )
            else:
                raise

    finally:
        try:
            custom_api.delete_namespaced_custom_object(
                KSERVE_GROUP,
                "v1alpha2",
                namespace,
                LLMISVC_PLURAL,
                svc_name,
            )
            logger.info(f"Deleted LLMInferenceService {svc_name!r}")
        except client.rest.ApiException:
            pass
