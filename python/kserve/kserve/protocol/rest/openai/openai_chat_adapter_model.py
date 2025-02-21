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
from typing import Any, Dict, Optional, cast, AsyncGenerator, Union
from fastapi import Request

from kserve.protocol.rest.openai.types import (
    ChatCompletionRequest,
    CompletionRequest,
    ChatCompletion,
    ChatCompletionChoice,
    ChatCompletionLogProb,
    ChatCompletionLogProbs,
    ChatCompletionChunk,
    ChatMessage,
    ChatCompletionLogProbsContent,
    ChoiceDelta,
    ChunkChoice,
    Completion,
    CompletionChoice,
    CompletionChunk,
    CompletionChunkChoice,
    CompletionLogProbs,
    ErrorResponse,
)

from kserve.errors import InvalidInput
from kserve.protocol.rest.openai.openai_model import AsyncMappingIterator

from .openai_model import (
    OpenAIGenerativeModel,
    ChatPrompt,
)


class OpenAIChatAdapterModel(OpenAIGenerativeModel):
    """
    A helper on top the OpenAI model that automatically maps chat completion requests (/v1/chat/completions)
    to completion requests (/v1/completions).

    Users should extend this model and implement the abstract methods in order to expose these endpoints.
    """

    @abstractmethod
    def apply_chat_template(
        self,
        request: ChatCompletionRequest,
    ) -> ChatPrompt:
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        pass

    @classmethod
    def chat_completion_params_to_completion_params(
        cls, request: ChatCompletionRequest, prompt: str
    ) -> CompletionRequest:

        return CompletionRequest(
            prompt=prompt,
            model=request.model,
            frequency_penalty=request.frequency_penalty,
            logit_bias=request.logit_bias,
            max_tokens=request.max_tokens,
            n=request.n,
            presence_penalty=request.presence_penalty,
            seed=request.seed,
            stop=request.stop,
            stream=request.stream,
            temperature=request.temperature,
            top_p=request.top_p,
            user=request.user,
            logprobs=request.top_logprobs,
        )

    @classmethod
    def to_choice_logprobs(cls, logprobs: CompletionLogProbs) -> ChatCompletionLogProbs:
        chat_completion_logprobs = []
        for i in range(len(logprobs.tokens)):
            token = logprobs.tokens[i]
            token_logprob = logprobs.token_logprobs[i]
            top_logprobs_dict = logprobs.top_logprobs[i]
            top_logprobs = [
                ChatCompletionLogProb(
                    token=token,
                    bytes=[int(b) for b in token.encode("utf8")],
                    logprob=logprob,
                )
                for token, logprob in top_logprobs_dict.items()
            ]
            chat_completion_logprobs.append(
                ChatCompletionLogProbsContent(
                    token=token,
                    bytes=[int(b) for b in token.encode("utf8")],
                    logprob=token_logprob,
                    top_logprobs=top_logprobs,
                )
            )

        return ChatCompletionLogProbs(content=chat_completion_logprobs)

    @classmethod
    def to_chat_completion_choice(
        cls, completion_choice: CompletionChoice, role: str
    ) -> ChatCompletionChoice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = (
            cls.to_choice_logprobs(completion_choice.logprobs)
            if completion_choice.logprobs is not None
            else None
        )
        return ChatCompletionChoice(
            index=0,
            finish_reason=completion_choice.finish_reason,
            logprobs=choice_logprobs,
            message=ChatMessage(content=completion_choice.text, role=role),
        )

    @classmethod
    def to_chat_completion_chunk_choice(
        cls, completion_choice: CompletionChunkChoice, role: str
    ) -> ChunkChoice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = (
            cls.to_choice_logprobs(completion_choice.logprobs)
            if completion_choice.logprobs is not None
            else None
        )
        choice_logprobs = (
            ChatCompletionLogProbs(content=choice_logprobs.content)
            if choice_logprobs is not None
            else None
        )
        return ChunkChoice(
            delta=ChoiceDelta(content=completion_choice.text, role=role),
            index=0,
            finish_reason=completion_choice.finish_reason,
            logprobs=choice_logprobs,
        )

    @classmethod
    def completion_to_chat_completion(
        cls, completion: Completion, role: str
    ) -> ChatCompletion:
        completion_choice = (
            completion.choices[0] if len(completion.choices) > 0 else None
        )
        choices = (
            [cls.to_chat_completion_choice(completion_choice, role)]
            if completion_choice is not None
            else []
        )
        return ChatCompletion(
            id=completion.id,
            choices=choices,
            created=completion.created,
            model=completion.model,
            object="chat.completion",
            system_fingerprint=completion.system_fingerprint,
            usage=completion.usage,
        )

    @classmethod
    def completion_to_chat_completion_chunk(
        cls, completion: Completion, role: str
    ) -> ChatCompletionChunk:
        completion_choice = (
            completion.choices[0] if len(completion.choices) > 0 else None
        )
        choices = (
            [cls.to_chat_completion_chunk_choice(completion_choice, role)]
            if completion_choice is not None
            else []
        )
        return ChatCompletionChunk(
            id=completion.id,
            choices=choices,
            created=completion.created,
            model=completion.model,
            object="chat.completion.chunk",
            system_fingerprint=completion.system_fingerprint,
        )

    async def create_chat_completion(
        self,
        request: ChatCompletionRequest,
        raw_request: Optional[Request] = None,
        context: Optional[Dict[str, Any]] = None,
    ) -> Union[AsyncGenerator[str, None], ChatCompletion, ErrorResponse]:

        if request.n != 1:
            raise InvalidInput("n != 1 is not supported")

        # Convert the messages into a prompt
        chat_prompt = self.apply_chat_template(request)
        # Translate the chat completion request to a completion request
        completion_params = self.chat_completion_params_to_completion_params(
            request, chat_prompt.prompt
        )

        if not request.stream:
            completion = cast(
                Completion, await self.create_completion(completion_params, raw_request)
            )
            return self.completion_to_chat_completion(
                completion, chat_prompt.response_role
            )
        else:
            completion_iterator = await self.create_completion(
                completion_params, raw_request
            )

            def mapper(completion_str: str) -> ChatCompletionChunk:

                chunk = completion_str.removeprefix("data: ")
                if chunk == "[DONE]\n\n":
                    return

                completion = CompletionChunk.model_validate_json(chunk)

                return self.completion_to_chat_completion_chunk(
                    completion, chat_prompt.response_role
                )

            completion = AsyncMappingIterator(
                iterator=completion_iterator, mapper=mapper
            )

            async def stream_results() -> AsyncGenerator[str, None]:
                async for chunk in completion:
                    yield f"data: {chunk.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return stream_results()
