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
from typing import AsyncIterator, Iterable, Union, cast

from kserve.protocol.rest.openai.types import (
    ChatCompletion,
    ChatCompletionChoice,
    ChatCompletionChoiceLogprobs,
    ChatCompletionChunk,
    ChatCompletionRequestMessage,
    ChatCompletionResponseMessage,
    ChatCompletionTokenLogprob,
    ChoiceDelta,
    ChunkChoice,
    Completion,
    CompletionChoice,
    CreateChatCompletionRequest,
    CreateCompletionRequest,
    Logprobs,
    TopLogprob,
)

from ....errors import InvalidInput
from .openai_model import (
    OpenAIModel,
    ChatPrompt,
    CompletionRequest,
    ChatCompletionRequest,
    AsyncMappingIterator,
)


class OpenAIChatAdapterModel(OpenAIModel):
    """
    A helper on top the OpenAI model that automatically maps chat completion requests (/v1/chat/completions)
    to completion requests (/v1/completions).

    Users should extend this model and implement the abstract methods in order to expose these endpoints.
    """

    @abstractmethod
    def apply_chat_template(
        self, messages: Iterable[ChatCompletionRequestMessage]
    ) -> ChatPrompt:
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        pass

    @classmethod
    def chat_completion_params_to_completion_params(
        cls, params: CreateChatCompletionRequest, prompt: str
    ) -> CreateCompletionRequest:

        return CreateCompletionRequest(
            prompt=prompt,
            model=params.model,
            frequency_penalty=params.frequency_penalty,
            logit_bias=params.logit_bias,
            max_tokens=params.max_tokens,
            n=params.n,
            presence_penalty=params.presence_penalty,
            seed=params.seed,
            stop=params.stop,
            stream=params.stream,
            temperature=params.temperature,
            top_p=params.top_p,
            user=params.user,
            logprobs=params.top_logprobs,
        )

    @classmethod
    def to_choice_logprobs(cls, logprobs: Logprobs) -> ChatCompletionChoiceLogprobs:
        chat_completion_logprobs = []
        for i in range(len(logprobs.tokens)):
            token = logprobs.tokens[i]
            token_logprob = logprobs.token_logprobs[i]
            top_logprobs_dict = logprobs.top_logprobs[i]
            top_logprobs = [
                TopLogprob(
                    token=token,
                    bytes=[int(b) for b in token.encode("utf8")],
                    logprob=logprob,
                )
                for token, logprob in top_logprobs_dict.items()
            ]
            chat_completion_logprobs.append(
                ChatCompletionTokenLogprob(
                    token=token,
                    bytes=[int(b) for b in token.encode("utf8")],
                    logprob=token_logprob,
                    top_logprobs=top_logprobs,
                )
            )

        return ChatCompletionChoiceLogprobs(content=chat_completion_logprobs)

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
            message=ChatCompletionResponseMessage(
                content=completion_choice.text, role=role
            ),
        )

    @classmethod
    def to_chat_completion_chunk_choice(
        cls, completion_choice: CompletionChoice, role: str
    ) -> ChunkChoice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = (
            cls.to_choice_logprobs(completion_choice.logprobs)
            if completion_choice.logprobs is not None
            else None
        )
        choice_logprobs = (
            ChatCompletionChoiceLogprobs(content=choice_logprobs.content)
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
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        params = request.params

        if params.n != 1:
            raise InvalidInput("n != 1 is not supported")

        # Convert the messages into a prompt
        chat_prompt = self.apply_chat_template(params.messages, params.chat_template)
        # Translate the chat completion request to a completion request
        completion_params = self.chat_completion_params_to_completion_params(
            params, chat_prompt.prompt
        )

        completion_request = CompletionRequest(
            request_id=request.request_id,
            params=completion_params,
            context=request.context,
        )

        if not params.stream:
            completion = cast(
                Completion, await self.create_completion(completion_request)
            )
            return self.completion_to_chat_completion(
                completion, chat_prompt.response_role
            )
        else:
            completion_iterator = cast(
                AsyncIterator[Completion],
                await self.create_completion(completion_request),
            )

            def mapper(completion: Completion) -> ChatCompletionChunk:
                return self.completion_to_chat_completion_chunk(
                    completion, chat_prompt.response_role
                )

            return AsyncMappingIterator(iterator=completion_iterator, mapper=mapper)
