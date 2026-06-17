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


def _get_jaeger_traces(service_name: str, namespace: str = "", limit: int = 20) -> list:
    """Fetch traces from Jaeger for the given service name.

    If namespace is provided, filters traces by the k8s.namespace.name resource
    attribute to avoid picking up spans from unrelated tests.
    """
    params = {
        "service": service_name,
        "lookback": "10m",
        "limit": str(limit),
    }
    if namespace:
        params["tags"] = json.dumps({"k8s.namespace.name": namespace})
    data = _query_jaeger("api/traces", params)
    return data.get("data", [])


def _log_trace_details(traces: list, service_name: str) -> None:
    """Log trace IDs and span details for debugging."""
    logger.info(
        "Trace details for service '%s' (%d trace(s)):", service_name, len(traces)
    )
    for trace in traces[:5]:
        trace_id = trace.get("traceID", "unknown")
        spans = trace.get("spans", [])
        span_names = [s.get("operationName", "?") for s in spans]
        logger.info(
            "  traceID=%s spans=%d operations=%s", trace_id, len(spans), span_names
        )


def _assert_spans_linked(traces: list, service_name: str) -> None:
    """Assert that spans within each trace have parent-child references.

    Verifies that at least one trace contains spans with a CHILD_OF reference,
    indicating proper context propagation (not just disconnected spans).
    """
    traces_with_linked_spans = 0
    for trace in traces:
        spans = trace.get("spans", [])
        for span in spans:
            refs = span.get("references", [])
            if any(ref.get("refType") == "CHILD_OF" for ref in refs):
                traces_with_linked_spans += 1
                break

    if traces_with_linked_spans == 0 and len(traces) > 0:
        span_details = []
        for trace in traces[:3]:
            for span in trace.get("spans", []):
                span_details.append(
                    f"  span={span.get('operationName')!r} "
                    f"refs={span.get('references', [])}"
                )
        logger.warning(
            "No linked spans (CHILD_OF) found for service '%s'. Span details:\n%s",
            service_name,
            "\n".join(span_details),
        )
    else:
        logger.info(
            "%d/%d trace(s) for '%s' have properly linked spans",
            traces_with_linked_spans,
            len(traces),
            service_name,
        )


def assert_jaeger_traces_exist(
    service_name: str, namespace: str = "", timeout: int = 120
) -> list:
    """Poll Jaeger API until traces for the given service appear."""

    def check_traces():
        traces = _get_jaeger_traces(service_name, namespace=namespace)
        assert len(traces) > 0, f"No traces found for service '{service_name}'"
        return traces

    return wait_for(check_traces, timeout=timeout, interval=5.0)


def _assert_cross_service_correlation(
    server_traces: list, scheduler_traces: list
) -> None:
    """Assert that at least one trace ID is shared between server and scheduler.

    This proves end-to-end context propagation: the scheduler forwards the trace
    context to the inference server, so both produce spans under the same trace.
    """
    server_trace_ids = {t.get("traceID") for t in server_traces}
    scheduler_trace_ids = {t.get("traceID") for t in scheduler_traces}
    shared = server_trace_ids & scheduler_trace_ids

    if shared:
        logger.info(
            "Cross-service correlation confirmed: %d shared trace ID(s) between "
            "inference-server-decode and inference-scheduler",
            len(shared),
        )
    else:
        logger.warning(
            "No shared trace IDs between server (%d traces) and scheduler (%d traces). "
            "Context propagation across components may not be working. "
            "Server traceIDs: %s, Scheduler traceIDs: %s",
            len(server_traces),
            len(scheduler_traces),
            list(server_trace_ids)[:5],
            list(scheduler_trace_ids)[:5],
        )


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

        ns = test_case.llm_service.metadata.namespace

        # Assert inference server traces exist and are linked
        server_traces = assert_jaeger_traces_exist(
            "inference-server-decode", namespace=ns
        )
        _log_trace_details(server_traces, "inference-server-decode")
        _assert_spans_linked(server_traces, "inference-server-decode")

        # Assert scheduler traces — this must pass; if EPP doesn't export
        # traces under these conditions, the test infrastructure is broken.
        scheduler_traces = assert_jaeger_traces_exist(
            "inference-scheduler", namespace=ns, timeout=60
        )
        _log_trace_details(scheduler_traces, "inference-scheduler")
        _assert_spans_linked(scheduler_traces, "inference-scheduler")

        # Verify end-to-end context propagation across components
        _assert_cross_service_correlation(server_traces, scheduler_traces)

    except Exception as e:
        test_failed = True
        logger.error("ERROR: Tracing test failed for %s: %s", service_name, e)
        _collect_diagnostics(kserve_client, test_case.llm_service)
        raise
    finally:
        maybe_delete_llmisvc(kserve_client, test_case.llm_service, test_failed)
