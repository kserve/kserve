import asyncio
import os
from collections.abc import AsyncIterable
from typing import AsyncGenerator, Dict

from fastapi import APIRouter, FastAPI, Request, Response
from fastapi.exceptions import RequestValidationError
from openai.types import CompletionCreateParams
from openai.types.chat import \
    CompletionCreateParams as ChatCompletionCreateParams
from pydantic import TypeAdapter, ValidationError
from starlette.responses import StreamingResponse

from ....errors import ModelNotReady
from .dataplane import OpenAIDataPlane

OPENAI_ROUTE_PREFIX = os.environ.get("KSERVE_OPENAI_ROUTE_PREFIX", "/openai")

if len(OPENAI_ROUTE_PREFIX) > 0 and not OPENAI_ROUTE_PREFIX.startswith("/"):
    OPENAI_ROUTE_PREFIX = f"/{OPENAI_ROUTE_PREFIX}"


CompletionCreateParamsAdapter = TypeAdapter(CompletionCreateParams)
ChatCompletionCreateParamsAdapter = TypeAdapter(ChatCompletionCreateParams)


class OpenAIEndpoints:
    def __init__(self, dataplane: OpenAIDataPlane):
        self.dataplane = dataplane

    async def create_completion(
        self,
        raw_request: Request,
        request_body: Dict,
    ) -> Response:
        """Create completion handler.

        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (CompletionCreateParams): Completion params body.

        Returns:
            InferenceResponse: Inference response object.
        """
        try:
            params = CompletionCreateParamsAdapter.validate_python(request_body)
        except ValidationError as e:
            raise RequestValidationError(errors=e.errors())
        model_name = params["model"]
        model_ready = self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        request_headers = dict(raw_request.headers)
        completion = await self.dataplane.create_completion(
            model_name=model_name, request=params, headers=request_headers
        )
        if isinstance(completion, AsyncIterable):

            async def stream_results() -> AsyncGenerator[str, None]:
                async for partial_completion in completion:
                    yield f"data: {partial_completion.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return StreamingResponse(stream_results(), media_type="text/event-stream")
        else:
            return completion

    async def create_chat_completion(
        self,
        raw_request: Request,
        request_body: Dict,
    ) -> Response:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (ChatCompletionCreateParams): Chat completion params body.

        Returns:
            InferenceResponse: Inference response object.
        """
        try:
            params = ChatCompletionCreateParamsAdapter.validate_python(request_body)
        except ValidationError as e:
            raise RequestValidationError(errors=e.errors())
        model_name = params["model"]
        model_ready = self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        request_headers = dict(raw_request.headers)
        completion = await self.dataplane.create_chat_completion(
            model_name=model_name, request=request_body, headers=request_headers
        )
        if isinstance(completion, AsyncIterable):

            async def stream_results() -> AsyncGenerator[str, None]:
                async for chunk in completion:
                    yield f"data: {chunk.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return StreamingResponse(stream_results(), media_type="text/event-stream")
        else:
            return completion


def register_openai_endpoints(app: FastAPI, dataplane: OpenAIDataPlane):
    endpoints = OpenAIEndpoints(dataplane)
    openai_router = APIRouter(prefix=OPENAI_ROUTE_PREFIX, tags=["OpenAI"])
    openai_router.add_api_route(
        r"/v1/completions",
        endpoints.create_completion,
        methods=["POST"],
    )
    openai_router.add_api_route(
        r"/v1/chat/completions",
        endpoints.create_chat_completion,
        methods=["POST"],
    )
    app.include_router(openai_router)
