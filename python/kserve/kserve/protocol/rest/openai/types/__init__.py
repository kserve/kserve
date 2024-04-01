from typing import Union

from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionRequestAssistantMessage,
    ChatCompletionRequestFunctionMessage, ChatCompletionRequestSystemMessage,
    ChatCompletionRequestToolMessage, ChatCompletionRequestUserMessage,
    ChatCompletionResponseMessage)
from kserve.protocol.rest.openai.types.openapi import \
    ChatCompletionStreamResponseDelta as ChoiceDelta
from kserve.protocol.rest.openai.types.openapi import \
    ChatCompletionTokenLogprob
from kserve.protocol.rest.openai.types.openapi import \
    Choice as CompletionChoice
from kserve.protocol.rest.openai.types.openapi import \
    Choice1 as ChatCompletionChoice
from kserve.protocol.rest.openai.types.openapi import Choice3 as ChunkChoice
from kserve.protocol.rest.openai.types.openapi import \
    CreateChatCompletionRequest
from kserve.protocol.rest.openai.types.openapi import \
    CreateChatCompletionResponse as ChatCompletion
from kserve.protocol.rest.openai.types.openapi import \
    CreateChatCompletionStreamResponse as ChatCompletionChunk
from kserve.protocol.rest.openai.types.openapi import CreateCompletionRequest
from kserve.protocol.rest.openai.types.openapi import \
    CreateCompletionResponse as Completion
from kserve.protocol.rest.openai.types.openapi import Logprobs
from kserve.protocol.rest.openai.types.openapi import \
    Logprobs2 as ChatCompletionChoiceLogprobs
from kserve.protocol.rest.openai.types.openapi import TopLogprob

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
    "Logprobs",
    "TopLogprob",
]
