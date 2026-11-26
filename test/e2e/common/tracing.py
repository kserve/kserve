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

"""Jaeger queries, trace assertions, and diagnostics shared by component E2Es."""

from __future__ import annotations

import json
import logging
import os
import time
from collections.abc import Iterable

from kserve import KServeClient, constants
from kubernetes import client

from .utils import get_isvc_endpoint

logger = logging.getLogger(__name__)

JAEGER_NAMESPACE = os.getenv("JAEGER_NAMESPACE", "observability")
JAEGER_SERVICE = os.getenv("JAEGER_SERVICE", "jaeger")
JAEGER_QUERY_PORT = os.getenv("JAEGER_QUERY_PORT", "16686")
JAEGER_OTLP_ENDPOINT = os.getenv(
    "JAEGER_OTLP_ENDPOINT", "http://jaeger.observability.svc.cluster.local:4317"
)
TRACE_ABSENCE_OBSERVATION_SECONDS = 25


def _query_jaeger(path: str, params: dict[str, str]) -> dict:
    resource_path = (
        f"/api/v1/namespaces/{JAEGER_NAMESPACE}/services/"
        f"{JAEGER_SERVICE}:{JAEGER_QUERY_PORT}/proxy/{path}"
    )
    response = client.ApiClient().call_api(
        resource_path,
        "GET",
        query_params=list(params.items()),
        auth_settings=["BearerToken"],
        _preload_content=False,
        _return_http_data_only=True,
    )
    return json.loads(response.data)


def _get_jaeger_traces(service_name: str, namespace: str) -> list[dict]:
    response = _query_jaeger(
        "api/traces",
        {
            "service": f"{service_name}-predictor",
            "lookback": "10m",
            "limit": "20",
            "tags": json.dumps({"k8s.namespace.name": namespace}),
        },
    )
    return response.get("data", [])


def _span_resource_attributes(trace: dict, span: dict) -> dict:
    process = trace.get("processes", {}).get(span.get("processID"), {})
    attributes = {"serviceName": process.get("serviceName")}
    attributes.update(
        {
            tag["key"]: tag.get("value")
            for tag in process.get("tags", [])
            if tag.get("key")
        }
    )
    return attributes


async def _infer_v1(
    kserve_client: KServeClient,
    rest_v1_client,
    service_name: str,
    payload: dict,
    network_layer: str,
    namespace: str,
    extra_headers: dict[str, str] | None = None,
) -> tuple[dict, dict[str, str]]:
    isvc = kserve_client.get(service_name, namespace=namespace)
    scheme, cluster_ip, host, path = get_isvc_endpoint(isvc, network_layer)
    headers = {"Host": host, "Content-Type": "application/json"}
    if extra_headers:
        headers.update(extra_headers)
    response_headers: dict[str, str] = {}
    response = await rest_v1_client.infer(
        f"{scheme}://{cluster_ip}{path}",
        payload,
        model_name=service_name,
        headers=headers,
        response_headers=response_headers,
    )
    return response, response_headers


def _response_header(headers: dict[str, str], name: str) -> str | None:
    name = name.lower()
    return next((value for key, value in headers.items() if key.lower() == name), None)


def _trace_id_from_traceparent(traceparent: str) -> str:
    parts = traceparent.split("-")
    if len(parts) != 4 or len(parts[1]) != 32:
        raise AssertionError(f"Invalid traceparent returned by KServe: {traceparent!r}")
    return parts[1]


def _wait_for_connected_trace(
    service_name: str,
    trace_id: str,
    namespace: str,
    timeout: int = 180,
) -> dict:
    deadline = time.monotonic() + timeout
    observed = []
    transformer_service = f"{service_name}-transformer"
    predictor_service = f"{service_name}-predictor"
    last_error = ""
    while True:
        traces = _get_jaeger_traces(service_name, namespace)
        observed.extend(trace.get("traceID") for trace in traces)
        matching = [trace for trace in traces if trace.get("traceID") == trace_id]
        if matching:
            try:
                return assert_connected_trace(
                    matching, transformer_service, predictor_service
                )
            except AssertionError as error:
                last_error = str(error)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(5, remaining))
    raise AssertionError(
        f"Timed out waiting for linked trace {trace_id!r} for {service_name!r}; "
        f"observed={sorted(set(observed))!r}; last_error={last_error}"
    )


