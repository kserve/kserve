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

import pytest

from kserve.protocol import tracing
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
