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

"""Helpers for LLMInferenceService e2e tests on the DisaggregatedSet backend.

The DisaggregatedSet (DS) and its child LeaderWorkerSets (LWS) are created by
the LWS operator from the DS that the llmisvc controller writes. Child LWSes
are named ``<ds>-<slice>-<revision>-<role>``, and their pods carry the labels
``disaggregatedset.x-k8s.io/{name,role,slice,revision}``.
"""

import json
import logging
from dataclasses import dataclass
from typing import Dict, List, Optional

import yaml
from kserve import constants
from kubernetes import client

from ..common.utils import KSERVE_NAMESPACE
from .autoscaling_helpers import (
    _get_custom_resource,
    wait_for_resource,
    wait_for_resource_deleted,
)
from .test_llm_inference_service import (
    KSERVE_PLURAL_LLMINFERENCESERVICE,
    get_llmisvc,
    wait_for,
)

logger = logging.getLogger(__name__)

INFERENCESERVICE_CONFIGMAP = "inferenceservice-config"
LLMISVC_CONFIGMAP_KEY = "llmisvc"
DISAGGREGATED_SET_GATE = "disaggregatedSet"

DS_GROUP = "disaggregatedset.x-k8s.io"
DS_VERSION = "v1"
DS_PLURAL = "disaggregatedsets"
DS_KIND = "DisaggregatedSet"
DS_ROLE_SCALER_PLURAL = "disaggregatedsetrolescalers"
DS_ROLE_SCALER_KIND = "DisaggregatedSetRoleScaler"
DS_REVISION_HASH_VERSION_ANNOTATION = f"{DS_GROUP}/revision-hash-version"

DS_NAME_LABEL = f"{DS_GROUP}/name"
DS_ROLE_LABEL = f"{DS_GROUP}/role"
DS_REVISION_LABEL = f"{DS_GROUP}/revision"

LWS_GROUP = "leaderworkerset.x-k8s.io"
LWS_VERSION = "v1"
LWS_PLURAL = "leaderworkersets"

ROLE_DECODE = "decode"
ROLE_PREFILL = "prefill"
ROLES = (ROLE_DECODE, ROLE_PREFILL)

DISAGGREGATED_SET_ANNOTATION = "serving.kserve.io/enable-disaggregated-set"
DISAGGREGATED_SET_USED = "DisaggregatedSetUsed"
MIGRATING_TO_REASON = "MigratingToDisaggregatedSet"
MIGRATING_FROM_REASON = "MigratingFromDisaggregatedSet"

# Mirrors disaggregatedSetNameMaxLength in
# pkg/controller/v1alpha2/llmisvc/workload_disaggregated.go. Longer names are
# truncated and hashed by the controller; the tests keep names short instead.
_DS_NAME_SUFFIX = "-kserve-pd"
_DS_NAME_MAX_LENGTH = 28


def require_disaggregated_set_gate(kserve_client) -> None:
    """Fail unless the DisaggregatedSet feature gate is on in the cluster."""
    cm = kserve_client.core_api.read_namespaced_config_map(
        INFERENCESERVICE_CONFIGMAP, KSERVE_NAMESPACE
    )
    raw = (cm.data or {}).get(LLMISVC_CONFIGMAP_KEY, "") or "{}"
    gates = json.loads(raw).get("featureGates", {})
    assert gates.get(DISAGGREGATED_SET_GATE) is True, (
        f"The {DISAGGREGATED_SET_GATE} feature gate is not enabled in "
        f"{KSERVE_NAMESPACE}/{INFERENCESERVICE_CONFIGMAP} (key "
        f"{LLMISVC_CONFIGMAP_KEY!r} is {raw!r}). Run "
        "test/scripts/gh-actions/enable-llmisvc-feature-gates.sh "
        f"{DISAGGREGATED_SET_GATE} before these tests."
    )


def ds_name(service_name: str) -> str:
    """Return the DisaggregatedSet name the controller uses for a service."""
    name = service_name + _DS_NAME_SUFFIX
    assert len(name) <= _DS_NAME_MAX_LENGTH, (
        f"Service name {service_name!r} gives DisaggregatedSet name {name!r}, "
        f"longer than {_DS_NAME_MAX_LENGTH} characters; use a shorter service name"
    )
    return name


def role_scaler_name(service_name: str, role: str) -> str:
    return f"{ds_name(service_name)}-{role}"


