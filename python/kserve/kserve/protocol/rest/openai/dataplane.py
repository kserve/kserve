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

from http import HTTPStatus
from typing import AsyncGenerator, List, Union

from fastapi import Request, Response
from starlette.datastructures import Headers

from kserve.protocol.rest.openai.errors import create_error_response
from kserve.protocol.rest.openai.types import (
    ChatCompletion,
    ChatCompletionRequest,
    Completion,
    CompletionRequest,
    Embedding,
    EmbeddingRequest,
    ErrorResponse,
    RerankRequest,
    Rerank,
)
from ...dataplane import DataPlane
from .openai_model import (
    OpenAIModel,
    OpenAIGenerativeModel,
    OpenAIEncoderModel,
)


class OpenAIDataPlane(DataPlane):
    """OpenAI DataPlane"""

    async def create_completion(
        self,
        model_name: str,
        request: CompletionRequest,
        raw_request: Request,
        headers: Headers,
        response: Response,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (CompletionRequest): Params to create a completion.
            raw_request (Request): fastapi request object.
            headers: (Headers): Request headers.
            response: (Response): FastAPI response object
        Returns:
            response: A non-streaming or streaming completion response or an error response.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAIGenerativeModel):
            return create_error_response(
                message=f"Model {model_name} does not support Completions API",
                status_code=HTTPStatus.BAD_REQUEST,
            )

        context = {"headers": dict(headers), "response": response}
        return await model.create_completion(
            request=request, raw_request=raw_request, context=context
        )

    async def create_chat_completion(
        self,
        model_name: str,
        request: ChatCompletionRequest,
        raw_request: Request,
        headers: Headers,
        response: Response,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (CreateChatCompletionRequest): Params to create a chat completion.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: A non-streaming or streaming chat completion response or an error response.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAIGenerativeModel):
            return create_error_response(
                message=f"Model {model_name} does not support Chat Completion API",
                status_code=HTTPStatus.BAD_REQUEST,
            )

        context = {"headers": dict(headers), "response": response}
        return await model.create_chat_completion(
            request=request, raw_request=raw_request, context=context
        )

    async def create_embedding(
        self,
        model_name: str,
        request: EmbeddingRequest,
        raw_request: Request,
        headers: Headers,
        response: Response,
    ) -> Union[AsyncGenerator[str, None], Embedding, ErrorResponse]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (EmbeddingRequest): Params to create a embedding.
            raw_request (Request): fastapi request object.
            headers: (Headers): Request headers.
            response: (Response): FastAPI response object
        Returns:
            response: A non-streaming or streaming embedding response or an error response.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAIEncoderModel):
            return create_error_response(
                message=f"Model {model_name} does not support Embeddings API",
                status_code=HTTPStatus.BAD_REQUEST,
            )

        context = {"headers": dict(headers), "response": response}
        return await model.create_embedding(
            request=request, raw_request=raw_request, context=context
        )

    async def create_rerank(
        self,
        model_name: str,
        request: RerankRequest,
        raw_request: Request,
        headers: Headers,
        response: Response,
    ) -> Union[AsyncGenerator[str, None], Rerank, ErrorResponse]:
        """Generate the text with the provided text prompt.
        Args:
            model_name (str): Model name.
            request (RerankRequest): Params to create rerank response.
            raw_request (Request): fastapi request object.
            headers: (Headers): Request headers.
            response: (Response): FastAPI response object
        Returns:
            Returns:
            response: A non-streaming or streaming embedding response or an error response.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAIEncoderModel):
            return create_error_response(
                message=f"Model {model_name} does not support Rerank API",
                status_code=HTTPStatus.BAD_REQUEST,
            )

        context = {"headers": dict(headers), "response": response}
        return await model.create_rerank(
            request=request, raw_request=raw_request, context=context
        )

    async def models(self) -> List[OpenAIModel]:
        """Retrieve a list of models

        Returns:
            response: A list of OpenAIModel instances
        """
        return [
            model
            for model in self.model_registry.get_models().values()
            if isinstance(model, OpenAIModel)
        ]
