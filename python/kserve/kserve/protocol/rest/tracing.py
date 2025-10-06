# Copyright 2022 The KServe Authors.
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

import logging
import os
from typing import Optional

import fastapi
from fastapi import Request
from starlette.middleware.base import BaseHTTPMiddleware

from opentelemetry import trace
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace import format_span_id, format_trace_id
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

module_logger = logging.getLogger(__name__)

tracer_provider = TracerProvider(resource=Resource.create({SERVICE_NAME: "kserve"}))
trace.set_tracer_provider(tracer_provider)

OTEL_COLLECTOR_ENDPOINT_ENV = "OTEL_COLLECTOR_ENDPOINT"
otel_collector_endpoint = os.getenv(OTEL_COLLECTOR_ENDPOINT_ENV, "localhost:4317")

TRACE_RESPONSE_HEADER_ENV = "TRACE_RESPONSE_HEADER_NAME"
TRACE_RESPONSE_HEADER_NAME = os.getenv(TRACE_RESPONSE_HEADER_ENV, "traceparent")

TRACE_RESPONSE_TRACESTATE_HEADER_ENV = "TRACE_RESPONSE_TRACESTATE_HEADER_NAME"
TRACE_RESPONSE_TRACESTATE_HEADER_NAME = os.getenv(
    TRACE_RESPONSE_TRACESTATE_HEADER_ENV, "tracestate"
)

ENABLE_OTEL_EXPORTER_ENV = "ENABLE_OTEL_EXPORTER"
ENABLE_OTEL_EXPORTER = os.getenv(ENABLE_OTEL_EXPORTER_ENV, "true").lower() in {
    "true",
    "1",
    "yes",
    "on",
}

if ENABLE_OTEL_EXPORTER:
    otlp_exporter = OTLPSpanExporter(endpoint=otel_collector_endpoint, insecure=True)
    span_processor = BatchSpanProcessor(otlp_exporter)
    tracer_provider.add_span_processor(span_processor)
else:
    module_logger.info(
        "OpenTelemetry exporter disabled via %s", ENABLE_OTEL_EXPORTER_ENV
    )


def instrument_app(app: fastapi.FastAPI) -> None:
    """Attach OpenTelemetry instrumentation to the given FastAPI app."""

    FastAPIInstrumentor.instrument_app(app, tracer_provider=tracer_provider)
    module_logger.info("Opentelemetry tracing enabled")
    if ENABLE_OTEL_EXPORTER:
        module_logger.info(
            "OpenTelemetry exporter enabled. Exporting to %s", otel_collector_endpoint
        )


class TraceResponseHeaderMiddleware(BaseHTTPMiddleware):
    """ASGI middleware that adds W3C Trace Context headers to every response."""

    def __init__(
        self,
        app: fastapi.FastAPI,
        header_name: Optional[str] = None,
        tracestate_header_name: Optional[str] = None,
    ):
        super().__init__(app)
        self._header_name = header_name or TRACE_RESPONSE_HEADER_NAME
        self._tracestate_header_name = (
            tracestate_header_name or TRACE_RESPONSE_TRACESTATE_HEADER_NAME
        )

    async def dispatch(self, request: Request, call_next):
        span = trace.get_current_span()
        response = await call_next(request)

        if span is None:
            return response

        span_context = span.get_span_context()
        if not (span_context and span_context.is_valid):
            return response

        trace_id = format_trace_id(span_context.trace_id)
        span_id = format_span_id(span_context.span_id)
        trace_flags = f"{int(span_context.trace_flags):02x}"

        if self._header_name and trace_id and span_id:
            # Compose the W3C traceparent header from the span context
            traceparent_value = f"00-{trace_id}-{span_id}-{trace_flags}"
            response.headers[self._header_name] = traceparent_value

        if self._tracestate_header_name and span_context.trace_state:
            # Propagate additional vendor state per the W3C Trace Context specification
            tracestate_value = str(span_context.trace_state)
            if tracestate_value:
                response.headers[self._tracestate_header_name] = tracestate_value

        return response


__all__ = [
    "ENABLE_OTEL_EXPORTER",
    "ENABLE_OTEL_EXPORTER_ENV",
    "TRACE_RESPONSE_HEADER_NAME",
    "TRACE_RESPONSE_HEADER_ENV",
    "TRACE_RESPONSE_TRACESTATE_HEADER_NAME",
    "TRACE_RESPONSE_TRACESTATE_HEADER_ENV",
    "TraceResponseHeaderMiddleware",
    "instrument_app",
]
