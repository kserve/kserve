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

"""E2E tests for prefill/decode services on the DisaggregatedSet backend.

These tests need the ``disaggregatedSet`` llmisvc feature gate, which is
cluster-wide: with it on, every prefill/decode service in the cluster runs on
a DisaggregatedSet. They are therefore selected only by the
``llmisvc_disaggregated_set`` marker, which only the dedicated CI job runs,
after it enables the gate with
``test/scripts/gh-actions/enable-llmisvc-feature-gates.sh disaggregatedSet``.
No test here changes the gate.

The KEDA test also needs KEDA and Prometheus, which that job installs.
"""

import contextlib
import os
import threading

import pytest
from kserve import KServeClient
from kubernetes import client

from .autoscaling_helpers import (
    KEDA_GROUP,
    KEDA_PLURAL,
    KEDA_VERSION,
    _get_custom_resource,
    assert_scaled_object_condition,
    scaled_object_name,
    send_load,
)
from .diagnostic import collect_diagnostics
from .disaggregated_set_helpers import (
    DISAGGREGATED_SET_ANNOTATION,
    DISAGGREGATED_SET_USED,
    DS_GROUP,
    DS_KIND,
    DS_REVISION_HASH_VERSION_ANNOTATION,
    DS_ROLE_SCALER_KIND,
    DS_ROLE_SCALER_PLURAL,
    DS_VERSION,
    MIGRATING_FROM_REASON,
    MIGRATING_TO_REASON,
    ROLE_DECODE,
    ROLE_PREFILL,
    ROLES,
    assert_ds_owned_by,
    assert_status_workloads_reference_ds,
    dump_disaggregated_set,
    get_condition,
    get_ds,
    list_role_pods,
    patch_llmisvc_spec,
    require_disaggregated_set_gate,
    role_lws_names,
    role_scaler_name,
    wait_for_condition,
    wait_for_ds,
    wait_for_ds_deleted,
    wait_for_event,
    wait_for_pd_deployments,
    wait_for_role_pods,
)
from .fixtures import generate_test_id, inject_k8s_proxy
from .logging import log_execution, logger
from .test_llm_inference_service import (
    TestCase,
    assert_200_with_choices,
    assert_address_origins,
    create_llmisvc,
    get_llm_service_url,
    maybe_delete_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)
from .test_llm_inference_service_stop import (
    restart_llmisvc,
    stop_llmisvc,
    wait_for_llm_isvc_stopped,
)

pytestmark = [
    pytest.mark.llmisvc_disaggregated_set,
    pytest.mark.cluster_cpu,
    pytest.mark.cluster_single_node,
]

SIMULATOR_PD = ["router-managed", "workload-llmd-simulator-pd"]

# Rollouts and migrations replace pods; the simulator starts in seconds, but
# the LWS operator runs with small CPU limits in CI.
POD_TIMEOUT = 600


def _new_kserve_client() -> KServeClient:
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


@pytest.fixture(autouse=True)
def _require_gate():
    inject_k8s_proxy()
    require_disaggregated_set_gate(_new_kserve_client())


@contextlib.contextmanager
def _service(kserve_client, test_case: TestCase):
    """Create the service, wait for it to be Ready, and clean it up afterwards.

    On failure, dumps the usual diagnostics plus the DisaggregatedSet tree.
    """
    llm_isvc = test_case.llm_service
    failed = False
    try:
        create_llmisvc(kserve_client, llm_isvc)
        wait_for_llm_isvc_ready(kserve_client, llm_isvc, test_case.wait_timeout)
        yield llm_isvc
    except BaseException:
        failed = True
        logger.error(
            "Test failed for %s, collecting diagnostics", llm_isvc.metadata.name
        )
        dump_disaggregated_set(
            llm_isvc.metadata.name, llm_isvc.metadata.namespace, log=logger.info
        )
        collect_diagnostics(
            llm_isvc.metadata.name,
            llm_isvc.metadata.namespace,
            kserve_client=kserve_client,
            log=logger.info,
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, llm_isvc, failed)


def _assert_serving(kserve_client, test_case: TestCase):
    wait_for_llm_isvc_ready(
        kserve_client, test_case.llm_service, test_case.wait_timeout
    )
    wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)


