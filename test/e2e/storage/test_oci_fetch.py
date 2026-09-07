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
E2E tests for the oci+fetch:// storageUri scheme (KServe issue #4083, Step 3).

Unlike oci+native:// (which mounts a Kubernetes ImageVolume and is asserted at
the pod-spec level only), oci+fetch:// reuses the regular storage-initializer
init container: the Python handler (Storage._download_oci) pulls the OCI image's
model layers with oras-py and extracts the /models/ subtree into a shared
emptyDir at /mnt/models.

Two tests:

  test_oci_fetch_inference_service_pulls_and_extracts_model
      Real, end-to-end pull against a public multi-arch fixture image. Asserts
      the storage-initializer init container terminates with exit code 0 (which
      proves the pull + extract succeeded — the handler raises if the image has
      no /models/ directory) and then execs into the kserve-container to confirm
      model.joblib is present at /mnt/models.

  test_oci_fetch_with_image_pull_secret_spec_only
      Spec-only assertion of the registry-credential wiring: when the predictor
      declares an imagePullSecret, the webhook projects it as a docker
      config.json volume (kserve-oci-fetch-docker-config) on the init container
      and signals its path via the KSERVE_OCI_DOCKER_CONFIG env var. This case
      does NOT wait for the pod to run — the referenced secret need not exist.
"""

import os
import time

import pytest
from kubernetes import client
from kubernetes.stream import stream
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

# Public, multi-arch (linux/amd64 + linux/arm64) modelcar-layout fixture image.
# Contains a single model file at /models/model.joblib and is anonymously
# pullable from ghcr.io. Published out-of-band for KServe oci+fetch:// E2E.
OCI_FETCH_TEST_IMAGE = "ghcr.io/kliukovkin/kserve-oci-test-fixture:v1"

# Model file the fixture is known to contain under /models/.
EXPECTED_MODEL_FILE = "model.joblib"

# Name of the storage-initializer init container injected by the webhook.
STORAGE_INITIALIZER_CONTAINER = "storage-initializer"

# Projected docker-config volume + env var the webhook adds when an
# imagePullSecret is present (see ConfigureOciFetchToContainer).
OCI_FETCH_DOCKER_CONFIG_VOLUME = "kserve-oci-fetch-docker-config"
OCI_FETCH_DOCKER_CONFIG_ENV = "KSERVE_OCI_DOCKER_CONFIG"

# Label selector KServe applies to InferenceService pods.
ISVC_LABEL_KEY = "serving.kserve.io/inferenceservice"

# Shared model mount path inside the pod.
MODEL_MOUNT_PATH = "/mnt/models"

POD_WAIT_TIMEOUT = 90
INIT_CONTAINER_TIMEOUT = 300
MAIN_CONTAINER_RUNNING_TIMEOUT = 180


def _wait_for_pod(core_api, namespace, label_selector, timeout=POD_WAIT_TIMEOUT):
    """Poll until at least one matching pod appears or timeout is reached."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        pods = core_api.list_namespaced_pod(namespace, label_selector=label_selector)
        if pods.items:
            return pods.items[0]
        time.sleep(3)
    return None


def _init_container_status(pod, name):
    """Return the V1ContainerStatus for init container `name`, or None."""
    for status in pod.status.init_container_statuses or []:
        if status.name == name:
            return status
    return None


def _wait_for_init_container_terminated(
    core_api, namespace, label_selector, name, timeout=INIT_CONTAINER_TIMEOUT
):
    """Poll until the named init container reaches a terminated state.

    Returns the V1ContainerStateTerminated (with exit_code), or None on timeout.
    Fails fast surfacing the waiting reason so an ImagePullBackOff / Error on the
    init container itself is reported rather than silently timing out.
    """
    deadline = time.monotonic() + timeout
    last_reason = None
    while time.monotonic() < deadline:
        pods = core_api.list_namespaced_pod(namespace, label_selector=label_selector)
        if pods.items:
            status = _init_container_status(pods.items[0], name)
            if status is not None and status.state is not None:
                if status.state.terminated is not None:
                    return status.state.terminated
                if status.state.waiting is not None:
                    last_reason = status.state.waiting.reason
        time.sleep(5)
    if last_reason:
        pytest.fail(
            f"Init container '{name}' did not terminate within {timeout}s; "
            f"last waiting reason: {last_reason}"
        )
    return None


