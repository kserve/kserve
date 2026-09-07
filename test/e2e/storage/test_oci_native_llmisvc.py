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
Spec-level smoke test for oci+native:// storageUri on LLMInferenceService (KEP-4639).

Mirrors test_oci_native.py but targets the LLMInferenceService CRD instead of
InferenceService.  The LLMInferenceService controller applies the oci+native://
storage configuration in workload_storage.go; the test asserts that the resulting
pod spec contains the expected Kubernetes ImageVolume and a matching read-only
VolumeMount on kserve-container.

Assertion level: pod *spec* only — the test does not wait for the pod to become
Running or send any inference requests.  The placeholder test image
(ghcr.io/kserve/oci-native-test-fixture:v1) does not exist and will never
successfully pull; the test is expected to pass even while the pod is in
ImagePullBackOff.

# TODO: publish ghcr.io/kserve/oci-native-test-fixture:v1 before running this
#       test in CI to allow the end-to-end inference path to be exercised.

Skip conditions (evaluated at test start):
  - Cluster Kubernetes minor version < 31: ImageVolume not supported at all.
  - Cluster Kubernetes minor version in [31, 32]: ImageVolume is alpha (feature-
    gated). The test cannot reliably detect whether the gate is enabled from
    outside the cluster, so it skips with a guidance message rather than failing.
