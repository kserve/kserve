import time
from abc import ABC, abstractmethod
from typing import Iterable, Callable, Union, AsyncIterator, cast
from pydantic import BaseModel
from openai.types.chat import (
    ChatCompletion,
    CompletionCreateParams as ChatCompletionCreateParams,
    ChatCompletionMessageParam,
    ChatCompletionChunk,
    ChatCompletionMessage as BaseChatCompletionMessage,
)
from openai.types.chat.chat_completion import (
    Choice,
    ChoiceLogprobs,
)
from openai.types.chat.chat_completion_chunk import (
    Choice as ChunkChoice,
    ChoiceDelta,
    ChoiceLogprobs as ChunkChoiceLogprobs,
)
from openai.types.chat.chat_completion_token_logprob import (
    TopLogprob,
    ChatCompletionTokenLogprob,
)
from openai.types.chat.completion_create_params import (
    ResponseFormat,
)
from openai.types import Completion, CompletionCreateParams, CompletionChoice
from openai.types.completion_choice import Logprobs
from openai.types.completion_create_params import (
    CompletionCreateParamsNonStreaming,
    CompletionCreateParamsStreaming,
)

from ...errors import InvalidInput
from ...utils.utils import generate_uuid


class ChatPrompt(BaseModel):
    response_role: str = "assistant"
    prompt: str


class ChatCompletionMessage(BaseChatCompletionMessage):
    role: str


class OpenAIModel(ABC):
    """
    An abstract model with methods for implementing OpenAI's completions (v1/completions)
    and chat completions (v1/chat/completions) endpoints.

    Users should extend this model and implement the abstract methods in order to expose
    these endpoints.
    """

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


CompletionChunkMapper = Callable[[Completion], ChatCompletionChunk]


class AsyncChunkIterator:
    def __init__(
        self,
        completion_iterator: AsyncIterator[Completion],
        mapper: CompletionChunkMapper,
    ):
        self.completion_iterator = completion_iterator
        self.mapper = mapper

    def __aiter__(self):
        return self

    async def __anext__(self) -> ChatCompletionChunk:
        # This will raise StopAsyncIteration when there are no more completions.
        # We don't catch it so it will stop our iterator as well.
        completion = await anext(self.completion_iterator)
        return self.mapper(completion)


class OpenAIChatAdapterModel(OpenAIModel):
    """
    A helper on top the OpenAI model that automatically maps chat completion requests (v1/chat/completions)
    to completion requests (v1/completions).

    Users should extend this model and implement the abstract methods in order to expose these endpoints.
    """

    @abstractmethod
    def apply_chat_template(
        self, messages: Iterable[ChatCompletionMessageParam]
    ) -> ChatPrompt:
        """
        Given a list of chat completion messages, convert them to a prompt.
        """
        pass

    @classmethod
    def chat_completion_params_to_completion_params(
        cls, params: ChatCompletionCreateParams, prompt: str
    ) -> CompletionCreateParams:
        params_cls = (
            CompletionCreateParamsStreaming
            if params.get("stream", False)
            else CompletionCreateParamsNonStreaming
        )
        kwargs = {
            "prompt": prompt,
            "model": params.get("model"),
        }
        optional_params = {
            "frequency_penalty",
            "logit_bias",
            "max_tokens",
            "n",
            "presence_penalty",
            "seed",
            "stop",
            "stream",
            "temperature",
            "top_p",
            "user",
        }
        for param in optional_params:
            if param in params:
                kwargs[param] = params[param]

        if "logprobs" in params:
            kwargs["logprobs"] = params.get("top_logprobs", 1)
        return params_cls(**kwargs)

    @classmethod
    def to_choice_logprobs(cls, logprobs: Logprobs) -> ChoiceLogprobs:
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

        clp = ChoiceLogprobs(content=chat_completion_logprobs)
        print("HERERER")
        print(clp)
        return clp

    @classmethod
    def to_chat_completion_choice(
        cls, completion_choice: CompletionChoice, role: str
    ) -> Choice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = cls.to_choice_logprobs(completion_choice.logprobs)
        return Choice(
            index=0,
            finish_reason=completion_choice.finish_reason,
            logprobs=choice_logprobs,
            message=ChatCompletionMessage(content=completion_choice.text, role=role),
        )

    @classmethod
    def to_chat_completion_chunk_choice(
        cls, completion_choice: CompletionChoice, role: str
    ) -> ChunkChoice:
        # translate Token -> ChatCompletionTokenLogprob
        choice_logprobs = cls.to_choice_logprobs(completion_choice.logprobs)
        choice_logprobs = ChunkChoiceLogprobs(content=choice_logprobs.content)
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
        self, params: ChatCompletionCreateParams
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        if params.get("n", 1) != 1:
            raise InvalidInput("n != 1 is not supported")

        chat_prompt = self.apply_chat_template(params["messages"])
        completion_params = self.chat_completion_params_to_completion_params(
            params, chat_prompt.prompt
        )

        if not params.get("stream", False):
            completion = cast(
                Completion, await self.create_completion(completion_params)
            )
            return self.completion_to_chat_completion(
                completion, chat_prompt.response_role
            )
        else:
            completion_iterator = cast(
                AsyncIterator[Completion],
                await self.create_completion(completion_params),
            )

            def mapper(completion: Completion) -> ChatCompletionChunk:
                return self.completion_to_chat_completion_chunk(
                    completion, chat_prompt.response_role
                )

            return AsyncChunkIterator(
                completion_iterator=completion_iterator, mapper=mapper
            )
