import contextvars
import logging
from contextlib import contextmanager
from typing import Any

from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import Response, JSONResponse

REQUEST_ID = 'request_id'

request_id_context: contextvars.ContextVar[frozenset] = contextvars.ContextVar(
    REQUEST_ID, default=frozenset()
)


def reset_context(state: contextvars.Token):
    """Resets the state of the logging context to that at the given state."""
    request_id_context.reset(state)


def clear_context():
    """Resets the state of the logging context to empty."""
    request_id_context.set(frozenset())


def update_context(**kwargs) -> contextvars.Token:
    """Updates the state of the logging context.
    Returns a token that can be used to recover the previous state using `reset_context`
    """
    context = request_id_context.get()
    return request_id_context.set(context.union(kwargs.items()))


def get_from_context(key: str, default: Any = None) -> Any:
    """Fetch a key from the logging context variable"""
    ctx = request_id_context.get()
    for k, v in ctx:
        if k == key:
            return v
    return default


@contextmanager
def logging_context(**kwargs):
    token = update_context(**kwargs)
    try:
        yield
    finally:
        reset_context(token)


class LoggingMiddleware(BaseHTTPMiddleware):
    async def dispatch(
            self, request: Request, call_next: RequestResponseEndpoint
    ) -> Response:
        request_id = request.headers.get(REQUEST_ID, 'NA')
        with logging_context(request_id=request_id):
            response: Response = await call_next(request)
            response.headers[REQUEST_ID] = request_id
            return response
