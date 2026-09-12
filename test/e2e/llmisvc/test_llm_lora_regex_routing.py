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

"""Prove exact/regex routing and escaped near-misses against a real gateway.

The strategy is cluster-wide; these tests run serially and leave the key unset.
Route-shape and failure/recovery behavior are covered by envtest.
"""

import concurrent.futures
import contextlib
import json
import os

import pytest
from kserve import KServeClient
from kubernetes import client

from .diagnostic import collect_diagnostics
from .fixtures import (
    LLMINFERENCESERVICE_CONFIGS,
    ensure_pvc_with_lora_adapter,
    inject_k8s_proxy,
)
from .logging import log_execution
from .test_llm_inference_service import (
    MODEL_ROUTING_HEADER,
    TestCase,
    create_llmisvc,
    get_managed_httproutes,
    get_model_routing_url,
    maybe_delete_llmisvc,
    publisher_model,
    wait_for,
    wait_for_llm_isvc_ready,
)
from ..common.http_retry import DEFAULT_RETRY_TOTAL, post_with_retry
from ..common.utils import KSERVE_NAMESPACE

INFERENCESERVICE_CONFIGMAP = "inferenceservice-config"
STRATEGY_KEY = "loraModelRoutingStrategy"

BASE_MODEL = "facebook/opt-125m"
ROUTER_REFS = ["router-managed", "workload-single-cpu"]
# Real LoRA artifacts under names that exercise slash and dot escaping. The
# regex run carries a prefix of the realistic-v1 fixture sized past the exact
# strategy's per-rule match budget (LORA_ADAPTER_COUNT, raise it with
# E2E_LORA_ADAPTER_COUNT), so it proves the claim on a real gateway; the default
# strategy keeps two.
PROBE_CONCURRENCY = 8
STRATEGY_ANNOTATION = "serving.kserve.io/lora-model-routing-strategy"
LORA_PRESETS = {
    "regex": "model-fb-opt-125m-with-lora-realistic",
    None: "model-fb-opt-125m-with-lora-adversarial-names",
}


def adapter_names(preset):
    lora = LLMINFERENCESERVICE_CONFIGS[preset]["model"]["lora"]
    return [adapter["name"] for adapter in lora["adapters"]]


def adapters_on_pvc(preset):
    lora = LLMINFERENCESERVICE_CONFIGS[preset]["model"]["lora"]
    return any(adapter["uri"].startswith("pvc://") for adapter in lora["adapters"])


@contextlib.contextmanager
def cluster_routing_strategy(kserve_client, strategy):
    core = kserve_client.core_api
    cm = core.read_namespaced_config_map(INFERENCESERVICE_CONFIGMAP, KSERVE_NAMESPACE)
    ingress = json.loads(cm.data["ingress"])
    if strategy is None:
        ingress.pop(STRATEGY_KEY, None)  # Exercise the default even on custom installs.
    else:
        ingress[STRATEGY_KEY] = strategy
    try:
        core.patch_namespaced_config_map(
            INFERENCESERVICE_CONFIGMAP,
            KSERVE_NAMESPACE,
            {"data": {"ingress": json.dumps(ingress)}},
        )
        yield
    finally:
        # Reset to the shipped default (key absent) rather than to whatever was
        # found on entry, so an interrupted run cannot hand its flip to the next.
        ingress.pop(STRATEGY_KEY, None)
        core.patch_namespaced_config_map(
            INFERENCESERVICE_CONFIGMAP,
            KSERVE_NAMESPACE,
            {"data": {"ingress": json.dumps(ingress)}},
        )


def assert_route_shape(kserve_client, llm_service, strategy, adapters):
    matches = [
        header
        for route in get_managed_httproutes(
            kserve_client, llm_service.metadata.name, llm_service.metadata.namespace
        )
        for rule in route.get("spec", {}).get("rules", [])
        for match in rule.get("matches", [])
        for header in match.get("headers", [])
        if header.get("name", "").lower() == MODEL_ROUTING_HEADER.lower()
    ]
    match_type = "RegularExpression" if strategy == "regex" else "Exact"
    assert matches and all(h.get("type", "Exact") == match_type for h in matches), (
        f"Expected {match_type} model matches, got {matches}"
    )
    if strategy != "regex":
        expected = {
            publisher_model(llm_service.metadata.namespace, name)
            for name in [BASE_MODEL, *adapters]
        }
        assert expected <= {h["value"] for h in matches}