def _wait_for_parent_trace(
    service_name: str,
    trace_id: str,
    parent_span_id: str,
    namespace: str,
    timeout: int = 180,
) -> dict:
    deadline = time.monotonic() + timeout
    observed = []
    transformer_service = f"{service_name}-transformer"
    predictor_service = f"{service_name}-predictor"
    while True:
        traces = _get_jaeger_traces(service_name, namespace)
        observed.extend(trace.get("traceID") for trace in traces)
        for trace in traces:
            if trace.get("traceID") != trace_id:
                continue
            try:
                assert_trace_descends_from_parent(
                    trace,
                    trace_id,
                    parent_span_id,
                    transformer_service,
                    predictor_service,
                )
                return trace
            except AssertionError:
                continue
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(5, remaining))
    raise AssertionError(
        f"Timed out waiting for trace {trace_id!r} to descend from parent "
        f"{parent_span_id!r} for {service_name!r}; observed={sorted(set(observed))!r}"
    )


def _assert_no_traces_for_service(
    service_name: str, namespace: str, observation_seconds: int
) -> None:
    deadline = time.monotonic() + observation_seconds
    while True:
        traces = _get_jaeger_traces(service_name, namespace)
        assert not traces, (
            f"unexpected Jaeger spans for untraced service {service_name!r}: {traces!r}"
        )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return
        time.sleep(min(5, remaining))


def _collect_isvc_diagnostics(
    kserve_client: KServeClient, service_name: str, namespace: str
) -> None:
    """Capture bounded startup diagnostics before cleanup removes failed pods.

    Only status, events, and logs are collected, not manifests containing env or
    credentials. An unavailable resource/log must not prevent other diagnostics.
    """
    core = kserve_client.core_api
    try:
        isvc = kserve_client.api_instance.get_namespaced_custom_object(
            constants.KSERVE_GROUP,
            constants.KSERVE_V1BETA1_VERSION,
            namespace,
            constants.KSERVE_PLURAL_INFERENCESERVICE,
            service_name,
            _request_timeout=10,
        )
        logger.info("ISVC %s status: %s", service_name, isvc.get("status", {}))
    except Exception:
        logger.warning("Cannot read ISVC %s status", service_name, exc_info=True)

    _collect_isvc_events(core, namespace, service_name)

    try:
        pods = core.list_namespaced_pod(
            namespace,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
            _request_timeout=10,
        ).items
    except Exception:
        logger.warning("Cannot read pods for %s", service_name, exc_info=True)
        return
    for pod in pods:
        logger.info(
            "Pod %s phase=%s conditions=%s",
            pod.metadata.name,
            pod.status.phase,
            pod.status.conditions,
        )
        statuses = {
            status.name: status
            for status in (pod.status.init_container_statuses or [])
            + (pod.status.container_statuses or [])
        }
        for container in (pod.spec.init_containers or []) + pod.spec.containers:
            status = statuses.get(container.name)
            logger.info(
                "Container %s state=%s last_state=%s",
                container.name,
                status.state if status else None,
                status.last_state if status else None,
            )
            for previous in (
                [False, True] if status and status.restart_count else [False]
            ):
                try:
                    logs = core.read_namespaced_pod_log(
                        name=pod.metadata.name,
                        namespace=namespace,
                        container=container.name,
                        previous=previous,
                        tail_lines=200,
                        _request_timeout=10,
                    )
                    logger.info(
                        "Pod %s container=%s previous=%s logs:\n%s",
                        pod.metadata.name,
                        container.name,
                        previous,
                        logs,
                    )
                except Exception:
                    logger.warning(
                        "Cannot read logs for %s/%s previous=%s",
                        pod.metadata.name,
                        container.name,
                        previous,
                        exc_info=True,
                    )


def _collect_isvc_events(core, namespace: str, service_name: str) -> None:
    # Admission rejection can leave no pod at all; ReplicaSet events explain why.
    # Paginate (within a fixed request budget), then log the newest matching events.
    events = []
    continuation = None
    try:
        for _ in range(5):
            page = core.list_namespaced_event(
                namespace, limit=100, _continue=continuation, _request_timeout=10
            )
            events.extend(
                event
                for event in page.items
                if event.involved_object.name == service_name
                or (event.involved_object.name or "").startswith(f"{service_name}-")
            )
            continuation = page.metadata._continue
            if not continuation:
                break
        if continuation:
            logger.warning(
                "Event diagnostics for %s truncated after five pages", service_name
            )
    except Exception:
        logger.warning("Cannot read events for %s", service_name, exc_info=True)
    events.sort(
        key=lambda event: str(
            event.last_timestamp or getattr(event, "first_timestamp", None) or ""
        )
    )
    for event in events[-50:]:
        logger.info("Event %s: %s", event.reason, event.message)


