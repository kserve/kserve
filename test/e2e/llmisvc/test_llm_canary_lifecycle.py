"""Zero-downtime canary rollout lifecycle under continuous traffic.

Self-contained: deploys members, runs canary lifecycle under load,
validates zero errors and correct distribution, then cleans up.

Uses Service backends (works with any gateway). InferencePool scenarios
require Istio and are in a separate test job.

Marked @pytest.mark.traffic for selective runs.

Note: this file uses its own resource management (apply_member, apply_config,
wait_ready, etc.) rather than the test_case fixture from fixtures.py. The
test_case/TestCase pattern is designed for single-service "create, wait, query,
delete" workflows and does not support group membership, mid-test weight
patching, or multi-member lifecycle mutations. This is consistent with other
specialized test files (test_flow_control.py, test_rolling_upgrade.py).
A future unification could extend TestCase with group/weight fields and
patching support.
"""

import json
import logging
import os
import requests
import time
from dataclasses import dataclass
from urllib.parse import urlparse

import pytest
from kubernetes import client as k8s_client, config

from .fixtures import LLMD_SIMULATOR_SECURITY_CONTEXT

logger = logging.getLogger(__name__)

KUBE_CONTEXT = os.environ.get("KUBE_CONTEXT", None)
KSERVE_GROUP = "serving.kserve.io"
KSERVE_VERSION = "v1alpha2"
KSERVE_PLURAL = "llminferenceservices"
KSERVE_CONFIG_PLURAL = "llminferenceserviceconfigs"

MODEL = "tiny-llama"
GROUP = "tiny-llama"
MODEL_URI = "hf://hmellor/tiny-random-LlamaForCausalLM"

INFERENCE_SIM_IMAGE = os.environ.get(
    "INFERENCE_SIM_IMAGE",
    "ghcr.io/llm-d/llm-d-inference-sim:v0.8.2",
)


@dataclass
class WorkloadConfig:
    """Workload container configuration. Pluggable - swap for inference-sim,
    GPU vLLM, or any OpenAI-compatible server."""

    name: str  # config name in the namespace
    image: str
    args: list = None
    env: list = None
    resources: dict = None

    command: list = None  # overrides the controller's entrypoint wrapper
    security_context: dict = None
    storage_initializer: bool = True  # False for non-vLLM workloads

    def to_spec(self) -> dict:
        container = {"name": "main", "image": self.image}
        if self.command:
            container["command"] = self.command
        if self.args:
            container["args"] = self.args
        if self.env:
            container["env"] = self.env
        if self.resources:
            container["resources"] = self.resources
        if self.security_context:
            container["securityContext"] = self.security_context
        spec = {"template": {"containers": [container]}}
        if not self.storage_initializer:
            spec["storageInitializer"] = {"enabled": False}
        return spec


INFERENCE_SIM = WorkloadConfig(
    name="canary-workload",
    image=INFERENCE_SIM_IMAGE,
    command=["/app/llm-d-inference-sim"],
    args=["--port", "8000", "--model", MODEL, "--mode", "echo"],
    env=[
        {"name": "POD_NAME", "valueFrom": {"fieldRef": {"fieldPath": "metadata.name"}}},
    ],
    resources={
        "requests": {"cpu": "50m", "memory": "64Mi"},
        "limits": {"cpu": "200m", "memory": "128Mi"},
    },
    security_context=LLMD_SIMULATOR_SECURITY_CONTEXT,
    storage_initializer=False,
)


DIV_GROUP = "divergence-test"
DIV_V1 = "test-div-v1"
DIV_V2 = "test-div-v2"
DIV_MODEL_V1 = "model-alpha"
DIV_MODEL_V2 = "model-beta"


def apply_divergence_member(api, name, model_name, weight, ns):
    """Deploy a member for divergence tests (service backend, no scheduler)."""
    model_spec = {"name": model_name, "uri": MODEL_URI}
    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_VERSION}",
        "kind": "LLMInferenceService",
        "metadata": {"name": name, "namespace": ns},
        "spec": {
            "model": model_spec,
            "baseRefs": [{"name": INFERENCE_SIM.name}],
            "router": {
                "route": {
                    "group": DIV_GROUP,
                    "weight": weight,
                    "http": {},
                },
            },
        },
    }
    try:
        api.create_namespaced_custom_object(
            KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, body
        )
    except k8s_client.ApiException as e:
        if e.status == 409:
            api.patch_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name, body
            )
        else:
            raise
    logger.info(f"Applied divergence member {name} (model={model_name})")


