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

"""Fixtures and assertions shared by the ISVC tracing tests."""

from __future__ import annotations

import json
import logging
import os
import time
from collections.abc import Iterable

import pytest
from kserve import (
    KServeClient,
    V1beta1CanarySpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    V1beta1TransformerSpec,
    constants,
)
from kserve.models.v1beta1_tracing_spec import V1beta1TracingSpec
from kubernetes import client
from kubernetes.client import V1Container, V1EnvVar, V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE, get_isvc_endpoint

logger = logging.getLogger(__name__)

JAEGER_NAMESPACE = os.getenv("JAEGER_NAMESPACE", "observability")
JAEGER_SERVICE = os.getenv("JAEGER_SERVICE", "jaeger")
JAEGER_QUERY_PORT = os.getenv("JAEGER_QUERY_PORT", "16686")
JAEGER_OTLP_ENDPOINT = os.getenv(
    "JAEGER_OTLP_ENDPOINT",
    "http://jaeger.observability.svc.cluster.local:4317",
)
MODEL_URI = "gs://kfserving-examples/models/sklearn/1.0/model"
OPT_125M_MODEL_URI = os.getenv("OPT_125M_OCI_MODEL_URI", "hf://facebook/opt-125m")
VLLM_IMAGE = "vllm/vllm-openai-cpu:v0.19.0"
TRANSFORMER_COMMAND = ["python", "-m", "custom_transformer.openai_trace_transformer"]
ISVC_READY_TIMEOUT_S = 1200
TRACE_ABSENCE_OBSERVATION_SECONDS = 25
CANARY_VARIANT = "v2"


def _require_gateway_api(network_layer: str) -> None:
    if network_layer == "gateway-api" or "gatewayapi" in network_layer:
        return
    pytest.fail(
        "deterministic weighted canary routing requires Gateway API; "
        f"received network layer {network_layer!r}"
    )


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


def _trace_has_canary_span(
    trace: dict, service_name: str, namespace: str, variant: str
) -> bool:
    for span in trace.get("spans", []):
        attributes = _span_resource_attributes(trace, span)
        if (
            attributes.get("serviceName") == f"{service_name}-predictor"
            and attributes.get("k8s.namespace.name") == namespace
            and attributes.get("isvc.name") == service_name
            and attributes.get("isvc.component") == "predictor"
            and attributes.get("isvc.predictor.variant") == variant
        ):
            return True
    return False


def _wait_for_canary_trace(
    service_name: str,
    namespace: str,
    variant: str,
    timeout: int = 120,
) -> dict:
    deadline = time.monotonic() + timeout
    observed = {}

    while True:
        traces = _get_jaeger_traces(service_name, namespace)
        for trace in traces:
            trace_id = trace.get("traceID", "<missing>")
            observed[trace_id] = {
                process_id: {
                    "serviceName": process.get("serviceName"),
                    "tags": process.get("tags", []),
                }
                for process_id, process in trace.get("processes", {}).items()
            }

        for trace in traces:
            if _trace_has_canary_span(trace, service_name, namespace, variant):
                return trace

        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(5, remaining))

    raise AssertionError(
        f"Timed out waiting for canary trace for {service_name!r} with variant "
        f"{variant!r}; observed trace IDs and process tags: "
        f"{json.dumps(observed, sort_keys=True, default=str)}"
    )


def _make_predictor(name=None, min_replicas=1) -> V1beta1PredictorSpec:
    predictor_kwargs = {
        "min_replicas": min_replicas,
        "model": V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri=MODEL_URI,
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    }
    if name is not None:
        predictor_kwargs["name"] = name
    return V1beta1PredictorSpec(**predictor_kwargs)


def _tracing_spec() -> V1beta1TracingSpec:
    return V1beta1TracingSpec(
        exporter="otlp",
        exporter_endpoint=JAEGER_OTLP_ENDPOINT,
        sampler="parentbased_traceidratio",
        sampler_arg="1.0",
    )


def _standard_annotations() -> dict[str, str]:
    return {
        "serving.kserve.io/deploymentMode": "Standard",
        "serving.kserve.io/autoscalerClass": "none",
        "serving.knative.dev/progress-deadline": "20m",
    }