def _safe_delete(
    kserve_client: KServeClient, service_name: str, namespace: str
) -> None:
    try:
        kserve_client.delete(service_name, namespace)
    except Exception:
        logger.exception("Failed to delete InferenceService %s", service_name)


def _span_service_name(trace: dict, span: dict) -> str | None:
    process = trace.get("processes", {}).get(span.get("processID"), {})
    return process.get("serviceName")


def _span_ancestors(
    span: dict, spans_by_id: dict[str, dict], trace_id: str
) -> Iterable[str]:
    """Yield span IDs referenced by CHILD_OF edges, recursively."""
    seen: set[str] = set()
    pending = []
    for reference in span.get("references", []):
        if (
            reference.get("refType") == "CHILD_OF"
            and reference.get("spanID")
            and reference.get("traceID") == trace_id
        ):
            pending.append(reference["spanID"])
    while pending:
        ancestor_id = pending.pop()
        if ancestor_id in seen:
            continue
        seen.add(ancestor_id)
        yield ancestor_id
        ancestor = spans_by_id.get(ancestor_id)
        if ancestor is not None:
            pending.extend(
                reference["spanID"]
                for reference in ancestor.get("references", [])
                if reference.get("refType") == "CHILD_OF"
                and reference.get("spanID")
                and reference.get("traceID") == trace_id
            )


def _assert_span_descends_from(
    span: dict,
    spans_by_id: dict[str, dict],
    ancestor_ids: set[str],
    description: str,
    trace_id: str,
) -> None:
    if not ancestor_ids.intersection(_span_ancestors(span, spans_by_id, trace_id)):
        raise AssertionError(
            f"{description} does not have a CHILD_OF path to {sorted(ancestor_ids)}: "
            f"span={span.get('spanID')!r}, references={span.get('references', [])!r}"
        )


def assert_connected_trace(
    traces: list[dict], transformer_service: str, predictor_service: str
) -> dict:
    """Return a trace containing linked transformer and predictor spans."""
    for trace in traces:
        trace_id = trace.get("traceID")
        if not trace_id:
            continue
        spans = trace.get("spans", [])
        spans_by_id = {span.get("spanID"): span for span in spans if span.get("spanID")}
        transformer_spans = {
            span.get("spanID")
            for span in spans
            if _span_service_name(trace, span) == transformer_service
            and span.get("spanID")
        }
        predictor_spans = [
            span
            for span in spans
            if _span_service_name(trace, span) == predictor_service
        ]
        if not transformer_spans or not predictor_spans:
            continue
        for span in predictor_spans:
            try:
                _assert_span_descends_from(
                    span,
                    spans_by_id,
                    transformer_spans,
                    f"predictor span {span.get('spanID')!r}",
                    trace_id,
                )
            except AssertionError:
                continue
            return trace

    raise AssertionError(
        f"No trace contains a linked transformer ({transformer_service!r}) and "
        f"predictor ({predictor_service!r}) span; trace IDs={[t.get('traceID') for t in traces]!r}"
    )


def assert_trace_descends_from_parent(
    trace: dict,
    trace_id: str,
    parent_span_id: str,
    transformer_service: str,
    predictor_service: str,
) -> None:
    """Require every relevant span to descend from an inbound parent span."""
    actual_trace_id = trace.get("traceID")
    if actual_trace_id != trace_id:
        raise AssertionError(f"Expected trace ID {trace_id!r}, got {actual_trace_id!r}")

    spans = trace.get("spans", [])
    spans_by_id = {span.get("spanID"): span for span in spans if span.get("spanID")}
    transformer_spans = [
        span for span in spans if _span_service_name(trace, span) == transformer_service
    ]
    predictor_spans = [
        span for span in spans if _span_service_name(trace, span) == predictor_service
    ]
    if not transformer_spans or not predictor_spans:
        raise AssertionError(
            f"Trace {trace_id!r} must contain transformer {transformer_service!r} "
            f"and predictor {predictor_service!r} spans"
        )

    for span in transformer_spans + predictor_spans:
        _assert_span_descends_from(
            span,
            spans_by_id,
            {parent_span_id},
            f"span {span.get('spanID')!r}",
            trace_id,
        )

    transformer_ids = {
        span.get("spanID") for span in transformer_spans if span.get("spanID")
    }
    for span in predictor_spans:
        _assert_span_descends_from(
            span,
            spans_by_id,
            transformer_ids,
            f"predictor span {span.get('spanID')!r}",
            trace_id,
        )