@dataclass
class MemberSpec:
    """Describes a group member's routing and backend configuration."""

    name: str
    weight: int
    scheduler: bool  # True = InferencePool (EPP), False = plain Service
    workload: WorkloadConfig = None  # defaults to VLLM_CPU

    def __post_init__(self):
        if self.workload is None:
            self.workload = INFERENCE_SIM


@dataclass
class Scenario:
    """A canary test scenario - two members with specified backends."""

    name: str
    v1: MemberSpec
    v2: MemberSpec
    description: str = ""


SCENARIOS = {
    "service": Scenario(
        name="service",
        description="Both members use plain Service backend (no scheduler)",
        v1=MemberSpec(name="canary-v1", weight=9, scheduler=False),
        v2=MemberSpec(name="canary-v2", weight=1, scheduler=False),
    ),
}


# ---------------------------------------------------------------------------
# K8s helpers
# ---------------------------------------------------------------------------


def get_api():
    config.load_kube_config(context=KUBE_CONTEXT)
    return k8s_client.CustomObjectsApi()


def apply_config(api, name, ns, spec):
    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_VERSION}",
        "kind": "LLMInferenceServiceConfig",
        "metadata": {"name": name, "namespace": ns},
        "spec": spec,
    }
    try:
        api.create_namespaced_custom_object(
            KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_CONFIG_PLURAL, body
        )
    except k8s_client.ApiException as e:
        if e.status == 409:
            api.patch_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_CONFIG_PLURAL, name, body
            )
        else:
            raise


def apply_member(api, member: MemberSpec, ns):
    spec = {
        "model": {"name": MODEL, "uri": MODEL_URI},
        "baseRefs": [
            {"name": member.workload.name},
        ],
        "router": {
            "route": {
                "group": GROUP,
                "weight": member.weight,
                "http": {},
            },
        },
    }
    if member.scheduler:
        spec["router"]["scheduler"] = {}

    body = {
        "apiVersion": f"{KSERVE_GROUP}/{KSERVE_VERSION}",
        "kind": "LLMInferenceService",
        "metadata": {
            "name": member.name,
            "namespace": ns,
        },
        "spec": spec,
    }
    try:
        api.create_namespaced_custom_object(
            KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, body
        )
    except k8s_client.ApiException as e:
        if e.status == 409:
            api.patch_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, member.name, body
            )
        else:
            raise
    logger.info(
        f"Applied {member.name} (weight={member.weight}, scheduler={member.scheduler})"
    )


def delete_member(api, name, ns, wait=False):
    try:
        api.delete_namespaced_custom_object(
            KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
        )
    except k8s_client.ApiException as e:
        if e.status != 404:
            raise
        return
    if wait:
        deadline = time.monotonic() + 120
        while time.monotonic() < deadline:
            try:
                api.get_namespaced_custom_object(
                    KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
                )
            except k8s_client.ApiException as e:
                if e.status == 404:
                    return
            time.sleep(2)
        logger.warning(f"{name} still exists after 120s")


def wait_ready(api, name, ns, timeout=600):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            obj = api.get_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
            )
            conditions = obj.get("status", {}).get("conditions", [])
            for c in conditions:
                if c.get("type") == "Ready" and c.get("status") == "True":
                    return
        except k8s_client.ApiException:
            pass
        time.sleep(5)
    raise TimeoutError(f"{name} not Ready within {timeout}s")


def get_gateway_base_url(api, name, ns, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            obj = api.get_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
            )
            url = obj.get("status", {}).get("url", "")
            if not url:
                addresses = obj.get("status", {}).get("addresses", [])
                if addresses:
                    url = addresses[0].get("url", "")
            if url:
                parsed = urlparse(url)
                return f"{parsed.scheme}://{parsed.netloc}"
        except k8s_client.ApiException:
            pass
        time.sleep(2)
    raise TimeoutError(f"No URL in {name} status after {timeout}s")


