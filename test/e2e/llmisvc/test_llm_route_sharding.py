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

"""Inline routes sharded across several managed HTTPRoutes, on a real gateway.

Two behaviors that envtest cannot show, because they depend on how the
gateway orders rules across HTTPRoutes:

* boundary tie: two rules that tie on precedence and do different things sit
  at the shard boundary; they must land in one HTTPRoute so list order still
  decides, and the gateway must keep serving the first one.
* resharding under traffic: an edit that moves the serving rule to another
  shard, and back, must not drop a request in between.

The rules are built from the rule the controller renders for the service, so
the backend and any rewrite stay whatever the installed presets produce.
"""

import copy
import os

import pytest
from kserve import KServeClient, constants
from kubernetes import client

from .diagnostic import collect_diagnostics
from .fixtures import inject_k8s_proxy
from .logging import log_execution
from .test_llm_inference_service import (
    KSERVE_PLURAL_LLMINFERENCESERVICE,
    TestCase,
    create_llmisvc,
    get_llm_service_url,
    get_managed_httproutes,
    maybe_delete_llmisvc,
    wait_for,
    wait_for_llm_isvc_ready,
)
from ..common.http_retry import post_with_retry

MAX_SHARDS_ANNOTATION = "serving.kserve.io/httproute-max-shards"
RULE_HEADER = "x-kserve-e2e-rule"
MODEL = "facebook/opt-125m"
BASE_REFS = [
    "router-with-managed-route",
    "model-fb-opt-125m",
    "workload-llmd-simulator",
]
# One HTTPRoute holds at most 16 rules (Gateway API v1.5).
RULES_PER_ROUTE = 16


def _kserve_client():
    inject_k8s_proxy()
    return KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )


def _rendered_route(kserve_client, llm_service):
    """The single managed HTTPRoute the presets render for the service."""

    def one_route():
        routes = get_managed_httproutes(
            kserve_client, llm_service.metadata.name, llm_service.metadata.namespace
        )
        assert len(routes) == 1, f"expected one managed HTTPRoute, got {len(routes)}"
        assert routes[0]["spec"].get("rules"), "managed HTTPRoute has no rules yet"
        return routes[0]

    return wait_for(one_route, timeout=300, interval=5)


def _filler(template, index):
    """A rule on its own path that no test request hits.

    The path sits one segment below the serving rule's path. The controller
    advertises the shallowest service-backed path as the status URL, so a
    top-level filler path such as /kserve-e2e-filler-00 would become the URL
    the test sends traffic to.
    """
    rule = copy.deepcopy(template)
    rule.pop("name", None)
    base = "/"
    for match in template.get("matches") or []:
        path = match.get("path") or {}
        if path.get("type", "PathPrefix") == "PathPrefix" and path.get("value"):
            base = path["value"]
            break
    rule["matches"] = [
        {
            "path": {
                "type": "PathPrefix",
                "value": f"{base.rstrip('/')}/kserve-e2e-filler-{index:02d}",
            }
        }
    ]
    return rule


def _service_url(kserve_client, llm_service):
    """The completions URL, read before any inline rules are set.

    Once the test owns the route spec, the advertised URL follows whatever
    paths the inline rules carry, so it is captured from the rendered route.
    """
    return get_llm_service_url(kserve_client, llm_service) + "/v1/completions"


def _tagged(template, tag, trailing_slash=None):
    """A copy of the serving rule that adds RULE_HEADER=tag to responses.

    With trailing_slash set, every PathPrefix value other than "/" gets (True)
    or loses (False) its trailing slash, which ties with the original on
    Istio and Envoy Gateway and in the controller's precedence grouping.
    """
    rule = copy.deepcopy(template)
    rule.pop("name", None)
    for match in rule.get("matches") or []:
        path = match.get("path")
        if (
            trailing_slash is None
            or not path
            or path.get("type", "PathPrefix") != "PathPrefix"
        ):
            continue
        value = path.get("value", "/")
        if value != "/":
            value = value.rstrip("/")
            path["value"] = value + "/" if trailing_slash else value
    filters = rule.setdefault("filters", [])
    modifier = next(
        (f for f in filters if f.get("type") == "ResponseHeaderModifier"), None
    )
    if modifier is None:
        modifier = {"type": "ResponseHeaderModifier", "responseHeaderModifier": {}}
        filters.append(modifier)
    modifier["responseHeaderModifier"].setdefault("set", []).append(
        {"name": RULE_HEADER, "value": tag}
    )
    return rule


def _set_inline_rules(kserve_client, llm_service, route, rules):
    body = {
        "spec": {
            "router": {
                "route": {
                    "http": {
                        "spec": {
                            "parentRefs": route["spec"].get("parentRefs", []),
                            "rules": rules,
                        }
                    }
                }
            }
        }
    }
    kserve_client.api_instance.patch_namespaced_custom_object(
        constants.KSERVE_GROUP,
        llm_service.api_version.split("/")[1],
        llm_service.metadata.namespace,
        KSERVE_PLURAL_LLMINFERENCESERVICE,
        llm_service.metadata.name,
        body,
    )


def _wait_for_shards(kserve_client, llm_service, count, check=None):
    def shards():
        routes = get_managed_httproutes(
            kserve_client, llm_service.metadata.name, llm_service.metadata.namespace
        )
        assert len(routes) == count, (
            f"expected {count} managed HTTPRoutes, got {len(routes)}"
        )
        if check:
            check(routes)
        return routes

    return wait_for(shards, timeout=300, interval=3)


