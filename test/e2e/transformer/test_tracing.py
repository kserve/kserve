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

import os
import uuid

import pytest
from kserve import (
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
from kubernetes.client import V1Container, V1ResourceRequirements

from ..common.tracing import (
    JAEGER_OTLP_ENDPOINT,
    _collect_isvc_diagnostics,
    _infer_v1,
    _response_header,
    _safe_delete,
    _trace_id_from_traceparent,
    _wait_for_connected_trace,
    _wait_for_parent_trace,
    logger,
)


def _make_traced_sklearn_isvc(service_name, namespace):
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
            predictor=V1beta1PredictorSpec(
                min_replicas=1,
                model=V1beta1ModelSpec(
                    model_format=V1beta1ModelFormat(name="sklearn"),
                    storage_uri="s3://example-models/sklearn/",
                    protocol_version="v1",
                    resources=V1ResourceRequirements(
                        requests={"cpu": "50m", "memory": "128Mi"},
                        limits={"cpu": "100m", "memory": "256Mi"},
                    ),
                ),
            ),
            transformer=V1beta1TransformerSpec(
                min_replicas=1,
                containers=[
                    V1Container(
                        name="kserve-container",
                        image=os.environ["IMAGE_TRANSFORMER_IMG_TAG"],
                        command=[
                            "python",
                            "-m",
                            "custom_transformer.trace_transformer",
                        ],
                        args=["--model_name", service_name],
                        resources=V1ResourceRequirements(
                            requests={"cpu": "100m", "memory": "512Mi"},
                            limits={"cpu": "1", "memory": "1Gi"},
                        ),
                    )
                ],
            ),
            tracing=V1beta1TracingSpec(
                exporter="otlp",
                exporter_endpoint=JAEGER_OTLP_ENDPOINT,
                sampler="parentbased_traceidratio",
                sampler_arg="1.0",
            ),
        ),
    )


@pytest.fixture
def traced_sklearn_isvc(kserve_client, test_namespace):
    namespace = test_namespace
    # Function scope keeps service/trace ownership independent across xdist workers.
    service_name = f"trace-sklearn-{uuid.uuid4().hex[:8]}"
    try:
        kserve_client.create(_make_traced_sklearn_isvc(service_name, namespace))
        kserve_client.wait_isvc_ready(service_name, namespace=namespace)
        yield service_name
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
async def test_transformer_predictor_trace_is_connected(
    kserve_client,
    rest_v1_client,
    network_layer,
    traced_sklearn_isvc,
    test_namespace,
):
    service_name = traced_sklearn_isvc
    response, response_headers = await _infer_v1(
        kserve_client,
        rest_v1_client,
        service_name,
        {"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]},
        network_layer,
        namespace=test_namespace,
    )
    assert response["predictions"] == [1, 1]

    traceparent = _response_header(response_headers, "traceparent")
    assert traceparent, f"KServe tracing response header missing: {response_headers!r}"
    trace_id = _trace_id_from_traceparent(traceparent)
    trace = _wait_for_connected_trace(service_name, trace_id, test_namespace)
    assert trace.get("traceID") == trace_id


@pytest.mark.raw
@pytest.mark.tracing
@pytest.mark.asyncio(scope="session")
async def test_inbound_traceparent_is_preserved_through_transformer_and_predictor(
    kserve_client,
    rest_v1_client,
    network_layer,
    traced_sklearn_isvc,
    test_namespace,
):
    service_name = traced_sklearn_isvc
    trace_id = uuid.uuid4().hex
    parent_span_id = uuid.uuid4().hex[:16]
    traceparent = f"00-{trace_id}-{parent_span_id}-01"
    response, response_headers = await _infer_v1(
        kserve_client,
        rest_v1_client,
        service_name,
        {"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]},
        network_layer,
        namespace=test_namespace,
        extra_headers={"traceparent": traceparent},
    )
    assert response["predictions"] == [1, 1]
    returned_traceparent = _response_header(response_headers, "traceparent")
    assert returned_traceparent, (
        f"KServe tracing response header missing: {response_headers!r}"
    )
    assert _trace_id_from_traceparent(returned_traceparent) == trace_id

    trace = _wait_for_parent_trace(
        service_name, trace_id, parent_span_id, test_namespace
    )
    assert trace.get("traceID") == trace_id