def wait_for_condition(
    api, name, ns, condition_type, status=None, reason=None, timeout=120
):
    """Wait for a condition to reach the expected state."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            obj = api.get_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
            )
            conditions = obj.get("status", {}).get("conditions", [])
            for c in conditions:
                if c["type"] == condition_type:
                    match = True
                    if status and c.get("status") != status:
                        match = False
                    if reason and c.get("reason") != reason:
                        match = False
                    if match:
                        return c
        except k8s_client.ApiException:
            pass
        time.sleep(1)
    raise TimeoutError(
        f"{name}: condition {condition_type} not reached (status={status}, reason={reason})"
    )


def wait_for_condition_absent(api, name, ns, condition_type, timeout=120):
    """Wait for a condition to be removed from the status."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            obj = api.get_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
            )
            conditions = obj.get("status", {}).get("conditions", [])
            if not any(c["type"] == condition_type for c in conditions):
                return
        except k8s_client.ApiException:
            pass
        time.sleep(1)
    raise TimeoutError(
        f"{name}: condition {condition_type} still present after {timeout}s"
    )


def get_group_members(api, name, ns):
    """Return the list of member names from group status."""
    obj = api.get_namespaced_custom_object(
        KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, name
    )
    group = obj.get("status", {}).get("router", {}).get("group", {})
    return [m["name"] for m in group.get("members", [])]


def patch_model_name(api, name, model_name, ns):
    """Patch the model.name field on an LLMInferenceService."""
    api.patch_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_VERSION,
        ns,
        KSERVE_PLURAL,
        name,
        {"spec": {"model": {"name": model_name}}},
    )


def patch_annotations(api, name, ns, annotations):
    api.patch_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_VERSION,
        ns,
        KSERVE_PLURAL,
        name,
        {"metadata": {"annotations": annotations}},
    )


def patch_stop(api, name, ns, stopped=True):
    patch_annotations(api, name, ns, {"serving.kserve.io/stop": str(stopped).lower()})
    logger.info(f"{'Stopped' if stopped else 'Resumed'} {name}")


def get_member_status(api, observer, member, ns):
    obj = api.get_namespaced_custom_object(
        KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, observer
    )
    for m in (
        obj.get("status", {}).get("router", {}).get("group", {}).get("members", [])
    ):
        if m.get("name") == member:
            return m
    return None


def wait_for_healthy_route(gateway_url, headers, payload, consecutive=3, timeout=30):
    """Poll the gateway until we get consecutive 2xx responses.

    Requires multiple consecutive successes to avoid false positives from
    stale pre-mutation routes that haven't been reprogrammed yet.
    """
    streak = 0
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            resp = requests.post(gateway_url, headers=headers, json=payload, timeout=5)
            if 200 <= resp.status_code < 300:
                streak += 1
                if streak >= consecutive:
                    return
            else:
                streak = 0
        except requests.RequestException:
            streak = 0
        time.sleep(0.5)
    raise TimeoutError(
        f"Route not healthy after {timeout}s ({streak}/{consecutive} consecutive 2xx) from {gateway_url}"
    )


def wait_for_member_count(api, name, ns, count, timeout=120):
    deadline = time.monotonic() + timeout
    members = []
    while time.monotonic() < deadline:
        members = get_group_members(api, name, ns)
        if len(members) == count:
            return members
        time.sleep(2)
    raise TimeoutError(
        f"{name}: expected {count} group members, got {len(members)}: {members}"
    )


def wait_for_member_stopped(api, observer, member, ns, stopped=True, timeout=120):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        m = get_member_status(api, observer, member, ns)
        if m and m.get("stopped", False) == stopped:
            return m
        time.sleep(2)
    raise TimeoutError(f"{member} stopped={stopped} not seen from {observer}")


def patch_weight(api, name, weight, ns):
    api.patch_namespaced_custom_object(
        KSERVE_GROUP,
        KSERVE_VERSION,
        ns,
        KSERVE_PLURAL,
        name,
        body={"spec": {"router": {"route": {"weight": weight}}}},
    )
    logger.info(f"Patched {name} weight={weight}")


def get_group_weight(api, observer, member, ns):
    obj = api.get_namespaced_custom_object(
        KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, observer
    )
    for m in (
        obj.get("status", {}).get("router", {}).get("group", {}).get("members", [])
    ):
        if m.get("name") == member:
            return m.get("weight")
    return None


