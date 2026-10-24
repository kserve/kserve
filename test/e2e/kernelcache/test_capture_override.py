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

"""Capture selection E2E tests for clusters with KernelCache enabled.

Each test creates its own namespace, KCC, and ISVC. The initial-selection and
ReplicaSet-rollout cases can run separately or in either order.

Pods remain unscheduled so capture cannot finish before the rollout. This tests
the real ISVC, Deployment, ReplicaSet, admission, and capture controllers without
requiring GPU hardware, image pulls, or registry access. It does not test the
runtime capture or artifact publishing flow.
"""

import json
import os
from uuid import uuid4

import pytest
from kubernetes import client, watch
from kserve import (
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)

from ..common.namespace import (
    create_namespace,
    delete_namespace,
    provision_serving_runtimes,
    skip_resource_deletion,
)

CAPTURE_PLURAL = "kernelcachecaptures"
CAPTURE_ANNOTATION = "serving.kserve.io/kernelcache-capture"
REPORTER_ANNOTATION = "internal.serving.kserve.io/kernelcache-reporter-secret"
POD_WAIT_TIMEOUT = 90
CAPTURE_OBSERVATION_TIMEOUT = 10


@pytest.fixture(scope="function")
def capture_namespace(kserve_client):
    """Give each test isolated resources and delete them through namespace cleanup."""
    namespace = f"e2e-kcc-{uuid4().hex[:10]}"
    core_api = kserve_client.core_api
    try:
        create_namespace(core_api, namespace)
        provision_serving_runtimes(kserve_client.api_instance, namespace)
        yield namespace
    finally:
        if not skip_resource_deletion():
            delete_namespace(core_api, namespace)


def _wait_for_resource(list_resources, predicate, description, **kwargs):
    """List, then watch from that version so resource creation cannot be missed."""
    resources = list_resources(**kwargs)
    for resource in resources.items:
        if predicate(resource):
            return resource
    watcher = watch.Watch()
    try:
        for event in watcher.stream(
            list_resources,
            resource_version=resources.metadata.resource_version,
            timeout_seconds=POD_WAIT_TIMEOUT,
            **kwargs,
        ):
            if event["type"] in ("ADDED", "MODIFIED") and predicate(event["object"]):
                return event["object"]
    finally:
        watcher.stop()
    pytest.fail(f"Timed out waiting for {description}")


def _controller_owner(resource, kind):
    owners = [
        owner
        for owner in resource.metadata.owner_references or []
        if owner.controller and owner.kind == kind
    ]
    assert len(owners) == 1, f"Expected one {kind} owner for {resource.metadata.name}"
    return owners[0]


def _assert_pod_uses_capture(pod, capture):
    """Check admission selected this KCC and copied its capture settings to MCV."""
    capture_name = capture["metadata"]["name"]
    assert pod.metadata.annotations.get(CAPTURE_ANNOTATION) == capture_name, (
        f"Pod {pod.metadata.name} did not inherit the KCC selection annotation"
    )
    sidecars = [
        container for container in pod.spec.containers if container.name == "mcv"
    ]
    assert len(sidecars) == 1, f"Expected an MCV sidecar in {pod.metadata.name}"
    env = {variable.name: variable.value for variable in sidecars[0].env}
    assert "MCV_CAPTURE_CONFIG" in env, f"Pod {pod.metadata.name} has no capture config"
    config = json.loads(env["MCV_CAPTURE_CONFIG"])
    assert config["capture"]["name"] == capture_name, (
        f"Pod {pod.metadata.name} selected a different KCC instead of {capture_name}"
    )
    assert config["capture"]["namespace"] == capture["metadata"]["namespace"]
    assert config["capture"]["sessionID"]
    assert config["targetImage"] == capture["spec"]["targetImage"]
    assert config["cachePaths"] == capture["spec"]["cachePaths"]
    return config


def _wait_for_reporter_secret(kserve_client, pod, previous_uid=None):
    """Wait for the capture controller's observable result after Pod admission."""
    reporter_name = pod.metadata.annotations[REPORTER_ANNOTATION]
    return _wait_for_resource(
        kserve_client.core_api.list_namespaced_secret,
        lambda secret: secret.metadata.deletion_timestamp is None
        and secret.metadata.uid != previous_uid,
        f"the reporter Secret for {pod.metadata.name}",
        namespace=pod.metadata.namespace,
        field_selector=f"metadata.name={reporter_name}",
    )


