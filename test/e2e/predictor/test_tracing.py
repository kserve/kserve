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
import time
import uuid

import pytest
from kserve import (
    KServeClient,
    V1beta1CanarySpec,
    V1beta1ModelFormat,
    V1beta1ModelSpec,
    V1beta1InferenceService,
    V1beta1InferenceServiceSpec,
    V1beta1PredictorSpec,
    V1beta1SKLearnSpec,
    constants,
)
from kserve.models.v1beta1_tracing_spec import V1beta1TracingSpec
from kubernetes import client
from kubernetes.client import V1ContainerPort, V1ResourceRequirements

from ..common.tracing import (
    JAEGER_OTLP_ENDPOINT,
    TRACE_ABSENCE_OBSERVATION_SECONDS,
    _assert_no_traces_for_service,
    _collect_isvc_diagnostics,
    _get_jaeger_traces,
    _infer_v1,
    _response_header,
    _safe_delete,
    _span_resource_attributes,
    logger,
)
from ..common.utils import (
    predict_grpc,
    predict_isvc,
    wait_for_pod_logs,
)


@pytest.mark.skip(
    reason="need to enable `kubernetes.podspec-fieldref` in knative to set OTEL_RESOURCE_ATTRIBUTES_NODE_NAME and OTEL_RESOURCE_ATTRIBUTES_POD_NAME"
)
@pytest.mark.predictor
@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_sklearn_traces_rest_inference_in_knative_mode(
    rest_v2_client, network_layer, test_namespace
):
    service_name = "sklearn-tracing-rest"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
        ),
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            tracing=V1beta1TracingSpec(exporter="console", sampler="always_on"),
        ),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

        response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
            namespace=test_namespace,
        )
        assert response.outputs[0].data == [1, 1]

        pods = kserve_client.core_api.list_namespaced_pod(
            test_namespace,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        assert len(pods.items) == 1
        logs = await wait_for_pod_logs(
            kserve_client.core_api,
            pods.items[0].metadata.name,
            test_namespace,
            expected_substring=f"POST /v2/models/{service_name}/infer",
        )
        assert "POST /v2/models/{service_name}/infer" in logs
        assert f'"service.name": "{service_name}-predictor"' in logs
    finally:
        kserve_client.delete(service_name, test_namespace)


# The dual-protocol suite runs in Standard mode with a Gateway API provider,
# which is required to expose both the REST and gRPC ports.
@pytest.mark.tracing
@pytest.mark.dual_protocol
@pytest.mark.asyncio(scope="session")
async def test_sklearn_traces_rest_and_grpc_inference(
    rest_v2_client, network_layer, test_namespace
):
    service_name = "sklearn-tracing"
    predictor = V1beta1PredictorSpec(
        min_replicas=1,
        sklearn=V1beta1SKLearnSpec(
            storage_uri="gs://kfserving-examples/models/sklearn/1.0/model",
            resources=V1ResourceRequirements(
                requests={"cpu": "50m", "memory": "128Mi"},
                limits={"cpu": "100m", "memory": "256Mi"},
            ),
            ports=[
                V1ContainerPort(container_port=8080, name="http-rest", protocol="TCP"),
                V1ContainerPort(container_port=8081, name="grpc-port", protocol="TCP"),
            ],
        ),
    )
    isvc = V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(name=service_name, namespace=test_namespace),
        spec=V1beta1InferenceServiceSpec(
            predictor=predictor,
            tracing=V1beta1TracingSpec(
                exporter="console",
                sampler="always_on",
            ),
        ),
    )
    kserve_client = KServeClient(
        config_file=os.environ.get("KUBECONFIG", "~/.kube/config")
    )
    try:
        kserve_client.create(isvc)
        kserve_client.wait_isvc_ready(service_name, namespace=test_namespace)

        rest_response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
            namespace=test_namespace,
        )
        assert rest_response.outputs[0].data == [1, 1]

        with open("./data/iris_input_v2_grpc.json") as input_file:
            grpc_inputs = json.load(input_file)["inputs"]
        grpc_response = await predict_grpc(
            service_name=service_name,
            payload=grpc_inputs,
            model_name=service_name,
            network_layer=network_layer,
            namespace=test_namespace,
        )
        assert grpc_response.outputs[0].data == [1, 1]

        pods = kserve_client.core_api.list_namespaced_pod(
            test_namespace,
            label_selector=f"serving.kserve.io/inferenceservice={service_name}",
        )
        assert len(pods.items) == 1
        logs = await wait_for_pod_logs(
            kserve_client.core_api,
            pods.items[0].metadata.name,
            test_namespace,
            expected_substring="/inference.GRPCInferenceService/ModelInfer",
        )
        assert "POST /v2/models/{model_name}/infer" in logs
        assert f'"service.name": "{service_name}-predictor"' in logs
    finally:
        kserve_client.delete(service_name, test_namespace)


