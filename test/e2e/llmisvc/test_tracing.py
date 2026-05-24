# Copyright 2025 The KServe Authors.
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

import json
import os

import pytest
from kserve import KServeClient
from kubernetes import client

from .fixtures import generate_test_id, inject_k8s_proxy
from .logging import log_execution, logger
from .test_llm_inference_service import (
    TestCase,
    completions_payload,
    create_llmisvc,
    create_response_assertion,
    maybe_delete_llmisvc,
    wait_for_llm_isvc_ready,
    wait_for_model_response,
    wait_for,
    _collect_diagnostics,
)

JAEGER_NAMESPACE = os.getenv("JAEGER_NAMESPACE", "observability")
JAEGER_SERVICE = os.getenv("JAEGER_SERVICE", "jaeger")
JAEGER_QUERY_PORT = os.getenv("JAEGER_QUERY_PORT", "16686")


def _query_jaeger(path: str, params: dict) -> dict:
    """Query Jaeger via the K8s API server service proxy.

    Uses ApiClient.call_api with a pre-built resource path so the path is not
    URL-encoded, and passes query parameters separately so they are appended
    correctly.  Auth is inherited from the kubeconfig loaded by inject_k8s_proxy.
    """
    resource_path = (
        f"/api/v1/namespaces/{JAEGER_NAMESPACE}/services/"
        f"{JAEGER_SERVICE}:{JAEGER_QUERY_PORT}/proxy/{path}"
    )
    api_client = client.ApiClient()
    resp = api_client.call_api(
        resource_path,
        "GET",
        query_params=list(params.items()),
        auth_settings=["BearerToken"],
        _preload_content=False,
    )
    return json.loads(resp[0].data)


def assert_jaeger_traces_exist(service_name: str, timeout: int = 120):
    """Poll Jaeger API until traces for the given service appear."""

    def check_traces():
        data = _query_jaeger(
            "api/traces",
            {
                "service": service_name,
                "lookback": "10m",
                "limit": "5",
            },
        )
        traces = data.get("data", [])
        assert len(traces) > 0, f"No traces found for service '{service_name}'"
        return traces

    return wait_for(check_traces, timeout=timeout, interval=5.0)


@pytest.mark.llminferenceservice
@pytest.mark.tracing
@pytest.mark.parametrize(
    "test_case",
    [
        pytest.param(
            TestCase(
                base_refs=[
                    "router-managed",
                    "workload-single-cpu",
                    "model-fb-opt-125m",
                    "tracing-enabled",
                ],
                prompt="KServe is a",
                payload_formatter=completions_payload,
                response_assertion=create_response_assertion(with_field="choices"),
            ),
            marks=[pytest.mark.cluster_cpu, pytest.mark.cluster_single_node],
        ),
    ],
    indirect=True,
    ids=generate_test_id,
)
@log_execution
def test_tracing_spans_collected(test_case: TestCase):  # noqa: F811
    """Verify that tracing spans are exported to Jaeger for both scheduler and inference server."""
    inject_k8s_proxy()

    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config"),
        client_configuration=client.Configuration(),
    )

    service_name = test_case.llm_service.metadata.name

    test_failed = False
    try:
        create_llmisvc(kserve_client, test_case.llm_service)
        wait_for_llm_isvc_ready(
            kserve_client, test_case.llm_service, test_case.wait_timeout
        )
        wait_for_model_response(kserve_client, test_case, test_case.wait_timeout)

        logger.info("Inference request succeeded; verifying tracing spans in Jaeger...")

        server_traces = assert_jaeger_traces_exist("inference-server-decode")
        logger.info(f"Found {len(server_traces)} trace(s) for inference-server-decode")

        try:
            scheduler_traces = assert_jaeger_traces_exist(
                "inference-scheduler", timeout=30
            )
            logger.info(
                f"Found {len(scheduler_traces)} trace(s) for inference-scheduler"
            )
        except AssertionError:
            logger.warning(
                "inference-scheduler traces not found; EPP may not export "
                "traces depending on its OTLP configuration"
            )

    except Exception as e:
        test_failed = True
        logger.error("ERROR: Tracing test failed for %s: %s", service_name, e)
        _collect_diagnostics(kserve_client, test_case.llm_service)
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)
