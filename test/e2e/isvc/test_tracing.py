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
import logging
import os
import time
import uuid

import pytest
from kserve import (
    KServeClient,
    V1beta1CanarySpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1PredictorSpec,
    constants,
)
from kserve.models.v1beta1_tracing_spec import V1beta1TracingSpec
from kubernetes import client
from kubernetes.client import V1ResourceRequirements

from ..common.utils import KSERVE_TEST_NAMESPACE, predict_isvc

logger = logging.getLogger(__name__)

JAEGER_NAMESPACE = os.getenv("JAEGER_NAMESPACE", "observability")
JAEGER_SERVICE = os.getenv("JAEGER_SERVICE", "jaeger")
JAEGER_QUERY_PORT = os.getenv("JAEGER_QUERY_PORT", "16686")
JAEGER_OTLP_ENDPOINT = os.getenv(
    "JAEGER_OTLP_ENDPOINT",
    "http://jaeger.observability.svc.cluster.local:4317",
)
MODEL_URI = "gs://kfserving-examples/models/sklearn/1.0/model"
CANARY_VARIANT = "v2"


def _require_gateway_api(network_layer: str) -> None:
    if network_layer == "gateway-api" or "gatewayapi" in network_layer:
        return
    pytest.skip(
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
            tracing=V1beta1TracingSpec(
                exporter="otlp",
                exporter_endpoint=JAEGER_OTLP_ENDPOINT,
                sampler="parentbased_traceidratio",
                sampler_arg="1.0",
            ),
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


@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_canary_trace_contains_predictor_variant(
    kserve_client,
    rest_v2_client,
    network_layer,
):
    _require_gateway_api(network_layer)

    service_name = f"trace-canary-{uuid.uuid4().hex[:5]}"
    try:
        kserve_client.create(_make_isvc(service_name))
        kserve_client.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        )
        _wait_for_canary_ready(kserve_client, service_name)

        response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
        )
        assert response.outputs[0].data == [1, 1]

        trace = _wait_for_canary_trace(
            service_name,
            KSERVE_TEST_NAMESPACE,
            CANARY_VARIANT,
        )
        assert trace
    finally:
        _safe_delete(kserve_client, service_name)