def _completion(url):
    return post_with_retry(
        url,
        json_data={"model": MODEL, "prompt": "hi", "max_tokens": 1},
        timeout=30,
    )


def _with_service(test_case, annotation, body):
    kserve_client = _kserve_client()
    llm_service = test_case.llm_service
    llm_service.spec.setdefault("annotations", {})[MAX_SHARDS_ANNOTATION] = annotation
    test_failed = False
    try:
        create_llmisvc(kserve_client, llm_service)
        wait_for_llm_isvc_ready(kserve_client, llm_service)
        body(kserve_client, llm_service)
    except Exception:
        test_failed = True
        collect_diagnostics(
            llm_service.metadata.name,
            llm_service.metadata.namespace,
            kserve_client=kserve_client,
        )
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, llm_service, test_failed)


@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@pytest.mark.llmd_simulator
@pytest.mark.parametrize("test_case", [TestCase(base_refs=BASE_REFS)], indirect=True)
@log_execution
def test_route_sharding_boundary_tie(test_case):
    """Rules that tie at the shard boundary stay in one HTTPRoute, first wins."""

    def body(kserve_client, llm_service):
        route = _rendered_route(kserve_client, llm_service)
        url = _service_url(kserve_client, llm_service)
        template = route["spec"]["rules"][0]
        fillers = [_filler(template, i) for i in range(RULES_PER_ROUTE - 1)]
        # Rules 15 and 16: plain packing would put them in different shards.
        first = _tagged(template, "first", trailing_slash=False)
        second = _tagged(template, "second", trailing_slash=True)
        _set_inline_rules(kserve_client, llm_service, route, [*fillers, first, second])

        def tied_rules_share_a_route(routes):
            holders = {
                r["metadata"]["name"]
                for r in routes
                for rule in r["spec"].get("rules", [])
                for f in rule.get("filters") or []
                for h in (f.get("responseHeaderModifier") or {}).get("set", [])
                if h.get("name") == RULE_HEADER
            }
            assert len(holders) == 1, (
                f"tied rules split across HTTPRoutes {sorted(holders)}"
            )

        _wait_for_shards(kserve_client, llm_service, 2, tied_rules_share_a_route)
        wait_for_llm_isvc_ready(kserve_client, llm_service)

        def first_rule_serves():
            response = _completion(url)
            assert response.status_code == 200, (
                f"{response.status_code} {response.text[:200]}"
            )
            assert response.headers.get(RULE_HEADER) == "first", (
                f"expected the first of the tied rules to serve, got {response.headers.get(RULE_HEADER)!r}"
            )

        wait_for(first_rule_serves, timeout=120, interval=2)
        for _ in range(10):
            first_rule_serves()

    _with_service(test_case, "2", body)


@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@pytest.mark.llmd_simulator
@pytest.mark.llmisvc_serial
@pytest.mark.traffic
@pytest.mark.parametrize("test_case", [TestCase(base_refs=BASE_REFS)], indirect=True)
@log_execution
def test_route_resharding_under_traffic(test_case, traffic_driver):
    """Moving the serving rule to another shard and back drops no request."""

    def body(kserve_client, llm_service):
        route = _rendered_route(kserve_client, llm_service)
        url = _service_url(kserve_client, llm_service)
        template = route["spec"]["rules"][0]
        serving = _tagged(template, "serving")
        fillers = [_filler(template, i) for i in range(2 * RULES_PER_ROUTE)]

        def serving_in(index):
            def check(routes):
                by_name = {r["metadata"]["name"]: r for r in routes}
                names = sorted(by_name, key=lambda n: (len(n), n))
                holder = [
                    n
                    for n in names
                    for rule in by_name[n]["spec"].get("rules", [])
                    for f in rule.get("filters") or []
                    for h in (f.get("responseHeaderModifier") or {}).get("set", [])
                    if h.get("value") == "serving"
                ]
                assert holder == [names[index]], (
                    f"serving rule in {holder}, want {names[index]}"
                )

            return check

        # Start with the serving rule in the first shard.
        _set_inline_rules(kserve_client, llm_service, route, [serving, *fillers[:4]])
        _wait_for_shards(kserve_client, llm_service, 1, serving_in(0))
        wait_for_llm_isvc_ready(kserve_client, llm_service)

        driver = traffic_driver(
            url=url,
            payload={"model": MODEL, "prompt": "hi", "max_tokens": 1},
            rate=5,
            timeout=15.0,
            warmup=True,
        )
        driver.mark("baseline")
        driver.collect(20)

        # Forward: fillers ahead of it push the serving rule into shard 2.
        driver.mark("forward")
        _set_inline_rules(kserve_client, llm_service, route, [*fillers, serving])
        _wait_for_shards(kserve_client, llm_service, 3, serving_in(2))
        driver.mark("forward_settled")
        driver.collect(20)

        # Back: the fillers move behind it, so it returns to shard 0 while
        # they move forward, the case neither fixed order handles.
        driver.mark("back")
        _set_inline_rules(kserve_client, llm_service, route, [serving, *fillers])
        _wait_for_shards(kserve_client, llm_service, 3, serving_in(0))
        driver.mark("back_settled")
        driver.collect(20)

        report = driver.stop()
        report.since("baseline").assert_no_errors("resharding the inline route")

    _with_service(test_case, "3", body)
