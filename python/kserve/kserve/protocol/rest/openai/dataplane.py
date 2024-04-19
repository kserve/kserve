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

from typing import AsyncIterator, Dict, Optional, Union

from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionRequest,
    CreateChatCompletionResponse as ChatCompletion,
    CreateChatCompletionStreamResponse as ChatCompletionChunk,
    CreateCompletionRequest,
    CreateCompletionResponse as Completion,
)

from ...dataplane import DataPlane
from .openai_model import OpenAIModel, CompletionRequest


class OpenAIDataPlane(DataPlane):
    """OpenAI DataPlane"""

    async def create_completion(
        self,
        model_name: str,
        request: CreateCompletionRequest,
        headers: Optional[Dict[str, str]] = None,
    ) -> Union[Completion, AsyncIterator[Completion]]:
        """Generate the text with the provided text prompt.

        Args:
            model_name (str): Model name.
            request (CreateCompletionRequest): Params to create a completion.
            headers: (Optional[Dict[str, str]]): Request headers.

        Returns:
            response: A non-streaming or streaming completion response.

        Raises:
            InvalidInput: An error when the body bytes can't be decoded as JSON.
        """
        model = self.get_model(model_name)
        if not isinstance(model, OpenAIModel):
            raise RuntimeError(f"Model {model_name} does not support completion")

        completion_request = CompletionRequest(
            request_id=headers.get("x-request-id", None),
            params=request,
            context=headers,
        )
        return await model.create_completion(completion_request)

    async def create_chat_completion(
        self,
        model_name: str,
        request: CreateChatCompletionRequest,
        headers: Optional[Dict[str, str]] = None,
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
        model = self.get_model(model_name)
        if not isinstance(model, OpenAIModel):
            raise RuntimeError(f"Model {model_name} does not support chat completion")

        completion_request = CompletionRequest(
            request_id=headers.get("x-request-id", None),
            params=request,
            context=headers,
        )
        return await model.create_chat_completion(completion_request)