def wait_for_group_weight(api, observer, member, expected, ns, timeout=30):
    deadline = time.monotonic() + timeout
    w = None
    while time.monotonic() < deadline:
        w = get_group_weight(api, observer, member, ns)
        if w == expected:
            return
        time.sleep(1)
    raise TimeoutError(f"{member} weight={w}, expected {expected} (from {observer})")


# ---------------------------------------------------------------------------
# Test fixtures
# ---------------------------------------------------------------------------


@pytest.fixture
def canary_env(request, test_namespace):
    """Deploy a canary scenario - namespace teardown handles cleanup."""
    scenario_name = request.param
    scenario = SCENARIOS[scenario_name]
    api = get_api()
    ns = test_namespace

    # Apply workload configs (deduplicate if both members use the same one)
    seen_configs = set()
    for member in [scenario.v1, scenario.v2]:
        if member.workload.name not in seen_configs:
            apply_config(api, member.workload.name, ns, member.workload.to_spec())
            seen_configs.add(member.workload.name)

    apply_member(api, scenario.v1, ns)
    apply_member(api, scenario.v2, ns)

    logger.info(f"Waiting for {scenario.v1.name} Ready...")
    wait_ready(api, scenario.v1.name, ns)
    logger.info(f"Waiting for {scenario.v2.name} Ready...")
    wait_ready(api, scenario.v2.name, ns)

    yield scenario, api, ns


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