def get_ds(service_name: str, namespace: str) -> Optional[dict]:
    return _get_custom_resource(
        DS_GROUP, DS_VERSION, DS_PLURAL, ds_name(service_name), namespace
    )


def wait_for_ds(service_name: str, namespace: str, timeout: int = 120) -> dict:
    wait_for_resource(
        DS_GROUP, DS_VERSION, DS_PLURAL, ds_name(service_name), namespace, timeout
    )
    return get_ds(service_name, namespace)


def wait_for_ds_deleted(service_name: str, namespace: str, timeout: int = 180):
    wait_for_resource_deleted(
        DS_GROUP, DS_VERSION, DS_PLURAL, ds_name(service_name), namespace, timeout
    )


def list_role_lws(service_name: str, namespace: str, role: str) -> List[dict]:
    """List the child LeaderWorkerSets of one DisaggregatedSet role."""
    selector = f"{DS_NAME_LABEL}={ds_name(service_name)},{DS_ROLE_LABEL}={role}"
    result = client.CustomObjectsApi().list_namespaced_custom_object(
        LWS_GROUP, LWS_VERSION, namespace, LWS_PLURAL, label_selector=selector
    )
    return result.get("items", [])


def role_lws_names(service_name: str, namespace: str, role: str) -> List[str]:
    return sorted(
        lws["metadata"]["name"] for lws in list_role_lws(service_name, namespace, role)
    )


@dataclass(frozen=True)
class RolePod:
    name: str
    uid: str
    revision: str
    ready: bool
    labels: Dict[str, str]


def _pod_ready(pod: client.V1Pod) -> bool:
    if pod.metadata.deletion_timestamp is not None:
        return False
    for cond in pod.status.conditions or []:
        if cond.type == "Ready":
            return cond.status == "True"
    return False


def list_role_pods(service_name: str, namespace: str, role: str) -> List[RolePod]:
    """List the pods of one DisaggregatedSet role, including terminating ones."""
    selector = f"{DS_NAME_LABEL}={ds_name(service_name)},{DS_ROLE_LABEL}={role}"
    pods = client.CoreV1Api().list_namespaced_pod(namespace, label_selector=selector)
    return [
        RolePod(
            name=p.metadata.name,
            uid=p.metadata.uid,
            revision=(p.metadata.labels or {}).get(DS_REVISION_LABEL, ""),
            ready=_pod_ready(p),
            labels=dict(p.metadata.labels or {}),
        )
        for p in pods.items
    ]


def wait_for_role_pods(
    service_name: str,
    namespace: str,
    role: str,
    count: int,
    exclude_revision: Optional[str] = None,
    timeout: int = 600,
) -> List[RolePod]:
    """Wait until a role has exactly ``count`` pods, all Ready.

    With ``exclude_revision``, also wait until no pod of that revision is left.
    """

    def _check():
        pods = list_role_pods(service_name, namespace, role)
        summary = [(p.name, p.revision, p.ready) for p in pods]
        assert len(pods) == count and all(p.ready for p in pods), (
            f"{role} role of {ds_name(service_name)}: want {count} Ready pods, "
            f"got {summary}"
        )
        if exclude_revision is not None:
            assert all(p.revision != exclude_revision for p in pods), (
                f"{role} role of {ds_name(service_name)} still has pods of "
                f"revision {exclude_revision}: {summary}"
            )
        return pods

    return wait_for(_check, timeout=timeout, interval=5.0)


def get_condition(kserve_client, llm_isvc, condition_type: str) -> Optional[dict]:
    svc = get_llmisvc(
        kserve_client, llm_isvc.metadata.name, llm_isvc.metadata.namespace, "v1alpha2"
    )
    for cond in svc.get("status", {}).get("conditions", []) or []:
        if cond.get("type") == condition_type:
            return cond
    return None


def wait_for_condition(
    kserve_client,
    llm_isvc,
    condition_type: str,
    status: str,
    reason: Optional[str] = None,
    timeout: int = 120,
) -> dict:
    def _check():
        cond = get_condition(kserve_client, llm_isvc, condition_type)
        assert cond is not None, f"{condition_type} condition not found"
        assert cond.get("status") == status, (
            f"{condition_type} is {cond.get('status')}, want {status}: {cond}"
        )
        if reason is not None:
            assert cond.get("reason") == reason, (
                f"{condition_type} reason is {cond.get('reason')}, want {reason}: {cond}"
            )
        return cond

    return wait_for(_check, timeout=timeout, interval=2.0)


