import time
from abc import ABC, abstractmethod
from typing import Iterable, Union, AsyncIterator
from pydantic import BaseModel
from openai.types.chat import (
    ChatCompletion,
    CompletionCreateParams as ChatCompletionCreateParams,
    ChatCompletionMessageParam,
    ChatCompletionChunk,
    ChatCompletionMessage,
)
from openai.types.chat.chat_completion import (
    Choice,
    ChoiceLogprobs,
    ChatCompletionTokenLogprob,
)
from openai.types.chat.chat_completion_token_logprob import TopLogprob
from openai.types.chat.completion_create_params import ResponseFormat
from openai.types import Completion, CompletionCreateParams, CompletionChoice
from openai.types.completion_choice import Logprobs

from ...errors import InvalidInput
from ...utils.utils import generate_uuid


class ChatPrompt(BaseModel):
    response_role: str = "assistant"
    prompt: str


class BaseOpenAIModel(ABC):
    @abstractmethod
    async def create_completion(
        self, params: CompletionCreateParams
    ) -> Union[Completion, AsyncIterator[Completion]]:
        pass

    @abstractmethod
    async def create_chat_completion(
        self, params: ChatCompletionCreateParams
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        pass


class OpenAIModel(BaseOpenAIModel):
    def to_completion_params(
        self, params: ChatCompletionCreateParams, prompt: str
    ) -> CompletionCreateParams:
        return CompletionCreateParams(
            prompt=prompt,
            model=params.model,
            frequency_penalty=params.frequency_penalty,
            logit_bias=params.logit_bias,
            logprobs=params.logprobs,
            max_tokens=params.max_tokens,
            n=params.n,
            presence_penalty=params.presence_penalty,
            response_format=ResponseFormat(type="json_object"),
            seed=params.seed,
            stop=params.stop,
            temperature=params.temperature,
            tool_choice="none",
            tools=[],
            top_p=params.top_p,
            user=params.user,
        )

    def to_choice_logprobs(self, logprobs: Logprobs) -> ChoiceLogprobs:
        chat_completion_logprobs = []
        for i in range(len(logprobs.tokens)):
            token = logprobs.tokens[i]
            token_logprob = logprobs.token_logprobs[i]
            top_logprobs_dict = logprobs.top_lobprobs[token]
            top_logprobs = [
                TopLogprob(token=token, logprob=logprob)
                for token, logprob in top_logprobs_dict.items()
            ]
            chat_completion_logprobs.append(
                ChatCompletionTokenLogprob(
                    token=token,
                    logprob=token_logprob,
                    top_logprobs=top_logprobs,
                )
            )
        return ChoiceLogprobs(content=chat_completion_logprobs)

    def to_chat_completion_choice(
        self, completion_choice: CompletionChoice, role: str
    ) -> Choice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = self.to_choice_logprobs(completion_choice.logprobs)
        return Choice(
            index=0,
            finish_reason=completion_choice.finish_reason,
            logprobs=choice_logprobs,
            message=ChatCompletionMessage(content=completion_choice.text, role=role),
        )

    @abstractmethod
    async def apply_chat_template(
        self, messages: Iterable[ChatCompletionMessageParam]
    ) -> ChatPrompt:
        pass

    async def create_chat_completion(
        self, params: ChatCompletionCreateParams
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        if params.n != 1:
            raise InvalidInput("n != 1 is not supported")

        created_time = int(time.monotonic())
        chat_prompt = self.apply_chat_template(params.messages)
        completion_params = OpenAIModel.to_chat_completion_params(params)
        completion = self.create_completion(completion_params)
        completion_choice = completion.choices[0]

        if not params.stream:
            return ChatCompletion(
                id=generate_uuid(),
                choices=[
                    self.to_chat_completion_choice(completion_choice, chat_prompt.role)
                ],
                created=created_time,
                model=params.model,
                object="chat.completion",
                system_fingerprint=completion.system_fingerprint,
                usage=completion.usage,
            )
        else:
            # TODO: handle streaming
            pass
