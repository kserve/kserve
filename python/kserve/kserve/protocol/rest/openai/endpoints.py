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
import time
from typing import AsyncGenerator

from fastapi import APIRouter, FastAPI, Request, Response
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from pydantic import TypeAdapter, ValidationError
from starlette.responses import StreamingResponse

from kserve.protocol.rest.openai.types import (
    ChatCompletionRequest,
    CompletionRequest,
    EmbeddingRequest,
    ErrorResponse,
    Model,
    ModelList,
)

from ....errors import ModelNotReady
from .dataplane import OpenAIDataPlane
from .errors import OpenAIError, openai_error_handler

OPENAI_ROUTE_PREFIX = os.environ.get("KSERVE_OPENAI_ROUTE_PREFIX", "/openai")

if len(OPENAI_ROUTE_PREFIX) > 0 and not OPENAI_ROUTE_PREFIX.startswith("/"):
    OPENAI_ROUTE_PREFIX = f"/{OPENAI_ROUTE_PREFIX}"


CreateCompletionRequestAdapter = TypeAdapter(CompletionRequest)
ChatCompletionRequestAdapter = TypeAdapter(ChatCompletionRequest)
EmbeddingRequestAdapter = TypeAdapter(EmbeddingRequest)


class OpenAIEndpoints:
    def __init__(self, dataplane: OpenAIDataPlane):
        self.dataplane = dataplane
        self.start_time = int(time.time())

    async def create_completion(
        self,
        raw_request: Request,
        request_body: CompletionRequest,
        response: Response,
    ) -> Response:
        """Create completion handler.

        Args:
            raw_request (Request): fastapi request object,
            request_body (CompletionCreateParams): Completion params body.
            response (Response): fastapi response object

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
            raw_request=raw_request,
            headers=raw_request.headers,
            response=response,
        )
        if isinstance(completion, ErrorResponse):
            return JSONResponse(
                content=completion.model_dump(), status_code=int(completion.error.code)
            )
        elif isinstance(completion, AsyncGenerator):
            return StreamingResponse(completion, media_type="text/event-stream")
        else:
            return completion

    async def create_chat_completion(
        self,
        raw_request: Request,
        request_body: ChatCompletionRequest,
        response: Response,
    ) -> Response:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,
            request_body (ChatCompletionRequestAdapter): Chat completion params body.
            response (Response): fastapi response object

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
            raw_request=raw_request,
            headers=request_headers,
            response=response,
        )
        if isinstance(completion, ErrorResponse):
            return JSONResponse(
                content=completion.model_dump(), status_code=int(completion.error.code)
            )
        elif isinstance(completion, AsyncGenerator):
            return StreamingResponse(completion, media_type="text/event-stream")
        else:
            return completion

    async def create_embedding(
        self,
        raw_request: Request,
        request_body: EmbeddingRequest,
        response: Response,
    ) -> Response:
        """Create embedding handler.
        Args:
            raw_request (Request): fastapi request object,
            model_name (str): Model name.
            request_body (EmbeddingRequestAdapter): Embedding params body.
        Returns:
            InferenceResponse: Inference response object.
        """
        try:
            params = EmbeddingRequestAdapter.validate_python(request_body)
        except ValidationError as e:
            raise RequestValidationError(errors=e.errors())
        params = request_body
        model_name = params.model
        model_ready = await self.dataplane.model_ready(model_name)

        if not model_ready:
            raise ModelNotReady(model_name)

        embedding = await self.dataplane.create_embedding(
            model_name=model_name,
            request=params,
            raw_request=raw_request,
            headers=raw_request.headers,
            response=response,
        )
        if isinstance(embedding, ErrorResponse):
            return JSONResponse(
                content=embedding.model_dump(), status_code=int(embedding.error.code)
            )
        elif isinstance(embedding, AsyncGenerator):
            return StreamingResponse(embedding, media_type="text/event-stream")
        else:
            return embedding

    async def models(
        self,
    ) -> ModelList:
        """Create chat completion handler.

        Args:
            raw_request (Request): fastapi request object,

        Returns:
            ModelList: Model response object.
        """
        models = await self.dataplane.models()
        return ModelList(
            object="list",
            data=[
                Model(
                    object="model", id=model.name, created=self.start_time, owned_by=""
                )
                for model in models
            ],
        )

    async def health(self, model_name: str):
        try:
            model_ready = await self.dataplane.model_ready(model_name)
        except Exception as e:
            raise ModelNotReady(model_name) from e
        if not model_ready:
            raise ModelNotReady(model_name)


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
        r"/v1/embeddings",
        endpoints.create_embedding,
        methods=["POST"],
        response_model_exclude_none=True,
        response_model_exclude_unset=True,
    )
    openai_router.add_api_route(
        r"/v1/models",
        endpoints.models,
        methods=["GET"],
    )
    openai_router.add_api_route(
        r"/v1/models/{model_name}", endpoints.health, methods=["GET"]
    )
    app.include_router(openai_router)
    app.add_exception_handler(OpenAIError, openai_error_handler)
