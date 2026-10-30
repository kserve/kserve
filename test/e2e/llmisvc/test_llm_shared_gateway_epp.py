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

"""Each service on the shared gateway is scheduled by its own EPP.

Every LLMInferenceService gets the same HTTPRoute rule names, and Istio before
1.29.9, 1.30.6 and 1.31.2 keys the InferencePool endpoint picker by bare rule
name, so on a shared gateway one service's EPP can schedule another service's
requests. Requests can still succeed and every status stays green; only the
EPPs' request counters show it. The same check also catches a dropped picker or
a bypassed EPP: the service's own EPP then counts nothing.
"""

import os
import re
from collections import Counter

import pytest
from kserve import KServeClient
from kubernetes import client

from ..common.http_retry import post_with_retry
from .diagnostic import collect_diagnostics
from .fixtures import (
    apply_epp_tls_workaround,
    has_istio,
    inject_k8s_proxy,
)
from .logging import log_execution, logger
from .test_llm_inference_service import (
    TestCase,
    completions_payload,
    create_llmisvc,
    get_llm_service_url,
    get_managed_httproutes,
    maybe_delete_llmisvc,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)

# Any other service on the shared gateway has the same rule names and would take
# part in the collision, so this runs alone: in the serial step on envoy-gateway
# and in the Istio traffic job.
pytestmark = [
    pytest.mark.llmisvc_serial,
    pytest.mark.traffic,
    pytest.mark.llmd_simulator,
]

REQUESTS_PER_SERVICE = 10
EPP_METRICS_PORT = 9090

_REQUEST_TOTAL_SAMPLE = re.compile(
    r"^llm_d_epp_request_total(?:\{[^}]*\})?\s+(\S+)", re.MULTILINE
)


def epp_request_total(core_api: client.CoreV1Api, namespace: str, name: str) -> float:
    """Total requests the service's EPP has scheduled, over all label sets and
    scheduler pods (HA replicas).

    A missing pod raises rather than reading as zero. A missing series reads as
    zero: the counter has no series until its first request, and the EPP that
    loses its traffic to another service never gets one. The test catches a
    renamed counter through the own-EPP check instead.
    """
    pods = core_api.list_namespaced_pod(
        namespace,
        label_selector=(
            "app.kubernetes.io/component=llminferenceservice-router-scheduler,"
            f"app.kubernetes.io/name={name}"
        ),
    ).items
    running = [p.metadata.name for p in pods if p.status.phase == "Running"]
    if not running:
        raise AssertionError(
            f"no running scheduler pod for {namespace}/{name} "
            f"(pods: {[p.metadata.name for p in pods]})"
        )

    total = 0.0
    for pod in running:
        resp = core_api.connect_get_namespaced_pod_proxy_with_path(
            f"{pod}:{EPP_METRICS_PORT}",
            namespace,
            "metrics",
            _preload_content=False,
        )
        total += sum(
            float(v) for v in _REQUEST_TOTAL_SAMPLE.findall(resp.data.decode())
        )
    return total


def _two_services(*extra_refs: str) -> TestCase:
    """Service A plus peer B with a different model, both with extra_refs."""
    base = [
        "router-managed",
        "scheduler-prometheus-scrape",
        "workload-llmd-simulator",
        *extra_refs,
    ]
    return TestCase(
        base_refs=base,
        prompt="KServe is a",
        payload_formatter=completions_payload,
        peers=[
            TestCase(
                base_refs=[*base, "model-qwen2.5-0.5b"],
                prompt="KServe is a",
                payload_formatter=completions_payload,
            ),
        ],
    )


@pytest.mark.parametrize(
    "test_case, unique_names",
    [
        pytest.param(
            _two_services(),
            False,
            id="preset-rule-names",
            marks=pytest.mark.cluster_cpu,
        ),
        # The opt-in escape hatch for gateways whose Istio predates the fix.
        pytest.param(
            _two_services("route-rule-naming-unique"),
            True,
            id="unique-rule-names",
            marks=pytest.mark.cluster_cpu,
        ),
    ],
    indirect=["test_case"],
)
@log_execution
def test_each_service_scheduled_by_own_epp(test_case: TestCase, unique_names: bool):
    """Requests to each service raise its own EPP's request counter by at least
    the number sent and leave the other service's EPP counter unchanged."""
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )
    core_api = client.CoreV1Api()
    services = [test_case, *test_case.peers]
    names = [tc.llm_service.metadata.name for tc in services]
    namespace = test_case.namespace

    test_failed = False
    try:
        # Both routes must be on the gateway before either is measured.
        for tc in services:
            create_llmisvc(kserve_client, tc.llm_service)
        for tc in services:
            wait_for_llm_isvc_ready(kserve_client, tc.llm_service, tc.wait_timeout)
        # A fixed gateway passes either way, so check the names are what this
        # param is about: shared preset names are the collision's precondition.
        rule_names = [
            {
                rule["name"]
                for route in get_managed_httproutes(kserve_client, name, namespace)
                for rule in route["spec"]["rules"]
                if rule.get("name")
            }
            for name in names
        ]
        if unique_names:
            assert rule_names[0].isdisjoint(rule_names[1]), (
                f"unique naming left shared rule names: {rule_names}"
            )
        else:
            assert rule_names[0] == rule_names[1], (
                f"preset rule names differ between the services: {rule_names}"
            )

        if has_istio():
            apply_epp_tls_workaround(namespace)
        for tc in services:
            wait_for_model_response(kserve_client, tc, tc.wait_timeout)

        violations = []
        for tc in services:
            own = tc.llm_service.metadata.name
            url = get_llm_service_url(kserve_client, tc.llm_service) + tc.endpoint

            before = {n: epp_request_total(core_api, namespace, n) for n in names}
            statuses = Counter(
                post_with_retry(
                    url,
                    json_data=completions_payload(tc),
                    timeout=tc.response_timeout,
                    # A retry would count again on an EPP and hide the status.
                    retry_status_codes=(),
                ).status_code
                for _ in range(REQUESTS_PER_SERVICE)
            )
            after = {n: epp_request_total(core_api, namespace, n) for n in names}

            delta = {n: after[n] - before[n] for n in names}
            logger.info(
                "%d requests to %s: statuses %s, EPP counter before %s, after %s",
                REQUESTS_PER_SERVICE,
                own,
                dict(statuses),
                before,
                after,
            )
            # A lower bound only: a gateway may call the EPP more than once per
            # request (Envoy Gateway counted 16 per request here), so the
            # other-EPP check is the exact one.
            if delta[own] < REQUESTS_PER_SERVICE:
                violations.append(
                    f"{own}'s EPP counted {delta[own]:g} of its own "
                    f"{REQUESTS_PER_SERVICE} requests (statuses {dict(statuses)})"
                )
            for other in names:
                if other != own and delta[other] != 0:
                    violations.append(
                        f"{other}'s EPP counted {delta[other]:g} of {own}'s "
                        f"{REQUESTS_PER_SERVICE} requests"
                    )

        assert not violations, "requests not scheduled by their own EPP: " + "; ".join(
            violations
        )
    except Exception:
        test_failed = True
        for name in names:
            collect_diagnostics(
                name, namespace, kserve_client=kserve_client, log=logger.info
            )
        raise
    finally:
        for tc in services:
            maybe_delete_llmisvc(kserve_client, tc.llm_service, test_failed)
