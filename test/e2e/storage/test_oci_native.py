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
Spec-level smoke test for oci+native:// storageUri support (KEP-4639 ImageVolume).

This test verifies that the KServe admission webhook correctly materializes an
oci+native:// storageUri as a Kubernetes ImageVolume (spec.volumes[*].image) with
a matching read-only VolumeMount on the kserve-container.

Assertion level: pod *spec* only — the test does not wait for the pod to become
Running or send any inference requests.  The placeholder test image
(ghcr.io/kserve/oci-native-test-fixture:v1) does not exist and will never
successfully pull; the test is expected to pass even while the pod is in
ImagePullBackOff.

Skip conditions (evaluated once per session):
  - Cluster Kubernetes minor version < 31: ImageVolume not supported at all.
  - Cluster Kubernetes minor version in [31, 32]: ImageVolume is alpha (feature-
    gated). The test cannot reliably detect whether the gate is enabled from
    outside the cluster, so it skips with a guidance message rather than failing.
"""

import os
import time

import pytest
from kubernetes import client
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    V1beta1ModelFormat,
    constants,
)
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE

# Placeholder OCI image reference.  The webhook asserts on the *reference string*,
# not on whether the image can be pulled.
# TODO: replace with a real tiny model image once one is published to ghcr.io/kserve.
OCI_NATIVE_TEST_IMAGE = "ghcr.io/kserve/oci-native-test-fixture:v1"

# Label selector used by KServe to tag pods belonging to an InferenceService.
ISVC_LABEL_KEY = "serving.kserve.io/inferenceservice"

# Seconds to wait for a pod to appear after ISVC creation.
POD_WAIT_TIMEOUT = 90


def _get_k8s_minor() -> int:
    """Return the cluster's Kubernetes minor version as an integer.

    Strips the trailing "+" that some distributions append (e.g. "31+").
    Returns -1 on any error so callers can decide whether to skip or proceed.
    """
    try:
        version_info = client.VersionApi().get_code()
        return int(version_info.minor.rstrip("+"))
    except Exception:  # noqa: BLE001
        return -1


def _wait_for_pod(
    core_api,
    namespace: str,
    label_selector: str,
    timeout: float = POD_WAIT_TIMEOUT,
) -> client.V1Pod | None:
    """Poll until at least one matching pod appears or timeout is reached."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        pods = core_api.list_namespaced_pod(namespace, label_selector=label_selector)
        if pods.items:
            return pods.items[0]
        time.sleep(3)
    return None


# ---------------------------------------------------------------------------
# Skip guards — evaluated once when the module is collected
# ---------------------------------------------------------------------------


def _skip_if_unsupported() -> None:
    minor = _get_k8s_minor()
    if minor < 0:
        pytest.skip(
            "Could not determine cluster Kubernetes version; skipping oci+native:// test"
        )
    if minor < 31:
        pytest.skip(
            f"Cluster Kubernetes minor={minor} < 31: ImageVolume not supported "
            "(introduced in K8s 1.31, KEP-4639). Upgrade to K8s >= 1.33 (beta) "
            "or K8s 1.31/1.32 with --feature-gates=ImageVolume=true."
        )
    if minor < 33:
        pytest.skip(
            f"Cluster Kubernetes minor={minor} is in alpha range [31, 32]: "
            "ImageVolume feature gate status cannot be detected externally. "
            "Re-run on K8s >= 1.33 (beta, gate enabled by default) or confirm "
            "--feature-gates=ImageVolume=true is set and remove this skip."
        )


# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------


@pytest.mark.storage
def test_oci_native_image_volume_spec(tmp_path):
    """An ISVC with oci+native:// storageUri produces an ImageVolume pod spec.

    The test:
    1. Checks K8s version and skips on unsupported clusters.
    2. Creates an InferenceService with a placeholder oci+native:// storageUri.
    3. Waits for the admission webhook to materialise the pod.
    4. Asserts the pod spec contains a Volume with ImageVolumeSource whose
       reference matches the expected image string.
    5. Asserts the kserve-container has a read-only VolumeMount pointing at
       the same volume.
    6. Deletes the InferenceService in teardown.
    """
    _skip_if_unsupported()

    service_name = "isvc-oci-native-smoke"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=f"oci+native://{OCI_NATIVE_TEST_IMAGE}",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name, namespace=KSERVE_TEST_NAMESPACE
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)

        pod = _wait_for_pod(
            kserve_client.core_api,
            KSERVE_TEST_NAMESPACE,
            label_selector=f"{ISVC_LABEL_KEY}={service_name}",
        )
        assert pod is not None, (
            f"No pod appeared for InferenceService '{service_name}' "
            f"within {POD_WAIT_TIMEOUT}s"
        )

        # Inspect pod spec as a dict so the assertion is robust against
        # Python kubernetes-client schema differences across versions.
        pod_dict = pod.to_dict()
        volumes = pod_dict.get("spec", {}).get("volumes", []) or []

        image_volumes = [v for v in volumes if v.get("image") is not None]
        assert image_volumes, (
            "Expected at least one ImageVolume in pod spec, but found none. "
            f"Volumes present: {[v.get('name') for v in volumes]}"
        )

        # Verify the reference string matches our expected image.
        expected_ref = OCI_NATIVE_TEST_IMAGE
        matched = [
            v for v in image_volumes if v["image"].get("reference") == expected_ref
        ]
        assert matched, (
            f"No ImageVolume with reference={expected_ref!r}. "
            f"Found image volumes: {[v['image'] for v in image_volumes]}"
        )

        image_vol_name = matched[0]["name"]

        # Verify kserve-container has a read-only VolumeMount for this volume.
        containers = pod_dict.get("spec", {}).get("containers", []) or []
        kserve_container = next(
            (c for c in containers if c.get("name") == "kserve-container"), None
        )
        assert kserve_container is not None, (
            "kserve-container not found in pod spec. "
            f"Container names: {[c.get('name') for c in containers]}"
        )

        mounts = kserve_container.get("volume_mounts", []) or []
        image_mount = next((m for m in mounts if m.get("name") == image_vol_name), None)
        assert image_mount is not None, (
            f"No VolumeMount for ImageVolume '{image_vol_name}' on kserve-container. "
            f"Mounts present: {[m.get('name') for m in mounts]}"
        )
        assert image_mount.get("read_only") is True, (
            f"Expected VolumeMount '{image_vol_name}' to be read-only"
        )

    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