def patch_llmisvc_spec(kserve_client, llm_isvc, spec_patch: dict) -> None:
    """Apply a JSON merge patch to the service's spec."""
    kserve_client.api_instance.patch_namespaced_custom_object(
        constants.KSERVE_GROUP,
        llm_isvc.api_version.split("/")[1],
        llm_isvc.metadata.namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        llm_isvc.metadata.name,
        {"spec": spec_patch},
    )


def _read_deployment(name: str, namespace: str) -> Optional[client.V1Deployment]:
    try:
        return client.AppsV1Api().read_namespaced_deployment(name, namespace)
    except client.rest.ApiException as e:
        if e.status == 404:
            return None
        raise


def pd_deployment_names(service_name: str) -> List[str]:
    return [f"{service_name}-kserve", f"{service_name}-kserve-prefill"]


def wait_for_pd_deployments(
    service_name: str, namespace: str, exist: bool, timeout: int = 300
):
    """Wait until both P/D Deployments are gone, or exist with an available pod."""

    def _check():
        for name in pd_deployment_names(service_name):
            deployment = _read_deployment(name, namespace)
            if not exist:
                assert deployment is None, f"Deployment {namespace}/{name} still exists"
                continue
            assert deployment is not None, f"Deployment {namespace}/{name} not found"
            available = deployment.status.available_replicas or 0
            assert available >= 1, (
                f"Deployment {namespace}/{name} has {available} available replicas"
            )

    wait_for(_check, timeout=timeout, interval=2.0)


def wait_for_event(llm_isvc, reason: str, timeout: int = 120) -> client.CoreV1Event:
    """Wait for an event with ``reason`` on the LLMInferenceService."""
    selector = (
        f"involvedObject.kind=LLMInferenceService,"
        f"involvedObject.name={llm_isvc.metadata.name}"
    )

    def _check():
        events = client.CoreV1Api().list_namespaced_event(
            llm_isvc.metadata.namespace, field_selector=selector
        )
        matching = [e for e in events.items if e.reason == reason]
        assert matching, (
            f"No {reason} event on {llm_isvc.metadata.name}; got "
            f"{sorted({e.reason for e in events.items})}"
        )
        return matching[0]

    return wait_for(_check, timeout=timeout, interval=2.0)


def assert_ds_owned_by(ds: dict, llm_isvc) -> None:
    owners = ds["metadata"].get("ownerReferences", []) or []
    controller = [o for o in owners if o.get("controller")]
    assert len(controller) == 1, f"Want one controller owner, got {owners}"
    owner = controller[0]
    assert owner.get("kind") == "LLMInferenceService", owner
    assert owner.get("name") == llm_isvc.metadata.name, owner


def assert_status_workloads_reference_ds(kserve_client, llm_isvc) -> None:
    svc = get_llmisvc(
        kserve_client, llm_isvc.metadata.name, llm_isvc.metadata.namespace, "v1alpha2"
    )
    workloads = svc.get("status", {}).get("workloads", {}) or {}
    want = ds_name(llm_isvc.metadata.name)
    for key in ("primary", "prefill"):
        ref = workloads.get(key) or {}
        assert ref.get("kind") == DS_KIND and ref.get("name") == want, (
            f"status.workloads.{key} should reference {DS_KIND} {want}, got {ref}"
        )
        assert ref.get("apiGroup") == DS_GROUP, (
            f"status.workloads.{key}.apiGroup should be {DS_GROUP}, got {ref}"
        )


def dump_disaggregated_set(service_name: str, namespace: str, log=logger.info):
    """Log the DisaggregatedSet, its child LeaderWorkerSets and their pods."""
    try:
        ds = get_ds(service_name, namespace)
        log(
            f"DisaggregatedSet {namespace}/{ds_name(service_name)}:\n"
            f"{yaml.safe_dump(ds) if ds else '<not found>'}"
        )
        for role in ROLES:
            for lws in list_role_lws(service_name, namespace, role):
                lws.get("metadata", {}).pop("managedFields", None)
                log(
                    f"LeaderWorkerSet {lws['metadata']['name']}:\n{yaml.safe_dump(lws)}"
                )
            for pod in list_role_pods(service_name, namespace, role):
                log(f"{role} pod {pod.name} revision={pod.revision} ready={pod.ready}")
    except Exception as e:  # diagnostics must not mask the test failure
        log(f"Failed to dump DisaggregatedSet {ds_name(service_name)}: {e}")
