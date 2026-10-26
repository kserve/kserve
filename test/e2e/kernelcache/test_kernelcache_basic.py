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

from .utils import (
    _load_k8s_config,
    wait_for_kernelcache_nodes,
)

_NODE_LABEL_KEY: str = os.environ.get(
    "KERNELCACHE_NODE_LABEL_KEY", "kernelcache.example.com/group"
)
_NODE_LABEL_VALUE: str = os.environ.get("KERNELCACHE_NODE_LABEL_VALUE", "workers")


@pytest.mark.kernelcache
def test_kernelcache_basic(
    kc_node_group,
    kc_session,
):
    """Core KernelCache lifecycle: node creation → capture → verification.

    Phase 1 — KernelCacheNode creation:
      [TEST VERIFIES] Controller creates one KernelCacheNode CR per worker node.
      [TEST VERIFIES] Agent reconciles each KCN (status.counts field populated).

    Phase 2 — Cache capture (driven by kc_session fixture):
      [FIXTURE CREATES] ISVC using kernelcache-test-runtime (producer pod).
      [FIXTURE VERIFIES] Controller auto-creates KernelCacheCapture for the ISVC.
      [FIXTURE VERIFIES] Webhook injects MCV sidecar into ISVC pod.
      [FIXTURE VERIFIES] KernelCacheCapture.status.phase reaches 'Complete' or 'Unchanged'.
      [FIXTURE VERIFIES] KernelCache CR exists with verification.state=Succeeded.
      [TEST VERIFIES] KernelCache CR has a non-empty artifact imageReference.
      [TEST VERIFIES] KernelCache CR verification.verified == True.
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
    # This test verifies that each worker node has a corresponding KCN (by name)
    # and that the agent has reconciled it (status.counts populated).
    worker_node_names = [node.metadata.name for node in worker_nodes]
    wait_for_kernelcache_nodes(worker_node_names, timeout=120)

    # ── Phase 2: assert on the session-level capture ──────────────────────────
    # kc_session has already performed the full capture and verified the KC.
    # Assert on the resulting KC state to make the test outcome explicit.
    kc = kc_session["kc"]
    verification = kc.get("status", {}).get("verification", {})
    assert verification.get("verified") is True, (
        f"KernelCache {kc_session['kc_namespace']}/{kc_session['kc_name']} "
        f"verification.verified is not True: {verification}"
    )
    assert verification.get("state") == "Succeeded", (
        f"KernelCache verification.state={verification.get('state')!r}, expected 'Succeeded'"
    )
    image_ref = kc.get("spec", {}).get("artifact", {}).get("imageReference", "")
    assert image_ref, (
        f"KernelCache {kc_session['kc_name']} has empty spec.artifact.imageReference"
    )