"""

import os
import time

import pytest
from kubernetes import client, config as k8s_config
from kserve import KServeClient, constants

from ..common.utils import KSERVE_TEST_NAMESPACE

# Placeholder OCI image reference — same fixture used by the InferenceService variant.
# TODO: replace with a real tiny model image once one is published to ghcr.io/kserve.
OCI_NATIVE_TEST_IMAGE = "ghcr.io/kserve/oci-native-test-fixture:v1"

# Custom-objects API coordinates for LLMInferenceService.
KSERVE_GROUP = constants.KSERVE_GROUP  # "serving.kserve.io"
LLMISVC_VERSION = constants.KSERVE_V1ALPHA1_VERSION  # "v1alpha1"
LLMISVC_PLURAL = "llminferenceservices"

# Pod label keys set by the LLMInferenceService controller on every managed pod.
LLMISVC_LABEL_NAME = "app.kubernetes.io/name"
LLMISVC_LABEL_PART_OF = "app.kubernetes.io/part-of"
LLMISVC_LABEL_PART_OF_VALUE = "llminferenceservice"

# Container name used in the inline pod template we submit. The LLMISVC
# controller merges this template with its base config templates
# (config/llmisvcconfig/config-llm-*.yaml), whose model-serving container is
# named "main" — that is the container the controller attaches the model
# volume mount to, not the template-provided container below.
KSERVE_CONTAINER_NAME = "kserve-container"

# The LLMISVC controller's canonical model-serving container. The oci+native://
# ImageVolume mount lands here (the base templates all define `name: main`), so
# assertions about the model VolumeMount must target this container, not the
# template's KSERVE_CONTAINER_NAME.
LLMISVC_MODEL_CONTAINER_NAME = "main"

# Seconds to wait for a pod to appear after LLMInferenceService creation.
POD_WAIT_TIMEOUT = 120


def _get_k8s_minor() -> int:
    """Return the cluster's Kubernetes minor version as an integer.

    Strips the trailing "+" that some distributions append (e.g. "31+").
    Returns -1 on any error so callers can decide whether to skip or proceed.
    """
    try:
        try:
            k8s_config.load_kube_config(
                config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
            )
        except Exception:  # noqa: BLE001
            k8s_config.load_incluster_config()
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
# Skip guards
# ---------------------------------------------------------------------------


def _skip_if_unsupported() -> None:
    minor = _get_k8s_minor()
    if minor < 0:
        pytest.skip(
            "Could not determine cluster Kubernetes version; skipping oci+native:// "
            "LLMInferenceService test"
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
def test_oci_native_image_volume_spec_llmisvc(tmp_path):
    """An LLMInferenceService with oci+native:// model URI produces an ImageVolume pod spec.

    The test:
    1. Checks K8s version and skips on unsupported clusters.
    2. Creates a minimal LLMInferenceService with oci+native:// model URI and an
       inline pod template (no baseRefs required).
    3. Waits for the controller to materialise a pod.
    4. Asserts the pod spec contains a Volume with ImageVolumeSource whose
       reference matches the expected image string.
    5. Asserts kserve-container has a read-only VolumeMount pointing at that volume.
    6. Deletes the LLMInferenceService in teardown.
    """
    _skip_if_unsupported()

    service_name = "llmisvc-oci-native-smoke"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    # Minimal LLMInferenceService: oci+native:// model URI + inline pod template.
    # The controller calls workload_storage.go which applies ConfigureOciNativeToContainer
    # on the merged pod spec, adding the ImageVolume and read-only VolumeMount.
    llmisvc_body = {
        "apiVersion": f"{KSERVE_GROUP}/{LLMISVC_VERSION}",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": service_name,
            "namespace": KSERVE_TEST_NAMESPACE,
        },
        "spec": {
            "model": {
                "uri": f"oci+native://{OCI_NATIVE_TEST_IMAGE}",
            },
            "template": {
                "containers": [
                    {
                        "name": KSERVE_CONTAINER_NAME,
                        # Placeholder image — will not pull; we only assert pod spec.
                        "image": OCI_NATIVE_TEST_IMAGE,
                        "resources": {
                            "requests": {"cpu": "50m", "memory": "128Mi"},
                            "limits": {"cpu": "100m", "memory": "256Mi"},
                        },
                    }
                ]
            },
        },
    }

    try:
        kserve_client.api_instance.create_namespaced_custom_object(
            KSERVE_GROUP,
            LLMISVC_VERSION,
            KSERVE_TEST_NAMESPACE,
            LLMISVC_PLURAL,
            llmisvc_body,
        )

        label_selector = (
            f"{LLMISVC_LABEL_NAME}={service_name},"
            f"{LLMISVC_LABEL_PART_OF}={LLMISVC_LABEL_PART_OF_VALUE}"
        )
        pod = _wait_for_pod(
            kserve_client.core_api,
            KSERVE_TEST_NAMESPACE,
            label_selector=label_selector,
        )
        assert pod is not None, (
            f"No pod appeared for LLMInferenceService '{service_name}' "
            f"within {POD_WAIT_TIMEOUT}s"
        )

        pod_dict = pod.to_dict()
        volumes = pod_dict.get("spec", {}).get("volumes", []) or []

        image_volumes = [v for v in volumes if v.get("image") is not None]
        assert image_volumes, (
            "Expected at least one ImageVolume in pod spec, but found none. "
            f"Volumes present: {[v.get('name') for v in volumes]}"
        )

        expected_ref = OCI_NATIVE_TEST_IMAGE
        matched = [
            v for v in image_volumes if v["image"].get("reference") == expected_ref
        ]
        assert matched, (
            f"No ImageVolume with reference={expected_ref!r}. "
            f"Found image volumes: {[v['image'] for v in image_volumes]}"
        )

        image_vol_name = matched[0]["name"]

        containers = pod_dict.get("spec", {}).get("containers", []) or []
        # The LLMISVC controller attaches the model volume mount to its canonical
        # "main" container (from the merged base template), not to the container
        # name we supplied in the inline template.
        model_container = next(
            (c for c in containers if c.get("name") == LLMISVC_MODEL_CONTAINER_NAME),
            None,
        )
        assert model_container is not None, (
            f"{LLMISVC_MODEL_CONTAINER_NAME} container not found in pod spec. "
            f"Container names: {[c.get('name') for c in containers]}"
        )

        mounts = model_container.get("volume_mounts", []) or []
        image_mount = next((m for m in mounts if m.get("name") == image_vol_name), None)
        assert image_mount is not None, (
            f"No VolumeMount for ImageVolume '{image_vol_name}' on "
            f"{LLMISVC_MODEL_CONTAINER_NAME}. Mounts present: "
            f"{[m.get('name') for m in mounts]}"
        )
        assert image_mount.get("read_only") is True, (
            f"Expected VolumeMount '{image_vol_name}' to be read-only"
        )

    finally:
        try:
            kserve_client.api_instance.delete_namespaced_custom_object(
                KSERVE_GROUP,
                LLMISVC_VERSION,
                KSERVE_TEST_NAMESPACE,
                LLMISVC_PLURAL,
                service_name,
            )
        except Exception:  # noqa: BLE001
            pass
