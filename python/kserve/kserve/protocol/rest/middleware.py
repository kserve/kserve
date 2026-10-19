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

import logging
import os
from typing import Optional

import fastapi
from fastapi import Request
from opentelemetry import trace
from opentelemetry.trace import format_span_id, format_trace_id
from starlette.middleware.base import BaseHTTPMiddleware

module_logger = logging.getLogger(__name__)

TRACE_RESPONSE_HEADER_ENV = "TRACE_RESPONSE_HEADER_NAME"
TRACE_RESPONSE_HEADER_NAME = os.getenv(TRACE_RESPONSE_HEADER_ENV, "traceparent")

TRACE_RESPONSE_TRACESTATE_HEADER_ENV = "TRACE_RESPONSE_TRACESTATE_HEADER_NAME"
TRACE_RESPONSE_TRACESTATE_HEADER_NAME = os.getenv(
    TRACE_RESPONSE_TRACESTATE_HEADER_ENV, "tracestate"
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
