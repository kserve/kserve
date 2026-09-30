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
E2E test for model-based routing only.

serving.kserve.io/model-based-routing-only: "true" on the Gateway, or in the
service's spec.annotations, makes the controller drop the per-model path rules
from the managed HTTPRoute. The model is then reachable only through
model-based routing, and status.url points at the Gateway root.
"""

from urllib.parse import urlparse

import pytest
import requests
from kserve import V1alpha1LLMInferenceService
from kubernetes import client

from .diagnostic import collect_diagnostics
from .fixtures import create_router_resources, generate_k8s_safe_suffix
from .test_gateway_section_name import _create_llmisvc_configs, _get_kserve_client
from .test_llm_inference_service import (
    MODEL_ROUTING_HEADER,
    TestCase,
    assert_model_field_matches,
    completions_payload,
    create_llmisvc,
    get_llm_service_url,
    get_managed_httproutes,
    get_model_routing_url,
    publisher_model,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
)
from .test_resources import make_router_gateway

GATEWAY_NAME = "model-routing-only-gateway"
MODEL_NAME = "facebook/opt-125m"
MODEL_ROUTING_ONLY = {"serving.kserve.io/model-based-routing-only": "true"}


@pytest.mark.cluster_cpu
@pytest.mark.cluster_single_node
@pytest.mark.llmd_simulator
@pytest.mark.custom_gateway
@pytest.mark.model_routing
@pytest.mark.parametrize("set_on", ["gateway", "service"])
def test_model_based_routing_only(set_on, test_namespace):
    kserve_client = _get_kserve_client()
    service_name = generate_k8s_safe_suffix("model-routing-only")

    create_router_resources(
        gateways=[
            make_router_gateway(
                GATEWAY_NAME,
                test_namespace,
                annotations=MODEL_ROUTING_ONLY if set_on == "gateway" else None,
            )
        ],
        kserve_client=kserve_client,
    )

    config_names = _create_llmisvc_configs(
        kserve_client,
        ["router-with-managed-route", "model-fb-opt-125m", "workload-llmd-simulator"],
        service_name,
        test_namespace,
    )
    llm_service = V1alpha1LLMInferenceService(
        api_version="serving.kserve.io/v1alpha1",
        kind="LLMInferenceService",
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec={
            "baseRefs": [{"name": name} for name in config_names],
            "annotations": MODEL_ROUTING_ONLY if set_on == "service" else {},
            "router": {
                "gateway": {
                    "refs": [{"name": GATEWAY_NAME, "namespace": test_namespace}]
                }
            },
        },
    )
    test_case = TestCase(
        base_refs=config_names,
        prompt="KServe is a",
        payload_formatter=completions_payload,
        response_assertion=assert_model_field_matches(MODEL_NAME),
        url_getter=get_model_routing_url,
        extra_headers={
            MODEL_ROUTING_HEADER: publisher_model(test_namespace, MODEL_NAME)
        },
        namespace=test_namespace,
        llm_service=llm_service,
        model_name=MODEL_NAME,
    )

    try:
        create_llmisvc(kserve_client, llm_service)
        wait_for_llm_isvc_ready(kserve_client, llm_service, test_case.wait_timeout)

        routes = get_managed_httproutes(kserve_client, service_name, test_namespace)
        assert routes, f"Expected a managed HTTPRoute for {service_name}"
        path_only = [
            match
            for route in routes
            for rule in route["spec"].get("rules", [])
            for match in rule.get("matches", [])
            if not match.get("headers")
        ]
        assert not path_only, f"Expected no path-only matches, got {path_only}"

        status_url = get_llm_service_url(kserve_client, llm_service)
        assert urlparse(status_url).path in ("", "/"), (
            f"Expected status.url to be the gateway root, got {status_url}"
        )

        wait_for_model_response(
            kserve_client,
            test_case,
            test_case.wait_timeout,
            extra_headers=test_case.extra_headers,
        )

        path_url = (
            f"{status_url.rstrip('/')}/{test_namespace}/{service_name}/v1/completions"
        )
        response = requests.post(
            path_url,
            json=completions_payload(test_case),
            timeout=test_case.response_timeout,
        )
        assert response.status_code == 404, (
            f"Expected the per-model path URL {path_url} to 404, "
            f"got {response.status_code}: {response.text[:500]}"
        )
    except Exception:
        collect_diagnostics(service_name, test_namespace, kserve_client=kserve_client)
        raise
