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

from unittest.mock import Mock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import (
    InMemorySpanExporter,
)
from opentelemetry.trace import SpanKind
from opentelemetry.util.http import parse_excluded_urls

from kserve import Model, ModelRepository
from kserve.protocol import tracing
from kserve.protocol.dataplane import DataPlane
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.protocol.rest import server as rest_server
from kserve.protocol.rest.server import REST_TRACE_EXCLUDED_URLS


@pytest.fixture(autouse=True)
def reset_tracing(monkeypatch):
    for variable in (
        tracing.OTEL_TRACES_EXPORTER,
        tracing.OTEL_EXPORTER_OTLP_ENDPOINT,
        tracing.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT,
        tracing.OTEL_EXPORTER_OTLP_PROTOCOL,
        tracing.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL,
        tracing.OTEL_SDK_DISABLED,
    ):
        monkeypatch.delenv(variable, raising=False)
    monkeypatch.setattr(tracing, "_TRACER_PROVIDER", None)
    monkeypatch.setattr(tracing, "_TRACING_INITIALIZED", False)


def test_trace_specific_otlp_protocol_takes_precedence(monkeypatch):
    grpc_exporter = Mock()
    http_exporter = Mock(return_value=Mock())
    monkeypatch.setattr(tracing, "OTLPGrpcSpanExporter", grpc_exporter)
    monkeypatch.setattr(tracing, "OTLPHttpSpanExporter", http_exporter)
    monkeypatch.setenv(tracing.OTEL_EXPORTER_OTLP_PROTOCOL, "grpc")
    monkeypatch.setenv(tracing.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL, "http/protobuf")

    tracing._create_otlp_exporter()

    http_exporter.assert_called_once_with()
    grpc_exporter.assert_not_called()


def test_invalid_tracing_configuration_is_logged_once(monkeypatch, caplog):
    set_tracer_provider = Mock()
    monkeypatch.setattr(tracing.trace, "set_tracer_provider", set_tracer_provider)
    monkeypatch.setenv(tracing.OTEL_TRACES_EXPORTER, "otlp")
    monkeypatch.setenv(tracing.OTEL_EXPORTER_OTLP_TRACES_PROTOCOL, "invalid")

    assert tracing.get_tracer_provider() is None
    assert tracing.get_tracer_provider() is None

    assert (
        caplog.messages.count(
            "OpenTelemetry tracing disabled due to invalid configuration: "
            "Unsupported OpenTelemetry OTLP protocol: 'invalid'; supported protocols are "
            "'grpc' and 'http/protobuf'"
        )
        == 1
    )
    set_tracer_provider.assert_not_called()


# The OpenTelemetry ASGI middleware checks exclusions against the full request
# URL, so the tests use the same matcher and URL shape.
@pytest.mark.parametrize(
    "url",
    [
        "http://127.0.0.1:8080/",
        "http://10.0.0.5:8080/metrics",
        "https://model.default.example.com/v2/health/live",
        "http://localhost/v2/health/ready",
    ],
)
def test_rest_trace_exclusions_match_non_inference_routes(url):
    excluded_urls = parse_excluded_urls(",".join(REST_TRACE_EXCLUDED_URLS))

    assert excluded_urls.url_disabled(url)


@pytest.mark.parametrize(
    "url",
    [
        "http://localhost/v2",
        "http://localhost/v2/models/model",
        "http://localhost/v2/models/model/infer",
        "http://localhost/v1/models/model:predict",
        "http://localhost/.well-known/appspecific/com.chrome.devtools.json",
        "http://localhost/not-found",
        "http://localhost/metrics/extra",
    ],
)
def test_rest_trace_exclusions_preserve_inference_and_unknown_routes(url):
    excluded_urls = parse_excluded_urls(",".join(REST_TRACE_EXCLUDED_URLS))

    assert not excluded_urls.url_disabled(url)


class _ReadyModel(Model):
    def __init__(self, name):
        super().__init__(name)
        self.ready = True

    async def predict(self, payload, headers=None):
        return {"predictions": payload["instances"]}


def test_rest_server_skips_spans_for_health_and_metrics_routes(monkeypatch):
    exporter = InMemorySpanExporter()
    tracer_provider = TracerProvider()
    tracer_provider.add_span_processor(SimpleSpanProcessor(exporter))
    monkeypatch.setattr(rest_server, "get_tracer_provider", lambda: tracer_provider)
    model_repository = ModelRepository()
    model_repository.update(_ReadyModel("model"))
    server = rest_server.RESTServer(
        app="dummy:app",
        data_plane=DataPlane(model_registry=model_repository),
        model_repository_extension=ModelRepositoryExtension(model_repository),
        http_port=8080,
    )
    app = FastAPI()
    server._register_endpoints(app)
    client = TestClient(app)

    for path in ("/", "/metrics", "/v2/health/live", "/v2/health/ready"):
        assert client.get(path).status_code == 200
    assert not exporter.get_finished_spans()

    response = client.post("/v1/models/model:predict", json={"instances": [[1, 2]]})

    assert response.status_code == 200
    server_spans = [
        span.name
        for span in exporter.get_finished_spans()
        if span.kind == SpanKind.SERVER
    ]
    assert server_spans == ["POST /v1/models/{model_name}:predict"]
