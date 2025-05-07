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

from typing import AsyncIterator, Union, List

from fastapi import Response
from starlette.datastructures import Headers

from kserve.protocol.rest.openai.types.openapi import CreateChatCompletionRequest
from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionResponse as ChatCompletion,
)
from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionStreamResponse as ChatCompletionChunk,
)
from kserve.protocol.rest.openai.types.openapi import CreateCompletionRequest
from kserve.protocol.rest.openai.types.openapi import (
    CreateCompletionResponse as Completion,
)
from kserve.protocol.rest.openai.types.openapi import CreateEmbeddingRequest
from kserve.protocol.rest.openai.types.openapi import (
    CreateEmbeddingResponse as Embedding,
)

from ...dataplane import DataPlane
from .openai_model import (
    ChatCompletionRequest,
    CompletionRequest,
    EmbeddingRequest,
    OpenAIModel,
    OpenAICompletionModel,
    OpenAIEmbeddingModel,
)


class OpenAIDataPlane(DataPlane):
    """OpenAI DataPlane"""

    async def create_completion(
        self,
        model_name: str,
        request: CreateCompletionRequest,
        headers: Headers,
        response: Response,
    ) -> Union[Completion, AsyncIterator[Completion]]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (CreateCompletionRequest): Params to create a completion.
            headers: (Headers): Request headers.
            response: (Response): FastAPI response object
        Returns:
            response: A non-streaming or streaming completion response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAICompletionModel):
            raise RuntimeError(f"Model {model_name} does not support completion")

        completion_request = CompletionRequest(
            request_id=headers.get("x-request-id", None),
            params=request,
            context={"headers": dict(headers), "response": response},
        )
        return await model.create_completion(completion_request)

    async def create_chat_completion(
        self,
        model_name: str,
        request: CreateChatCompletionRequest,
        headers: Headers,
        response: Response,
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (CreateChatCompletionRequest): Params to create a chat completion.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: A non-streaming or streaming chat completion response

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAICompletionModel):
            raise RuntimeError(f"Model {model_name} does not support chat completion")

        completion_request = ChatCompletionRequest(
            request_id=headers.get("x-request-id", None),
            params=request,
            # We pass the response object in the context so it can be used to set response headers or a custom status code
            context={"headers": dict(headers), "response": response},
        )
        return await model.create_chat_completion(completion_request)

    async def create_embedding(
        self,
        model_name: str,
        request: CreateEmbeddingRequest,
        headers: Headers,
        response: Response,
    ) -> Embedding:
        """Creates an embedding vector representing the input text.

        Args:
            model_name (str): Model name.
            request (CreateEmbeddingRequest): Params to create the embedding.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: A non-streaming embedding response

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = await self.get_model(model_name)
        if not isinstance(model, OpenAIEmbeddingModel):
            raise RuntimeError(f"Model {model_name} does not support embeddings")

        embedding_request = EmbeddingRequest(
            request_id=headers.get("x-request-id", None),
            params=request,
            context={"headers": dict(headers), "response": response},
        )
        return await model.create_embedding(embedding_request)

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