def _assert_only_referenced_capture(api, capture):
    """Reject extra KCCs, including any created and deleted before the final list."""
    namespace = capture["metadata"]["namespace"]
    capture_uid = capture["metadata"]["uid"]
    # Replay events since this test created the KCC, then observe for ten seconds
    # to allow asynchronous capture reconciliation to create any unexpected KCC.
    watcher = watch.Watch()
    try:
        for event in watcher.stream(
            api.list_namespaced_custom_object,
            constants.KSERVE_GROUP,
            constants.KSERVE_V1ALPHA1_VERSION,
            namespace,
            CAPTURE_PLURAL,
            resource_version=capture["metadata"]["resourceVersion"],
            timeout_seconds=CAPTURE_OBSERVATION_TIMEOUT,
        ):
            assert event["type"] != "DELETED", "The selected KCC was deleted"
            assert event["object"]["metadata"]["uid"] == capture_uid, (
                f"An additional KCC was created: {event['object']['metadata']['name']}"
            )
    finally:
        watcher.stop()
    captures = api.list_namespaced_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        namespace,
        CAPTURE_PLURAL,
    )["items"]
    actual_captures = [
        (item["metadata"]["name"], item["metadata"]["uid"]) for item in captures
    ]
    assert actual_captures == [(capture["metadata"]["name"], capture_uid)], (
        f"Expected only the original user-created KCC, got {actual_captures}"
    )
    assert captures[0]["spec"] == capture["spec"], "The selected KCC spec was changed"
    assert not captures[0]["metadata"].get("ownerReferences"), (
        "The user-created KCC was adopted by a workload controller"
    )


def _create_capture(kserve_client, namespace, service_name, capture_name):
    """Create a user-owned KCC with explicit target image and cache-path settings."""
    config_map = kserve_client.core_api.read_namespaced_config_map(
        "inferenceservice-config", os.environ.get("KSERVE_NAMESPACE", "kserve")
    )
    config = json.loads(config_map.data["kernelcache"])
    assert config.get("enabled"), "Enable KernelCache with setup-kernelcache.sh first"

    endpoint = config.get("registry", {}).get("endpoint") or "registry.example.com"
    return kserve_client.api_instance.create_namespaced_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1ALPHA1_VERSION,
        namespace,
        CAPTURE_PLURAL,
        {
            "apiVersion": constants.KSERVE_V1ALPHA1,
            "kind": "KernelCacheCapture",
            "metadata": {"name": capture_name, "namespace": namespace},
            "spec": {
                "sourceRef": {"kind": "InferenceService", "name": service_name},
                "targetImage": f"{endpoint}/{namespace}/cache:selected",
                "cachePaths": [
                    {
                        "containerName": "kserve-container",
                        "containerPath": "/tmp/selected-cache",
                        "ociPath": "io.vllm.cache",
                    }
                ],
            },
        },
    )


def _make_isvc(namespace, service_name, capture_name):
    """Build a Standard-mode ISVC whose Pods select the named KCC."""
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=namespace,
            annotations={
                "serving.kserve.io/deploymentMode": "Standard",
                "serving.kserve.io/kernelcache-supported": "true",
                "serving.kserve.io/kernelcache-sidecar-injection": "true",
                CAPTURE_ANNOTATION: capture_name,
            },
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                max_replicas=1,
                # No node has this unique label. Admission and controller
                # reconciliation still run, while the sidecar cannot start.
                node_selector={"kernelcache.kserve.io/e2e": namespace},
                model=V1beta1ModelSpec(
                    model_format=V1beta1ModelFormat(name="sklearn"),
                    runtime="kserve-sklearnserver",
                    storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
                    readiness_probe=client.V1Probe(
                        http_get=client.V1HTTPGetAction(path="/health", port=8080)
                    ),
                    resources=client.V1ResourceRequirements(
                        requests={"cpu": "50m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
            )
        ),
    )


def _wait_for_predictor_pod(
    kserve_client, namespace, service_name, previous_rs_uid=None
):
    """Wait for a live predictor Pod, optionally owned by a different ReplicaSet."""
    pod_selector = f"serving.kserve.io/inferenceservice={service_name}"
    description = (
        f"a predictor Pod from a new ReplicaSet for {service_name}"
        if previous_rs_uid
        else f"the initial predictor Pod for {service_name}"
    )
    return _wait_for_resource(
        kserve_client.core_api.list_namespaced_pod,
        lambda pod: pod.metadata.deletion_timestamp is None
        and _controller_owner(pod, "ReplicaSet").uid != previous_rs_uid,
        description,
        namespace=namespace,
        label_selector=pod_selector,
    )