def _make_vllm_isvc(service_name: str) -> V1beta1InferenceService:
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        model=V1beta1ModelSpec(
            image=VLLM_IMAGE,
            model_format=V1beta1ModelFormat(name="vLLM"),
            storage_uri=OPT_125M_MODEL_URI,
            args=["--max-model-len", "256", "--enforce-eager"],
            env=[V1EnvVar(name="VLLM_CPU_KVCACHE_SPACE", value="1")],
            resources=V1ResourceRequirements(
                requests={"cpu": "2", "memory": "8Gi"},
                limits={"cpu": "2", "memory": "8Gi"},
            ),
        ),
    )
    transformer = V1beta1TransformerSpec(
        min_replicas=1,
        containers=[
            V1Container(
                name="kserve-container",
                image=os.environ.get("IMAGE_TRANSFORMER_IMG_TAG"),
                command=TRANSFORMER_COMMAND,
                args=["--model_name", service_name],
                resources=V1ResourceRequirements(
                    requests={"cpu": "100m", "memory": "512Mi"},
                    limits={"cpu": "1", "memory": "1Gi"},
                ),
            )
        ],
    )
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=_standard_annotations(),
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            transformer=transformer,
            tracing=_tracing_spec(),
        ),
    )


def _make_untraced_sklearn_isvc(service_name: str) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations=_standard_annotations(),
        ),
        spec=V1beta1InferenceServiceSpec(predictor=_make_predictor()),
    )


async def _infer_v1(
    kserve_client: KServeClient,
    rest_v1_client,
    service_name: str,
    payload: dict,
    network_layer: str,
    extra_headers: dict[str, str] | None = None,
) -> tuple[dict, dict[str, str]]:
    isvc = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
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
    timeout: int = 180,
) -> dict:
    deadline = time.monotonic() + timeout
    observed = []
    transformer_service = f"{service_name}-transformer"
    predictor_service = f"{service_name}-predictor"
    last_error = ""
    while True:
        traces = _get_jaeger_traces(service_name, KSERVE_TEST_NAMESPACE)
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
    timeout: int = 180,
) -> dict:
    deadline = time.monotonic() + timeout
    observed = []
    transformer_service = f"{service_name}-transformer"
    predictor_service = f"{service_name}-predictor"
    while True:
        traces = _get_jaeger_traces(service_name, KSERVE_TEST_NAMESPACE)
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


def _assert_no_traces_for_service(service_name: str, observation_seconds: int) -> None:
    deadline = time.monotonic() + observation_seconds
    while True:
        traces = _get_jaeger_traces(service_name, KSERVE_TEST_NAMESPACE)
        assert not traces, (
            f"unexpected Jaeger spans for untraced service {service_name!r}: {traces!r}"
        )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return
        time.sleep(min(5, remaining))


def _assert_predictor_image(kserve_client: KServeClient, service_name: str) -> None:
    pods = kserve_client.core_api.list_namespaced_pod(
        KSERVE_TEST_NAMESPACE,
        label_selector=f"serving.kserve.io/inferenceservice={service_name}",
    )
    images = [
        container.image
        for pod in pods.items
        for container in pod.spec.containers
        if container.name == "kserve-container"
    ]
    assert images, f"No predictor container found for {service_name!r}"
    assert VLLM_IMAGE in images, (
        f"Expected predictor to use pinned image {VLLM_IMAGE!r}, got {images!r}"
    )


def _make_isvc(service_name: str) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=KSERVE_TEST_NAMESPACE,
            annotations={
                "serving.kserve.io/deploymentMode": "Standard",
                "serving.kserve.io/autoscalerClass": "none",
            },
        ),
        spec=V1beta1InferenceServiceSpec(
            predictor=_make_predictor(),
            canary=[
                V1beta1CanarySpec(
                    traffic_percent=100,
                    predictor=_make_predictor(
                        name=CANARY_VARIANT,
                        min_replicas=None,
                    ),
                )
            ],
            tracing=_tracing_spec(),
        ),
    )


def _wait_for_canary_ready(
    kserve_client: KServeClient,
    service_name: str,
    timeout: int = 120,
) -> None:
    deadline = time.monotonic() + timeout
    last_conditions = []
    last_canary_statuses = []

    while True:
        isvc = kserve_client.get(service_name, namespace=KSERVE_TEST_NAMESPACE)
        status = isvc.get("status", {})
        last_conditions = status.get("conditions", [])
        last_canary_statuses = status.get("canaryStatuses", [])

        canary_condition_ready = any(
            condition.get("type") == "CanaryPredictorReady"
            and condition.get("status") == "True"
            for condition in last_conditions
        )
        canary_status_ready = any(
            canary_status.get("name") == CANARY_VARIANT
            and canary_status.get("ready") is True
            for canary_status in last_canary_statuses
        )
        if canary_condition_ready and canary_status_ready:
            return

        remaining = deadline - time.monotonic()
        if remaining <= 0:
            break
        time.sleep(min(5, remaining))

    raise AssertionError(
        f"Timed out waiting for canary {CANARY_VARIANT!r} readiness for "
        f"{service_name!r}; observed conditions={last_conditions!r}, "
        f"canaryStatuses={last_canary_statuses!r}"
    )


def _safe_delete(kserve_client: KServeClient, service_name: str) -> None:
    try:
        kserve_client.delete(service_name, KSERVE_TEST_NAMESPACE)
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
