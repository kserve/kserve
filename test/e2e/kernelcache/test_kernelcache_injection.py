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

import pytest
from kserve import KServeClient

from .conftest import _STORAGE_URI
from .utils import (
    _KERNEL_CACHE_LINKER_CONTAINER,
    _KERNEL_CACHE_SOURCE_VOLUME,
    _KERNEL_CACHE_USAGE_ANNOTATION,
    check_pod_logs_for_cache_usage,
    make_producer_isvc,
    pod_has_mcv_sidecar,
    verify_cache_files_accessible,
    wait_for_cache_injected,
    wait_for_isvc_pod_ready,
    wait_for_kernelcachenode_cache_ready,
)


@pytest.mark.kernelcache
def test_kernelcache_injection(kc_session):
    """Cache injection into a consumer ISVC pod.

    Uses the KernelCache captured by the session fixture (kc_session) so
    setup is not duplicated across tests. Both the producer and consumer
    ISVCs live in the same session namespace — this is required because the
    injection webhook selects candidates where kc.namespace == pod.namespace.

    Phase 2 — Wait for the KernelCacheNode agent to pull the captured OCI
    artifact and mark it Ready. This is the prerequisite for injection: the
    pod mutator webhook reads KernelCacheNode.status.cacheStatus to find
    Ready candidates. Without a Ready entry the webhook falls back to
    injecting the MCV sidecar instead of mounting the cache.

    Phase 3 — Create a consumer ISVC with the same runtime and storage URI.
    The pod mutator finds the Ready KC, mounts it as a kernel-cache-source
    OCI volume, prepends the kernel-cache-linker init container, and skips
    MCV injection. The test verifies the pod spec and waits for Running.

    Phase 4 — Verify the injected cache is accessible and used:
      a) Check cache files exist and are readable via exec into the pod
      b) Check pod logs for torch.compile cache hit indicators

    The producer runs torch.compile which writes FX graph cache files to
    $XDG_CACHE_HOME/torch/inductor-cache/. By setting XDG_CACHE_HOME to
    /root/.cache/vllm, these files are written where MCV can detect them.
    The consumer pod runs the same operation with cache already mounted,
    so logs should show cache hits rather than compilation.
    """
    kc = kc_session["kc"]
    kc_name = kc_session["kc_name"]
    kc_namespace = kc_session["kc_namespace"]
    namespace = kc_session["namespace"]

    # ── Phase 2: wait for KernelCacheNode to prepare the KC on a node ─────────
    # The injection webhook reads KernelCacheNode.status.cacheStatus; if the
    # entry is absent or not yet Ready, no cache is mounted and MCV is injected
    # instead — defeating the purpose of this test.
    wait_for_kernelcachenode_cache_ready(kc_namespace, kc_name, timeout=300)

    # ── Phase 3: consumer ISVC — cache injection ──────────────────────────────
    # The consumer must be in the same namespace as the KC: the webhook filters
    # cacheStatus candidates by kc.namespace == pod.namespace.
    consumer_name = "kc-inject-consumer"
    kserve = KServeClient()
    kserve.create(make_producer_isvc(consumer_name, namespace, _STORAGE_URI))

    # The pod mutator runs at admission time, so by the time the pod is
    # visible the injection decision is already made.
    pod = wait_for_cache_injected(namespace, consumer_name, timeout=120)

    # Verify the OCI volume is present and references the captured artifact.
    source_volumes = [
        v for v in (pod.spec.volumes or []) if v.name == _KERNEL_CACHE_SOURCE_VOLUME
    ]
    assert len(source_volumes) == 1, (
        f"Expected one '{_KERNEL_CACHE_SOURCE_VOLUME}' volume, "
        f"got {[v.name for v in (pod.spec.volumes or [])]}"
    )
    assert source_volumes[0].image is not None, (
        f"Volume '{_KERNEL_CACHE_SOURCE_VOLUME}' has no image source"
    )
    assert (
        source_volumes[0].image.reference == kc["spec"]["artifact"]["imageReference"]
    ), (
        f"Volume image reference {source_volumes[0].image.reference!r} does not match "
        f"KC artifact {kc['spec']['artifact']['imageReference']!r}"
    )

    # Verify the linker init container is present.
    linker_containers = [
        c
        for c in (pod.spec.init_containers or [])
        if c.name == _KERNEL_CACHE_LINKER_CONTAINER
    ]
    assert len(linker_containers) == 1, (
        f"Expected one '{_KERNEL_CACHE_LINKER_CONTAINER}' init container, "
        f"got {[c.name for c in (pod.spec.init_containers or [])]}"
    )

    # Verify the pod records which KC it is using.
    usage = (pod.metadata.annotations or {}).get(_KERNEL_CACHE_USAGE_ANNOTATION, "")
    assert usage == f"{kc_namespace}/{kc_name}", (
        f"Pod annotation {_KERNEL_CACHE_USAGE_ANNOTATION!r} is {usage!r}, "
        f"expected '{kc_namespace}/{kc_name}'"
    )

    # Verify MCV was NOT injected — the cache mount replaces capture.
    assert not pod_has_mcv_sidecar(namespace, consumer_name), (
        f"Consumer ISVC {namespace}/{consumer_name} pod has MCV sidecar — "
        "expected cache injection instead of capture"
    )

    # Wait for pod to be Ready (readiness probe passed).
    # This ensures torch.compile has completed and the HTTP server started,
    # so logs will contain compilation activity when we read them.
    wait_for_isvc_pod_ready(namespace, consumer_name, timeout=300)

    # ── Phase 4: verify cache is accessible and used ──────────────────────────
    # The kernel-cache-linker init container symlinks the OCI image contents.
    # PyTorch writes cache to TORCHINDUCTOR_CACHE_DIR:
    # /root/.cache/vllm/torch_compile_cache/inductor_cache
    # Verify these files are present and readable in the consumer pod.
    verify_cache_files_accessible(namespace, consumer_name, timeout=60)

    # Check pod logs for cache hit indicators. The consumer pod runs the same
    # torch.compile operation as the producer, but with cache files already
    # mounted. Logs should show "FX graph cache hit" messages proving cache reuse.
    log_check = check_pod_logs_for_cache_usage(
        namespace, consumer_name, since_seconds=300
    )
    assert log_check["logs_checked"], (
        f"Could not retrieve logs from consumer pod {namespace}/{consumer_name}"
    )
    assert log_check["cache_hit_found"], (
        f"No cache hit found in consumer pod {namespace}/{consumer_name} logs. "
        f"Cache was injected (symlinks verified) but torch.compile did not reuse it. "
        f"cache_miss_found={log_check['cache_miss_found']}"
    )
