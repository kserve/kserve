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

import re
from unittest.mock import Mock

from fastapi import FastAPI
from fastapi.testclient import TestClient
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.trace import TracerProvider
import pytest

from kserve.protocol import tracing
from kserve.protocol.rest import server as rest_server
from kserve.protocol.rest.middleware import TraceResponseHeaderMiddleware
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


def test_missing_tracing_configuration_disables_instrumentation(monkeypatch):
    set_tracer_provider = Mock()
    monkeypatch.setattr(tracing.trace, "set_tracer_provider", set_tracer_provider)

    assert tracing.get_tracer_provider() is None

    set_tracer_provider.assert_not_called()


def test_explicit_none_keeps_instrumentation_without_exporters(monkeypatch):
    tracer_provider = Mock()
    tracer_provider_class = Mock(return_value=tracer_provider)
    set_tracer_provider = Mock()
    monkeypatch.setattr(tracing, "TracerProvider", tracer_provider_class)
    monkeypatch.setattr(tracing.trace, "set_tracer_provider", set_tracer_provider)
    monkeypatch.setenv(tracing.OTEL_TRACES_EXPORTER, "none")

    assert tracing.get_tracer_provider() is tracer_provider

    tracer_provider_class.assert_called_once()
    tracer_provider.add_span_processor.assert_not_called()
    set_tracer_provider.assert_called_once_with(tracer_provider)


@pytest.mark.parametrize("provider_configured", [False, True])
def test_response_header_middleware_requires_tracing_provider(
    monkeypatch, provider_configured
):
    provider = Mock() if provider_configured else None
    monkeypatch.setattr(rest_server, "get_tracer_provider", lambda: provider)
    app = FastAPI()
    server = rest_server.RESTServer(
        app="dummy:app",
        data_plane=Mock(),
        model_repository_extension=Mock(),
        http_port=8080,
    )

    server._add_middlewares(app)

    middleware_classes = {middleware.cls for middleware in app.user_middleware}
    assert (TraceResponseHeaderMiddleware in middleware_classes) is provider_configured


def test_enabled_tracing_adds_response_trace_header():
    app = FastAPI()

    @app.get("/inference")
    async def inference():
        return {"ok": True}

    FastAPIInstrumentor.instrument_app(app, tracer_provider=TracerProvider())
    app.add_middleware(TraceResponseHeaderMiddleware)

    response = TestClient(app).get("/inference")

    assert response.status_code == 200
    assert re.fullmatch(
        r"00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}",
        response.headers["traceparent"],
    )


@pytest.mark.parametrize(
    "url",
    [
        "/",
        "/metrics",
        "/v2/health/live",
        "/v2/health/ready",
    ],
)
def test_rest_trace_exclusions_match_non_inference_routes(url):
    pattern = re.compile("|".join(REST_TRACE_EXCLUDED_URLS))

    assert pattern.search(url)


@pytest.mark.parametrize(
    "url",
    [
        "/v2",
        "/v2/models/model",
        "/v2/models/model/infer",
        "/v1/models/model:predict",
        "/.well-known/appspecific/com.chrome.devtools.json",
        "/not-found",
    ],
)
def test_rest_trace_exclusions_preserve_inference_and_unknown_routes(url):
    pattern = re.compile("|".join(REST_TRACE_EXCLUDED_URLS))

    assert not pattern.search(url)