def probe(routing_url, header_model, body_model, total_retries=DEFAULT_RETRY_TOTAL):
    return post_with_retry(
        routing_url + "/v1/completions",
        headers={MODEL_ROUTING_HEADER: header_model},
        json_data={"model": body_model, "prompt": "hi", "max_tokens": 1},
        timeout=60,
        total_retries=total_retries,
    )


def assert_routing(routing_url, namespace, adapters):
    def expect_served(name):
        identity = publisher_model(namespace, name)
        for body_model in (name, identity):
            response = probe(routing_url, identity, body_model)
            assert response.status_code == 200, (
                f"{identity!r} (body={body_model!r}): "
                f"{response.status_code} {response.text[:200]}"
            )
            # vLLM may normalize base-model aliases to the canonical name.
            assert response.json().get("model") in (name, identity)

    # Warm up on the base model, then fan the adapters out: the engine batches
    # concurrent requests, so a hundred adapters probe in seconds, not minutes.
    expect_served(BASE_MODEL)
    with concurrent.futures.ThreadPoolExecutor(max_workers=PROBE_CONCURRENCY) as pool:
        pending = {pool.submit(expect_served, name): name for name in adapters}
        failures = [
            f"{pending[done]}: {done.exception()}"
            for done in concurrent.futures.as_completed(pending)
            if done.exception() is not None
        ]
    assert not failures, "adapters not served:\n" + "\n".join(failures)

    # Use a valid body so vLLM cannot mask an over-broad gateway match by
    # rejecting an unknown model. Successful probes above rule out cold start.
    valid_body = publisher_model(namespace, BASE_MODEL)
    adapter = adapters[0]
    for miss in (
        publisher_model(namespace, "no-such-adapter"),
        publisher_model(namespace, adapter[:-1]),
        publisher_model(namespace, adapter + "x"),
        publisher_model(namespace, "x" + adapter),
        publisher_model(namespace, f"evil/{adapter}/tail"),
        "other/" + publisher_model(namespace, adapter),
        publisher_model("other-namespace", adapter),
        publisher_model(namespace, adapter.replace(".", "X")),
    ):
        # Unmatched routes answer 404, which is the expected result, not a retry.
        response = probe(routing_url, miss, valid_body, total_retries=0)
        assert response.status_code != 200, f"Near-miss identity {miss!r} was served"


@pytest.mark.cluster_cpu
@pytest.mark.lora
@pytest.mark.lora_routing
@pytest.mark.model_routing
@pytest.mark.parametrize(
    "cluster_strategy, annotation, test_case",
    [
        pytest.param(
            cluster_strategy,
            annotation,
            TestCase(
                base_refs=[*ROUTER_REFS, LORA_PRESETS[annotation or cluster_strategy]]
            ),
            id=test_id,
        )
        for cluster_strategy, annotation, test_id in (
            ("regex", None, "regex-strategy"),
            (None, "regex", "annotation-regex"),
            (None, None, "exact-default"),
        )
    ],
    indirect=["test_case"],
)
@log_execution
def test_lora_routing_strategy_dataplane(cluster_strategy, annotation, test_case):
    inject_k8s_proxy()
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )
    llm_service = test_case.llm_service
    namespace = test_case.namespace
    # The service annotation wins over the cluster-wide key when both are set.
    strategy = annotation or cluster_strategy
    if annotation:
        llm_service.spec.setdefault("annotations", {})[STRATEGY_ANNOTATION] = annotation
    adapters = adapter_names(LORA_PRESETS[strategy])
    test_failed = False
    # The test_case fixture owns the configs and the namespace; this test owns
    # the cluster-wide strategy key and its own service.
    with cluster_routing_strategy(kserve_client, cluster_strategy):
        try:
            if adapters_on_pvc(LORA_PRESETS[strategy]):
                ensure_pvc_with_lora_adapter(namespace)
            create_llmisvc(kserve_client, llm_service)
            wait_for_llm_isvc_ready(kserve_client, llm_service)
            wait_for(
                lambda: assert_route_shape(
                    kserve_client, llm_service, strategy, adapters
                ),
                timeout=300,
                interval=5,
            )
            assert_routing(
                get_model_routing_url(kserve_client, llm_service), namespace, adapters
            )
        except Exception:
            test_failed = True
            collect_diagnostics(
                llm_service.metadata.name, namespace, kserve_client=kserve_client
            )
            raise
        finally:
            maybe_delete_llmisvc(kserve_client, llm_service, test_failed)
