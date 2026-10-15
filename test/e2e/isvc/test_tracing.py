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

import uuid

import pytest

from ..common.utils import predict_isvc
from .fixtures import (
    CANARY_VARIANT,
    KSERVE_TEST_NAMESPACE,
    TRACE_ABSENCE_OBSERVATION_SECONDS,
    _assert_no_traces_for_service,
    _assert_predictor_image,
    _infer_v1,
    _make_isvc,
    _make_untraced_sklearn_isvc,
    _require_gateway_api,
    _response_header,
    _safe_delete,
    _trace_id_from_traceparent,
    _wait_for_canary_ready,
    _wait_for_canary_trace,
    _wait_for_connected_trace,
    _wait_for_parent_trace,
)


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


@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_transformer_vllm_trace_is_connected(
    kserve_client,
    rest_v1_client,
    network_layer,
    traced_vllm_isvc,
):
    service_name = traced_vllm_isvc
    _assert_predictor_image(kserve_client, service_name)
    response, response_headers = await _infer_v1(
        kserve_client,
        rest_v1_client,
        service_name,
        {"instances": [{"prompt": "KServe is"}]},
        network_layer,
    )
    prediction = response.get("predictions", [None])[0]
    assert isinstance(prediction, str) and prediction.strip(), response

    traceparent = _response_header(response_headers, "traceparent")
    assert traceparent, f"KServe tracing response header missing: {response_headers!r}"
    trace_id = _trace_id_from_traceparent(traceparent)
    trace = _wait_for_connected_trace(service_name, trace_id)
    assert trace.get("traceID") == trace_id


@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_omitted_tracing_has_no_instrumentation_or_spans(
    kserve_client,
    rest_v1_client,
    network_layer,
):
    service_name = f"trace-disabled-{uuid.uuid4().hex[:8]}"
    try:
        kserve_client.create(_make_untraced_sklearn_isvc(service_name))
        kserve_client.wait_isvc_ready(
            service_name,
            namespace=KSERVE_TEST_NAMESPACE,
        )
        response, response_headers = await _infer_v1(
            kserve_client,
            rest_v1_client,
            service_name,
            {
                "instances": [
                    [6.8, 2.8, 4.8, 1.4],
                    [6.0, 3.4, 4.5, 1.6],
                ]
            },
            network_layer,
        )
        assert response["predictions"] == [1, 1]
        assert _response_header(response_headers, "traceparent") is None

        pods = kserve_client.core_api.list_namespaced_pod(
            KSERVE_TEST_NAMESPACE,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        assert pods.items, f"No serving pod found for {service_name!r}"
        otel_env = [
            env.name
            for pod in pods.items
            for container in pod.spec.containers
            for env in (container.env or [])
            if env.name and env.name.startswith("OTEL_")
        ]
        assert not otel_env, (
            f"Untraced service received OpenTelemetry env: {otel_env!r}"
        )
        _assert_no_traces_for_service(service_name, TRACE_ABSENCE_OBSERVATION_SECONDS)
    finally:
        _safe_delete(kserve_client, service_name)


@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_inbound_traceparent_is_preserved_through_transformer_and_vllm(
    kserve_client,
    rest_v1_client,
    network_layer,
    traced_vllm_isvc,
):
    service_name = traced_vllm_isvc
    trace_id = uuid.uuid4().hex
    parent_span_id = uuid.uuid4().hex[:16]
    traceparent = f"00-{trace_id}-{parent_span_id}-01"
    _assert_predictor_image(kserve_client, service_name)
    response, response_headers = await _infer_v1(
        kserve_client,
        rest_v1_client,
        service_name,
        {"instances": [{"prompt": "KServe is"}]},
        network_layer,
        extra_headers={"traceparent": traceparent},
    )
    prediction = response.get("predictions", [None])[0]
    assert isinstance(prediction, str) and prediction.strip(), response
    returned_traceparent = _response_header(response_headers, "traceparent")
    assert returned_traceparent, (
        f"KServe tracing response header missing: {response_headers!r}"
    )
    assert _trace_id_from_traceparent(returned_traceparent) == trace_id

    trace = _wait_for_parent_trace(service_name, trace_id, parent_span_id)
    assert trace.get("traceID") == trace_id
