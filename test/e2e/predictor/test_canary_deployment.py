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

import logging
import os
import time

import pytest
from kubernetes import client
from kubernetes.client import V1ResourceRequirements
from kubernetes.client.rest import ApiException

from kserve import (
    KServeClient,
    V1beta1CanarySpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)

from ..common.utils import KSERVE_TEST_NAMESPACE

logger = logging.getLogger(__name__)

STABLE_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.0/model"
CANARY_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.3/mixedtype"

RESOURCES = V1ResourceRequirements(
    requests={"cpu": "50m", "memory": "128Mi"},
    limits={"cpu": "100m", "memory": "256Mi"},
)


def _kserve_client():
    return KServeClient(config_file=os.environ.get("KUBECONFIG", "~/.kube/config"))


def _apps_v1():
    return client.AppsV1Api()


def _make_predictor(storage_uri, name=None, min_replicas=2):
    spec = V1beta1PredictorSpec(
        min_replicas=min_replicas,
        model=V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=storage_uri,
            resources=RESOURCES,
        ),
    )
    if name:
        spec.name = name
    return spec


def _make_isvc(service_name, canaries=None):
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={
                "serving.kserve.io/deploymentMode": "Standard",
                "serving.kserve.io/autoscalerClass": "none",
            },
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=_make_predictor(STABLE_MODEL_URI),
            canary=canaries,
        ),
    )


def _safe_delete(kserve, service_name):
    try:
        kserve.delete(service_name, KSERVE_TEST_NAMESPACE)
    except Exception:
        pass


def _wait_for_deployment(apps_v1, name, namespace, timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            dep = apps_v1.read_namespaced_deployment(name, namespace)
            if dep.status.available_replicas and dep.status.available_replicas > 0:
                return dep
        except ApiException as e:
            if e.status != 404:
                raise
        time.sleep(5)
    raise TimeoutError(f"Deployment {name} not ready within {timeout}s")


def _wait_for_deployment_gone(apps_v1, name, namespace, timeout=120):
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            apps_v1.read_namespaced_deployment(name, namespace)
        except ApiException as e:
            if e.status == 404:
                return
            raise
        time.sleep(5)
    raise TimeoutError(f"Deployment {name} still exists after {timeout}s")


def _get_pod_uids(apps_v1, deployment_name, namespace):
    core_v1 = client.CoreV1Api()
    dep = apps_v1.read_namespaced_deployment(deployment_name, namespace)
    selector = dep.spec.selector.match_labels
    label_selector = ",".join(f"{k}={v}" for k, v in selector.items())
    pods = core_v1.list_namespaced_pod(namespace, label_selector=label_selector)
    return {pod.metadata.uid for pod in pods.items}


@pytest.mark.predictor
def test_canary_create():
    service_name = "isvc-canary-create"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        stable_dep = _wait_for_deployment(
            apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
        )
        canary_dep = _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        assert stable_dep is not None
        assert canary_dep is not None

        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)

        conditions = got.get("status", {}).get("conditions", [])
        canary_condition = next(
            (c for c in conditions if c["type"] == "CanaryPredictorReady"), None
        )
        assert canary_condition is not None, "CanaryPredictorReady condition missing"
        assert canary_condition["status"] == "True"

        canary_status = got.get("status", {}).get("canary", [])
        assert len(canary_status) > 0, "canary status should be populated"
        assert canary_status[0]["name"] == "v2"
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
def test_canary_promote():
    service_name = "isvc-canary-promote"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        canary_pod_uids = _get_pod_uids(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        assert len(canary_pod_uids) > 0

        promoted = _make_isvc(service_name)
        promoted.spec.predictor = _make_predictor(CANARY_MODEL_URI, name="v2")
        promoted.spec.canary = None

        patch_resp = kserve.patch(
            service_name, promoted, namespace=KSERVE_TEST_NAMESPACE
        )
        kserve.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            expected_generation=patch_resp["metadata"]["generation"],
        )

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        _wait_for_deployment_gone(
            apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE
        )

        post_promote_uids = _get_pod_uids(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        assert canary_pod_uids == post_promote_uids, (
            f"Pod UIDs changed during promotion: before={canary_pod_uids}, after={post_promote_uids}"
        )
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
def test_canary_rollback():
    service_name = "isvc-canary-rollback"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        rolled_back = _make_isvc(service_name)
        rolled_back.spec.canary = None

        patch_resp = kserve.patch(
            service_name, rolled_back, namespace=KSERVE_TEST_NAMESPACE
        )
        kserve.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            expected_generation=patch_resp["metadata"]["generation"],
        )

        _wait_for_deployment_gone(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )
        _wait_for_deployment(apps, f"{service_name}-predictor", KSERVE_TEST_NAMESPACE)

        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        conditions = got.get("status", {}).get("conditions", [])
        canary_condition = next(
            (c for c in conditions if c["type"] == "CanaryPredictorReady"), None
        )
        assert canary_condition is None, (
            "CanaryPredictorReady should be cleared after rollback"
        )
    finally:
        _safe_delete(kserve, service_name)


@pytest.mark.predictor
def test_canary_force_stop():
    service_name = "isvc-canary-stop"
    kserve = _kserve_client()
    apps = _apps_v1()

    isvc = _make_isvc(
        service_name,
        canaries=[
            V1beta1CanarySpec(
                traffic_percent=20,
                predictor=_make_predictor(
                    CANARY_MODEL_URI, name="v2", min_replicas=None
                ),
            ),
        ],
    )

    try:
        kserve.create(isvc)
        kserve.wait_isvc_ready(service_name, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        stop_patch = V1beta1InferenceService(
            api_version=constants.KSERVE_V1BETA1,
            kind=constants.KSERVE_KIND_INFERENCESERVICE,
            metadata=client.V1ObjectMeta(
                name=service_name,
                namespace=KSERVE_TEST_NAMESPACE,
                annotations={
                    "serving.kserve.io/force-stop-canary-v2": "true",
                },
            ),
            spec=isvc.spec,
        )

        kserve.patch(service_name, stop_patch, namespace=KSERVE_TEST_NAMESPACE)

        _wait_for_deployment_gone(
            apps, f"{service_name}-v2-predictor", KSERVE_TEST_NAMESPACE
        )

        got = kserve.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        conditions = got.get("status", {}).get("conditions", [])
        canary_condition = next(
            (c for c in conditions if c["type"] == "CanaryPredictorReady"), None
        )
        assert canary_condition is not None
        assert canary_condition["reason"] == "Stopped"
    finally:
        _safe_delete(kserve, service_name)
