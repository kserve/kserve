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

from typing import Union

from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionRequestAssistantMessage,
    ChatCompletionRequestFunctionMessage,
    ChatCompletionRequestSystemMessage,
    ChatCompletionRequestToolMessage,
    ChatCompletionRequestUserMessage,
    ChatCompletionResponseMessage,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionStreamResponseDelta as ChoiceDelta,
)
from kserve.protocol.rest.openai.types.openapi import ChatCompletionTokenLogprob
from kserve.protocol.rest.openai.types.openapi import Choice as CompletionChoice
from kserve.protocol.rest.openai.types.openapi import Choice1 as ChatCompletionChoice
from kserve.protocol.rest.openai.types.openapi import Choice3 as ChunkChoice
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
from kserve.protocol.rest.openai.types.openapi import Logprobs
from kserve.protocol.rest.openai.types.openapi import (
    Logprobs2 as ChatCompletionChoiceLogprobs,
)
from kserve.protocol.rest.openai.types.openapi import TopLogprob
from kserve.protocol.rest.openai.types.openapi import ErrorResponse
from kserve.protocol.rest.openai.types.openapi import CompletionUsage

ChatCompletionRequestMessage = Union[
    ChatCompletionRequestSystemMessage,
    ChatCompletionRequestUserMessage,
    ChatCompletionRequestAssistantMessage,
    ChatCompletionRequestToolMessage,
    ChatCompletionRequestFunctionMessage,
]

__all__ = [
    "ChatCompletion",
    "ChatCompletionChoice",
    "ChatCompletionChoiceLogprobs",
    "ChatCompletionChoiceLogprobs",
    "ChatCompletionChunk",
    "ChatCompletionRequestAssistantMessage",
    "ChatCompletionRequestFunctionMessage",
    "ChatCompletionRequestMessage",
    "ChatCompletionRequestSystemMessage",
    "ChatCompletionRequestToolMessage",
    "ChatCompletionRequestUserMessage",
    "ChatCompletionResponseMessage",
    "ChatCompletionTokenLogprob",
    "ChoiceDelta",
    "ChunkChoice",
    "Completion",
    "CompletionChoice",
    "CreateChatCompletionRequest",
    "CreateCompletionRequest",
    "ErrorResponse",
    "Logprobs",
    "TopLogprob",
    "CompletionUsage",
]