@pytest.mark.kernelcache
def test_capture_override_uses_referenced_capture(kserve_client, capture_namespace):
    """Test that an annotated ISVC uses an existing KCC instead of creating one.

    The test:
    1. Creates a user-owned KCC with a custom target image and cache path.
    2. Creates an ISVC whose kernelcache-capture annotation references that KCC.
    3. Verifies the admitted Pod and capture controller use the referenced KCC.
    4. Verifies the original KCC is unchanged and no automatic KCC was created.
    """
    namespace = capture_namespace
    service_name = "capture-override-initial"
    capture_name = "user-created-capture"

    # The referenced KCC must exist before the ISVC's first Pod is admitted.
    capture = _create_capture(kserve_client, namespace, service_name, capture_name)
    kserve_client.create(_make_isvc(namespace, service_name, capture_name))
    pod = _wait_for_predictor_pod(kserve_client, namespace, service_name)

    # Verify both Pod admission and controller reconciliation selected this KCC.
    _assert_pod_uses_capture(pod, capture)
    reporter = _wait_for_reporter_secret(kserve_client, pod)
    assert (
        _controller_owner(reporter, "KernelCacheCapture").uid
        == capture["metadata"]["uid"]
    ), "The reporter Secret is not owned by the referenced KCC"

    # Check object identity as well as count, so deletion/recreation also fails.
    _assert_only_referenced_capture(kserve_client.api_instance, capture)


@pytest.mark.kernelcache
def test_capture_override_reused_across_replicasets(kserve_client, capture_namespace):
    """Test that a new ReplicaSet reuses the ISVC's selected KCC.

    The test:
    1. Creates its own user-owned KCC and annotated ISVC.
    2. Records the first Pod's ReplicaSet and selected KCC.
    3. Updates a predictor-template annotation to create a new ReplicaSet.
    4. Verifies the new Pod and capture controller still use the original KCC.
    5. Verifies no additional KCC was created during the rollout.
    """
    namespace = capture_namespace
    service_name = "capture-override-rollout"
    capture_name = "user-created-capture"
    core_api = kserve_client.core_api
    api = kserve_client.api_instance
    apps_api = kserve_client.app_api

    # Create this test's resources independently of the initial-selection test.
    capture = _create_capture(kserve_client, namespace, service_name, capture_name)
    capture_uid = capture["metadata"]["uid"]
    kserve_client.create(_make_isvc(namespace, service_name, capture_name))
    first_pod = _wait_for_predictor_pod(kserve_client, namespace, service_name)

    # Record the original ReplicaSet and confirm its Pod selected the KCC.
    first_config = _assert_pod_uses_capture(first_pod, capture)
    first_owner = _controller_owner(first_pod, "ReplicaSet")
    first_rs = apps_api.read_namespaced_replica_set(first_owner.name, namespace)
    assert first_rs.metadata.uid == first_owner.uid

    reporter_name = first_pod.metadata.annotations[REPORTER_ANNOTATION]
    reporter = _wait_for_reporter_secret(kserve_client, first_pod)
    assert _controller_owner(reporter, "KernelCacheCapture").uid == capture_uid
    # Remove this test's reporter Secret before creating the new Pod. Its
    # recreation lets us observe capture-controller work for the new revision;
    # checking MCV configuration alone would only exercise Pod admission.
    core_api.delete_namespaced_secret(reporter_name, namespace)
    # Change only a predictor-template annotation: Kubernetes must create a new
    # ReplicaSet while the model and selected KCC stay the same.
    api.patch_namespaced_custom_object(
        constants.KSERVE_GROUP,
        constants.KSERVE_V1BETA1_VERSION,
        namespace,
        constants.KSERVE_PLURAL_INFERENCESERVICE,
        service_name,
        {
            "spec": {
                "predictor": {"annotations": {"kernelcache.kserve.io/rollout": "2"}}
            }
        },
    )
    second_pod = _wait_for_predictor_pod(
        kserve_client, namespace, service_name, previous_rs_uid=first_owner.uid
    )

    # The new Pod must belong to a different ReplicaSet of the same Deployment.
    second_config = _assert_pod_uses_capture(second_pod, capture)
    second_owner = _controller_owner(second_pod, "ReplicaSet")
    second_rs = apps_api.read_namespaced_replica_set(second_owner.name, namespace)
    assert second_rs.metadata.uid == second_owner.uid
    assert second_rs.metadata.uid != first_rs.metadata.uid, (
        "No new ReplicaSet was created"
    )
    assert (
        second_pod.metadata.labels["pod-template-hash"]
        != first_pod.metadata.labels["pod-template-hash"]
    ), "The predictor Pod template did not change"
    assert (
        _controller_owner(second_rs, "Deployment").uid
        == _controller_owner(first_rs, "Deployment").uid
    ), "The new ReplicaSet belongs to a different Deployment"
    assert second_config["capture"]["sessionID"] != first_config["capture"]["sessionID"]
    assert second_pod.metadata.annotations[REPORTER_ANNOTATION] == reporter_name
    new_reporter = _wait_for_reporter_secret(
        kserve_client, second_pod, previous_uid=reporter.metadata.uid
    )
    assert _controller_owner(new_reporter, "KernelCacheCapture").uid == capture_uid, (
        "The new Pod's reporter Secret is not owned by the original KCC"
    )

    # Reuse means the original name and UID survive, with no per-revision KCCs.
    _assert_only_referenced_capture(api, capture)