def _assert_on_disaggregated_set(kserve_client, llm_isvc):
    """Assert the service runs on its DisaggregatedSet, not on Deployments."""
    name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
    wait_for_condition(kserve_client, llm_isvc, DISAGGREGATED_SET_USED, "True")
    ds = wait_for_ds(name, ns)
    assert_ds_owned_by(ds, llm_isvc)
    role_names = sorted(r["name"] for r in ds["spec"]["roles"])
    assert role_names == sorted(ROLES), f"DisaggregatedSet roles: {role_names}"
    wait_for_pd_deployments(name, ns, exist=False)
    wait_for(
        lambda: assert_status_workloads_reference_ds(kserve_client, llm_isvc),
        timeout=120,
        interval=2.0,
    )
    return ds


def _role_pods_by_role(name, ns):
    return {role: list_role_pods(name, ns, role) for role in ROLES}


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=["router-managed", "workload-pd-cpu", "model-fb-opt-125m"],
                prompt="KServe is a",
                service_name="ds-vllm",
                response_assertion=assert_200_with_choices,
            ),
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_pd_vllm(test_case: TestCase):
    """A P/D vLLM service runs on a DisaggregatedSet and serves completions."""
    kserve_client = _new_kserve_client()
    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
        _assert_on_disaggregated_set(kserve_client, llm_isvc)

        for role in ROLES:
            pods = wait_for_role_pods(name, ns, role, count=1, timeout=POD_TIMEOUT)
            assert pods[0].labels.get("llm-d.ai/role") == role, (
                f"{role} pod {pods[0].name} has llm-d.ai/role="
                f"{pods[0].labels.get('llm-d.ai/role')}"
            )

        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)
        assert_address_origins(kserve_client, llm_isvc)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=SIMULATOR_PD,
                prompt="KServe is a",
                service_name="ds-scale",
            ),
            marks=[pytest.mark.llmd_simulator],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_scale_keeps_pods(test_case: TestCase):
    """Scaling both roles adds pods without replacing the running ones."""
    kserve_client = _new_kserve_client()
    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
        _assert_on_disaggregated_set(kserve_client, llm_isvc)

        before = {
            role: wait_for_role_pods(name, ns, role, count=1, timeout=POD_TIMEOUT)
            for role in ROLES
        }
        ds = get_ds(name, ns)
        hash_version = (ds["metadata"].get("annotations") or {}).get(
            DS_REVISION_HASH_VERSION_ANNOTATION
        )
        assert hash_version, (
            f"{DS_REVISION_HASH_VERSION_ANNOTATION} not set: "
            f"{ds['metadata'].get('annotations')}"
        )
        lws_before = {role: role_lws_names(name, ns, role) for role in ROLES}

        # Static roles must be all zero or all non-zero, so scale them together.
        patch_llmisvc_spec(
            kserve_client, llm_isvc, {"replicas": 2, "prefill": {"replicas": 2}}
        )

        for role in ROLES:
            after = wait_for_role_pods(name, ns, role, count=2, timeout=POD_TIMEOUT)
            kept = {p.uid for p in before[role]} & {p.uid for p in after}
            assert kept == {p.uid for p in before[role]}, (
                f"{role} pods were replaced on scale-up: before "
                f"{[p.name for p in before[role]]}, after {[p.name for p in after]}"
            )
            assert role_lws_names(name, ns, role) == lws_before[role], (
                f"{role} LeaderWorkerSets changed on scale-up: before "
                f"{lws_before[role]}, after {role_lws_names(name, ns, role)}"
            )

        ds = get_ds(name, ns)
        hash_version_after = (ds["metadata"].get("annotations") or {}).get(
            DS_REVISION_HASH_VERSION_ANNOTATION
        )
        assert hash_version_after == hash_version, (
            f"{DS_REVISION_HASH_VERSION_ANNOTATION} changed from {hash_version!r}: "
            f"{ds['metadata'].get('annotations')}"
        )

        _assert_serving(kserve_client, test_case)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=SIMULATOR_PD,
                prompt="KServe is a",
                service_name="ds-rollout",
            ),
            marks=[pytest.mark.llmd_simulator],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_template_change_rolls_out(test_case: TestCase):
    """A decode template change moves both roles to a new revision."""
    kserve_client = _new_kserve_client()
    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
        _assert_on_disaggregated_set(kserve_client, llm_isvc)

        before = {
            role: wait_for_role_pods(name, ns, role, count=1, timeout=POD_TIMEOUT)
            for role in ROLES
        }
        old_revision = before[ROLE_DECODE][0].revision
        assert old_revision, f"decode pod has no revision label: {before[ROLE_DECODE]}"
        assert before[ROLE_PREFILL][0].revision == old_revision, (
            f"roles start on different revisions: {before}"
        )
        lws_before = {role: role_lws_names(name, ns, role) for role in ROLES}

        patch_llmisvc_spec(
            kserve_client,
            llm_isvc,
            {
                "template": {
                    "containers": [
                        {
                            "name": "main",
                            "env": [{"name": "E2E_DS_ROLLOUT", "value": "v2"}],
                        }
                    ]
                }
            },
        )

        for role in ROLES:
            after = wait_for_role_pods(
                name,
                ns,
                role,
                count=1,
                exclude_revision=old_revision,
                timeout=POD_TIMEOUT,
            )
            assert after[0].uid not in {p.uid for p in before[role]}

            def _old_lws_gone(role=role):
                remaining = set(role_lws_names(name, ns, role)) & set(lws_before[role])
                assert not remaining, f"old {role} LeaderWorkerSets remain: {remaining}"

            wait_for(_old_lws_gone, timeout=POD_TIMEOUT, interval=5.0)

        _assert_serving(kserve_client, test_case)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=SIMULATOR_PD,
                prompt="KServe is a",
                service_name="ds-migrate",
            ),
            marks=[pytest.mark.llmd_simulator],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_migration_round_trip(test_case: TestCase):
    """The per-service annotation moves a service onto and off a DisaggregatedSet."""
    kserve_client = _new_kserve_client()
    # Opt out first; the decode preset requests the DisaggregatedSet by default.
    test_case.llm_service.spec["annotations"] = {DISAGGREGATED_SET_ANNOTATION: "false"}

    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace

        wait_for_pd_deployments(name, ns, exist=True)
        assert get_ds(name, ns) is None, "DisaggregatedSet exists while opted out"
        assert get_condition(kserve_client, llm_isvc, DISAGGREGATED_SET_USED) is None
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)

        patch_llmisvc_spec(
            kserve_client,
            llm_isvc,
            {"annotations": {DISAGGREGATED_SET_ANNOTATION: "true"}},
        )
        wait_for_event(llm_isvc, MIGRATING_TO_REASON)
        _assert_on_disaggregated_set(kserve_client, llm_isvc)
        for role in ROLES:
            wait_for_role_pods(name, ns, role, count=1, timeout=POD_TIMEOUT)
        _assert_serving(kserve_client, test_case)

        patch_llmisvc_spec(
            kserve_client,
            llm_isvc,
            {"annotations": {DISAGGREGATED_SET_ANNOTATION: "false"}},
        )
        wait_for_event(llm_isvc, MIGRATING_FROM_REASON)
        wait_for_ds_deleted(name, ns)
        wait_for_pd_deployments(name, ns, exist=True)

        def _condition_cleared():
            cond = get_condition(kserve_client, llm_isvc, DISAGGREGATED_SET_USED)
            assert cond is None, f"{DISAGGREGATED_SET_USED} still set: {cond}"

        wait_for(_condition_cleared, timeout=120, interval=2.0)
        _assert_serving(kserve_client, test_case)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=SIMULATOR_PD,
                prompt="KServe is a",
                service_name="ds-stop",
            ),
            marks=[pytest.mark.llmd_simulator],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_stop_and_restart(test_case: TestCase):
    """Stopping deletes the DisaggregatedSet; restarting brings it back."""
    kserve_client = _new_kserve_client()
    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
        _assert_on_disaggregated_set(kserve_client, llm_isvc)

        stop_llmisvc(kserve_client, llm_isvc)
        wait_for_llm_isvc_stopped(kserve_client, llm_isvc)
        wait_for_ds_deleted(name, ns)

        def _no_role_pods():
            pods = _role_pods_by_role(name, ns)
            assert not any(pods.values()), f"role pods remain after stop: {pods}"

        wait_for(_no_role_pods, timeout=POD_TIMEOUT, interval=5.0)

        restart_llmisvc(kserve_client, llm_isvc)
        _assert_on_disaggregated_set(kserve_client, llm_isvc)
        for role in ROLES:
            wait_for_role_pods(name, ns, role, count=1, timeout=POD_TIMEOUT)
        _assert_serving(kserve_client, test_case)


