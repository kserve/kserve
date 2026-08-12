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

"""
Regression test for oci:// (modelcar) uidModelcar on InferenceService.

When uidModelcar is configured in the storageInitializer config,
the modelcar sidecar and the inference container must run as the
same UID.  Without this, ptrace_scope=1 (the default on many
Linux distributions) blocks the inference container from
traversing /proc/<pid>/root/ of the modelcar sidecar, breaking
the symlink-based model mount.
"""

import json
import os

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements
from kserve import (
    KServeClient,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    V1beta1ModelFormat,
    constants,
)

from ..common.utils import KSERVE_NAMESPACE, KSERVE_TEST_NAMESPACE

SKLEARN_MODELCAR_URI = os.environ.get("SKLEARN_MODELCAR_URI")

ISVC_LABEL_KEY = "serving.kserve.io/inferenceservice"
KSERVE_CONTAINER_NAME = "kserve-container"
MODELCAR_CONTAINER_NAME = "modelcar"


def _get_uid_modelcar_from_config(core_api, namespace: str) -> int | None:
    """Read uidModelcar from the inferenceservice-config."""
    try:
        cm = core_api.read_namespaced_config_map("inferenceservice-config", namespace)
        raw = cm.data.get("storageInitializer", "{}")
        cfg = json.loads(raw)
        return cfg.get("uidModelcar")
    except Exception:  # noqa: BLE001
        return None


def _assert_modelcar_uid(pod, uid_modelcar, resource_kind):
    """Assert shared PID namespace and matching UIDs."""
    pod_dict = pod.to_dict()
    pod_spec = pod_dict.get("spec", {})

    assert pod_spec.get("share_process_namespace") is True, (
        f"{resource_kind}: expected ShareProcessNamespace "
        "to be enabled for modelcar OCI serving"
    )

    containers = pod_spec.get("containers", []) or []

    kserve_ctr = next(
        (c for c in containers if c.get("name") == KSERVE_CONTAINER_NAME),
        None,
    )
    assert kserve_ctr is not None, (
        f"{resource_kind}: {KSERVE_CONTAINER_NAME} "
        f"not found. "
        f"Names: {[c.get('name') for c in containers]}"
    )

    modelcar_ctr = next(
        (c for c in containers if c.get("name") == MODELCAR_CONTAINER_NAME),
        None,
    )
    assert modelcar_ctr is not None, (
        f"{resource_kind}: {MODELCAR_CONTAINER_NAME} "
        f"not found. "
        f"Names: {[c.get('name') for c in containers]}"
    )

    kserve_sc = kserve_ctr.get("security_context") or {}
    modelcar_sc = modelcar_ctr.get("security_context") or {}

    kserve_uid = kserve_sc.get("run_as_user")
    assert kserve_uid == uid_modelcar, (
        f"{resource_kind}: expected "
        f"{KSERVE_CONTAINER_NAME} "
        f"RunAsUser={uid_modelcar}, got {kserve_uid}"
    )
    modelcar_uid = modelcar_sc.get("run_as_user")
    assert modelcar_uid == uid_modelcar, (
        f"{resource_kind}: expected "
        f"{MODELCAR_CONTAINER_NAME} "
        f"RunAsUser={uid_modelcar}, got {modelcar_uid}"
    )


@pytest.mark.predictor
def test_oci_modelcar_uid_isvc():
    """ISVC with oci:// produces matching UIDs on containers.

    1. Reads uidModelcar from the inferenceservice-config.
    2. Creates an InferenceService with oci:// storageUri.
    3. Waits for the ISVC to become ready.
    4. Asserts ShareProcessNamespace + matching RunAsUser.
    """
    if not SKLEARN_MODELCAR_URI:
        pytest.skip("SKLEARN_MODELCAR_URI not set")

    service_name = "isvc-oci-modelcar-uid-smoke"
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )

    uid_modelcar = _get_uid_modelcar_from_config(
        kserve_client.core_api, KSERVE_NAMESPACE
    )
    if uid_modelcar is None:
        pytest.skip("uidModelcar is not configured in inferenceservice-config")

    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=SKLEARN_MODELCAR_URI,
            resources=V1ResourceRequirements(
                requests={
                    "cpu": "50m",
                    "memory": "128Mi",
                },
                limits={
                    "cpu": "100m",
                    "memory": "256Mi",
                },
            ),
        ),
    )

    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        ),
        spec=V1beta1InferenceServiceSpec(predictor=predictor),
    )

    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"{ISVC_LABEL_KEY}={service_name}",
        )
        assert pods.items, f"No pod found for ISVC '{service_name}' after ready"
        pod = pods.items[0]

        _assert_modelcar_uid(pod, uid_modelcar, "InferenceService")

    finally:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