@pytest.mark.traffic
@pytest.mark.llminferenceservice
@pytest.mark.cluster_cpu
class TestCanaryLifecycle:
    """Zero-downtime canary rollout under continuous traffic."""

    @pytest.mark.parametrize(
        "canary_env",
        ["service"],
        indirect=True,
        ids=["service"],
    )
    def test_canary_service_backend(self, canary_env, traffic_driver):
        """Service backend - works with any gateway."""
        self._run_canary_lifecycle(canary_env, traffic_driver)

    def test_model_name_divergence(self, test_namespace):
        """Members with different model.name form independent sub-groups.

        Each sub-group is individually ready (GroupReady=True), but the group
        is degraded (GroupDegraded=True/MemberDivergence) because members
        diverge on model identity.
        """
        api = get_api()
        ns = test_namespace

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_divergence_member(api, DIV_V1, DIV_MODEL_V1, 5, ns)
        apply_divergence_member(api, DIV_V2, DIV_MODEL_V2, 5, ns)

        wait_ready(api, DIV_V1, ns)
        wait_ready(api, DIV_V2, ns)

        # Both members should be GroupReady (each sub-group works)
        for name in [DIV_V1, DIV_V2]:
            c = wait_for_condition(api, name, ns, "GroupReady", status="True")
            assert c["status"] == "True", f"{name}: GroupReady={c['status']}"

        # Both should be GroupDegraded with MemberDivergence
        for name in [DIV_V1, DIV_V2]:
            c = wait_for_condition(
                api,
                name,
                ns,
                "GroupDegraded",
                status="True",
                reason="MemberDivergence",
            )
            assert c["status"] == "True", f"{name}: GroupDegraded={c['status']}"
            assert c["reason"] == "MemberDivergence", f"{name}: reason={c['reason']}"

        # Each member's sub-group should contain only itself
        v1_members = get_group_members(api, DIV_V1, ns)
        v2_members = get_group_members(api, DIV_V2, ns)
        assert v1_members == [DIV_V1], f"v1 group members: {v1_members}"
        assert v2_members == [DIV_V2], f"v2 group members: {v2_members}"

        logger.info("Model name divergence verified")

    def test_model_name_divergence_resolves(self, test_namespace):
        """Fixing a divergent model.name clears the degraded condition.

        Start with divergent model names, then patch v2 to match v1. Both
        members should appear in each other's group and GroupDegraded should
        clear.
        """
        api = get_api()
        ns = test_namespace

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_divergence_member(api, DIV_V1, DIV_MODEL_V1, 5, ns)
        apply_divergence_member(api, DIV_V2, DIV_MODEL_V2, 5, ns)

        wait_ready(api, DIV_V1, ns)
        wait_ready(api, DIV_V2, ns)

        # Confirm divergence is detected first
        wait_for_condition(
            api,
            DIV_V1,
            ns,
            "GroupDegraded",
            status="True",
            reason="MemberDivergence",
        )

        # Fix: patch v2's model name to match v1
        patch_model_name(api, DIV_V2, DIV_MODEL_V1, ns)
        logger.info(f"Patched {DIV_V2} model name to {DIV_MODEL_V1}")

        # Wait for ready again (model name change triggers reconcile)
        wait_ready(api, DIV_V2, ns)

        # GroupReady should still be True
        for name in [DIV_V1, DIV_V2]:
            c = wait_for_condition(api, name, ns, "GroupReady", status="True")
            assert c["status"] == "True", f"{name}: GroupReady={c['status']}"

        # GroupDegraded should be cleared (controller removes the condition
        # via ClearCondition, so it becomes absent rather than status=False)
        for name in [DIV_V1, DIV_V2]:
            wait_for_condition_absent(api, name, ns, "GroupDegraded")

        # Both members should now appear in each other's group.
        # Wait for group membership to converge (may lag behind condition update).
        for name in [DIV_V1, DIV_V2]:
            wait_for_member_count(api, name, ns, 2)
            members = get_group_members(api, name, ns)
            assert DIV_V1 in members, f"{name} missing {DIV_V1}: {members}"
            assert DIV_V2 in members, f"{name} missing {DIV_V2}: {members}"

        logger.info("Model name divergence resolution verified")

    def test_weight_without_group_rejected(self, test_namespace):
        """Webhook rejects LLMISVC with weight but no group. (spike step 11)"""
        api = get_api()
        ns = test_namespace

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())

        body = {
            "apiVersion": f"{KSERVE_GROUP}/{KSERVE_VERSION}",
            "kind": "LLMInferenceService",
            "metadata": {"name": "webhook-reject", "namespace": ns},
            "spec": {
                "model": {"name": MODEL, "uri": MODEL_URI},
                "baseRefs": [{"name": INFERENCE_SIM.name}],
                "router": {
                    "route": {
                        "weight": 5,
                        "http": {},
                    },
                },
            },
        }

        with pytest.raises(k8s_client.ApiException) as exc_info:
            api.create_namespaced_custom_object(
                KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, body
            )
        assert exc_info.value.status == 422, (
            f"Expected 422 webhook rejection, got {exc_info.value.status}"
        )
        err_msg = json.loads(exc_info.value.body).get("message", "")
        assert "weight requires group" in err_msg.lower(), (
            f"Expected 'weight requires group' in error message: {err_msg}"
        )
        logger.info("Webhook rejection verified: weight without group")

    def _run_canary_lifecycle(self, canary_env, traffic_driver):
        scenario, api, ns = canary_env
        v1 = scenario.v1.name
        v2 = scenario.v2.name

        gateway = get_gateway_base_url(api, v1, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        # Phase 1: baseline (v1=9, v2=1)
        driver.mark("baseline")
        time.sleep(20)

        # Phase 2: canary ramp (v1=7, v2=3)
        driver.mark("canary_mutation")
        patch_weight(api, v2, 3, ns)
        patch_weight(api, v1, 7, ns)
        wait_for_group_weight(api, v1, v2, 3, ns)
        wait_for_healthy_route(
            f"{gateway}/v1/completions",
            {
                "X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}",
                "Content-Type": "application/json",
            },
            {"model": MODEL, "prompt": "Hello", "max_tokens": 5},
        )
        driver.mark("canary")
        time.sleep(20)

        # Phase 3: promote (v1=0, v2=9)
        driver.mark("promote_mutation")
        patch_weight(api, v1, 0, ns)
        patch_weight(api, v2, 9, ns)
        wait_for_group_weight(api, v2, v1, 0, ns)
        wait_for_healthy_route(
            f"{gateway}/v1/completions",
            {
                "X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}",
                "Content-Type": "application/json",
            },
            {"model": MODEL, "prompt": "Hello", "max_tokens": 5},
        )
        driver.mark("promote")
        time.sleep(20)

        report = driver.stop()

        # --- Sample validity ---
        for name in ["baseline", "canary", "promote"]:
            phase = report.phase(name)
            phase.assert_min_samples(10, name)
            assert phase.achieved_rate >= 0.5, (
                f"{name}: rate collapsed to {phase.achieved_rate:.1f} rps\n"
                f"{phase.summary()}"
            )

        # --- Zero errors in stable phases. A 5s gateway convergence delay
        # after each mutation excludes transient 500s during route programming. ---
        for name in ["baseline", "canary", "promote"]:
            report.phase(name).assert_no_errors(f"stable phase: {name}")

        # --- Distribution (via inference-sim's x-inference-pod header) ---
        def is_v1(r):
            pod = r.headers.get("x-inference-pod", "")
            return pod.startswith(v1)

        def is_v2(r):
            pod = r.headers.get("x-inference-pod", "")
            return pod.startswith(v2)

        baseline = report.phase("baseline")
        v1_pct = baseline.where(is_v1).count * 100 / baseline.count
        assert 65 <= v1_pct <= 100, (
            f"Baseline v1={v1_pct:.0f}% outside [65-100%]\n{baseline.summary()}"
        )

        canary = report.phase("canary")
        v2_pct = canary.where(is_v2).count * 100 / canary.count
        assert 10 <= v2_pct <= 55, (
            f"Canary v2={v2_pct:.0f}% outside [10-55%]\n{canary.summary()}"
        )

        promote = report.phase("promote")
        v1_after = promote.where(is_v1).count
        assert v1_after <= 3, (
            f"v1 traffic after promote: {v1_after}/{promote.count}\n{promote.summary()}"
        )

        # --- Propagation delay ---
        for mutation, settled in [
            ("canary_mutation", "canary"),
            ("promote_mutation", "promote"),
        ]:
            delay = report.marks[settled] - report.marks[mutation]
            logger.info(f"Propagation {mutation} -> {settled}: {delay:.1f}s")
            assert delay < 60, f"{mutation} -> {settled} took {delay:.1f}s"

        logger.info(
            f"Canary lifecycle ({scenario.name}) complete: {report.all.summary()}"
        )

    # ------------------------------------------------------------------
    # Group lifecycle edge cases (ported from controlled-deployment spike)
    # ------------------------------------------------------------------

    def test_force_stop(self, test_namespace, traffic_driver):
        """Force-stop a member - scales to zero, stopped=true in group status,
        traffic shifts to remaining member. (spike step 8)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="stop-v1", weight=9, scheduler=False)
        v2 = MemberSpec(name="stop-v2", weight=1, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        driver.mark("before_stop")
        time.sleep(10)

        patch_stop(api, v2.name, ns)
        wait_for_member_stopped(api, v1.name, v2.name, ns, stopped=True)

        wait_for_healthy_route(
            f"{gateway}/v1/completions",
            {"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            {"model": MODEL, "prompt": "Hello", "max_tokens": 5},
        )
        driver.mark("settled")
        time.sleep(15)

        report = driver.stop()

        settled = report.phase("settled")
        settled.assert_no_errors(
            "force-stop: traffic should flow to v1 after convergence"
        )

        def is_v2(r):
            return r.headers.get("x-inference-pod", "").startswith(v2.name)

        v2_after = settled.where(is_v2).count
        assert v2_after == 0, f"v2 got {v2_after} requests after stop"

        m = get_member_status(api, v1.name, v2.name, ns)
        assert m is not None, "v2 should still be in group status"
        assert m.get("stopped") is True, f"v2 stopped={m.get('stopped')}"
        assert m.get("weight") == 1, f"v2 declared weight={m.get('weight')}, expected 1"

        # Verify workload scaled down (deleted or scaled to zero)
        apps = k8s_client.AppsV1Api()
        deadline = time.monotonic() + 60
        while time.monotonic() < deadline:
            deps = apps.list_namespaced_deployment(
                ns,
                label_selector=f"app.kubernetes.io/part-of=llminferenceservice,app.kubernetes.io/instance={v2.name}",
            )
            if not deps.items or all(d.spec.replicas == 0 for d in deps.items):
                break
            time.sleep(2)
        else:
            replicas = [d.spec.replicas for d in deps.items]
            raise AssertionError(f"v2 deployment not scaled down: replicas={replicas}")

        logger.info("Force-stop verified")

    def test_decommission(self, test_namespace, traffic_driver):
        """Delete a member from the group - remaining member stays Ready,
        traffic continues. (spike step 9)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="decom-v1", weight=9, scheduler=False)
        v2 = MemberSpec(name="decom-v2", weight=1, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        driver.mark("before_delete")
        time.sleep(10)

        driver.mark("delete")
        delete_member(api, v2.name, ns, wait=True)
        wait_for_member_count(api, v1.name, ns, 1)

        driver.mark("settled")
        time.sleep(15)

        report = driver.stop()

        # Intentionally strict: zero errors across the entire run including
        # the delete-to-route-reprogramming window. This is the zero-downtime
        # deletion claim. If it flakes, that's a real gateway routing gap
        # worth investigating rather than masking with tolerance.
        report.all.assert_no_errors("decommission: zero errors including transition")

        members = get_group_members(api, v1.name, ns)
        assert v2.name not in members, f"v2 still in group: {members}"

        wait_for_condition(api, v1.name, ns, "Ready", status="True")

        logger.info("Decommission verified")

    def test_leave_group(self, test_namespace):
        """Remove group+weight from a member - leaves the group. (spike step 14)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="leave-v1", weight=5, scheduler=False)
        v2 = MemberSpec(name="leave-v2", weight=5, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)
        wait_for_member_count(api, v1.name, ns, 2)

        # Remove group and weight from v2
        api.patch_namespaced_custom_object(
            KSERVE_GROUP,
            KSERVE_VERSION,
            ns,
            KSERVE_PLURAL,
            v2.name,
            {"spec": {"router": {"route": {"group": None, "weight": None}}}},
        )
        logger.info(f"Removed group+weight from {v2.name}")

        wait_for_member_count(api, v1.name, ns, 1)

        members = get_group_members(api, v1.name, ns)
        assert v2.name not in members, f"v2 still in group: {members}"

        # v1 should still be Ready
        wait_for_condition(api, v1.name, ns, "Ready", status="True")

        # v2's routing-group label should be removed
        obj = api.get_namespaced_custom_object(
            KSERVE_GROUP, KSERVE_VERSION, ns, KSERVE_PLURAL, v2.name
        )
        labels = obj.get("metadata", {}).get("labels", {})
        assert "serving.kserve.io/routing-group" not in labels, (
            f"routing-group label still present: {labels}"
        )

        logger.info("Leave group verified")

    def test_three_member_group(self, test_namespace, traffic_driver):
        """Three members in a group all receive traffic. (spike step 15)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="tri-v1", weight=5, scheduler=False)
        v2 = MemberSpec(name="tri-v2", weight=3, scheduler=False)
        v3 = MemberSpec(name="tri-v3", weight=2, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        for m in [v1, v2, v3]:
            apply_member(api, m, ns)

        for m in [v1, v2, v3]:
            wait_ready(api, m.name, ns)
        wait_for_member_count(api, v1.name, ns, 3)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        time.sleep(30)
        report = driver.stop()

        report.all.assert_no_errors("three-member group")

        for m in [v1, v2, v3]:

            def match(r, prefix=m.name):
                return r.headers.get("x-inference-pod", "").startswith(prefix)

            count = report.all.where(match).count
            assert count > 0, f"{m.name} received no traffic ({report.all.count} total)"

        logger.info("Three-member group verified")

    def test_late_join(self, test_namespace, traffic_driver):
        """A member joins an already-running group. (spike step 16)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="late-v1", weight=9, scheduler=False)
        v2 = MemberSpec(name="late-v2", weight=1, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)

        wait_ready(api, v1.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        driver.mark("v1_only")
        time.sleep(10)

        # Late-join v2
        apply_member(api, v2, ns)
        wait_ready(api, v2.name, ns)
        wait_for_member_count(api, v1.name, ns, 2)
        wait_for_healthy_route(
            f"{gateway}/v1/completions",
            {"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            {"model": MODEL, "prompt": "Hello", "max_tokens": 5},
        )

        driver.mark("both")
        time.sleep(20)

        report = driver.stop()
        report.all.assert_no_errors("late-join")

        both = report.phase("both")

        def is_v2(r):
            return r.headers.get("x-inference-pod", "").startswith(v2.name)

        v2_count = both.where(is_v2).count
        assert v2_count > 0, f"v2 received no traffic after join ({both.count} total)"

        logger.info("Late-join verified")

    def test_delete_at_nonzero_weight(self, test_namespace, traffic_driver):
        """Delete a member with weight>0 - no route breakage. (spike step 17)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="delw-v1", weight=5, scheduler=False)
        v2 = MemberSpec(name="delw-v2", weight=5, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        time.sleep(10)
        driver.mark("delete")

        delete_member(api, v2.name, ns, wait=True)
        wait_for_member_count(api, v1.name, ns, 1)

        driver.mark("settled")
        time.sleep(15)

        report = driver.stop()

        report.all.assert_no_errors(
            "delete at weight>0: zero errors including transition"
        )

        logger.info("Delete at nonzero weight verified")

    def test_rollback(self, test_namespace, traffic_driver):
        """Promote v2 then rollback to v1 - traffic returns. (spike step 7)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="roll-v1", weight=9, scheduler=False)
        v2 = MemberSpec(name="roll-v2", weight=1, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        route_headers = {"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"}
        route_payload = {"model": MODEL, "prompt": "Hello", "max_tokens": 5}

        # Promote v2
        driver.mark("promote_mutation")
        patch_weight(api, v1.name, 0, ns)
        patch_weight(api, v2.name, 9, ns)
        wait_for_group_weight(api, v2.name, v1.name, 0, ns)
        wait_for_healthy_route(
            f"{gateway}/v1/completions", route_headers, route_payload
        )
        driver.mark("promoted")
        time.sleep(15)

        # Rollback to v1
        driver.mark("rollback_mutation")
        patch_weight(api, v1.name, 9, ns)
        patch_weight(api, v2.name, 0, ns)
        wait_for_group_weight(api, v1.name, v2.name, 0, ns)
        wait_for_healthy_route(
            f"{gateway}/v1/completions", route_headers, route_payload
        )
        driver.mark("settled")
        time.sleep(15)

        report = driver.stop()

        for phase in ["promoted", "settled"]:
            report.phase(phase).assert_no_errors(f"rollback: stable phase {phase}")

        settled = report.phase("settled")

        def is_v2(r):
            return r.headers.get("x-inference-pod", "").startswith(v2.name)

        v2_count = settled.where(is_v2).count
        assert v2_count <= 3, f"v2 got {v2_count} requests after rollback"

        logger.info("Rollback verified")

    def test_force_stop_route_owner(self, test_namespace, traffic_driver):
        """Force-stop the route owner - traffic shifts to other member. (spike step 18)"""
        api = get_api()
        ns = test_namespace

        v1 = MemberSpec(name="owner-v1", weight=5, scheduler=False)
        v2 = MemberSpec(name="owner-v2", weight=5, scheduler=False)

        apply_config(api, INFERENCE_SIM.name, ns, INFERENCE_SIM.to_spec())
        apply_member(api, v1, ns)
        apply_member(api, v2, ns)

        wait_ready(api, v1.name, ns)
        wait_ready(api, v2.name, ns)

        gateway = get_gateway_base_url(api, v1.name, ns)
        driver = traffic_driver(
            url=f"{gateway}/v1/completions",
            headers={"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            payload={"model": MODEL, "prompt": "Hello", "max_tokens": 5},
            rate=2,
            timeout=15.0,
            warmup=True,
        )

        time.sleep(10)
        driver.mark("before_stop")

        # v1 is the route owner (created first). Force-stop it.
        patch_stop(api, v1.name, ns)
        wait_for_member_stopped(api, v2.name, v1.name, ns, stopped=True)

        # Route ownership handoff: v1's route is deleted, v2 creates a new one.
        wait_for_healthy_route(
            f"{gateway}/v1/completions",
            {"X-Gateway-Model-Name": f"publishers/{ns}/models/{MODEL}"},
            {"model": MODEL, "prompt": "Hello", "max_tokens": 5},
        )
        driver.mark("settled")
        time.sleep(15)

        report = driver.stop()

        settled = report.phase("settled")
        settled.assert_no_errors("force-stop route owner: traffic should shift to v2")

        def is_v1(r):
            return r.headers.get("x-inference-pod", "").startswith(v1.name)

        v1_after = settled.where(is_v1).count
        assert v1_after == 0, f"v1 got {v1_after} requests after stop"

        logger.info("Force-stop route owner verified")