@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "prometheus-scrape-pd",
                    "workload-llmd-simulator-pd",
                    "scaling-decode-ds-keda",
                    "scaling-prefill-ds-keda",
                ],
                prompt="KServe is a",
                service_name="ds-keda",
            ),
            marks=[pytest.mark.llmd_simulator],
        ),
    ],
    indirect=["test_case"],
    ids=generate_test_id,
)
@log_execution
def test_disaggregated_set_keda_scales_roles(test_case: TestCase):
    """KEDA scales each DisaggregatedSet role through its role scaler."""
    kserve_client = _new_kserve_client()
    with _service(kserve_client, test_case) as llm_isvc:
        name, ns = llm_isvc.metadata.name, llm_isvc.metadata.namespace
        ds = _assert_on_disaggregated_set(kserve_client, llm_isvc)

        for role_spec in ds["spec"]["roles"]:
            mode = (role_spec.get("scaling") or {}).get("mode")
            assert mode == "External", (
                f"{role_spec['name']} role scaling mode is {mode}, want External"
            )

        for role, prefill in ((ROLE_DECODE, False), (ROLE_PREFILL, True)):
            so_name = scaled_object_name(name, prefill=prefill)
            scaled_object = _get_custom_resource(
                KEDA_GROUP, KEDA_VERSION, KEDA_PLURAL, so_name, ns
            )
            assert scaled_object is not None, f"ScaledObject {so_name} not found"
            target = scaled_object["spec"]["scaleTargetRef"]
            assert target.get("kind") == DS_ROLE_SCALER_KIND, target
            assert target.get("name") == role_scaler_name(name, role), target
            assert target.get("apiVersion") == f"{DS_GROUP}/{DS_VERSION}", target

            scaler = _get_custom_resource(
                DS_GROUP,
                DS_VERSION,
                DS_ROLE_SCALER_PLURAL,
                role_scaler_name(name, role),
                ns,
            )
            assert scaler is not None, (
                f"{DS_ROLE_SCALER_KIND} {role_scaler_name(name, role)} not found"
            )
            owners = scaler["metadata"].get("ownerReferences", []) or []
            assert any(
                o.get("kind") == DS_KIND and o.get("name") == ds["metadata"]["name"]
                for o in owners
            ), f"{DS_ROLE_SCALER_KIND} owners: {owners}"

        wait_for_condition(kserve_client, llm_isvc, "ScalingReady", "True")
        wait_for_condition(kserve_client, llm_isvc, "PrefillScalingReady", "True")

        service_url = get_llm_service_url(kserve_client, llm_isvc)
        # vllm:num_requests_running is an instantaneous gauge, so the load has
        # to be in flight while the ScaledObjects and pod counts are checked.
        load_thread = threading.Thread(
            target=send_load,
            args=(service_url, test_case.model_name),
            kwargs=dict(concurrency=10, duration_seconds=120, tolerate_failures=True),
            daemon=True,
        )
        load_thread.start()
        try:
            for prefill in (False, True):
                assert_scaled_object_condition(
                    name,
                    namespace=ns,
                    condition_type="Active",
                    expected_status="True",
                    prefill=prefill,
                    timeout=90,
                )

            for role in ROLES:

                def _scaled_up(role=role):
                    pods = list_role_pods(name, ns, role)
                    ready = [p for p in pods if p.revision and p.ready]
                    assert len(ready) >= 2, (
                        f"{role} role has {len(ready)} Ready pods, want at least 2: "
                        f"{[(p.name, p.revision, p.ready) for p in pods]}"
                    )

                wait_for(_scaled_up, timeout=120, interval=5.0)
        finally:
            load_thread.join(timeout=150)