def _wait_for_container_running(
    core_api, namespace, label_selector, name, timeout=MAIN_CONTAINER_RUNNING_TIMEOUT
):
    """Poll until container `name` is in the running state. Returns bool."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        pods = core_api.list_namespaced_pod(namespace, label_selector=label_selector)
        if pods.items:
            for status in pods.items[0].status.container_statuses or []:
                if status.name == name and status.state and status.state.running:
                    return True
        time.sleep(5)
    return False


@pytest.mark.storage
def test_oci_fetch_inference_service_pulls_and_extracts_model():
    """An oci+fetch:// ISVC pulls the image and extracts /models/ to /mnt/models.

    The test:
    1. Creates an InferenceService with an oci+fetch:// storageUri pointing at a
       public multi-arch fixture image.
    2. Waits for the pod to be materialised.
    3. Waits for the storage-initializer init container to terminate and asserts
       exit code 0 — proof the oras-py pull and /models/ extraction succeeded.
    4. Execs into the kserve-container and asserts model.joblib is present at
       /mnt/models (the shared emptyDir written by the init container).
    5. Deletes the InferenceService in teardown.
    """
    service_name = "isvc-oci-fetch-pull"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    label_selector = f"{ISVC_LABEL_KEY}={service_name}"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=f"oci+fetch://{OCI_FETCH_TEST_IMAGE}",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "1", "memory": "1Gi"},
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
            kserve_client.core_api, KSERVE_TEST_NAMESPACE, label_selector
        )
        assert pod is not None, (
            f"No pod appeared for InferenceService '{service_name}' "
            f"within {POD_WAIT_TIMEOUT}s"
        )

        terminated = _wait_for_init_container_terminated(
            kserve_client.core_api,
            KSERVE_TEST_NAMESPACE,
            label_selector,
            STORAGE_INITIALIZER_CONTAINER,
        )
        assert terminated is not None, (
            f"Init container '{STORAGE_INITIALIZER_CONTAINER}' did not terminate "
            f"within {INIT_CONTAINER_TIMEOUT}s"
        )
        assert terminated.exit_code == 0, (
            f"oci+fetch:// pull failed: init container "
            f"'{STORAGE_INITIALIZER_CONTAINER}' exited {terminated.exit_code} "
            f"(reason={terminated.reason})"
        )

        # Init container exit 0 already proves the pull + /models/ extraction
        # (the handler raises if the image has no /models/). Confirm the file is
        # visible in the shared mount from the serving container's side.
        running = _wait_for_container_running(
            kserve_client.core_api,
            KSERVE_TEST_NAMESPACE,
            label_selector,
            "kserve-container",
        )
        assert running, (
            "kserve-container did not reach running state within "
            f"{MAIN_CONTAINER_RUNNING_TIMEOUT}s; cannot verify model files"
        )

        pod = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE, label_selector=label_selector
        ).items[0]
        ls_output = stream(
            kserve_client.core_api.connect_get_namespaced_pod_exec,
            pod.metadata.name,
            KSERVE_TEST_NAMESPACE,
            container="kserve-container",
            command=["ls", MODEL_MOUNT_PATH],
            stderr=True,
            stdin=False,
            stdout=True,
            tty=False,
        )
        assert EXPECTED_MODEL_FILE in ls_output, (
            f"Expected '{EXPECTED_MODEL_FILE}' under {MODEL_MOUNT_PATH} after "
            f"oci+fetch:// pull, but `ls` returned: {ls_output!r}"
        )

    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)


@pytest.mark.storage
def test_oci_fetch_with_image_pull_secret_spec_only():
    """An imagePullSecret on an oci+fetch:// ISVC is wired as a docker config.

    Spec-only: the webhook must project the first imagePullSecret as a docker
    config.json volume on the storage-initializer init container and signal its
    path via the KSERVE_OCI_DOCKER_CONFIG env var. The referenced secret does
    not need to exist — only the materialised pod spec is asserted (the pod may
    never start). The InferenceService is deleted in teardown.
    """
    service_name = "isvc-oci-fetch-auth"
    secret_name = "oci-fetch-registry-creds"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    label_selector = f"{ISVC_LABEL_KEY}={service_name}"

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        image_pull_secrets=[client.V1LocalObjectReference(name=secret_name)],
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=f"oci+fetch://{OCI_FETCH_TEST_IMAGE}",
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
            kserve_client.core_api, KSERVE_TEST_NAMESPACE, label_selector
        )
        assert pod is not None, (
            f"No pod appeared for InferenceService '{service_name}' "
            f"within {POD_WAIT_TIMEOUT}s"
        )

        pod_dict = pod.to_dict()
        spec = pod_dict.get("spec", {})

        # 1. Projected docker-config volume sourced from the imagePullSecret.
        volumes = spec.get("volumes", []) or []
        docker_cfg_vol = next(
            (v for v in volumes if v.get("name") == OCI_FETCH_DOCKER_CONFIG_VOLUME),
            None,
        )
        assert docker_cfg_vol is not None, (
            f"Expected volume '{OCI_FETCH_DOCKER_CONFIG_VOLUME}' projecting the "
            f"imagePullSecret. Volumes present: {[v.get('name') for v in volumes]}"
        )
        secret_source = docker_cfg_vol.get("secret") or {}
        assert secret_source.get("secret_name") == secret_name, (
            f"docker-config volume should project secret '{secret_name}', got "
            f"{secret_source.get('secret_name')!r}"
        )

        # 2. The storage-initializer init container consumes it and is told where.
        init_containers = spec.get("init_containers", []) or []
        init = next(
            (
                c
                for c in init_containers
                if c.get("name") == STORAGE_INITIALIZER_CONTAINER
            ),
            None,
        )
        assert init is not None, (
            f"'{STORAGE_INITIALIZER_CONTAINER}' init container not found. "
            f"Init containers: {[c.get('name') for c in init_containers]}"
        )

        mounts = init.get("volume_mounts", []) or []
        cfg_mount = next(
            (m for m in mounts if m.get("name") == OCI_FETCH_DOCKER_CONFIG_VOLUME),
            None,
        )
        assert cfg_mount is not None, (
            f"Init container missing VolumeMount for "
            f"'{OCI_FETCH_DOCKER_CONFIG_VOLUME}'. Mounts: "
            f"{[m.get('name') for m in mounts]}"
        )

        env = init.get("env", []) or []
        cfg_env = next(
            (e for e in env if e.get("name") == OCI_FETCH_DOCKER_CONFIG_ENV), None
        )
        assert cfg_env is not None, (
            f"Init container missing env var '{OCI_FETCH_DOCKER_CONFIG_ENV}'. "
            f"Env names: {[e.get('name') for e in env]}"
        )
        assert cfg_env.get("value", "").endswith("/config.json"), (
            f"{OCI_FETCH_DOCKER_CONFIG_ENV} should point at a config.json, got "
            f"{cfg_env.get('value')!r}"
        )
        # The advertised path must be covered by the mount.
        assert cfg_env["value"].startswith(cfg_mount.get("mount_path", "\x00")), (
            f"{OCI_FETCH_DOCKER_CONFIG_ENV}={cfg_env['value']!r} is not under the "
            f"mounted dir {cfg_mount.get('mount_path')!r}"
        )

    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
