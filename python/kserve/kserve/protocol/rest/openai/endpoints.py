# Copyright 2023 The KServe Authors.
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
from collections.abc import AsyncIterable
from typing import AsyncGenerator
import time

from fastapi import APIRouter, FastAPI, Request, Response
from fastapi.exceptions import RequestValidationError
from pydantic import TypeAdapter, ValidationError
from starlette.responses import StreamingResponse

from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionRequest,
    CreateCompletionRequest,
    ListModelsResponse,
    Model,
)

from ....errors import ModelNotReady
from .dataplane import OpenAIDataPlane
from .errors import OpenAIError, openai_error_handler

OPENAI_ROUTE_PREFIX = os.environ.get("KSERVE_OPENAI_ROUTE_PREFIX", "/openai")

if len(OPENAI_ROUTE_PREFIX) > 0 and not OPENAI_ROUTE_PREFIX.startswith("/"):
    OPENAI_ROUTE_PREFIX = f"/{OPENAI_ROUTE_PREFIX}"


CreateCompletionRequestAdapter = TypeAdapter(CreateCompletionRequest)
ChatCompletionRequestAdapter = TypeAdapter(CreateChatCompletionRequest)


class OpenAIEndpoints:
    def __init__(self, dataplane: OpenAIDataPlane):
        self.dataplane = dataplane
        self.start_time = int(time.time())

    async def create_completion(
        self,
        raw_request: Request,
        request_body: CreateCompletionRequest,
        response: Response,
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
            params = CreateCompletionRequestAdapter.validate_python(request_body)
        except ValidationError as e:
            raise RequestValidationError(errors=e.errors())
        params = request_body
        model_name = params.model
        model_ready = await self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        completion = await self.dataplane.create_completion(
            model_name=model_name,
            request=params,
            headers=raw_request.headers,
            response=response,
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
        request_body: CreateChatCompletionRequest,
        response: Response,
    ) -> Response:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (ChatCompletionRequestAdapter): Chat completion params body.

        Returns:
            InferenceResponse: Inference response object.
        """
        try:
            params = ChatCompletionRequestAdapter.validate_python(request_body)
        except ValidationError as e:
            raise RequestValidationError(errors=e.errors())
        params = request_body
        model_name = params.model
        model_ready = await self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        request_headers = raw_request.headers
        completion = await self.dataplane.create_chat_completion(
            model_name=model_name,
            request=request_body,
            headers=request_headers,
            response=response,
        )
        if isinstance(completion, AsyncIterable):

            async def stream_results() -> AsyncGenerator[str, None]:
                async for chunk in completion:
                    yield f"data: {chunk.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return StreamingResponse(stream_results(), media_type="text/event-stream")
        else:
            return completion

    async def models(
        self,
    ) -> ListModelsResponse:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,

        Returns:
            ListModelsResponse: Model response object.
        """
        models = await self.dataplane.models()
        return ListModelsResponse(
            object="list",
            data=[
                Model(
                    object="model", id=model.name, created=self.start_time, owned_by=""
                )
                for model in models
            ],
        )


def register_openai_endpoints(app: FastAPI, dataplane: OpenAIDataPlane):
    endpoints = OpenAIEndpoints(dataplane)
    openai_router = APIRouter(prefix=OPENAI_ROUTE_PREFIX, tags=["OpenAI"])
    openai_router.add_api_route(
        r"/v1/completions",
        endpoints.create_completion,
        methods=["POST"],
        response_model_exclude_none=True,
        response_model_exclude_unset=True,
    )
    openai_router.add_api_route(
        r"/v1/chat/completions",
        endpoints.create_chat_completion,
        methods=["POST"],
        response_model_exclude_none=True,
        response_model_exclude_unset=True,
    )
    openai_router.add_api_route(
        r"/v1/models",
        endpoints.models,
        methods=["GET"],
    )
    app.include_router(openai_router)
    app.add_exception_handler(OpenAIError, openai_error_handler)
