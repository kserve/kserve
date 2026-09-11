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

"""Tracing utilities for the KServe REST server."""

from __future__ import annotations

import os
from typing import Optional

from opentelemetry import trace
from opentelemetry.environment_variables import OTEL_TRACES_EXPORTER
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import (
    OTLPSpanExporter as OTLPGrpcSpanExporter,
)
from opentelemetry.exporter.otlp.proto.http.trace_exporter import (
    OTLPSpanExporter as OTLPHttpSpanExporter,
)
from opentelemetry.sdk.environment_variables import (
    OTEL_EXPORTER_OTLP_ENDPOINT,
    OTEL_EXPORTER_OTLP_PROTOCOL,
    OTEL_EXPORTER_OTLP_TRACES_ENDPOINT,
    OTEL_EXPORTER_OTLP_TRACES_PROTOCOL,
    OTEL_SDK_DISABLED,
)
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import (
    BatchSpanProcessor,
    ConsoleSpanExporter,
    SimpleSpanProcessor,
    SpanExporter,
)

from kserve.constants.constants import KSERVE_MODEL_SERVER_NAME
from kserve.logging import logger


def _is_truthy(value: Optional[str]) -> bool:
    return value is not None and value.strip().lower() in {
        "true",
        "1",
        "yes",
        "on",
    }


def _configured_exporter_names() -> tuple[str, ...]:
    """Return the configured trace exporters.

    KServe deliberately does not create an implicit OTLP exporter. This keeps a
    model server with no tracing configuration from trying to connect to the
    SDK's localhost default. Setting either a standard OTLP endpoint or
    OTEL_TRACES_EXPORTER explicitly opts into exporting.
    """

    configured_exporters = os.getenv(OTEL_TRACES_EXPORTER, "").strip()
    if configured_exporters == "":
        if os.getenv(OTEL_EXPORTER_OTLP_ENDPOINT) or os.getenv(
            OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
        ):
            return ("otlp",)
        return ()

    exporters = tuple(
        exporter.strip().lower() for exporter in configured_exporters.split(",")
    )
    if "none" in exporters and exporters != ("none",):
        raise ValueError(
            f"{OTEL_TRACES_EXPORTER}=none cannot be combined with another exporter"
        )
    unsupported = set(exporters) - {"none", "otlp", "console"}
    if unsupported:
        raise ValueError(
            f"Unsupported OpenTelemetry trace exporter(s): {', '.join(sorted(unsupported))}"
        )
    return () if exporters == ("none",) else exporters


def _create_otlp_exporter() -> SpanExporter:
    """Create an OTLP exporter using the standard OpenTelemetry environment."""

    protocol = (
        os.getenv(
            OTEL_EXPORTER_OTLP_TRACES_PROTOCOL,
            os.getenv(OTEL_EXPORTER_OTLP_PROTOCOL, "grpc"),
        )
        .strip()
        .lower()
        or "grpc"
    )
    if protocol == "grpc":
        # The exporter reads endpoint, TLS, headers, and timeout settings from
        # the standard OTEL_EXPORTER_OTLP_* environment variables.
        return OTLPGrpcSpanExporter()
    if protocol == "http/protobuf":
        return OTLPHttpSpanExporter()

    raise ValueError(
        f"Unsupported OpenTelemetry OTLP protocol: {protocol!r}; "
        "supported protocols are 'grpc' and 'http/protobuf'"
    )


_TRACING_INITIALIZED = False
_TRACER_PROVIDER: Optional[TracerProvider] = None


def get_tracer_provider() -> Optional[TracerProvider]:
    """Return the OpenTelemetry TracerProvider, or None if tracing is disabled."""
    global _TRACER_PROVIDER, _TRACING_INITIALIZED
    if _TRACING_INITIALIZED:
        return _TRACER_PROVIDER

    _TRACING_INITIALIZED = True
    try:
        configured_exporters = _configured_exporter_names()
        span_processors = [
            BatchSpanProcessor(_create_otlp_exporter())
            if name == "otlp"
            else SimpleSpanProcessor(ConsoleSpanExporter())
            for name in configured_exporters
        ]
    except ValueError as error:
        logger.warning(
            "OpenTelemetry tracing disabled due to invalid configuration: %s",
            error,
        )
        return None

    if _is_truthy(os.getenv(OTEL_SDK_DISABLED)):
        logger.info("OpenTelemetry SDK disabled via 'OTEL_SDK_DISABLED'")
    else:
        tracer_provider = TracerProvider(
            resource=Resource.create({SERVICE_NAME: KSERVE_MODEL_SERVER_NAME})
        )
        trace.set_tracer_provider(tracer_provider)
        for processor in span_processors:
            tracer_provider.add_span_processor(processor)
        if configured_exporters:
            logger.info(
                "OpenTelemetry trace exporters configured: %s",
                ", ".join(configured_exporters),
            )
        else:
            logger.info("OpenTelemetry trace exporting disabled")

        _TRACER_PROVIDER = tracer_provider
    return _TRACER_PROVIDER
