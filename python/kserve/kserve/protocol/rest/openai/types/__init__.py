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

from kserve.protocol.rest.openai.types.openapi import ChatMessage

from kserve.protocol.rest.openai.types.openapi import (
    DeltaMessage as ChoiceDelta,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionLogprob,
)  # TODO: can not import
from kserve.protocol.rest.openai.types.openapi import (
    CompletionResponseChoice as CompletionChoice,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionResponseChoice as ChatCompletionChoice,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionResponseStreamChoice as ChunkChoice,
)
from kserve.protocol.rest.openai.types.openapi import ChatCompletionRequest
from kserve.protocol.rest.openai.types.openapi import ChatCompletionMessageParam
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionResponse as ChatCompletion,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionStreamResponse as ChatCompletionChunk,
)
from kserve.protocol.rest.openai.types.openapi import CompletionRequest
from kserve.protocol.rest.openai.types.openapi import (
    CompletionResponse as Completion,
)
from kserve.protocol.rest.openai.types.openapi import CompletionLogProbs
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionLogProbs as ChatCompletionChoiceLogprobs,
)
from kserve.protocol.rest.openai.types.openapi import TopLogprob
from kserve.protocol.rest.openai.types.openapi import ErrorResponse
from kserve.protocol.rest.openai.types.openapi import UsageInfo

__all__ = [
    "ChatCompletion",
    "ChatCompletionChoice",
    "ChatCompletionChoiceLogprobs",
    "ChatCompletionChunk",
    "ChatMessage",
    "ChatCompletionLogprob",
    "ChoiceDelta",
    "ChunkChoice",
    "Completion",
    "CompletionChoice",
    "ChatCompletionRequest",
    "CompletionRequest",
    "ErrorResponse",
    "CompletionLogProbs",
    "TopLogprob",
    "UsageInfo",
    "ChatCompletionMessageParam",
]
