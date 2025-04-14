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

from abc import abstractmethod
from typing import Any, AsyncGenerator, AsyncIterator, Callable, Dict, Optional, Union
import inspect

from pydantic import BaseModel
from fastapi import Request

from kserve.protocol.rest.openai.types import (
    ChatCompletion,
    Completion,
    CompletionRequest,
    ChatCompletionRequest,
    EmbeddingRequest,
    Embedding,
    ErrorResponse,
    RerankRequest,
    Rerank,
)

from ....model import BaseKServeModel


class ChatPrompt(BaseModel):
    response_role: str = "assistant"
    prompt: str


class OpenAIModel(BaseKServeModel):
    """
    An abstract model with methods for implementing OpenAI's endpoints.
    """

    def __init__(self, name: str):
        super().__init__(name)

        # We don't support the `load()` method on OpenAIModel yet
        # Assume the model is ready
        self.ready = True


class OpenAIGenerativeModel(OpenAIModel):
    """
    An abstract model with methods for implementing OpenAI's completions (v1/completions)
    and chat completions (v1/chat/completions) endpoints.

    Users should extend this model and implement the abstract methods in order to expose
    these endpoints.
    """

    @abstractmethod
    async def create_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        pass

    @abstractmethod
    async def create_chat_completion(
        self,
        request: ChatCompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:
        pass


class OpenAIEncoderModel(OpenAIModel):
    """
    An abstract model with methods for implementing Embeddings (v1/embeddings) and Rerank (v1/rerank) endpoint.

    Users should extend this model and implement the abstract methods in order to expose
    these endpoints.
    """

    @abstractmethod
    async def create_embedding(
        self,
        request: EmbeddingRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Embedding, ErrorResponse]:
        pass

    @abstractmethod
    async def create_rerank(
        self,
        request: RerankRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], Rerank, ErrorResponse]:
        pass


class AsyncMappingIterator:
    def __init__(
        self,
        iterator: AsyncIterator,
        mapper: Callable = lambda item: item,
        skip_none: bool = True,
        close: Optional[Callable] = None,
    ):
        self.iterator = iterator
        self.mapper = mapper
        self.skip_none = skip_none
        self.close = close

    def __aiter__(self):
        return self

    async def __anext__(self):
        # This will raise StopAsyncIteration when there are no more completions.
        # We don't catch it so it will stop our iterator as well.
        async def next():
            try:
                return self.mapper(await self.iterator.__anext__())
            except Exception as e:
                if self.close:
                    if inspect.iscoroutinefunction(self.close):
                        await self.close()
                    else:
                        self.close()
                raise e

        mapped_item = await next()
        if self.skip_none:
            while mapped_item is None:
                mapped_item = await next()
        return mapped_item
