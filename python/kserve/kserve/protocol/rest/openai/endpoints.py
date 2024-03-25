import os
from collections.abc import AsyncIterable
from typing import AsyncGenerator

from fastapi import FastAPI, Request, Response
from openai.types import CompletionCreateParams
from openai.types.chat import \
    CompletionCreateParams as ChatCompletionCreateParams
from starlette.responses import StreamingResponse

from ....errors import ModelNotReady
from .dataplane import OpenAIDataPlane

OPENAI_ROUTE_PREFIX = os.environ.get("KSERVE_OPENAI_ROUTE_PREFIX", "/openai")

if len(OPENAI_ROUTE_PREFIX) > 0 and not OPENAI_ROUTE_PREFIX.startswith("/"):
    OPENAI_ROUTE_PREFIX = f"/{OPENAI_ROUTE_PREFIX}"


class OpenAIEndpoints:
    def __init__(self, dataplane: OpenAIDataPlane):
        self.dataplane = dataplane

    async def create_completion(
        self,
        raw_request: Request,
        request_body: CompletionCreateParams,
    ) -> Response:
        """Create completion handler.

        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (CompletionCreateParams): Completion params body.

        Returns:
            InferenceResponse: Inference response object.
        """
        model_name = request_body.get("model")
        model_ready = self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        request_headers = dict(raw_request.headers)
        completion = await self.dataplane.create_completion(
            model_name=model_name, request=request_body, headers=request_headers
        )
        if isinstance(completion, AsyncIterable):

            async def stream_results() -> AsyncGenerator[str, None]:
                async for partial_completion in completion:
                    yield f"data: {partial_completion.model_dump_json()}\n\n"

            return StreamingResponse(stream_results())
        else:
            return completion

    async def create_chat_completion(
        self,
        raw_request: Request,
        request_body: ChatCompletionCreateParams,
    ) -> Response:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (ChatCompletionCreateParams): Chat completion params body.

        Returns:
            InferenceResponse: Inference response object.
        """
        model_name = request_body.get("model")
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

            return StreamingResponse(stream_results())
        else:
            return completion


def register_openai_endpoints(app: FastAPI, dataplane: OpenAIDataPlane):
    endpoints = OpenAIEndpoints(dataplane)
    app.add_api_route(
        f"{OPENAI_ROUTE_PREFIX}" + r"/v1/completions",
        endpoints.create_completion,
        methods=["POST"],
        tags=["OpenAI"],
    )
    app.add_api_route(
        f"{OPENAI_ROUTE_PREFIX}" + r"/v1/completions",
        endpoints.create_chat_completion,
        methods=["POST"],
        tags=["OpenAI"],
    )