CANARY_VARIANT = "v2"


def _require_gateway_api(network_layer: str) -> None:
    if network_layer == "gateway-api" or "gatewayapi" in network_layer:
        return
    pytest.fail(
        "deterministic weighted canary routing requires Gateway API; "
        f"received network layer {network_layer!r}"
    )


def _make_predictor(name=None, min_replicas=1) -> V1beta1PredictorSpec:
    predictor_kwargs = {
        "min_replicas": min_replicas,
        "model": V1beta1ModelSpec(
            model_format=V1beta1ModelFormat(name="sklearn"),
            storage_uri="s3://example-models/sklearn/",
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


def _make_untraced_sklearn_isvc(
    service_name: str, namespace: str
) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=namespace,
            annotations={
                "serving.kserve.io/deploymentMode": "Standard",
                "serving.kserve.io/autoscalerClass": "none",
            },
        ),
        spec=V1beta1InferenceServiceSpec(predictor=_make_predictor()),
    )


def _make_isvc(service_name: str, namespace: str) -> V1beta1InferenceService:
    return V1beta1InferenceService(
        api_version=constants.KSERVE_V1BETA1,
        kind=constants.KSERVE_KIND_INFERENCESERVICE,
        metadata=client.V1ObjectMeta(
            name=service_name,
            namespace=namespace,
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
    namespace: str,
    timeout: int = 120,
) -> None:
    deadline = time.monotonic() + timeout
    last_conditions = []
    last_canary_statuses = []

    while True:
        isvc = kserve_client.get(service_name, namespace=namespace)
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


@pytest.mark.raw
@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_canary_trace_contains_predictor_variant(
    kserve_client,
    rest_v2_client,
    network_layer,
    test_namespace,
):
    namespace = test_namespace
    _require_gateway_api(network_layer)

    service_name = f"trace-canary-{uuid.uuid4().hex[:5]}"
    try:
        kserve_client.create(_make_isvc(service_name, namespace))
        kserve_client.wait_isvc_ready(
            service_name,
            namespace=namespace,
        )
        _wait_for_canary_ready(kserve_client, service_name, namespace)

        response = await predict_isvc(
            rest_v2_client,
            service_name,
            "./data/iris_input_v2.json",
            network_layer=network_layer,
            namespace=namespace,
        )
        assert response.outputs[0].data == [1, 1]

        trace = _wait_for_canary_trace(
            service_name,
            namespace,
            CANARY_VARIANT,
        )
        assert trace
    except Exception:
        try:
            _collect_isvc_diagnostics(kserve_client, service_name, namespace)
        except Exception:
            logger.warning("Failed to collect tracing diagnostics", exc_info=True)
        raise
    finally:
        _safe_delete(kserve_client, service_name, namespace)


@pytest.mark.raw
@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_omitted_tracing_has_no_instrumentation_or_spans(
    kserve_client,
    rest_v1_client,
    network_layer,
    test_namespace,
):
    namespace = test_namespace
    service_name = f"trace-disabled-{uuid.uuid4().hex[:8]}"
    try:
        kserve_client.create(_make_untraced_sklearn_isvc(service_name, namespace))
        kserve_client.wait_isvc_ready(
            service_name,
            namespace=namespace,
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
            namespace,
        )
        assert response["predictions"] == [1, 1]
        assert _response_header(response_headers, "traceparent") is None

        pods = kserve_client.core_api.list_namespaced_pod(
            namespace,
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
        _assert_no_traces_for_service(
            service_name, namespace, TRACE_ABSENCE_OBSERVATION_SECONDS
        )
    except Exception:
        try:
            _collect_isvc_diagnostics(kserve_client, service_name, namespace)
        except Exception:
            logger.warning("Failed to collect tracing diagnostics", exc_info=True)
        raise
    finally:
        _safe_delete(kserve_client, service_name, namespace)
