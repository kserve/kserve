# Copyright 2024 The KServe Authors.
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

# adapted from vLLM

from __future__ import annotations

from pydantic import (
    AnyUrl,
    BaseModel,
    ConfigDict,
    PositiveFloat,
    RootModel,
    confloat,
    conint,
    constr,
    Field,
    model_validator,
)

import time
import uuid
import copy
from typing import Any, Dict, List, Literal, Optional, Union, Iterable, Callable, Set

from enum import Enum, IntEnum
from functools import cached_property

import torch  # TODO: install torch here?
import msgspec  # TODO: install msgspec here?
from dataclasses import dataclass
from openai.types.chat import (
    ChatCompletionContentPartParam,
    ChatCompletionMessageToolCallParam,
)
from openai.types.chat import (
    ChatCompletionMessageParam as OpenAIChatCompletionMessageParam,
)

from typing_extensions import (
    Annotated,
    Required,
    TypedDict,
)

# Does not install vllm here!

import logging
from logging import Logger


def init_logger(name: str) -> Logger:
    """The main purpose of this function is to ensure that loggers are
    retrieved in such a way that we can be sure the root vllm logger has
    already been configured."""

    return logging.getLogger(name)


logger = init_logger(__name__)

_SAMPLING_EPS = 1e-5
_MAX_TEMP = 1e-2


class SamplingType(IntEnum):
    GREEDY = 0
    RANDOM = 1
    RANDOM_SEED = 2


LogitsProcessor = Union[
    Callable[[List[int], torch.Tensor], torch.Tensor],
    Callable[[List[int], List[int], torch.Tensor], torch.Tensor],
]
"""LogitsProcessor is a function that takes a list
of previously generated tokens, the logits tensor
for the next token and, optionally, prompt tokens as a
first argument, and returns a modified tensor of logits
to sample from."""


# maybe make msgspec?
@dataclass
class GuidedDecodingParams:
    """One of these fields will be used to build a logit processor."""

    json: Optional[Union[str, Dict]] = None
    regex: Optional[str] = None
    choice: Optional[List[str]] = None
    grammar: Optional[str] = None
    json_object: Optional[bool] = None
    """These are other options that can be set"""
    backend: Optional[str] = None
    whitespace_pattern: Optional[str] = None

    @staticmethod
    def from_optional(
        json: Optional[Union[Dict, BaseModel, str]],
        regex: Optional[str] = None,
        choice: Optional[List[str]] = None,
        grammar: Optional[str] = None,
        json_object: Optional[bool] = None,
        backend: Optional[str] = None,
        whitespace_pattern: Optional[str] = None,
    ) -> "GuidedDecodingParams":
        # Extract json schemas from pydantic models
        if isinstance(json, (BaseModel, type(BaseModel))):
            json = json.model_json_schema()
        return GuidedDecodingParams(
            json=json,
            regex=regex,
            choice=choice,
            grammar=grammar,
            json_object=json_object,
            backend=backend,
            whitespace_pattern=whitespace_pattern,
        )

    def __post_init__(self):
        """Validate that some fields are mutually exclusive."""
        guide_count = sum(
            [
                self.json is not None,
                self.regex is not None,
                self.choice is not None,
                self.grammar is not None,
                self.json_object is not None,
            ]
        )
        if guide_count > 1:
            raise ValueError(
                "You can only use one kind of guided decoding but multiple are "
                f"specified: {self.__dict__}"
            )


class RequestOutputKind(Enum):
    # Return entire output so far in every RequestOutput
    CUMULATIVE = 0
    # Return only deltas in each RequestOutput
    DELTA = 1
    # Do not return intermediate RequestOuputs
    FINAL_ONLY = 2


class SamplingParams(
    msgspec.Struct,
    omit_defaults=True,  # type: ignore[call-arg]
    # required for @cached_property.
    dict=True,
):  # type: ignore[call-arg]
    """Sampling parameters for text generation.

    Overall, we follow the sampling parameters from the OpenAI text completion
    API (https://platform.openai.com/docs/api-reference/completions/create).
    In addition, we support beam search, which is not supported by OpenAI.

    Args:
        n: Number of output sequences to return for the given prompt.
        best_of: Number of output sequences that are generated from the prompt.
            From these `best_of` sequences, the top `n` sequences are returned.
            `best_of` must be greater than or equal to `n`. By default,
            `best_of` is set to `n`.
        presence_penalty: Float that penalizes new tokens based on whether they
            appear in the generated text so far. Values > 0 encourage the model
            to use new tokens, while values < 0 encourage the model to repeat
            tokens.
        frequency_penalty: Float that penalizes new tokens based on their
            frequency in the generated text so far. Values > 0 encourage the
            model to use new tokens, while values < 0 encourage the model to
            repeat tokens.
        repetition_penalty: Float that penalizes new tokens based on whether
            they appear in the prompt and the generated text so far. Values > 1
            encourage the model to use new tokens, while values < 1 encourage
            the model to repeat tokens.
        temperature: Float that controls the randomness of the sampling. Lower
            values make the model more deterministic, while higher values make
            the model more random. Zero means greedy sampling.
        top_p: Float that controls the cumulative probability of the top tokens
            to consider. Must be in (0, 1]. Set to 1 to consider all tokens.
        top_k: Integer that controls the number of top tokens to consider. Set
            to -1 to consider all tokens.
        min_p: Float that represents the minimum probability for a token to be
            considered, relative to the probability of the most likely token.
            Must be in [0, 1]. Set to 0 to disable this.
        seed: Random seed to use for the generation.
        stop: List of strings that stop the generation when they are generated.
            The returned output will not contain the stop strings.
        stop_token_ids: List of tokens that stop the generation when they are
            generated. The returned output will contain the stop tokens unless
            the stop tokens are special tokens.
        include_stop_str_in_output: Whether to include the stop strings in
            output text. Defaults to False.
        ignore_eos: Whether to ignore the EOS token and continue generating
            tokens after the EOS token is generated.
        max_tokens: Maximum number of tokens to generate per output sequence.
        min_tokens: Minimum number of tokens to generate per output sequence
            before EOS or stop_token_ids can be generated
        logprobs: Number of log probabilities to return per output token.
            When set to None, no probability is returned. If set to a non-None
            value, the result includes the log probabilities of the specified
            number of most likely tokens, as well as the chosen tokens.
            Note that the implementation follows the OpenAI API: The API will
            always return the log probability of the sampled token, so there
            may be up to `logprobs+1` elements in the response.
        prompt_logprobs: Number of log probabilities to return per prompt token.
        detokenize: Whether to detokenize the output. Defaults to True.
        skip_special_tokens: Whether to skip special tokens in the output.
        spaces_between_special_tokens: Whether to add spaces between special
            tokens in the output.  Defaults to True.
        logits_processors: List of functions that modify logits based on
            previously generated tokens, and optionally prompt tokens as
            a first argument.
        truncate_prompt_tokens: If set to an integer k, will use only the last k
            tokens from the prompt (i.e., left truncation). Defaults to None
            (i.e., no truncation).
        guided_decoding: If provided, the engine will construct a guided
            decoding logits processor from these parameters. Defaults to None.
        logit_bias: If provided, the engine will construct a logits processor
            that applies these logit biases. Defaults to None.
        allowed_token_ids: If provided, the engine will construct a logits
            processor which only retains scores for the given token ids.
            Defaults to None.
    """

    n: int = 1
    best_of: Optional[int] = None
    _real_n: Optional[int] = None
    presence_penalty: float = 0.0
    frequency_penalty: float = 0.0
    repetition_penalty: float = 1.0
    temperature: float = 1.0
    top_p: float = 1.0
    top_k: int = -1
    min_p: float = 0.0
    seed: Optional[int] = None
    stop: Optional[Union[str, List[str]]] = None
    stop_token_ids: Optional[List[int]] = None
    ignore_eos: bool = False
    max_tokens: Optional[int] = 16
    min_tokens: int = 0
    logprobs: Optional[int] = None
    prompt_logprobs: Optional[int] = None
    # NOTE: This parameter is only exposed at the engine level for now.
    # It is not exposed in the OpenAI API server, as the OpenAI API does
    # not support returning only a list of token IDs.
    detokenize: bool = True
    skip_special_tokens: bool = True
    spaces_between_special_tokens: bool = True
    # Optional[List[LogitsProcessor]] type. We use Any here because
    # Optional[List[LogitsProcessor]] type is not supported by msgspec.
    logits_processors: Optional[Any] = None
    include_stop_str_in_output: bool = False
    truncate_prompt_tokens: Optional[Annotated[int, msgspec.Meta(ge=1)]] = None
    output_kind: RequestOutputKind = RequestOutputKind.CUMULATIVE

    # The below fields are not supposed to be used as an input.
    # They are set in post_init.
    output_text_buffer_length: int = 0
    _all_stop_token_ids: Set[int] = msgspec.field(default_factory=set)

    # Fields used to construct logits processors
    guided_decoding: Optional[GuidedDecodingParams] = None
    logit_bias: Optional[Dict[int, float]] = None
    allowed_token_ids: Optional[List[int]] = None

    @staticmethod
    def from_optional(
        n: Optional[int] = 1,
        best_of: Optional[int] = None,
        presence_penalty: Optional[float] = 0.0,
        frequency_penalty: Optional[float] = 0.0,
        repetition_penalty: Optional[float] = 1.0,
        temperature: Optional[float] = 1.0,
        top_p: Optional[float] = 1.0,
        top_k: int = -1,
        min_p: float = 0.0,
        seed: Optional[int] = None,
        stop: Optional[Union[str, List[str]]] = None,
        stop_token_ids: Optional[List[int]] = None,
        include_stop_str_in_output: bool = False,
        ignore_eos: bool = False,
        max_tokens: Optional[int] = 16,
        min_tokens: int = 0,
        logprobs: Optional[int] = None,
        prompt_logprobs: Optional[int] = None,
        detokenize: bool = True,
        skip_special_tokens: bool = True,
        spaces_between_special_tokens: bool = True,
        logits_processors: Optional[List[LogitsProcessor]] = None,
        truncate_prompt_tokens: Optional[Annotated[int, msgspec.Meta(ge=1)]] = None,
        output_kind: RequestOutputKind = RequestOutputKind.CUMULATIVE,
        guided_decoding: Optional[GuidedDecodingParams] = None,
        logit_bias: Optional[Union[Dict[int, float], Dict[str, float]]] = None,
        allowed_token_ids: Optional[List[int]] = None,
    ) -> "SamplingParams":
        if logit_bias is not None:
            logit_bias = {int(token): bias for token, bias in logit_bias.items()}

        return SamplingParams(
            n=1 if n is None else n,
            best_of=best_of,
            presence_penalty=0.0 if presence_penalty is None else presence_penalty,
            frequency_penalty=0.0 if frequency_penalty is None else frequency_penalty,
            repetition_penalty=(
                1.0 if repetition_penalty is None else repetition_penalty
            ),
            temperature=1.0 if temperature is None else temperature,
            top_p=1.0 if top_p is None else top_p,
            top_k=top_k,
            min_p=min_p,
            seed=seed,
            stop=stop,
            stop_token_ids=stop_token_ids,
            include_stop_str_in_output=include_stop_str_in_output,
            ignore_eos=ignore_eos,
            max_tokens=max_tokens,
            min_tokens=min_tokens,
            logprobs=logprobs,
            prompt_logprobs=prompt_logprobs,
            detokenize=detokenize,
            skip_special_tokens=skip_special_tokens,
            spaces_between_special_tokens=spaces_between_special_tokens,
            logits_processors=logits_processors,
            truncate_prompt_tokens=truncate_prompt_tokens,
            output_kind=output_kind,
            guided_decoding=guided_decoding,
            logit_bias=logit_bias,
            allowed_token_ids=allowed_token_ids,
        )

    def __post_init__(self) -> None:
        # how we deal with `best_of``:
        # if `best_of`` is not set, we default to `n`;
        # if `best_of`` is set, we set `n`` to `best_of`,
        # and set `_real_n`` to the original `n`.
        # when we return the result, we will check
        # if we need to return `n` or `_real_n` results
        if self.best_of:
            if self.best_of < self.n:
                raise ValueError(
                    f"best_of must be greater than or equal to n, "
                    f"got n={self.n} and best_of={self.best_of}."
                )
            self._real_n = self.n
            self.n = self.best_of
        if 0 < self.temperature < _MAX_TEMP:
            logger.warning(
                "temperature %s is less than %s, which may cause numerical "
                "errors nan or inf in tensors. We have maxed it out to %s.",
                self.temperature,
                _MAX_TEMP,
                _MAX_TEMP,
            )  # TODO:
            self.temperature = max(self.temperature, _MAX_TEMP)
        if self.seed == -1:
            self.seed = None
        else:
            self.seed = self.seed
        if self.stop is None:
            self.stop = []
        elif isinstance(self.stop, str):
            self.stop = [self.stop]
        else:
            self.stop = list(self.stop)
        if self.stop_token_ids is None:
            self.stop_token_ids = []
        else:
            self.stop_token_ids = list(self.stop_token_ids)
        self.logprobs = 1 if self.logprobs is True else self.logprobs
        self.prompt_logprobs = (
            1 if self.prompt_logprobs is True else self.prompt_logprobs
        )

        # Number of characters to hold back for stop string evaluation
        # until sequence is finished.
        if self.stop and not self.include_stop_str_in_output:
            self.output_text_buffer_length = max(len(s) for s in self.stop) - 1

        self._verify_args()

        if self.temperature < _SAMPLING_EPS:
            # Zero temperature means greedy sampling.
            self.top_p = 1.0
            self.top_k = -1
            self.min_p = 0.0
            self._verify_greedy_sampling()
        # eos_token_id is added to this by the engine
        self._all_stop_token_ids = set(self.stop_token_ids)

    def _verify_args(self) -> None:
        if not isinstance(self.n, int):
            raise ValueError(f"n must be an int, but is of " f"type {type(self.n)}")
        if self.n < 1:
            raise ValueError(f"n must be at least 1, got {self.n}.")
        if not -2.0 <= self.presence_penalty <= 2.0:
            raise ValueError(
                "presence_penalty must be in [-2, 2], got " f"{self.presence_penalty}."
            )
        if not -2.0 <= self.frequency_penalty <= 2.0:
            raise ValueError(
                "frequency_penalty must be in [-2, 2], got "
                f"{self.frequency_penalty}."
            )
        if not 0.0 < self.repetition_penalty <= 2.0:
            raise ValueError(
                "repetition_penalty must be in (0, 2], got "
                f"{self.repetition_penalty}."
            )
        if self.temperature < 0.0:
            raise ValueError(
                f"temperature must be non-negative, got {self.temperature}."
            )
        if not 0.0 < self.top_p <= 1.0:
            raise ValueError(f"top_p must be in (0, 1], got {self.top_p}.")
        if self.top_k < -1 or self.top_k == 0:
            raise ValueError(
                f"top_k must be -1 (disable), or at least 1, " f"got {self.top_k}."
            )
        if not isinstance(self.top_k, int):
            raise TypeError(
                f"top_k must be an integer, got {type(self.top_k).__name__}"
            )
        if not 0.0 <= self.min_p <= 1.0:
            raise ValueError("min_p must be in [0, 1], got " f"{self.min_p}.")
        if self.max_tokens is not None and self.max_tokens < 1:
            raise ValueError(f"max_tokens must be at least 1, got {self.max_tokens}.")
        if self.min_tokens < 0:
            raise ValueError(
                f"min_tokens must be greater than or equal to 0, "
                f"got {self.min_tokens}."
            )
        if self.max_tokens is not None and self.min_tokens > self.max_tokens:
            raise ValueError(
                f"min_tokens must be less than or equal to "
                f"max_tokens={self.max_tokens}, got {self.min_tokens}."
            )
        if self.logprobs is not None and self.logprobs < 0:
            raise ValueError(f"logprobs must be non-negative, got {self.logprobs}.")
        if self.prompt_logprobs is not None and self.prompt_logprobs < 0:
            raise ValueError(
                f"prompt_logprobs must be non-negative, got " f"{self.prompt_logprobs}."
            )
        if self.truncate_prompt_tokens is not None and self.truncate_prompt_tokens < 1:
            raise ValueError(
                f"truncate_prompt_tokens must be >= 1, "
                f"got {self.truncate_prompt_tokens}"
            )
        assert isinstance(self.stop, list)
        if any(not stop_str for stop_str in self.stop):
            raise ValueError("stop cannot contain an empty string.")
        if self.stop and not self.detokenize:
            raise ValueError(
                "stop strings are only supported when detokenize is True. "
                "Set detokenize=True to use stop."
            )
        if self.best_of != self._real_n and self.output_kind == (
            RequestOutputKind.DELTA
        ):
            raise ValueError("best_of must equal n to use output_kind=DELTA")

    def _verify_greedy_sampling(self) -> None:
        if self.n > 1:
            raise ValueError(
                "n must be 1 when using greedy sampling, " f"got {self.n}."
            )

    def update_from_generation_config(
        self,
        generation_config: Dict[str, Any],
        model_eos_token_id: Optional[int] = None,
    ) -> None:
        """Update if there are non-default values from generation_config"""

        if model_eos_token_id is not None:
            # Add the eos token id into the sampling_params to support
            # min_tokens processing.
            self._all_stop_token_ids.add(model_eos_token_id)

        # Update eos_token_id for generation
        if (eos_ids := generation_config.get("eos_token_id")) is not None:
            # it can be either int or list of int
            eos_ids = {eos_ids} if isinstance(eos_ids, int) else set(eos_ids)
            if model_eos_token_id is not None:
                # We don't need to include the primary eos_token_id in
                # stop_token_ids since it's handled separately for stopping
                # purposes.
                eos_ids.discard(model_eos_token_id)
            if eos_ids:
                self._all_stop_token_ids.update(eos_ids)
                if not self.ignore_eos:
                    eos_ids.update(self.stop_token_ids)
                    self.stop_token_ids = list(eos_ids)

    @cached_property
    def sampling_type(self) -> SamplingType:
        if self.temperature < _SAMPLING_EPS:
            return SamplingType.GREEDY
        if self.seed is not None:
            return SamplingType.RANDOM_SEED
        return SamplingType.RANDOM

    @property
    def all_stop_token_ids(self) -> Set[int]:
        return self._all_stop_token_ids

    def clone(self) -> "SamplingParams":
        """Deep copy excluding LogitsProcessor objects.

        LogitsProcessor objects are excluded because they may contain an
        arbitrary, nontrivial amount of data.
        See https://github.com/vllm-project/vllm/issues/3087
        """

        logit_processor_refs = (
            None
            if self.logits_processors is None
            else {id(lp): lp for lp in self.logits_processors}
        )
        return copy.deepcopy(self, memo=logit_processor_refs)

    def __repr__(self) -> str:
        return (
            f"SamplingParams(n={self.n}, "
            f"presence_penalty={self.presence_penalty}, "
            f"frequency_penalty={self.frequency_penalty}, "
            f"repetition_penalty={self.repetition_penalty}, "
            f"temperature={self.temperature}, "
            f"top_p={self.top_p}, "
            f"top_k={self.top_k}, "
            f"min_p={self.min_p}, "
            f"seed={self.seed}, "
            f"stop={self.stop}, "
            f"stop_token_ids={self.stop_token_ids}, "
            f"include_stop_str_in_output={self.include_stop_str_in_output}, "
            f"ignore_eos={self.ignore_eos}, "
            f"max_tokens={self.max_tokens}, "
            f"min_tokens={self.min_tokens}, "
            f"logprobs={self.logprobs}, "
            f"prompt_logprobs={self.prompt_logprobs}, "
            f"skip_special_tokens={self.skip_special_tokens}, "
            "spaces_between_special_tokens="
            f"{self.spaces_between_special_tokens}, "
            f"truncate_prompt_tokens={self.truncate_prompt_tokens}), "
            f"guided_decoding={self.guided_decoding}"
        )


class BeamSearchParams(
    msgspec.Struct,
    omit_defaults=True,  # type: ignore[call-arg]
    # required for @cached_property.
    dict=True,
):  # type: ignore[call-arg]
    """Beam search parameters for text generation."""

    beam_width: int
    max_tokens: int
    ignore_eos: bool = False
    temperature: float = 0.0
    length_penalty: float = 1.0


class PoolingParams(
    msgspec.Struct, omit_defaults=True, array_like=True  # type: ignore[call-arg]
):  # type: ignore[call-arg]
    """Pooling parameters for pooling.

    Attributes:
        additional_data: Any additional data needed for pooling.
    """

    additional_data: Optional[Any] = None

    def clone(self) -> "PoolingParams":
        """Returns a deep copy of the PoolingParams instance."""
        return PoolingParams(
            additional_data=self.additional_data,
        )

    def __repr__(self) -> str:
        return f"PoolingParams(" f"additional_metadata={self.additional_data})"


# We use dataclass for now because it is used for
# openai server output, and msgspec is not serializable.
# TODO(sang): Fix it.
@dataclass
class Logprob:
    """Infos for supporting OpenAI compatible logprobs and token ranks.

    Attributes:
        logprob: The logprob of chosen token
        rank: The vocab rank of chosen token (>=1)
        decoded_token: The decoded chosen token index
    """

    logprob: float
    rank: Optional[int] = None
    decoded_token: Optional[str] = None


def random_uuid() -> str:
    return str(uuid.uuid4().hex)


class CustomChatCompletionMessageParam(TypedDict, total=False):
    """Enables custom roles in the Chat Completion API."""

    role: Required[str]
    """The role of the message's author."""

    content: Union[str, List[ChatCompletionContentPartParam]]
    """The contents of the message."""

    name: str
    """An optional name for the participant.

    Provides the model information to differentiate between participants of the
    same role.
    """

    tool_call_id: Optional[str]
    """Tool call that this message is responding to."""

    tool_calls: Optional[Iterable[ChatCompletionMessageToolCallParam]]
    """The tool calls generated by the model, such as function calls."""


class OpenAIBaseModel(BaseModel):
    # OpenAI API does not allow extra fields
    model_config = ConfigDict(extra="forbid")


class ErrorResponse(OpenAIBaseModel):
    object: str = "error"
    message: str
    type: str
    param: Optional[str] = None
    code: int


class ModelPermission(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"modelperm-{random_uuid()}")
    object: str = "model_permission"
    created: int = Field(default_factory=lambda: int(time.time()))
    allow_create_engine: bool = False
    allow_sampling: bool = True
    allow_logprobs: bool = True
    allow_search_indices: bool = False
    allow_view: bool = True
    allow_fine_tuning: bool = False
    organization: str = "*"
    group: Optional[str] = None
    is_blocking: bool = False


class ModelCard(OpenAIBaseModel):
    id: str
    object: str = "model"
    created: int = Field(default_factory=lambda: int(time.time()))
    owned_by: str = "vllm"
    root: Optional[str] = None
    parent: Optional[str] = None
    max_model_len: Optional[int] = None
    permission: List[ModelPermission] = Field(default_factory=list)


class ModelList(OpenAIBaseModel):
    object: str = "list"
    data: List[ModelCard] = Field(default_factory=list)


class UsageInfo(OpenAIBaseModel):
    prompt_tokens: int = 0
    total_tokens: int = 0
    completion_tokens: Optional[int] = 0


class RequestResponseMetadata(BaseModel):
    request_id: str
    final_usage_info: Optional[UsageInfo] = None


class JsonSchemaResponseFormat(OpenAIBaseModel):
    name: str
    description: Optional[str] = None
    # schema is the field in openai but that causes conflicts with pydantic so
    # instead use json_schema with an alias
    json_schema: Optional[Dict[str, Any]] = Field(default=None, alias="schema")
    strict: Optional[bool] = None


class ResponseFormat(OpenAIBaseModel):
    # type must be "json_schema", "json_object" or "text"
    type: Literal["text", "json_object", "json_schema"]
    json_schema: Optional[JsonSchemaResponseFormat] = None


class StreamOptions(OpenAIBaseModel):
    include_usage: Optional[bool] = True
    continuous_usage_stats: Optional[bool] = True


class FunctionDefinition(OpenAIBaseModel):
    name: str
    description: Optional[str] = None
    parameters: Optional[Dict[str, Any]] = None


class ChatCompletionToolsParam(OpenAIBaseModel):
    type: Literal["function"] = "function"
    function: FunctionDefinition


class ChatCompletionNamedFunction(OpenAIBaseModel):
    name: str


class ChatCompletionNamedToolChoiceParam(OpenAIBaseModel):
    function: ChatCompletionNamedFunction
    type: Literal["function"] = "function"


ChatCompletionMessageParam = Union[
    OpenAIChatCompletionMessageParam, CustomChatCompletionMessageParam
]


class ChatCompletionRequest(OpenAIBaseModel):
    # Ordered by official OpenAI API documentation
    # https://platform.openai.com/docs/api-reference/chat/create
    messages: List[ChatCompletionMessageParam]
    model: str
    frequency_penalty: Optional[float] = 0.0
    logit_bias: Optional[Dict[str, float]] = None
    logprobs: Optional[bool] = False
    top_logprobs: Optional[int] = 0
    max_tokens: Optional[int] = None
    n: Optional[int] = 1
    presence_penalty: Optional[float] = 0.0
    response_format: Optional[ResponseFormat] = None
    seed: Optional[int] = Field(None)
    stop: Optional[Union[str, List[str]]] = Field(default_factory=list)
    stream: Optional[bool] = False
    stream_options: Optional[StreamOptions] = None
    temperature: Optional[float] = 0.7
    top_p: Optional[float] = 1.0
    tools: Optional[List[ChatCompletionToolsParam]] = None
    tool_choice: Optional[
        Union[Literal["none"], Literal["auto"], ChatCompletionNamedToolChoiceParam]
    ] = "none"

    # NOTE this will be ignored by VLLM -- the model determines the behavior
    parallel_tool_calls: Optional[bool] = False
    user: Optional[str] = None

    # doc: begin-chat-completion-sampling-params
    best_of: Optional[int] = None
    use_beam_search: bool = False
    top_k: int = -1
    min_p: float = 0.0
    repetition_penalty: float = 1.0
    length_penalty: float = 1.0
    stop_token_ids: Optional[List[int]] = Field(default_factory=list)
    include_stop_str_in_output: bool = False
    ignore_eos: bool = False
    min_tokens: int = 0
    skip_special_tokens: bool = True
    spaces_between_special_tokens: bool = True
    truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]] = None
    prompt_logprobs: Optional[int] = None
    # doc: end-chat-completion-sampling-params

    # doc: begin-chat-completion-extra-params
    echo: bool = Field(
        default=False,
        description=(
            "If true, the new message will be prepended with the last message "
            "if they belong to the same role."
        ),
    )
    add_generation_prompt: bool = Field(
        default=True,
        description=(
            "If true, the generation prompt will be added to the chat template. "
            "This is a parameter used by chat template in tokenizer config of the "
            "model."
        ),
    )
    continue_final_message: bool = Field(
        default=False,
        description=(
            "If this is set, the chat will be formatted so that the final "
            "message in the chat is open-ended, without any EOS tokens. The "
            "model will continue this message rather than starting a new one. "
            'This allows you to "prefill" part of the model\'s response for it. '
            "Cannot be used at the same time as `add_generation_prompt`."
        ),
    )
    add_special_tokens: bool = Field(
        default=False,
        description=(
            "If true, special tokens (e.g. BOS) will be added to the prompt "
            "on top of what is added by the chat template. "
            "For most models, the chat template takes care of adding the "
            "special tokens so this should be set to false (as is the "
            "default)."
        ),
    )
    documents: Optional[List[Dict[str, str]]] = Field(
        default=None,
        description=(
            "A list of dicts representing documents that will be accessible to "
            "the model if it is performing RAG (retrieval-augmented generation)."
            " If the template does not support RAG, this argument will have no "
            "effect. We recommend that each document should be a dict containing "
            '"title" and "text" keys.'
        ),
    )
    chat_template: Optional[str] = Field(
        default=None,
        description=(
            "A Jinja template to use for this conversion. "
            "As of transformers v4.44, default chat template is no longer "
            "allowed, so you must provide a chat template if the tokenizer "
            "does not define one."
        ),
    )
    chat_template_kwargs: Optional[Dict[str, Any]] = Field(
        default=None,
        description=(
            "Additional kwargs to pass to the template renderer. "
            "Will be accessible by the chat template."
        ),
    )
    guided_json: Optional[Union[str, dict, BaseModel]] = Field(
        default=None,
        description=("If specified, the output will follow the JSON schema."),
    )
    guided_regex: Optional[str] = Field(
        default=None,
        description=("If specified, the output will follow the regex pattern."),
    )
    guided_choice: Optional[List[str]] = Field(
        default=None,
        description=("If specified, the output will be exactly one of the choices."),
    )
    guided_grammar: Optional[str] = Field(
        default=None,
        description=("If specified, the output will follow the context free grammar."),
    )
    guided_decoding_backend: Optional[str] = Field(
        default=None,
        description=(
            "If specified, will override the default guided decoding backend "
            "of the server for this specific request. If set, must be either "
            "'outlines' / 'lm-format-enforcer'"
        ),
    )
    guided_whitespace_pattern: Optional[str] = Field(
        default=None,
        description=(
            "If specified, will override the default whitespace pattern "
            "for guided json decoding."
        ),
    )
    priority: int = Field(
        default=0,
        description=(
            "The priority of the request (lower means earlier handling; "
            "default: 0). Any priority other than 0 will raise an error "
            "if the served model does not use priority scheduling."
        ),
    )

    # doc: end-chat-completion-extra-params

    def to_beam_search_params(self, default_max_tokens: int) -> BeamSearchParams:
        max_tokens = self.max_tokens
        if max_tokens is None:
            max_tokens = default_max_tokens

        n = self.n if self.n is not None else 1
        temperature = self.temperature if self.temperature is not None else 0.0

        return BeamSearchParams(
            beam_width=n,
            max_tokens=max_tokens,
            ignore_eos=self.ignore_eos,
            temperature=temperature,
            length_penalty=self.length_penalty,
        )

    def to_sampling_params(self, default_max_tokens: int) -> SamplingParams:
        max_tokens = self.max_tokens
        if max_tokens is None:
            max_tokens = default_max_tokens

        prompt_logprobs = self.prompt_logprobs
        if prompt_logprobs is None and self.echo:
            prompt_logprobs = self.top_logprobs

        guided_json_object = None
        if (
            self.response_format is not None
            and self.response_format.type == "json_object"
        ):
            guided_json_object = True

        guided_decoding = GuidedDecodingParams.from_optional(
            json=self._get_guided_json_from_tool() or self.guided_json,
            regex=self.guided_regex,
            choice=self.guided_choice,
            grammar=self.guided_grammar,
            json_object=guided_json_object,
            backend=self.guided_decoding_backend,
            whitespace_pattern=self.guided_whitespace_pattern,
        )

        return SamplingParams.from_optional(
            n=self.n,
            best_of=self.best_of,
            presence_penalty=self.presence_penalty,
            frequency_penalty=self.frequency_penalty,
            repetition_penalty=self.repetition_penalty,
            temperature=self.temperature,
            top_p=self.top_p,
            top_k=self.top_k,
            min_p=self.min_p,
            seed=self.seed,
            stop=self.stop,
            stop_token_ids=self.stop_token_ids,
            logprobs=self.top_logprobs if self.logprobs else None,
            prompt_logprobs=prompt_logprobs,
            ignore_eos=self.ignore_eos,
            max_tokens=max_tokens,
            min_tokens=self.min_tokens,
            skip_special_tokens=self.skip_special_tokens,
            spaces_between_special_tokens=self.spaces_between_special_tokens,
            include_stop_str_in_output=self.include_stop_str_in_output,
            truncate_prompt_tokens=self.truncate_prompt_tokens,
            output_kind=(
                RequestOutputKind.DELTA if self.stream else RequestOutputKind.FINAL_ONLY
            ),
            guided_decoding=guided_decoding,
            logit_bias=self.logit_bias,
        )

    def _get_guided_json_from_tool(self) -> Optional[Union[str, dict, BaseModel]]:
        # user has chosen to not use any tool
        if self.tool_choice == "none" or self.tools is None:
            return None

        # user has chosen to use a named tool
        if type(self.tool_choice) is ChatCompletionNamedToolChoiceParam:
            tool_name = self.tool_choice.function.name
            tools = {tool.function.name: tool.function for tool in self.tools}
            if tool_name not in tools:
                raise ValueError(f"Tool '{tool_name}' has not been passed in `tools`.")
            tool = tools[tool_name]
            return tool.parameters

        return None

    @model_validator(mode="before")
    @classmethod
    def validate_stream_options(cls, data):
        if data.get("stream_options") and not data.get("stream"):
            raise ValueError("Stream options can only be defined when `stream=True`.")

        return data

    @model_validator(mode="before")
    @classmethod
    def check_logprobs(cls, data):
        if (prompt_logprobs := data.get("prompt_logprobs")) is not None:
            if data.get("stream") and prompt_logprobs > 0:
                raise ValueError(
                    "`prompt_logprobs` are not available when `stream=True`."
                )

            if prompt_logprobs < 0:
                raise ValueError("`prompt_logprobs` must be a positive value.")

        if (top_logprobs := data.get("top_logprobs")) is not None:
            if top_logprobs < 0:
                raise ValueError("`top_logprobs` must be a positive value.")

            if not data.get("logprobs"):
                raise ValueError(
                    "when using `top_logprobs`, `logprobs` must be set to true."
                )

        return data

    @model_validator(mode="before")
    @classmethod
    def check_guided_decoding_count(cls, data):
        if isinstance(data, ValueError):
            raise data

        guide_count = sum(
            [
                "guided_json" in data and data["guided_json"] is not None,
                "guided_regex" in data and data["guided_regex"] is not None,
                "guided_choice" in data and data["guided_choice"] is not None,
            ]
        )
        # you can only use one kind of guided decoding
        if guide_count > 1:
            raise ValueError(
                "You can only use one kind of guided decoding "
                "('guided_json', 'guided_regex' or 'guided_choice')."
            )
        # you can only either use guided decoding or tools, not both
        if guide_count > 1 and data.get("tool_choice", "none") not in ("none", "auto"):
            raise ValueError(
                "You can only either use guided decoding or tools, not both."
            )
        return data

    @model_validator(mode="before")
    @classmethod
    def check_tool_usage(cls, data):

        # if "tool_choice" is not specified but tools are provided,
        # default to "auto" tool_choice
        if "tool_choice" not in data and data.get("tools"):
            data["tool_choice"] = "auto"

        # if "tool_choice" is specified -- validation
        if "tool_choice" in data:

            # ensure that if "tool choice" is specified, tools are present
            if "tools" not in data or data["tools"] is None:
                raise ValueError("When using `tool_choice`, `tools` must be set.")

            # make sure that tool choice is either a named tool
            # OR that it's set to "auto"
            if data["tool_choice"] != "auto" and not isinstance(
                data["tool_choice"], dict
            ):
                raise ValueError(
                    '`tool_choice` must either be a named tool or "auto". '
                    '`tool_choice="none" is not supported.'
                )

            # ensure that if "tool_choice" is specified as an object,
            # it matches a valid tool
            if isinstance(data["tool_choice"], dict):
                valid_tool = False
                specified_function = data["tool_choice"]["function"]
                if not specified_function:
                    raise ValueError(
                        "Incorrectly formatted `tool_choice`. Should be like "
                        '`{"type": "function",'
                        ' "function": {"name": "my_function"}}`'
                    )
                specified_function_name = specified_function["name"]
                if not specified_function_name:
                    raise ValueError(
                        "Incorrectly formatted `tool_choice`. Should be like "
                        '`{"type": "function", '
                        '"function": {"name": "my_function"}}`'
                    )
                for tool in data["tools"]:
                    if tool["function"]["name"] == specified_function_name:
                        valid_tool = True
                        break
                if not valid_tool:
                    raise ValueError(
                        "The tool specified in `tool_choice` does not match any"
                        " of the specified `tools`"
                    )
        return data

    @model_validator(mode="before")
    @classmethod
    def check_generation_prompt(cls, data):
        if data.get("continue_final_message") and data.get("add_generation_prompt"):
            raise ValueError(
                "Cannot set both `continue_final_message` and "
                "`add_generation_prompt` to True."
            )
        return data


class CompletionRequest(OpenAIBaseModel):
    # Ordered by official OpenAI API documentation
    # https://platform.openai.com/docs/api-reference/completions/create
    model: str
    prompt: Union[List[int], List[List[int]], str, List[str]]
    best_of: Optional[int] = None
    echo: Optional[bool] = False
    frequency_penalty: Optional[float] = 0.0
    logit_bias: Optional[Dict[str, float]] = None
    logprobs: Optional[int] = None
    max_tokens: Optional[int] = 16
    n: int = 1
    presence_penalty: Optional[float] = 0.0
    seed: Optional[int] = Field(None)
    stop: Optional[Union[str, List[str]]] = Field(default_factory=list)
    stream: Optional[bool] = False
    stream_options: Optional[StreamOptions] = None
    suffix: Optional[str] = None
    temperature: Optional[float] = 1.0
    top_p: Optional[float] = 1.0
    user: Optional[str] = None

    # doc: begin-completion-sampling-params
    use_beam_search: bool = False
    top_k: int = -1
    min_p: float = 0.0
    repetition_penalty: float = 1.0
    length_penalty: float = 1.0
    stop_token_ids: Optional[List[int]] = Field(default_factory=list)
    include_stop_str_in_output: bool = False
    ignore_eos: bool = False
    min_tokens: int = 0
    skip_special_tokens: bool = True
    spaces_between_special_tokens: bool = True
    truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]] = None
    allowed_token_ids: Optional[List[int]] = None
    prompt_logprobs: Optional[int] = None
    # doc: end-completion-sampling-params

    # doc: begin-completion-extra-params
    add_special_tokens: bool = Field(
        default=True,
        description=(
            "If true (the default), special tokens (e.g. BOS) will be added to "
            "the prompt."
        ),
    )
    response_format: Optional[ResponseFormat] = Field(
        default=None,
        description=(
            "Similar to chat completion, this parameter specifies the format of "
            "output. Only {'type': 'json_object'} or {'type': 'text' } is "
            "supported."
        ),
    )
    guided_json: Optional[Union[str, dict, BaseModel]] = Field(
        default=None,
        description="If specified, the output will follow the JSON schema.",
    )
    guided_regex: Optional[str] = Field(
        default=None,
        description=("If specified, the output will follow the regex pattern."),
    )
    guided_choice: Optional[List[str]] = Field(
        default=None,
        description=("If specified, the output will be exactly one of the choices."),
    )
    guided_grammar: Optional[str] = Field(
        default=None,
        description=("If specified, the output will follow the context free grammar."),
    )
    guided_decoding_backend: Optional[str] = Field(
        default=None,
        description=(
            "If specified, will override the default guided decoding backend "
            "of the server for this specific request. If set, must be one of "
            "'outlines' / 'lm-format-enforcer'"
        ),
    )
    guided_whitespace_pattern: Optional[str] = Field(
        default=None,
        description=(
            "If specified, will override the default whitespace pattern "
            "for guided json decoding."
        ),
    )
    priority: int = Field(
        default=0,
        description=(
            "The priority of the request (lower means earlier handling; "
            "default: 0). Any priority other than 0 will raise an error "
            "if the served model does not use priority scheduling."
        ),
    )

    # doc: end-completion-extra-params

    def to_beam_search_params(self, default_max_tokens: int) -> BeamSearchParams:
        max_tokens = self.max_tokens
        if max_tokens is None:
            max_tokens = default_max_tokens

        n = self.n if self.n is not None else 1
        temperature = self.temperature if self.temperature is not None else 0.0

        return BeamSearchParams(
            beam_width=n,
            max_tokens=max_tokens,
            ignore_eos=self.ignore_eos,
            temperature=temperature,
            length_penalty=self.length_penalty,
        )

    def to_sampling_params(self, default_max_tokens: int) -> SamplingParams:
        max_tokens = self.max_tokens
        if max_tokens is None:
            max_tokens = default_max_tokens

        prompt_logprobs = self.prompt_logprobs
        if prompt_logprobs is None and self.echo:
            prompt_logprobs = self.logprobs

        echo_without_generation = self.echo and self.max_tokens == 0

        guided_json_object = None
        if (
            self.response_format is not None
            and self.response_format.type == "json_object"
        ):
            guided_json_object = True

        guided_decoding = GuidedDecodingParams.from_optional(
            json=self.guided_json,
            regex=self.guided_regex,
            choice=self.guided_choice,
            grammar=self.guided_grammar,
            json_object=guided_json_object,
            backend=self.guided_decoding_backend,
            whitespace_pattern=self.guided_whitespace_pattern,
        )

        return SamplingParams.from_optional(
            n=self.n,
            best_of=self.best_of,
            presence_penalty=self.presence_penalty,
            frequency_penalty=self.frequency_penalty,
            repetition_penalty=self.repetition_penalty,
            temperature=self.temperature,
            top_p=self.top_p,
            top_k=self.top_k,
            min_p=self.min_p,
            seed=self.seed,
            stop=self.stop,
            stop_token_ids=self.stop_token_ids,
            logprobs=self.logprobs,
            ignore_eos=self.ignore_eos,
            max_tokens=max_tokens if not echo_without_generation else 1,
            min_tokens=self.min_tokens,
            prompt_logprobs=prompt_logprobs,
            skip_special_tokens=self.skip_special_tokens,
            spaces_between_special_tokens=self.spaces_between_special_tokens,
            include_stop_str_in_output=self.include_stop_str_in_output,
            truncate_prompt_tokens=self.truncate_prompt_tokens,
            output_kind=(
                RequestOutputKind.DELTA if self.stream else RequestOutputKind.FINAL_ONLY
            ),
            guided_decoding=guided_decoding,
            logit_bias=self.logit_bias,
            allowed_token_ids=self.allowed_token_ids,
        )

    @model_validator(mode="before")
    @classmethod
    def check_guided_decoding_count(cls, data):
        guide_count = sum(
            [
                "guided_json" in data and data["guided_json"] is not None,
                "guided_regex" in data and data["guided_regex"] is not None,
                "guided_choice" in data and data["guided_choice"] is not None,
            ]
        )
        if guide_count > 1:
            raise ValueError(
                "You can only use one kind of guided decoding "
                "('guided_json', 'guided_regex' or 'guided_choice')."
            )
        return data

    @model_validator(mode="before")
    @classmethod
    def check_logprobs(cls, data):
        if (prompt_logprobs := data.get("prompt_logprobs")) is not None:
            if data.get("stream") and prompt_logprobs > 0:
                raise ValueError(
                    "`prompt_logprobs` are not available when `stream=True`."
                )

            if prompt_logprobs < 0:
                raise ValueError("`prompt_logprobs` must be a positive value.")

        if (logprobs := data.get("logprobs")) is not None and logprobs < 0:
            raise ValueError("`logprobs` must be a positive value.")

        return data

    @model_validator(mode="before")
    @classmethod
    def validate_stream_options(cls, data):
        if data.get("stream_options") and not data.get("stream"):
            raise ValueError("Stream options can only be defined when `stream=True`.")

        return data


class EmbeddingCompletionRequest(OpenAIBaseModel):
    # Ordered by official OpenAI API documentation
    # https://platform.openai.com/docs/api-reference/embeddings
    model: str
    input: Union[List[int], List[List[int]], str, List[str]]
    encoding_format: Literal["float", "base64"] = "float"
    dimensions: Optional[int] = None
    user: Optional[str] = None
    truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]] = None

    # doc: begin-embedding-pooling-params
    additional_data: Optional[Any] = None
    # doc: end-embedding-pooling-params

    # doc: begin-embedding-extra-params
    add_special_tokens: bool = Field(
        default=True,
        description=(
            "If true (the default), special tokens (e.g. BOS) will be added to "
            "the prompt."
        ),
    )
    priority: int = Field(
        default=0,
        description=(
            "The priority of the request (lower means earlier handling; "
            "default: 0). Any priority other than 0 will raise an error "
            "if the served model does not use priority scheduling."
        ),
    )

    # doc: end-embedding-extra-params

    def to_pooling_params(self):
        return PoolingParams(additional_data=self.additional_data)


class EmbeddingChatRequest(OpenAIBaseModel):
    model: str
    messages: List[ChatCompletionMessageParam]

    encoding_format: Literal["float", "base64"] = "float"
    dimensions: Optional[int] = None
    user: Optional[str] = None
    truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]] = None

    # doc: begin-chat-embedding-pooling-params
    additional_data: Optional[Any] = None
    # doc: end-chat-embedding-pooling-params

    # doc: begin-chat-embedding-extra-params
    add_generation_prompt: bool = Field(
        default=True,
        description=(
            "If true, the generation prompt will be added to the chat template. "
            "This is a parameter used by chat template in tokenizer config of the "
            "model."
        ),
    )
    continue_final_message: bool = Field(
        default=False,
        description=(
            "If this is set, the chat will be formatted so that the final "
            "message in the chat is open-ended, without any EOS tokens. The "
            "model will continue this message rather than starting a new one. "
            'This allows you to "prefill" part of the model\'s response for it. '
            "Cannot be used at the same time as `add_generation_prompt`."
        ),
    )
    add_special_tokens: bool = Field(
        default=False,
        description=(
            "If true, special tokens (e.g. BOS) will be added to the prompt "
            "on top of what is added by the chat template. "
            "For most models, the chat template takes care of adding the "
            "special tokens so this should be set to false (as is the "
            "default)."
        ),
    )
    chat_template: Optional[str] = Field(
        default=None,
        description=(
            "A Jinja template to use for this conversion. "
            "As of transformers v4.44, default chat template is no longer "
            "allowed, so you must provide a chat template if the tokenizer "
            "does not define one."
        ),
    )
    chat_template_kwargs: Optional[Dict[str, Any]] = Field(
        default=None,
        description=(
            "Additional kwargs to pass to the template renderer. "
            "Will be accessible by the chat template."
        ),
    )
    priority: int = Field(
        default=0,
        description=(
            "The priority of the request (lower means earlier handling; "
            "default: 0). Any priority other than 0 will raise an error "
            "if the served model does not use priority scheduling."
        ),
    )
    # doc: end-chat-embedding-extra-params

    @model_validator(mode="before")
    @classmethod
    def check_generation_prompt(cls, data):
        if data.get("continue_final_message") and data.get("add_generation_prompt"):
            raise ValueError(
                "Cannot set both `continue_final_message` and "
                "`add_generation_prompt` to True."
            )
        return data

    def to_pooling_params(self):
        return PoolingParams(additional_data=self.additional_data)


EmbeddingRequest = Union[EmbeddingCompletionRequest, EmbeddingChatRequest]


class CompletionLogProbs(OpenAIBaseModel):
    text_offset: List[int] = Field(default_factory=list)
    token_logprobs: List[Optional[float]] = Field(default_factory=list)
    tokens: List[str] = Field(default_factory=list)
    top_logprobs: List[Optional[Dict[str, float]]] = Field(default_factory=list)


class CompletionResponseChoice(OpenAIBaseModel):
    index: int
    text: str
    logprobs: Optional[CompletionLogProbs] = None
    finish_reason: Optional[str] = None
    stop_reason: Optional[Union[int, str]] = Field(
        default=None,
        description=(
            "The stop string or token id that caused the completion "
            "to stop, None if the completion finished for some other reason "
            "including encountering the EOS token"
        ),
    )
    prompt_logprobs: Optional[List[Optional[Dict[int, Logprob]]]] = None


class CompletionResponse(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"cmpl-{random_uuid()}")
    object: str = "text_completion"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    choices: List[CompletionResponseChoice]
    usage: Optional[UsageInfo] = Field(default=None)
    system_fingerprint: Optional[str] = Field(
        None,
        description="This fingerprint represents the backend configuration that the model runs with.\nCan be used in conjunction with the `seed` request parameter to understand when backend changes have been made that might impact determinism.\n",
    )


class CompletionResponseStreamChoice(OpenAIBaseModel):
    index: int
    text: str
    logprobs: Optional[CompletionLogProbs] = None
    finish_reason: Optional[str] = None
    stop_reason: Optional[Union[int, str]] = Field(
        default=None,
        description=(
            "The stop string or token id that caused the completion "
            "to stop, None if the completion finished for some other reason "
            "including encountering the EOS token"
        ),
    )


class CompletionStreamResponse(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"cmpl-{random_uuid()}")
    object: str = "text_completion"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    choices: List[CompletionResponseStreamChoice]
    usage: Optional[UsageInfo] = Field(default=None)


class EmbeddingResponseData(OpenAIBaseModel):
    index: int
    object: str = "embedding"
    embedding: Union[List[float], str]


class EmbeddingResponse(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"cmpl-{random_uuid()}")
    object: str = "list"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    data: List[EmbeddingResponseData]
    usage: UsageInfo


class FunctionCall(OpenAIBaseModel):
    name: str
    arguments: str


class ToolCall(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"chatcmpl-tool-{random_uuid()}")
    type: Literal["function"] = "function"
    function: FunctionCall


class DeltaFunctionCall(BaseModel):
    name: Optional[str] = None
    arguments: Optional[str] = None


# a tool call delta where everything is optional
class DeltaToolCall(OpenAIBaseModel):
    id: str = Field(default_factory=lambda: f"chatcmpl-tool-{random_uuid()}")
    type: Literal["function"] = "function"
    index: int
    function: Optional[DeltaFunctionCall] = None


class ExtractedToolCallInformation(BaseModel):
    # indicate if tools were called
    tools_called: bool

    # extracted tool calls
    tool_calls: List[ToolCall]

    # content - per OpenAI spec, content AND tool calls can be returned rarely
    # But some models will do this intentionally
    content: Optional[str] = None


class ChatMessage(OpenAIBaseModel):
    role: str
    content: Optional[str] = None
    tool_calls: Optional[List[ToolCall]] = Field(default_factory=list)
    function_call: Optional[FunctionCall] = Field(
        None,
        description="Deprecated and replaced by `tool_calls`. The name and arguments of a function that should be called, as generated by the model.",
    )


class TopLogprob(OpenAIBaseModel):
    token: str = Field(..., description="The token.")
    logprob: float = Field(
        ...,
        description="The log probability of this token, if it is within the top 20 most likely tokens. Otherwise, the value `-9999.0` is used to signify that the token is very unlikely.",
    )
    bytes: Optional[List[int]] = Field(
        ...,
        description="A list of integers representing the UTF-8 bytes representation of the token. Useful in instances where characters are represented by multiple tokens and their byte representations must be combined to generate the correct text representation. Can be `null` if there is no bytes representation for the token.",
    )


class ChatCompletionLogProb(OpenAIBaseModel):
    token: str
    logprob: float = -9999.0
    bytes: Optional[List[int]] = None
    top_logprobs: Optional[List[TopLogprob]] = None


class ChatCompletionLogProbsContent(ChatCompletionLogProb):
    top_logprobs: List[ChatCompletionLogProb] = Field(default_factory=list)


class ChatCompletionLogProbs(OpenAIBaseModel):
    content: Optional[List[ChatCompletionLogProbsContent]] = None


class ChatCompletionResponseChoice(OpenAIBaseModel):
    index: int
    message: ChatMessage
    logprobs: Optional[ChatCompletionLogProbs] = None
    # per OpenAI spec this is the default
    finish_reason: Optional[str] = "stop"
    # not part of the OpenAI spec but included in vLLM for legacy reasons
    stop_reason: Optional[Union[int, str]] = None


class ChatCompletionResponse(OpenAIBaseModel):
    id: str = Field(
        default_factory=lambda: f"chatcmpl-{random_uuid()}"
    )  # TODO: Check this
    object: Literal["chat.completion"] = "chat.completion"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    choices: List[ChatCompletionResponseChoice]
    usage: UsageInfo
    prompt_logprobs: Optional[List[Optional[Dict[int, Logprob]]]] = None
    system_fingerprint: Optional[str] = Field(
        None,
        description="This fingerprint represents the backend configuration that the model runs with.\nCan be used in conjunction with the `seed` request parameter to understand when backend changes have been made that might impact determinism.\n",
    )


class DeltaMessage(OpenAIBaseModel):
    role: Optional[str] = None
    content: Optional[str] = None
    tool_calls: List[DeltaToolCall] = Field(default_factory=list)
    function_call: Optional[FunctionCall2] = Field(
        None,
        description="Deprecated and replaced by `tool_calls`. The name and arguments of a function that should be called, as generated by the model.",
    )


class ChatCompletionResponseStreamChoice(OpenAIBaseModel):
    index: int
    delta: DeltaMessage
    logprobs: Optional[ChatCompletionLogProbs] = None
    finish_reason: Optional[str] = None
    stop_reason: Optional[Union[int, str]] = None


class ChatCompletionStreamResponse(OpenAIBaseModel):
    id: str = Field(
        default_factory=lambda: f"chatcmpl-{random_uuid()}"
    )  # TODO: finish this
    object: Literal["chat.completion.chunk"] = "chat.completion.chunk"
    created: int = Field(default_factory=lambda: int(time.time()))
    model: str
    choices: List[ChatCompletionResponseStreamChoice]
    usage: Optional[UsageInfo] = Field(default=None)
    system_fingerprint: Optional[str] = Field(
        None,
        description="This fingerprint represents the backend configuration that the model runs with.\nCan be used in conjunction with the `seed` request parameter to understand when backend changes have been made that might impact determinism.\n",
    )


class BatchRequestInput(OpenAIBaseModel):
    """
    The per-line object of the batch input file.

    NOTE: Currently only the `/v1/chat/completions` endpoint is supported.
    """

    # A developer-provided per-request id that will be used to match outputs to
    # inputs. Must be unique for each request in a batch.
    custom_id: str

    # The HTTP method to be used for the request. Currently only POST is
    # supported.
    method: str

    # The OpenAI API relative URL to be used for the request. Currently
    # /v1/chat/completions is supported.
    url: str

    # The parameters of the request.
    body: Union[ChatCompletionRequest, EmbeddingRequest]


class BatchResponseData(OpenAIBaseModel):
    # HTTP status code of the response.
    status_code: int = 200

    # An unique identifier for the API request.
    request_id: str

    # The body of the response.
    body: Optional[Union[ChatCompletionResponse, EmbeddingResponse]] = None


class BatchRequestOutput(OpenAIBaseModel):
    """
    The per-line object of the batch output and error files
    """

    id: str

    # A developer-provided per-request id that will be used to match outputs to
    # inputs.
    custom_id: str

    response: Optional[BatchResponseData]

    # For requests that failed with a non-HTTP error, this will contain more
    # information on the cause of the failure.
    error: Optional[Any]


class TokenizeCompletionRequest(OpenAIBaseModel):
    model: str
    prompt: str

    add_special_tokens: bool = Field(default=True)


class TokenizeChatRequest(OpenAIBaseModel):
    model: str
    messages: List[ChatCompletionMessageParam]

    add_generation_prompt: bool = Field(default=True)
    continue_final_message: bool = Field(default=False)
    add_special_tokens: bool = Field(default=False)

    @model_validator(mode="before")
    @classmethod
    def check_generation_prompt(cls, data):
        if data.get("continue_final_message") and data.get("add_generation_prompt"):
            raise ValueError(
                "Cannot set both `continue_final_message` and "
                "`add_generation_prompt` to True."
            )
        return data


TokenizeRequest = Union[TokenizeCompletionRequest, TokenizeChatRequest]


class TokenizeResponse(OpenAIBaseModel):
    count: int
    max_model_len: int
    tokens: List[int]


class DetokenizeRequest(OpenAIBaseModel):
    model: str
    tokens: List[int]


class DetokenizeResponse(OpenAIBaseModel):
    prompt: str


class LoadLoraAdapterRequest(BaseModel):
    lora_name: str
    lora_path: str


class UnloadLoraAdapterRequest(BaseModel):
    lora_name: str
    lora_int_id: Optional[int] = Field(default=None)


class DeleteModelResponse(BaseModel):
    id: str
    deleted: bool
    object: str


class ImageUrl(BaseModel):
    url: AnyUrl = Field(
        ..., description="Either a URL of the image or the base64 encoded image data."
    )
    detail: Literal["auto", "low", "high"] = Field(
        "auto",
        description="Specifies the detail level of the image. Learn more in the [Vision guide](/docs/guides/vision/low-or-high-fidelity-image-understanding).",
    )


class ChatCompletionRequestMessageContentPartImage(BaseModel):
    type: Literal["image_url"] = Field(..., description="The type of the content part.")
    image_url: ImageUrl


class ChatCompletionRequestMessageContentPartText(BaseModel):
    type: Literal["text"] = Field(..., description="The type of the content part.")
    text: str = Field(..., description="The text content.")


class ChatCompletionRequestSystemMessage(BaseModel):
    content: str = Field(..., description="The contents of the system message.")
    role: Literal["system"] = Field(
        ..., description="The role of the messages author, in this case `system`."
    )
    name: Optional[str] = Field(
        None,
        description="An optional name for the participant. Provides the model information to differentiate between participants of the same role.",
    )


class ChatCompletionRequestToolMessage(BaseModel):
    role: Literal["tool"] = Field(
        ..., description="The role of the messages author, in this case `tool`."
    )
    content: str = Field(..., description="The contents of the tool message.")
    tool_call_id: str = Field(
        ..., description="Tool call that this message is responding to."
    )


class ChatCompletionRequestFunctionMessage(BaseModel):
    role: Literal["function"] = Field(
        ..., description="The role of the messages author, in this case `function`."
    )
    content: Optional[str] = Field(
        ..., description="The contents of the function message."
    )
    name: str = Field(..., description="The name of the function to call.")


class FunctionParameters(BaseModel):
    pass
    model_config = ConfigDict(
        extra="allow",
    )


class ChatCompletionFunctions(BaseModel):
    description: Optional[str] = Field(
        None,
        description="A description of what the function does, used by the model to choose when and how to call the function.",
    )
    name: str = Field(
        ...,
        description="The name of the function to be called. Must be a-z, A-Z, 0-9, or contain underscores and dashes, with a maximum length of 64.",
    )
    parameters: Optional[FunctionParameters] = None


class ChatCompletionFunctionCallOption(BaseModel):
    name: str = Field(..., description="The name of the function to call.")


class FunctionObject(BaseModel):
    description: Optional[str] = Field(
        None,
        description="A description of what the function does, used by the model to choose when and how to call the function.",
    )
    name: str = Field(
        ...,
        description="The name of the function to be called. Must be a-z, A-Z, 0-9, or contain underscores and dashes, with a maximum length of 64.",
    )
    parameters: Optional[FunctionParameters] = None


class Function(BaseModel):
    name: str = Field(..., description="The name of the function to call.")


class ChatCompletionNamedToolChoice(BaseModel):
    type: Literal["function"] = Field(
        ...,
        description="The type of the tool. Currently, only `function` is supported.",
    )
    function: Function


class Function1(BaseModel):
    name: str = Field(..., description="The name of the function to call.")
    arguments: str = Field(
        ...,
        description="The arguments to call the function with, as generated by the model in JSON format. Note that the model does not always generate valid JSON, and may hallucinate parameters not defined by your function schema. Validate the arguments in your code before calling your function.",
    )


class ChatCompletionMessageToolCall(BaseModel):
    id: str = Field(..., description="The ID of the tool call.")
    type: Literal["function"] = Field(
        ...,
        description="The type of the tool. Currently, only `function` is supported.",
    )
    function: Function1 = Field(..., description="The function that the model called.")


class Function2(BaseModel):
    name: Optional[str] = Field(None, description="The name of the function to call.")
    arguments: Optional[str] = Field(
        None,
        description="The arguments to call the function with, as generated by the model in JSON format. Note that the model does not always generate valid JSON, and may hallucinate parameters not defined by your function schema. Validate the arguments in your code before calling your function.",
    )


class ChatCompletionMessageToolCallChunk(BaseModel):
    index: int
    id: Optional[str] = Field(None, description="The ID of the tool call.")
    type: Optional[Literal["function"]] = Field(
        None,
        description="The type of the tool. Currently, only `function` is supported.",
    )
    function: Optional[Function2] = None


class ChatCompletionRole(
    RootModel[Literal["system", "user", "assistant", "tool", "function"]]
):
    root: Literal["system", "user", "assistant", "tool", "function"] = Field(
        ..., description="The role of the author of a message"
    )


class FunctionCall2(BaseModel):
    arguments: Optional[str] = Field(
        None,
        description="The arguments to call the function with, as generated by the model in JSON format. Note that the model does not always generate valid JSON, and may hallucinate parameters not defined by your function schema. Validate the arguments in your code before calling your function.",
    )
    name: Optional[str] = Field(None, description="The name of the function to call.")


class CreateChatCompletionImageResponse(BaseModel):
    pass


class CreateImageRequest(BaseModel):
    prompt: str = Field(
        ...,
        description="A text description of the desired image(s). The maximum length is 1000 characters for `dall-e-2` and 4000 characters for `dall-e-3`.",
        examples=["A cute baby sea otter"],
    )
    model: Optional[Union[Optional[str], Literal["dall-e-2", "dall-e-3"]]] = Field(
        "dall-e-2",
        description="The model to use for image generation.",
        examples=["dall-e-3"],
    )
    n: Optional[conint(ge=1, le=10)] = Field(
        1,
        description="The number of images to generate. Must be between 1 and 10. For `dall-e-3`, only `n=1` is supported.",
        examples=[1],
    )
    quality: Literal["standard", "hd"] = Field(
        "standard",
        description="The quality of the image that will be generated. `hd` creates images with finer details and greater consistency across the image. This param is only supported for `dall-e-3`.",
        examples=["standard"],
    )
    response_format: Optional[Literal["url", "b64_json"]] = Field(
        "url",
        description="The format in which the generated images are returned. Must be one of `url` or `b64_json`. URLs are only valid for 60 minutes after the image has been generated.",
        examples=["url"],
    )
    size: Optional[
        Literal["256x256", "512x512", "1024x1024", "1792x1024", "1024x1792"]
    ] = Field(
        "1024x1024",
        description="The size of the generated images. Must be one of `256x256`, `512x512`, or `1024x1024` for `dall-e-2`. Must be one of `1024x1024`, `1792x1024`, or `1024x1792` for `dall-e-3` models.",
        examples=["1024x1024"],
    )
    style: Optional[Literal["vivid", "natural"]] = Field(
        "vivid",
        description="The style of the generated images. Must be one of `vivid` or `natural`. Vivid causes the model to lean towards generating hyper-real and dramatic images. Natural causes the model to produce more natural, less hyper-real looking images. This param is only supported for `dall-e-3`.",
        examples=["vivid"],
    )
    user: Optional[str] = Field(
        None,
        description="A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse. [Learn more](/docs/guides/safety-best-practices/end-user-ids).\n",
        examples=["user-1234"],
    )


class Image(BaseModel):
    b64_json: Optional[str] = Field(
        None,
        description="The base64-encoded JSON of the generated image, if `response_format` is `b64_json`.",
    )
    url: Optional[str] = Field(
        None,
        description="The URL of the generated image, if `response_format` is `url` (default).",
    )
    revised_prompt: Optional[str] = Field(
        None,
        description="The prompt that was used to generate the image, if there was any revision to the prompt.",
    )


class CreateImageEditRequest(BaseModel):
    image: bytes = Field(
        ...,
        description="The image to edit. Must be a valid PNG file, less than 4MB, and square. If mask is not provided, image must have transparency, which will be used as the mask.",
    )
    prompt: str = Field(
        ...,
        description="A text description of the desired image(s). The maximum length is 1000 characters.",
        examples=["A cute baby sea otter wearing a beret"],
    )
    mask: Optional[bytes] = Field(
        None,
        description="An additional image whose fully transparent areas (e.g. where alpha is zero) indicate where `image` should be edited. Must be a valid PNG file, less than 4MB, and have the same dimensions as `image`.",
    )
    model: Optional[Union[Optional[str], Literal["dall-e-2"]]] = Field(
        "dall-e-2",
        description="The model to use for image generation. Only `dall-e-2` is supported at this time.",
        examples=["dall-e-2"],
    )
    n: Optional[conint(ge=1, le=10)] = Field(
        1,
        description="The number of images to generate. Must be between 1 and 10.",
        examples=[1],
    )
    size: Optional[Literal["256x256", "512x512", "1024x1024"]] = Field(
        "1024x1024",
        description="The size of the generated images. Must be one of `256x256`, `512x512`, or `1024x1024`.",
        examples=["1024x1024"],
    )
    response_format: Optional[Literal["url", "b64_json"]] = Field(
        "url",
        description="The format in which the generated images are returned. Must be one of `url` or `b64_json`. URLs are only valid for 60 minutes after the image has been generated.",
        examples=["url"],
    )
    user: Optional[str] = Field(
        None,
        description="A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse. [Learn more](/docs/guides/safety-best-practices/end-user-ids).\n",
        examples=["user-1234"],
    )


class CreateImageVariationRequest(BaseModel):
    image: bytes = Field(
        ...,
        description="The image to use as the basis for the variation(s). Must be a valid PNG file, less than 4MB, and square.",
    )
    model: Optional[Union[Optional[str], Literal["dall-e-2"]]] = Field(
        "dall-e-2",
        description="The model to use for image generation. Only `dall-e-2` is supported at this time.",
        examples=["dall-e-2"],
    )
    n: Optional[conint(ge=1, le=10)] = Field(
        1,
        description="The number of images to generate. Must be between 1 and 10. For `dall-e-3`, only `n=1` is supported.",
        examples=[1],
    )
    response_format: Optional[Literal["url", "b64_json"]] = Field(
        "url",
        description="The format in which the generated images are returned. Must be one of `url` or `b64_json`. URLs are only valid for 60 minutes after the image has been generated.",
        examples=["url"],
    )
    size: Optional[Literal["256x256", "512x512", "1024x1024"]] = Field(
        "1024x1024",
        description="The size of the generated images. Must be one of `256x256`, `512x512`, or `1024x1024`.",
        examples=["1024x1024"],
    )
    user: Optional[str] = Field(
        None,
        description="A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse. [Learn more](/docs/guides/safety-best-practices/end-user-ids).\n",
        examples=["user-1234"],
    )


class CreateModerationRequest(BaseModel):
    input: Union[str, List[str]] = Field(..., description="The input text to classify")
    model: Union[str, Literal["text-moderation-latest", "text-moderation-stable"]] = (
        Field(
            "text-moderation-latest",
            description="Two content moderations models are available: `text-moderation-stable` and `text-moderation-latest`.\n\nThe default is `text-moderation-latest` which will be automatically upgraded over time. This ensures you are always using our most accurate model. If you use `text-moderation-stable`, we will provide advanced notice before updating the model. Accuracy of `text-moderation-stable` may be slightly lower than for `text-moderation-latest`.\n",
            examples=["text-moderation-stable"],
        )
    )


class Categories(BaseModel):
    hate: bool = Field(
        ...,
        description="Content that expresses, incites, or promotes hate based on race, gender, ethnicity, religion, nationality, sexual orientation, disability status, or caste. Hateful content aimed at non-protected groups (e.g., chess players) is harassment.",
    )
    hate_threatening: bool = Field(
        ...,
        alias="hate/threatening",
        description="Hateful content that also includes violence or serious harm towards the targeted group based on race, gender, ethnicity, religion, nationality, sexual orientation, disability status, or caste.",
    )
    harassment: bool = Field(
        ...,
        description="Content that expresses, incites, or promotes harassing language towards any target.",
    )
    harassment_threatening: bool = Field(
        ...,
        alias="harassment/threatening",
        description="Harassment content that also includes violence or serious harm towards any target.",
    )
    self_harm: bool = Field(
        ...,
        alias="self-harm",
        description="Content that promotes, encourages, or depicts acts of self-harm, such as suicide, cutting, and eating disorders.",
    )
    self_harm_intent: bool = Field(
        ...,
        alias="self-harm/intent",
        description="Content where the speaker expresses that they are engaging or intend to engage in acts of self-harm, such as suicide, cutting, and eating disorders.",
    )
    self_harm_instructions: bool = Field(
        ...,
        alias="self-harm/instructions",
        description="Content that encourages performing acts of self-harm, such as suicide, cutting, and eating disorders, or that gives instructions or advice on how to commit such acts.",
    )
    sexual: bool = Field(
        ...,
        description="Content meant to arouse sexual excitement, such as the description of sexual activity, or that promotes sexual services (excluding sex education and wellness).",
    )
    sexual_minors: bool = Field(
        ...,
        alias="sexual/minors",
        description="Sexual content that includes an individual who is under 18 years old.",
    )
    violence: bool = Field(
        ..., description="Content that depicts death, violence, or physical injury."
    )
    violence_graphic: bool = Field(
        ...,
        alias="violence/graphic",
        description="Content that depicts death, violence, or physical injury in graphic detail.",
    )


class CategoryScores(BaseModel):
    hate: float = Field(..., description="The score for the category 'hate'.")
    hate_threatening: float = Field(
        ...,
        alias="hate/threatening",
        description="The score for the category 'hate/threatening'.",
    )
    harassment: float = Field(
        ..., description="The score for the category 'harassment'."
    )
    harassment_threatening: float = Field(
        ...,
        alias="harassment/threatening",
        description="The score for the category 'harassment/threatening'.",
    )
    self_harm: float = Field(
        ..., alias="self-harm", description="The score for the category 'self-harm'."
    )
    self_harm_intent: float = Field(
        ...,
        alias="self-harm/intent",
        description="The score for the category 'self-harm/intent'.",
    )
    self_harm_instructions: float = Field(
        ...,
        alias="self-harm/instructions",
        description="The score for the category 'self-harm/instructions'.",
    )
    sexual: float = Field(..., description="The score for the category 'sexual'.")
    sexual_minors: float = Field(
        ...,
        alias="sexual/minors",
        description="The score for the category 'sexual/minors'.",
    )
    violence: float = Field(..., description="The score for the category 'violence'.")
    violence_graphic: float = Field(
        ...,
        alias="violence/graphic",
        description="The score for the category 'violence/graphic'.",
    )


class Result(BaseModel):
    flagged: bool = Field(
        ..., description="Whether any of the below categories are flagged."
    )
    categories: Categories = Field(
        ...,
        description="A list of the categories, and whether they are flagged or not.",
    )
    category_scores: CategoryScores = Field(
        ...,
        description="A list of the categories along with their scores as predicted by model.",
    )


class CreateModerationResponse(BaseModel):
    id: str = Field(
        ..., description="The unique identifier for the moderation request."
    )
    model: str = Field(
        ..., description="The model used to generate the moderation results."
    )
    results: List[Result] = Field(..., description="A list of moderation objects.")


class CreateFileRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    file: bytes = Field(
        ..., description="The File object (not file name) to be uploaded.\n"
    )
    purpose: Literal["fine-tune", "assistants"] = Field(
        ...,
        description='The intended purpose of the uploaded file.\n\nUse "fine-tune" for [Fine-tuning](/docs/api-reference/fine-tuning) and "assistants" for [Assistants](/docs/api-reference/assistants) and [Messages](/docs/api-reference/messages). This allows us to validate the format of the uploaded file is correct for fine-tuning.\n',
    )


class DeleteFileResponse(BaseModel):
    id: str
    object: Literal["file"]
    deleted: bool


class Hyperparameters(BaseModel):
    batch_size: Union[Literal["auto"], conint(ge=1, le=256)] = Field(
        "auto",
        description="Number of examples in each batch. A larger batch size means that model parameters\nare updated less frequently, but with lower variance.\n",
    )
    learning_rate_multiplier: Union[Literal["auto"], PositiveFloat] = Field(
        "auto",
        description="Scaling factor for the learning rate. A smaller learning rate may be useful to avoid\noverfitting.\n",
    )
    n_epochs: Union[Literal["auto"], conint(ge=1, le=50)] = Field(
        "auto",
        description="The number of epochs to train the model for. An epoch refers to one full cycle\nthrough the training dataset.\n",
    )


class Wandb(BaseModel):
    project: str = Field(
        ...,
        description="The name of the project that the new run will be created under.\n",
        examples=["my-wandb-project"],
    )
    name: Optional[str] = Field(
        None,
        description="A display name to set for the run. If not set, we will use the Job ID as the name.\n",
    )
    entity: Optional[str] = Field(
        None,
        description="The entity to use for the run. This allows you to set the team or username of the WandB user that you would\nlike associated with the run. If not set, the default entity for the registered WandB API key is used.\n",
    )
    tags: Optional[List[str]] = Field(
        None,
        description='A list of tags to be attached to the newly created run. These tags are passed through directly to WandB. Some\ndefault tags are generated by OpenAI: "openai/finetune", "openai/{base-model}", "openai/{ftjob-abcdef}".\n',
    )


class Integration(BaseModel):
    type: Literal["wandb"] = Field(
        ...,
        description='The type of integration to enable. Currently, only "wandb" (Weights and Biases) is supported.\n',
    )
    wandb: Wandb = Field(
        ...,
        description="The settings for your integration with Weights and Biases. This payload specifies the project that\nmetrics will be sent to. Optionally, you can set an explicit display name for your run, add tags\nto your run, and set a default entity (team, username, etc) to be associated with your run.\n",
    )


class CreateFineTuningJobRequest(BaseModel):
    model: Union[str, Literal["babbage-002", "davinci-002", "gpt-3.5-turbo"]] = Field(
        ...,
        description="The name of the model to fine-tune. You can select one of the\n[supported models](/docs/guides/fine-tuning/what-models-can-be-fine-tuned).\n",
        examples=["gpt-3.5-turbo"],
    )
    training_file: str = Field(
        ...,
        description="The ID of an uploaded file that contains training data.\n\nSee [upload file](/docs/api-reference/files/upload) for how to upload a file.\n\nYour dataset must be formatted as a JSONL file. Additionally, you must upload your file with the purpose `fine-tune`.\n\nSee the [fine-tuning guide](/docs/guides/fine-tuning) for more details.\n",
        examples=["file-abc123"],
    )
    hyperparameters: Optional[Hyperparameters] = Field(
        None, description="The hyperparameters used for the fine-tuning job."
    )
    suffix: Optional[constr(min_length=1, max_length=40)] = Field(
        None,
        description='A string of up to 18 characters that will be added to your fine-tuned model name.\n\nFor example, a `suffix` of "custom-model-name" would produce a model name like `ft:gpt-3.5-turbo:openai:custom-model-name:7p4lURel`.\n',
    )
    validation_file: Optional[str] = Field(
        None,
        description="The ID of an uploaded file that contains validation data.\n\nIf you provide this file, the data is used to generate validation\nmetrics periodically during fine-tuning. These metrics can be viewed in\nthe fine-tuning results file.\nThe same data should not be present in both train and validation files.\n\nYour dataset must be formatted as a JSONL file. You must upload your file with the purpose `fine-tune`.\n\nSee the [fine-tuning guide](/docs/guides/fine-tuning) for more details.\n",
        examples=["file-abc123"],
    )
    integrations: Optional[List[Integration]] = Field(
        None, description="A list of integrations to enable for your fine-tuning job."
    )
    seed: Optional[conint(ge=0, le=2147483647)] = Field(
        None,
        description="The seed controls the reproducibility of the job. Passing in the same seed and job parameters should produce the same results, but may differ in rare cases.\nIf a seed is not specified, one will be generated for you.\n",
        examples=[42],
    )


class CreateEmbeddingRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    input: Union[str, List[str], List[int], List[List[Any]]] = Field(
        ...,
        description="Input text to embed, encoded as a string or array of tokens. To embed multiple inputs in a single request, pass an array of strings or array of token arrays. The input must not exceed the max input tokens for the model (8192 tokens for `text-embedding-ada-002`), cannot be an empty string, and any array must be 2048 dimensions or less. [Example Python code](https://cookbook.openai.com/examples/how_to_count_tokens_with_tiktoken) for counting tokens.\n",
        examples=["The quick brown fox jumped over the lazy dog"],
    )
    model: Union[
        str,
        Literal[
            "text-embedding-ada-002", "text-embedding-3-small", "text-embedding-3-large"
        ],
    ] = Field(
        ...,
        description="ID of the model to use. You can use the [List models](/docs/api-reference/models/list) API to see all of your available models, or see our [Model overview](/docs/models/overview) for descriptions of them.\n",
        examples=["text-embedding-3-small"],
    )
    encoding_format: Literal["float", "base64"] = Field(
        "float",
        description="The format to return the embeddings in. Can be either `float` or [`base64`](https://pypi.org/project/pybase64/).",
        examples=["float"],
    )
    dimensions: Optional[conint(ge=1)] = Field(
        None,
        description="The number of dimensions the resulting output embeddings should have. Only supported in `text-embedding-3` and later models.\n",
    )
    user: Optional[str] = Field(
        None,
        description="A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse. [Learn more](/docs/guides/safety-best-practices/end-user-ids).\n",
        examples=["user-1234"],
    )


class Usage(BaseModel):
    prompt_tokens: int = Field(
        ..., description="The number of tokens used by the prompt."
    )
    total_tokens: int = Field(
        ..., description="The total number of tokens used by the request."
    )


class CreateTranscriptionRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    file: bytes = Field(
        ...,
        description="The audio file object (not file name) to transcribe, in one of these formats: flac, mp3, mp4, mpeg, mpga, m4a, ogg, wav, or webm.\n",
    )
    model: Union[str, Literal["whisper-1"]] = Field(
        ...,
        description="ID of the model to use. Only `whisper-1` (which is powered by our open source Whisper V2 model) is currently available.\n",
        examples=["whisper-1"],
    )
    language: Optional[str] = Field(
        None,
        description="The language of the input audio. Supplying the input language in [ISO-639-1](https://en.wikipedia.org/wiki/List_of_ISO_639-1_codes) format will improve accuracy and latency.\n",
    )
    prompt: Optional[str] = Field(
        None,
        description="An optional text to guide the model's style or continue a previous audio segment. The [prompt](/docs/guides/speech-to-text/prompting) should match the audio language.\n",
    )
    response_format: Literal["json", "text", "srt", "verbose_json", "vtt"] = Field(
        "json",
        description="The format of the transcript output, in one of these options: `json`, `text`, `srt`, `verbose_json`, or `vtt`.\n",
    )
    temperature: float = Field(
        0,
        description="The sampling temperature, between 0 and 1. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic. If set to 0, the model will use [log probability](https://en.wikipedia.org/wiki/Log_probability) to automatically increase the temperature until certain thresholds are hit.\n",
    )
    timestamp_granularities__: List[Literal["word", "segment"]] = Field(
        ["segment"],
        alias="timestamp_granularities[]",
        description="The timestamp granularities to populate for this transcription. `response_format` must be set `verbose_json` to use timestamp granularities. Either or both of these options are supported: `word`, or `segment`. Note: There is no additional latency for segment timestamps, but generating word timestamps incurs additional latency.\n",
    )


class CreateTranscriptionResponseJson(BaseModel):
    text: str = Field(..., description="The transcribed text.")


class TranscriptionSegment(BaseModel):
    id: int = Field(..., description="Unique identifier of the segment.")
    seek: int = Field(..., description="Seek offset of the segment.")
    start: float = Field(..., description="Start time of the segment in seconds.")
    end: float = Field(..., description="End time of the segment in seconds.")
    text: str = Field(..., description="Text content of the segment.")
    tokens: List[int] = Field(
        ..., description="Array of token IDs for the text content."
    )
    temperature: float = Field(
        ..., description="Temperature parameter used for generating the segment."
    )
    avg_logprob: float = Field(
        ...,
        description="Average logprob of the segment. If the value is lower than -1, consider the logprobs failed.",
    )
    compression_ratio: float = Field(
        ...,
        description="Compression ratio of the segment. If the value is greater than 2.4, consider the compression failed.",
    )
    no_speech_prob: float = Field(
        ...,
        description="Probability of no speech in the segment. If the value is higher than 1.0 and the `avg_logprob` is below -1, consider this segment silent.",
    )


class TranscriptionWord(BaseModel):
    word: str = Field(..., description="The text content of the word.")
    start: float = Field(..., description="Start time of the word in seconds.")
    end: float = Field(..., description="End time of the word in seconds.")


class CreateTranscriptionResponseVerboseJson(BaseModel):
    language: str = Field(..., description="The language of the input audio.")
    duration: str = Field(..., description="The duration of the input audio.")
    text: str = Field(..., description="The transcribed text.")
    words: Optional[List[TranscriptionWord]] = Field(
        None, description="Extracted words and their corresponding timestamps."
    )
    segments: Optional[List[TranscriptionSegment]] = Field(
        None,
        description="Segments of the transcribed text and their corresponding details.",
    )


class CreateTranslationRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    file: bytes = Field(
        ...,
        description="The audio file object (not file name) translate, in one of these formats: flac, mp3, mp4, mpeg, mpga, m4a, ogg, wav, or webm.\n",
    )
    model: Union[str, Literal["whisper-1"]] = Field(
        ...,
        description="ID of the model to use. Only `whisper-1` (which is powered by our open source Whisper V2 model) is currently available.\n",
        examples=["whisper-1"],
    )
    prompt: Optional[str] = Field(
        None,
        description="An optional text to guide the model's style or continue a previous audio segment. The [prompt](/docs/guides/speech-to-text/prompting) should be in English.\n",
    )
    response_format: str = Field(
        "json",
        description="The format of the transcript output, in one of these options: `json`, `text`, `srt`, `verbose_json`, or `vtt`.\n",
    )
    temperature: float = Field(
        0,
        description="The sampling temperature, between 0 and 1. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic. If set to 0, the model will use [log probability](https://en.wikipedia.org/wiki/Log_probability) to automatically increase the temperature until certain thresholds are hit.\n",
    )


class CreateTranslationResponseJson(BaseModel):
    text: str


class CreateTranslationResponseVerboseJson(BaseModel):
    language: str = Field(
        ..., description="The language of the output translation (always `english`)."
    )
    duration: str = Field(..., description="The duration of the input audio.")
    text: str = Field(..., description="The translated text.")
    segments: Optional[List[TranscriptionSegment]] = Field(
        None,
        description="Segments of the translated text and their corresponding details.",
    )


class CreateSpeechRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    model: Union[str, Literal["tts-1", "tts-1-hd"]] = Field(
        ...,
        description="One of the available [TTS models](/docs/models/tts): `tts-1` or `tts-1-hd`\n",
    )
    input: constr(max_length=4096) = Field(
        ...,
        description="The text to generate audio for. The maximum length is 4096 characters.",
    )
    voice: Literal["alloy", "echo", "fable", "onyx", "nova", "shimmer"] = Field(
        ...,
        description="The voice to use when generating the audio. Supported voices are `alloy`, `echo`, `fable`, `onyx`, `nova`, and `shimmer`. Previews of the voices are available in the [Text to speech guide](/docs/guides/text-to-speech/voice-options).",
    )
    response_format: Literal["mp3", "opus", "aac", "flac", "wav", "pcm"] = Field(
        "mp3",
        description="The format to audio in. Supported formats are `mp3`, `opus`, `aac`, `flac`, `wav`, and `pcm`.",
    )
    speed: confloat(ge=0.25, le=4.0) = Field(
        1.0,
        description="The speed of the generated audio. Select a value from `0.25` to `4.0`. `1.0` is the default.",
    )


class Model(BaseModel):
    id: str = Field(
        ...,
        description="The model identifier, which can be referenced in the API endpoints.",
    )
    created: int = Field(
        ..., description="The Unix timestamp (in seconds) when the model was created."
    )
    object: Literal["model"] = Field(
        ..., description='The object type, which is always "model".'
    )
    owned_by: str = Field(..., description="The organization that owns the model.")


class OpenAIFile(BaseModel):
    id: str = Field(
        ...,
        description="The file identifier, which can be referenced in the API endpoints.",
    )
    bytes: int = Field(..., description="The size of the file, in bytes.")
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the file was created.",
    )
    filename: str = Field(..., description="The name of the file.")
    object: Literal["file"] = Field(
        ..., description="The object type, which is always `file`."
    )
    purpose: Literal[
        "fine-tune", "fine-tune-results", "assistants", "assistants_output"
    ] = Field(
        ...,
        description="The intended purpose of the file. Supported values are `fine-tune`, `fine-tune-results`, `assistants`, and `assistants_output`.",
    )
    status: Literal["uploaded", "processed", "error"] = Field(
        ...,
        description="Deprecated. The current status of the file, which can be either `uploaded`, `processed`, or `error`.",
    )
    status_details: Optional[str] = Field(
        None,
        description="Deprecated. For details on why a fine-tuning training file failed validation, see the `error` field on `fine_tuning.job`.",
    )


class Embedding(BaseModel):
    index: int = Field(
        ..., description="The index of the embedding in the list of embeddings."
    )
    embedding: List[float] = Field(
        ...,
        description="The embedding vector, which is a list of floats. The length of vector depends on the model as listed in the [embedding guide](/docs/guides/embeddings).\n",
    )
    object: Literal["embedding"] = Field(
        ..., description='The object type, which is always "embedding".'
    )


class Error1(BaseModel):
    code: str = Field(..., description="A machine-readable error code.")
    message: str = Field(..., description="A human-readable error message.")
    param: Optional[str] = Field(
        ...,
        description="The parameter that was invalid, usually `training_file` or `validation_file`. This field will be null if the failure was not parameter-specific.",
    )


class Hyperparameters1(BaseModel):
    n_epochs: Union[Literal["auto"], conint(ge=1, le=50)] = Field(
        ...,
        description='The number of epochs to train the model for. An epoch refers to one full cycle through the training dataset.\n"auto" decides the optimal number of epochs based on the size of the dataset. If setting the number manually, we support any number between 1 and 50 epochs.',
    )


class FineTuningIntegration(BaseModel):
    type: Literal["wandb"] = Field(
        ...,
        description="The type of the integration being enabled for the fine-tuning job",
    )
    wandb: Wandb = Field(
        ...,
        description="The settings for your integration with Weights and Biases. This payload specifies the project that\nmetrics will be sent to. Optionally, you can set an explicit display name for your run, add tags\nto your run, and set a default entity (team, username, etc) to be associated with your run.\n",
    )


class FineTuningJobEvent(BaseModel):
    id: str
    created_at: int
    level: Literal["info", "warn", "error"]
    message: str
    object: Literal["fine_tuning.job.event"]


class Metrics(BaseModel):
    step: Optional[float] = None
    train_loss: Optional[float] = None
    train_mean_token_accuracy: Optional[float] = None
    valid_loss: Optional[float] = None
    valid_mean_token_accuracy: Optional[float] = None
    full_valid_loss: Optional[float] = None
    full_valid_mean_token_accuracy: Optional[float] = None


class FineTuningJobCheckpoint(BaseModel):
    id: str = Field(
        ...,
        description="The checkpoint identifier, which can be referenced in the API endpoints.",
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the checkpoint was created.",
    )
    fine_tuned_model_checkpoint: str = Field(
        ..., description="The name of the fine-tuned checkpoint model that is created."
    )
    step_number: int = Field(
        ..., description="The step number that the checkpoint was created at."
    )
    metrics: Metrics = Field(
        ..., description="Metrics at the step number during the fine-tuning job."
    )
    fine_tuning_job_id: str = Field(
        ...,
        description="The name of the fine-tuning job that this checkpoint was created from.",
    )
    object: Literal["fine_tuning.job.checkpoint"] = Field(
        ...,
        description='The object type, which is always "fine_tuning.job.checkpoint".',
    )


class CompletionUsage(BaseModel):
    completion_tokens: int = Field(
        ..., description="Number of tokens in the generated completion."
    )
    prompt_tokens: int = Field(..., description="Number of tokens in the prompt.")
    total_tokens: int = Field(
        ...,
        description="Total number of tokens used in the request (prompt + completion).",
    )


class RunCompletionUsage(BaseModel):
    completion_tokens: int = Field(
        ..., description="Number of completion tokens used over the course of the run."
    )
    prompt_tokens: int = Field(
        ..., description="Number of prompt tokens used over the course of the run."
    )
    total_tokens: int = Field(
        ..., description="Total number of tokens used (prompt + completion)."
    )


class RunStepCompletionUsage(BaseModel):
    completion_tokens: int = Field(
        ...,
        description="Number of completion tokens used over the course of the run step.",
    )
    prompt_tokens: int = Field(
        ..., description="Number of prompt tokens used over the course of the run step."
    )
    total_tokens: int = Field(
        ..., description="Total number of tokens used (prompt + completion)."
    )


class DeleteAssistantResponse(BaseModel):
    id: str
    deleted: bool
    object: Literal["assistant.deleted"]


class AssistantToolsCode(BaseModel):
    type: Literal["code_interpreter"] = Field(
        ..., description="The type of tool being defined: `code_interpreter`"
    )


class AssistantToolsRetrieval(BaseModel):
    type: Literal["retrieval"] = Field(
        ..., description="The type of tool being defined: `retrieval`"
    )


class AssistantToolsFunction(BaseModel):
    type: Literal["function"] = Field(
        ..., description="The type of tool being defined: `function`"
    )
    function: FunctionObject


class LastError(BaseModel):
    code: Literal["server_error", "rate_limit_exceeded", "invalid_prompt"] = Field(
        ...,
        description="One of `server_error`, `rate_limit_exceeded`, or `invalid_prompt`.",
    )
    message: str = Field(..., description="A human-readable description of the error.")


class ModifyRunRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class ToolOutput(BaseModel):
    tool_call_id: Optional[str] = Field(
        None,
        description="The ID of the tool call in the `required_action` object within the run object the output is being submitted for.",
    )
    output: Optional[str] = Field(
        None,
        description="The output of the tool call to be submitted to continue the run.",
    )


class SubmitToolOutputsRunRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    tool_outputs: List[ToolOutput] = Field(
        ..., description="A list of tools for which the outputs are being submitted."
    )
    stream: Optional[bool] = Field(
        None,
        description="If `true`, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a `data: [DONE]` message.\n",
    )


class Function3(BaseModel):
    name: str = Field(..., description="The name of the function.")
    arguments: str = Field(
        ...,
        description="The arguments that the model expects you to pass to the function.",
    )


class RunToolCallObject(BaseModel):
    id: str = Field(
        ...,
        description="The ID of the tool call. This ID must be referenced when you submit the tool outputs in using the [Submit tool outputs to run](/docs/api-reference/runs/submitToolOutputs) endpoint.",
    )
    type: Literal["function"] = Field(
        ...,
        description="The type of tool call the output is required for. For now, this is always `function`.",
    )
    function: Function3 = Field(..., description="The function definition.")


class ThreadObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["thread"] = Field(
        ..., description="The object type, which is always `thread`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the thread was created.",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        ...,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class ModifyThreadRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class DeleteThreadResponse(BaseModel):
    id: str
    deleted: bool
    object: Literal["thread.deleted"]


class ListThreadsResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[ThreadObject]
    first_id: str = Field(..., examples=["asst_abc123"])
    last_id: str = Field(..., examples=["asst_abc456"])
    has_more: bool = Field(..., examples=[False])


class IncompleteDetails(BaseModel):
    reason: Literal[
        "content_filter", "max_tokens", "run_cancelled", "run_expired", "run_failed"
    ] = Field(..., description="The reason the message is incomplete.")


class CreateMessageRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    role: Literal["user", "assistant"] = Field(
        ...,
        description="The role of the entity that is creating the message. Allowed values include:\n- `user`: Indicates the message is sent by an actual user and should be used in most cases to represent user-generated messages.\n- `assistant`: Indicates the message is generated by the assistant. Use this value to insert messages from the assistant into the conversation.\n",
    )
    content: constr(min_length=1, max_length=256000) = Field(
        ..., description="The content of the message."
    )
    file_ids: List[str] = Field(
        [],
        description="A list of [File](/docs/api-reference/files) IDs that the message should use. There can be a maximum of 10 files attached to a message. Useful for tools like `retrieval` and `code_interpreter` that can access and use files.",
        max_length=10,
        min_length=1,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class ModifyMessageRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class DeleteMessageResponse(BaseModel):
    id: str
    deleted: bool
    object: Literal["thread.message.deleted"]


class ImageFile(BaseModel):
    file_id: str = Field(
        ...,
        description="The [File](/docs/api-reference/files) ID of the image in the message content.",
    )


class MessageContentImageFileObject(BaseModel):
    type: Literal["image_file"] = Field(..., description="Always `image_file`.")
    image_file: ImageFile


class ImageFile1(BaseModel):
    file_id: Optional[str] = Field(
        None,
        description="The [File](/docs/api-reference/files) ID of the image in the message content.",
    )


class MessageDeltaContentImageFileObject(BaseModel):
    index: int = Field(..., description="The index of the content part in the message.")
    type: Literal["image_file"] = Field(..., description="Always `image_file`.")
    image_file: Optional[ImageFile1] = None


class FileCitation(BaseModel):
    file_id: str = Field(
        ..., description="The ID of the specific File the citation is from."
    )
    quote: str = Field(..., description="The specific quote in the file.")


class MessageContentTextAnnotationsFileCitationObject(BaseModel):
    type: Literal["file_citation"] = Field(..., description="Always `file_citation`.")
    text: str = Field(
        ..., description="The text in the message content that needs to be replaced."
    )
    file_citation: FileCitation
    start_index: conint(ge=0)
    end_index: conint(ge=0)


class FilePath(BaseModel):
    file_id: str = Field(..., description="The ID of the file that was generated.")


class MessageContentTextAnnotationsFilePathObject(BaseModel):
    type: Literal["file_path"] = Field(..., description="Always `file_path`.")
    text: str = Field(
        ..., description="The text in the message content that needs to be replaced."
    )
    file_path: FilePath
    start_index: conint(ge=0)
    end_index: conint(ge=0)


class FileCitation1(BaseModel):
    file_id: Optional[str] = Field(
        None, description="The ID of the specific File the citation is from."
    )
    quote: Optional[str] = Field(None, description="The specific quote in the file.")


class MessageDeltaContentTextAnnotationsFileCitationObject(BaseModel):
    index: int = Field(
        ..., description="The index of the annotation in the text content part."
    )
    type: Literal["file_citation"] = Field(..., description="Always `file_citation`.")
    text: Optional[str] = Field(
        None, description="The text in the message content that needs to be replaced."
    )
    file_citation: Optional[FileCitation1] = None
    start_index: Optional[conint(ge=0)] = None
    end_index: Optional[conint(ge=0)] = None


class FilePath1(BaseModel):
    file_id: Optional[str] = Field(
        None, description="The ID of the file that was generated."
    )


class MessageDeltaContentTextAnnotationsFilePathObject(BaseModel):
    index: int = Field(
        ..., description="The index of the annotation in the text content part."
    )
    type: Literal["file_path"] = Field(..., description="Always `file_path`.")
    text: Optional[str] = Field(
        None, description="The text in the message content that needs to be replaced."
    )
    file_path: Optional[FilePath1] = None
    start_index: Optional[conint(ge=0)] = None
    end_index: Optional[conint(ge=0)] = None


class LastError1(BaseModel):
    code: Literal["server_error", "rate_limit_exceeded"] = Field(
        ..., description="One of `server_error` or `rate_limit_exceeded`."
    )
    message: str = Field(..., description="A human-readable description of the error.")


class MessageCreation(BaseModel):
    message_id: str = Field(
        ..., description="The ID of the message that was created by this run step."
    )


class RunStepDetailsMessageCreationObject(BaseModel):
    type: Literal["message_creation"] = Field(
        ..., description="Always `message_creation`."
    )
    message_creation: MessageCreation


class MessageCreation1(BaseModel):
    message_id: Optional[str] = Field(
        None, description="The ID of the message that was created by this run step."
    )


class RunStepDeltaStepDetailsMessageCreationObject(BaseModel):
    type: Literal["message_creation"] = Field(
        ..., description="Always `message_creation`."
    )
    message_creation: Optional[MessageCreation1] = None


class RunStepDetailsToolCallsCodeOutputLogsObject(BaseModel):
    type: Literal["logs"] = Field(..., description="Always `logs`.")
    logs: str = Field(
        ..., description="The text output from the Code Interpreter tool call."
    )


class RunStepDeltaStepDetailsToolCallsCodeOutputLogsObject(BaseModel):
    index: int = Field(..., description="The index of the output in the outputs array.")
    type: Literal["logs"] = Field(..., description="Always `logs`.")
    logs: Optional[str] = Field(
        None, description="The text output from the Code Interpreter tool call."
    )


class Image1(BaseModel):
    file_id: str = Field(
        ..., description="The [file](/docs/api-reference/files) ID of the image."
    )


class RunStepDetailsToolCallsCodeOutputImageObject(BaseModel):
    type: Literal["image"] = Field(..., description="Always `image`.")
    image: Image1


class Image2(BaseModel):
    file_id: Optional[str] = Field(
        None, description="The [file](/docs/api-reference/files) ID of the image."
    )


class RunStepDeltaStepDetailsToolCallsCodeOutputImageObject(BaseModel):
    index: int = Field(..., description="The index of the output in the outputs array.")
    type: Literal["image"] = Field(..., description="Always `image`.")
    image: Optional[Image2] = None


class RunStepDetailsToolCallsRetrievalObject(BaseModel):
    id: str = Field(..., description="The ID of the tool call object.")
    type: Literal["retrieval"] = Field(
        ...,
        description="The type of tool call. This is always going to be `retrieval` for this type of tool call.",
    )
    retrieval: Dict[str, Any] = Field(
        ..., description="For now, this is always going to be an empty object."
    )


class RunStepDeltaStepDetailsToolCallsRetrievalObject(BaseModel):
    index: int = Field(
        ..., description="The index of the tool call in the tool calls array."
    )
    id: Optional[str] = Field(None, description="The ID of the tool call object.")
    type: Literal["retrieval"] = Field(
        ...,
        description="The type of tool call. This is always going to be `retrieval` for this type of tool call.",
    )
    retrieval: Optional[Dict[str, Any]] = Field(
        None, description="For now, this is always going to be an empty object."
    )


class Function4(BaseModel):
    name: str = Field(..., description="The name of the function.")
    arguments: str = Field(..., description="The arguments passed to the function.")
    output: Optional[str] = Field(
        ...,
        description="The output of the function. This will be `null` if the outputs have not been [submitted](/docs/api-reference/runs/submitToolOutputs) yet.",
    )


class RunStepDetailsToolCallsFunctionObject(BaseModel):
    id: str = Field(..., description="The ID of the tool call object.")
    type: Literal["function"] = Field(
        ...,
        description="The type of tool call. This is always going to be `function` for this type of tool call.",
    )
    function: Function4 = Field(
        ..., description="The definition of the function that was called."
    )


class Function5(BaseModel):
    name: Optional[str] = Field(None, description="The name of the function.")
    arguments: Optional[str] = Field(
        None, description="The arguments passed to the function."
    )
    output: Optional[str] = Field(
        None,
        description="The output of the function. This will be `null` if the outputs have not been [submitted](/docs/api-reference/runs/submitToolOutputs) yet.",
    )


class RunStepDeltaStepDetailsToolCallsFunctionObject(BaseModel):
    index: int = Field(
        ..., description="The index of the tool call in the tool calls array."
    )
    id: Optional[str] = Field(None, description="The ID of the tool call object.")
    type: Literal["function"] = Field(
        ...,
        description="The type of tool call. This is always going to be `function` for this type of tool call.",
    )
    function: Optional[Function5] = Field(
        None, description="The definition of the function that was called."
    )


class AssistantFileObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["assistant.file"] = Field(
        ..., description="The object type, which is always `assistant.file`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the assistant file was created.",
    )
    assistant_id: str = Field(
        ..., description="The assistant ID that the file is attached to."
    )


class CreateAssistantFileRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    file_id: str = Field(
        ...,
        description='A [File](/docs/api-reference/files) ID (with `purpose="assistants"`) that the assistant should use. Useful for tools like `retrieval` and `code_interpreter` that can access files.',
    )


class DeleteAssistantFileResponse(BaseModel):
    id: str
    deleted: bool
    object: Literal["assistant.file.deleted"]


class ListAssistantFilesResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[AssistantFileObject]
    first_id: str = Field(..., examples=["file-abc123"])
    last_id: str = Field(..., examples=["file-abc456"])
    has_more: bool = Field(..., examples=[False])


class MessageFileObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["thread.message.file"] = Field(
        ..., description="The object type, which is always `thread.message.file`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the message file was created.",
    )
    message_id: str = Field(
        ...,
        description="The ID of the [message](/docs/api-reference/messages) that the [File](/docs/api-reference/files) is attached to.",
    )


class ListMessageFilesResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[MessageFileObject]
    first_id: str = Field(..., examples=["file-abc123"])
    last_id: str = Field(..., examples=["file-abc456"])
    has_more: bool = Field(..., examples=[False])


class ThreadStreamEvent1(BaseModel):
    event: Literal["thread.created"]
    data: ThreadObject


class ErrorEvent(BaseModel):
    event: Literal["error"]
    data: Error


class DoneEvent(BaseModel):
    event: Literal["done"]
    data: Literal["[DONE]"]


class ListModelsResponse(BaseModel):
    object: Literal["list"]
    data: List[Model]


class ChatCompletionRequestUserMessage(BaseModel):
    content: Union[
        str,
        List[
            Union[
                ChatCompletionRequestMessageContentPartText,
                ChatCompletionRequestMessageContentPartImage,
            ]
        ],
    ] = Field(..., description="The contents of the user message.\n")
    role: Literal["user"] = Field(
        ..., description="The role of the messages author, in this case `user`."
    )
    name: Optional[str] = Field(
        None,
        description="An optional name for the participant. Provides the model information to differentiate between participants of the same role.",
    )


class ChatCompletionTool(BaseModel):
    type: Literal["function"] = Field(
        ...,
        description="The type of the tool. Currently, only `function` is supported.",
    )
    function: FunctionObject


class Choice2(BaseModel):
    finish_reason: Literal["stop", "length", "function_call", "content_filter"] = Field(
        ...,
        description="The reason the model stopped generating tokens. This will be `stop` if the model hit a natural stop point or a provided stop sequence, `length` if the maximum number of tokens specified in the request was reached, `content_filter` if content was omitted due to a flag from our content filters, or `function_call` if the model called a function.\n",
    )
    index: int = Field(
        ..., description="The index of the choice in the list of choices."
    )
    message: ChatMessage


class CreateChatCompletionFunctionResponse(BaseModel):
    id: str = Field(..., description="A unique identifier for the chat completion.")
    choices: List[Choice2] = Field(
        ...,
        description="A list of chat completion choices. Can be more than one if `n` is greater than 1.",
    )
    created: int = Field(
        ...,
        description="The Unix timestamp (in seconds) of when the chat completion was created.",
    )
    model: str = Field(..., description="The model used for the chat completion.")
    system_fingerprint: Optional[str] = Field(
        None,
        description="This fingerprint represents the backend configuration that the model runs with.\n\nCan be used in conjunction with the `seed` request parameter to understand when backend changes have been made that might impact determinism.\n",
    )
    object: Literal["chat.completion"] = Field(
        ..., description="The object type, which is always `chat.completion`."
    )
    usage: Optional[CompletionUsage] = None


class ImagesResponse(BaseModel):
    created: int
    data: List[Image]


class ListFilesResponse(BaseModel):
    data: List[OpenAIFile]
    object: Literal["list"]


class ListFineTuningJobEventsResponse(BaseModel):
    data: List[FineTuningJobEvent]
    object: Literal["list"]


class ListFineTuningJobCheckpointsResponse(BaseModel):
    data: List[FineTuningJobCheckpoint]
    object: Literal["list"]
    first_id: Optional[str] = None
    last_id: Optional[str] = None
    has_more: bool


class CreateEmbeddingResponse(BaseModel):
    data: List[Embedding] = Field(
        ..., description="The list of embeddings generated by the model."
    )
    model: str = Field(
        ..., description="The name of the model used to generate the embedding."
    )
    object: Literal["list"] = Field(
        ..., description='The object type, which is always "list".'
    )
    usage: Usage = Field(..., description="The usage information for the request.")


class FineTuningJob(BaseModel):
    id: str = Field(
        ...,
        description="The object identifier, which can be referenced in the API endpoints.",
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the fine-tuning job was created.",
    )
    error: Optional[Error1] = Field(
        ...,
        description="For fine-tuning jobs that have `failed`, this will contain more information on the cause of the failure.",
    )
    fine_tuned_model: Optional[str] = Field(
        ...,
        description="The name of the fine-tuned model that is being created. The value will be null if the fine-tuning job is still running.",
    )
    finished_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the fine-tuning job was finished. The value will be null if the fine-tuning job is still running.",
    )
    hyperparameters: Hyperparameters1 = Field(
        ...,
        description="The hyperparameters used for the fine-tuning job. See the [fine-tuning guide](/docs/guides/fine-tuning) for more details.",
    )
    model: str = Field(..., description="The base model that is being fine-tuned.")
    object: Literal["fine_tuning.job"] = Field(
        ..., description='The object type, which is always "fine_tuning.job".'
    )
    organization_id: str = Field(
        ..., description="The organization that owns the fine-tuning job."
    )
    result_files: List[str] = Field(
        ...,
        description="The compiled results file ID(s) for the fine-tuning job. You can retrieve the results with the [Files API](/docs/api-reference/files/retrieve-contents).",
    )
    status: Literal[
        "validating_files", "queued", "running", "succeeded", "failed", "cancelled"
    ] = Field(
        ...,
        description="The current status of the fine-tuning job, which can be either `validating_files`, `queued`, `running`, `succeeded`, `failed`, or `cancelled`.",
    )
    trained_tokens: Optional[int] = Field(
        ...,
        description="The total number of billable tokens processed by this fine-tuning job. The value will be null if the fine-tuning job is still running.",
    )
    training_file: str = Field(
        ...,
        description="The file ID used for training. You can retrieve the training data with the [Files API](/docs/api-reference/files/retrieve-contents).",
    )
    validation_file: Optional[str] = Field(
        ...,
        description="The file ID used for validation. You can retrieve the validation results with the [Files API](/docs/api-reference/files/retrieve-contents).",
    )
    integrations: Optional[List[FineTuningIntegration]] = Field(
        None,
        description="A list of integrations to enable for this fine-tuning job.",
        max_length=5,
    )
    seed: int = Field(..., description="The seed used for the fine-tuning job.")


class AssistantObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["assistant"] = Field(
        ..., description="The object type, which is always `assistant`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the assistant was created.",
    )
    name: Optional[constr(max_length=256)] = Field(
        ...,
        description="The name of the assistant. The maximum length is 256 characters.\n",
    )
    description: Optional[constr(max_length=512)] = Field(
        ...,
        description="The description of the assistant. The maximum length is 512 characters.\n",
    )
    model: str = Field(
        ...,
        description="ID of the model to use. You can use the [List models](/docs/api-reference/models/list) API to see all of your available models, or see our [Model overview](/docs/models/overview) for descriptions of them.\n",
    )
    instructions: Optional[constr(max_length=256000)] = Field(
        ...,
        description="The system instructions that the assistant uses. The maximum length is 256,000 characters.\n",
    )
    tools: List[
        Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]
    ] = Field(
        ...,
        description="A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types `code_interpreter`, `retrieval`, or `function`.\n",
        max_length=128,
    )
    file_ids: List[str] = Field(
        ...,
        description="A list of [file](/docs/api-reference/files) IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order.\n",
        max_length=20,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        ...,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class CreateAssistantRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    model: Union[
        str,
        Literal[
            "gpt-4-turbo",
            "gpt-4-turbo-2024-04-09",
            "gpt-4-0125-preview",
            "gpt-4-turbo-preview",
            "gpt-4-1106-preview",
            "gpt-4-vision-preview",
            "gpt-4",
            "gpt-4-0314",
            "gpt-4-0613",
            "gpt-4-32k",
            "gpt-4-32k-0314",
            "gpt-4-32k-0613",
            "gpt-3.5-turbo",
            "gpt-3.5-turbo-16k",
            "gpt-3.5-turbo-0613",
            "gpt-3.5-turbo-1106",
            "gpt-3.5-turbo-0125",
            "gpt-3.5-turbo-16k-0613",
        ],
    ] = Field(
        ...,
        description="ID of the model to use. You can use the [List models](/docs/api-reference/models/list) API to see all of your available models, or see our [Model overview](/docs/models/overview) for descriptions of them.\n",
        examples=["gpt-4-turbo"],
    )
    name: Optional[constr(max_length=256)] = Field(
        None,
        description="The name of the assistant. The maximum length is 256 characters.\n",
    )
    description: Optional[constr(max_length=512)] = Field(
        None,
        description="The description of the assistant. The maximum length is 512 characters.\n",
    )
    instructions: Optional[constr(max_length=256000)] = Field(
        None,
        description="The system instructions that the assistant uses. The maximum length is 256,000 characters.\n",
    )
    tools: List[
        Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]
    ] = Field(
        [],
        description="A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types `code_interpreter`, `retrieval`, or `function`.\n",
        max_length=128,
    )
    file_ids: List[str] = Field(
        [],
        description="A list of [file](/docs/api-reference/files) IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order.\n",
        max_length=20,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class ModifyAssistantRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    model: Optional[str] = Field(
        None,
        description="ID of the model to use. You can use the [List models](/docs/api-reference/models/list) API to see all of your available models, or see our [Model overview](/docs/models/overview) for descriptions of them.\n",
    )
    name: Optional[constr(max_length=256)] = Field(
        None,
        description="The name of the assistant. The maximum length is 256 characters.\n",
    )
    description: Optional[constr(max_length=512)] = Field(
        None,
        description="The description of the assistant. The maximum length is 512 characters.\n",
    )
    instructions: Optional[constr(max_length=256000)] = Field(
        None,
        description="The system instructions that the assistant uses. The maximum length is 256,000 characters.\n",
    )
    tools: List[
        Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]
    ] = Field(
        [],
        description="A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types `code_interpreter`, `retrieval`, or `function`.\n",
        max_length=128,
    )
    file_ids: List[str] = Field(
        [],
        description="A list of [File](/docs/api-reference/files) IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order. If a file was previously attached to the list but does not show up in the list, it will be deleted from the assistant.\n",
        max_length=20,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class ListAssistantsResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[AssistantObject]
    first_id: str = Field(..., examples=["asst_abc123"])
    last_id: str = Field(..., examples=["asst_abc456"])
    has_more: bool = Field(..., examples=[False])


class SubmitToolOutputs(BaseModel):
    tool_calls: List[RunToolCallObject] = Field(
        ..., description="A list of the relevant tool calls."
    )


class RequiredAction(BaseModel):
    type: Literal["submit_tool_outputs"] = Field(
        ..., description="For now, this is always `submit_tool_outputs`."
    )
    submit_tool_outputs: SubmitToolOutputs = Field(
        ..., description="Details on the tool outputs needed for this run to continue."
    )


class RunObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["thread.run"] = Field(
        ..., description="The object type, which is always `thread.run`."
    )
    created_at: int = Field(
        ..., description="The Unix timestamp (in seconds) for when the run was created."
    )
    thread_id: str = Field(
        ...,
        description="The ID of the [thread](/docs/api-reference/threads) that was executed on as a part of this run.",
    )
    assistant_id: str = Field(
        ...,
        description="The ID of the [assistant](/docs/api-reference/assistants) used for execution of this run.",
    )
    status: Literal[
        "queued",
        "in_progress",
        "requires_action",
        "cancelling",
        "cancelled",
        "failed",
        "completed",
        "expired",
    ] = Field(
        ...,
        description="The status of the run, which can be either `queued`, `in_progress`, `requires_action`, `cancelling`, `cancelled`, `failed`, `completed`, or `expired`.",
    )
    required_action: Optional[RequiredAction] = Field(
        ...,
        description="Details on the action required to continue the run. Will be `null` if no action is required.",
    )
    last_error: Optional[LastError] = Field(
        ...,
        description="The last error associated with this run. Will be `null` if there are no errors.",
    )
    expires_at: Optional[int] = Field(
        ..., description="The Unix timestamp (in seconds) for when the run will expire."
    )
    started_at: Optional[int] = Field(
        ..., description="The Unix timestamp (in seconds) for when the run was started."
    )
    cancelled_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run was cancelled.",
    )
    failed_at: Optional[int] = Field(
        ..., description="The Unix timestamp (in seconds) for when the run failed."
    )
    completed_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run was completed.",
    )
    model: str = Field(
        ...,
        description="The model that the [assistant](/docs/api-reference/assistants) used for this run.",
    )
    instructions: str = Field(
        ...,
        description="The instructions that the [assistant](/docs/api-reference/assistants) used for this run.",
    )
    tools: List[
        Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]
    ] = Field(
        ...,
        description="The list of tools that the [assistant](/docs/api-reference/assistants) used for this run.",
        max_length=20,
    )
    file_ids: List[str] = Field(
        ...,
        description="The list of [File](/docs/api-reference/files) IDs the [assistant](/docs/api-reference/assistants) used for this run.",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        ...,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )
    usage: RunCompletionUsage
    temperature: Optional[float] = Field(
        None,
        description="The sampling temperature used for this run. If not set, defaults to 1.",
    )


class CreateRunRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    assistant_id: str = Field(
        ...,
        description="The ID of the [assistant](/docs/api-reference/assistants) to use to execute this run.",
    )
    model: Optional[
        Union[
            Optional[str],
            Literal[
                "gpt-4-turbo",
                "gpt-4-turbo-2024-04-09",
                "gpt-4-0125-preview",
                "gpt-4-turbo-preview",
                "gpt-4-1106-preview",
                "gpt-4-vision-preview",
                "gpt-4",
                "gpt-4-0314",
                "gpt-4-0613",
                "gpt-4-32k",
                "gpt-4-32k-0314",
                "gpt-4-32k-0613",
                "gpt-3.5-turbo",
                "gpt-3.5-turbo-16k",
                "gpt-3.5-turbo-0613",
                "gpt-3.5-turbo-1106",
                "gpt-3.5-turbo-0125",
                "gpt-3.5-turbo-16k-0613",
            ],
        ]
    ] = Field(
        None,
        description="The ID of the [Model](/docs/api-reference/models) to be used to execute this run. If a value is provided here, it will override the model associated with the assistant. If not, the model associated with the assistant will be used.",
        examples=["gpt-4-turbo"],
    )
    instructions: Optional[str] = Field(
        None,
        description="Overrides the [instructions](/docs/api-reference/assistants/createAssistant) of the assistant. This is useful for modifying the behavior on a per-run basis.",
    )
    additional_instructions: Optional[str] = Field(
        None,
        description="Appends additional instructions at the end of the instructions for the run. This is useful for modifying the behavior on a per-run basis without overriding other instructions.",
    )
    additional_messages: Optional[List[CreateMessageRequest]] = Field(
        None,
        description="Adds additional messages to the thread before creating the run.",
    )
    tools: Optional[
        List[Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]]
    ] = Field(
        None,
        description="Override the tools the assistant can use for this run. This is useful for modifying the behavior on a per-run basis.",
        max_length=20,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )
    temperature: Optional[confloat(ge=0.0, le=2.0)] = Field(
        1,
        description="What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.\n",
        examples=[1],
    )
    stream: Optional[bool] = Field(
        None,
        description="If `true`, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a `data: [DONE]` message.\n",
    )


class ListRunsResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[RunObject]
    first_id: str = Field(..., examples=["run_abc123"])
    last_id: str = Field(..., examples=["run_abc456"])
    has_more: bool = Field(..., examples=[False])


class CreateThreadRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    messages: Optional[List[CreateMessageRequest]] = Field(
        None,
        description="A list of [messages](/docs/api-reference/messages) to start the thread with.",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class Text(BaseModel):
    value: str = Field(..., description="The data that makes up the text.")
    annotations: List[
        Union[
            MessageContentTextAnnotationsFileCitationObject,
            MessageContentTextAnnotationsFilePathObject,
        ]
    ]


class MessageContentTextObject(BaseModel):
    type: Literal["text"] = Field(..., description="Always `text`.")
    text: Text


class Text1(BaseModel):
    value: Optional[str] = Field(None, description="The data that makes up the text.")
    annotations: Optional[
        List[
            Union[
                MessageDeltaContentTextAnnotationsFileCitationObject,
                MessageDeltaContentTextAnnotationsFilePathObject,
            ]
        ]
    ] = None


class MessageDeltaContentTextObject(BaseModel):
    index: int = Field(..., description="The index of the content part in the message.")
    type: Literal["text"] = Field(..., description="Always `text`.")
    text: Optional[Text1] = None


class CodeInterpreter(BaseModel):
    input: str = Field(..., description="The input to the Code Interpreter tool call.")
    outputs: List[
        Union[
            RunStepDetailsToolCallsCodeOutputLogsObject,
            RunStepDetailsToolCallsCodeOutputImageObject,
        ]
    ] = Field(
        ...,
        description="The outputs from the Code Interpreter tool call. Code Interpreter can output one or more items, including text (`logs`) or images (`image`). Each of these are represented by a different object type.",
    )


class RunStepDetailsToolCallsCodeObject(BaseModel):
    id: str = Field(..., description="The ID of the tool call.")
    type: Literal["code_interpreter"] = Field(
        ...,
        description="The type of tool call. This is always going to be `code_interpreter` for this type of tool call.",
    )
    code_interpreter: CodeInterpreter = Field(
        ..., description="The Code Interpreter tool call definition."
    )


class CodeInterpreter1(BaseModel):
    input: Optional[str] = Field(
        None, description="The input to the Code Interpreter tool call."
    )
    outputs: Optional[
        List[
            Union[
                RunStepDeltaStepDetailsToolCallsCodeOutputLogsObject,
                RunStepDeltaStepDetailsToolCallsCodeOutputImageObject,
            ]
        ]
    ] = Field(
        None,
        description="The outputs from the Code Interpreter tool call. Code Interpreter can output one or more items, including text (`logs`) or images (`image`). Each of these are represented by a different object type.",
    )


class RunStepDeltaStepDetailsToolCallsCodeObject(BaseModel):
    index: int = Field(
        ..., description="The index of the tool call in the tool calls array."
    )
    id: Optional[str] = Field(None, description="The ID of the tool call.")
    type: Literal["code_interpreter"] = Field(
        ...,
        description="The type of tool call. This is always going to be `code_interpreter` for this type of tool call.",
    )
    code_interpreter: Optional[CodeInterpreter1] = Field(
        None, description="The Code Interpreter tool call definition."
    )


class RunStreamEvent1(BaseModel):
    event: Literal["thread.run.created"]
    data: RunObject


class RunStreamEvent2(BaseModel):
    event: Literal["thread.run.queued"]
    data: RunObject


class RunStreamEvent3(BaseModel):
    event: Literal["thread.run.in_progress"]
    data: RunObject


class RunStreamEvent4(BaseModel):
    event: Literal["thread.run.requires_action"]
    data: RunObject


class RunStreamEvent5(BaseModel):
    event: Literal["thread.run.completed"]
    data: RunObject


class RunStreamEvent6(BaseModel):
    event: Literal["thread.run.failed"]
    data: RunObject


class RunStreamEvent7(BaseModel):
    event: Literal["thread.run.cancelling"]
    data: RunObject


class RunStreamEvent8(BaseModel):
    event: Literal["thread.run.cancelled"]
    data: RunObject


class RunStreamEvent9(BaseModel):
    event: Literal["thread.run.expired"]
    data: RunObject


class ChatCompletionRequestAssistantMessage(BaseModel):
    content: Optional[str] = Field(
        None,
        description="The contents of the assistant message. Required unless `tool_calls` or `function_call` is specified.\n",
    )
    role: Literal["assistant"] = Field(
        ..., description="The role of the messages author, in this case `assistant`."
    )
    name: Optional[str] = Field(
        None,
        description="An optional name for the participant. Provides the model information to differentiate between participants of the same role.",
    )
    tool_calls: Optional[List[ChatCompletionMessageToolCall]] = Field(
        None,
        description="The tool calls generated by the model, such as function calls.",
    )
    function_call: Optional[FunctionCall] = Field(
        None,
        description="Deprecated and replaced by `tool_calls`. The name and arguments of a function that should be called, as generated by the model.",
    )


class ListPaginatedFineTuningJobsResponse(BaseModel):
    data: List[FineTuningJob]
    has_more: bool
    object: Literal["list"]


class CreateThreadAndRunRequest(BaseModel):
    model_config = ConfigDict(
        extra="forbid",
    )
    assistant_id: str = Field(
        ...,
        description="The ID of the [assistant](/docs/api-reference/assistants) to use to execute this run.",
    )
    thread: Optional[CreateThreadRequest] = Field(
        None, description="If no thread is provided, an empty thread will be created."
    )
    model: Optional[
        Union[
            Optional[str],
            Literal[
                "gpt-4-turbo",
                "gpt-4-turbo-2024-04-09",
                "gpt-4-0125-preview",
                "gpt-4-turbo-preview",
                "gpt-4-1106-preview",
                "gpt-4-vision-preview",
                "gpt-4",
                "gpt-4-0314",
                "gpt-4-0613",
                "gpt-4-32k",
                "gpt-4-32k-0314",
                "gpt-4-32k-0613",
                "gpt-3.5-turbo",
                "gpt-3.5-turbo-16k",
                "gpt-3.5-turbo-0613",
                "gpt-3.5-turbo-1106",
                "gpt-3.5-turbo-0125",
                "gpt-3.5-turbo-16k-0613",
            ],
        ]
    ] = Field(
        None,
        description="The ID of the [Model](/docs/api-reference/models) to be used to execute this run. If a value is provided here, it will override the model associated with the assistant. If not, the model associated with the assistant will be used.",
        examples=["gpt-4-turbo"],
    )
    instructions: Optional[str] = Field(
        None,
        description="Override the default system message of the assistant. This is useful for modifying the behavior on a per-run basis.",
    )
    tools: Optional[
        List[Union[AssistantToolsCode, AssistantToolsRetrieval, AssistantToolsFunction]]
    ] = Field(
        None,
        description="Override the tools the assistant can use for this run. This is useful for modifying the behavior on a per-run basis.",
        max_length=20,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        None,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )
    temperature: Optional[confloat(ge=0.0, le=2.0)] = Field(
        1,
        description="What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.\n",
        examples=[1],
    )
    stream: Optional[bool] = Field(
        None,
        description="If `true`, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a `data: [DONE]` message.\n",
    )


class MessageObject(BaseModel):
    id: str = Field(
        ..., description="The identifier, which can be referenced in API endpoints."
    )
    object: Literal["thread.message"] = Field(
        ..., description="The object type, which is always `thread.message`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the message was created.",
    )
    thread_id: str = Field(
        ...,
        description="The [thread](/docs/api-reference/threads) ID that this message belongs to.",
    )
    status: Literal["in_progress", "incomplete", "completed"] = Field(
        ...,
        description="The status of the message, which can be either `in_progress`, `incomplete`, or `completed`.",
    )
    incomplete_details: Optional[IncompleteDetails] = Field(
        ...,
        description="On an incomplete message, details about why the message is incomplete.",
    )
    completed_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the message was completed.",
    )
    incomplete_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the message was marked as incomplete.",
    )
    role: Literal["user", "assistant"] = Field(
        ...,
        description="The entity that produced the message. One of `user` or `assistant`.",
    )
    content: List[Union[MessageContentImageFileObject, MessageContentTextObject]] = (
        Field(
            ...,
            description="The content of the message in array of text and/or images.",
        )
    )
    assistant_id: Optional[str] = Field(
        ...,
        description="If applicable, the ID of the [assistant](/docs/api-reference/assistants) that authored this message.",
    )
    run_id: Optional[str] = Field(
        ...,
        description="The ID of the [run](/docs/api-reference/runs) associated with the creation of this message. Value is `null` when messages are created manually using the create message or create thread endpoints.",
    )
    file_ids: List[str] = Field(
        ...,
        description="A list of [file](/docs/api-reference/files) IDs that the assistant should use. Useful for tools like retrieval and code_interpreter that can access files. A maximum of 10 files can be attached to a message.",
        max_length=10,
    )
    metadata: Optional[Dict[str, Any]] = Field(
        ...,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )


class Delta(BaseModel):
    role: Optional[Literal["user", "assistant"]] = Field(
        None,
        description="The entity that produced the message. One of `user` or `assistant`.",
    )
    content: Optional[
        List[Union[MessageDeltaContentImageFileObject, MessageDeltaContentTextObject]]
    ] = Field(
        None, description="The content of the message in array of text and/or images."
    )
    file_ids: List[str] = Field(
        [],
        description="A list of [file](/docs/api-reference/files) IDs that the assistant should use. Useful for tools like retrieval and code_interpreter that can access files. A maximum of 10 files can be attached to a message.",
        max_length=10,
    )


class MessageDeltaObject(BaseModel):
    id: str = Field(
        ...,
        description="The identifier of the message, which can be referenced in API endpoints.",
    )
    object: Literal["thread.message.delta"] = Field(
        ..., description="The object type, which is always `thread.message.delta`."
    )
    delta: Delta = Field(
        ...,
        description="The delta containing the fields that have changed on the Message.",
    )


class ListMessagesResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[MessageObject]
    first_id: str = Field(..., examples=["msg_abc123"])
    last_id: str = Field(..., examples=["msg_abc123"])
    has_more: bool = Field(..., examples=[False])


class RunStepDetailsToolCallsObject(BaseModel):
    type: Literal["tool_calls"] = Field(..., description="Always `tool_calls`.")
    tool_calls: List[
        Union[
            RunStepDetailsToolCallsCodeObject,
            RunStepDetailsToolCallsRetrievalObject,
            RunStepDetailsToolCallsFunctionObject,
        ]
    ] = Field(
        ...,
        description="An array of tool calls the run step was involved in. These can be associated with one of three types of tools: `code_interpreter`, `retrieval`, or `function`.\n",
    )


class RunStepDeltaStepDetailsToolCallsObject(BaseModel):
    type: Literal["tool_calls"] = Field(..., description="Always `tool_calls`.")
    tool_calls: Optional[
        List[
            Union[
                RunStepDeltaStepDetailsToolCallsCodeObject,
                RunStepDeltaStepDetailsToolCallsRetrievalObject,
                RunStepDeltaStepDetailsToolCallsFunctionObject,
            ]
        ]
    ] = Field(
        None,
        description="An array of tool calls the run step was involved in. These can be associated with one of three types of tools: `code_interpreter`, `retrieval`, or `function`.\n",
    )


class MessageStreamEvent1(BaseModel):
    event: Literal["thread.message.created"]
    data: MessageObject


class MessageStreamEvent2(BaseModel):
    event: Literal["thread.message.in_progress"]
    data: MessageObject


class MessageStreamEvent3(BaseModel):
    event: Literal["thread.message.delta"]
    data: MessageDeltaObject


class MessageStreamEvent4(BaseModel):
    event: Literal["thread.message.completed"]
    data: MessageObject


class MessageStreamEvent5(BaseModel):
    event: Literal["thread.message.incomplete"]
    data: MessageObject


class RunStepObject(BaseModel):
    id: str = Field(
        ...,
        description="The identifier of the run step, which can be referenced in API endpoints.",
    )
    object: Literal["thread.run.step"] = Field(
        ..., description="The object type, which is always `thread.run.step`."
    )
    created_at: int = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run step was created.",
    )
    assistant_id: str = Field(
        ...,
        description="The ID of the [assistant](/docs/api-reference/assistants) associated with the run step.",
    )
    thread_id: str = Field(
        ...,
        description="The ID of the [thread](/docs/api-reference/threads) that was run.",
    )
    run_id: str = Field(
        ...,
        description="The ID of the [run](/docs/api-reference/runs) that this run step is a part of.",
    )
    type: Literal["message_creation", "tool_calls"] = Field(
        ...,
        description="The type of run step, which can be either `message_creation` or `tool_calls`.",
    )
    status: Literal["in_progress", "cancelled", "failed", "completed", "expired"] = (
        Field(
            ...,
            description="The status of the run step, which can be either `in_progress`, `cancelled`, `failed`, `completed`, or `expired`.",
        )
    )
    step_details: Union[
        RunStepDetailsMessageCreationObject, RunStepDetailsToolCallsObject
    ] = Field(..., description="The details of the run step.")
    last_error: Optional[LastError1] = Field(
        ...,
        description="The last error associated with this run step. Will be `null` if there are no errors.",
    )
    expired_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run step expired. A step is considered expired if the parent run is expired.",
    )
    cancelled_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run step was cancelled.",
    )
    failed_at: Optional[int] = Field(
        ..., description="The Unix timestamp (in seconds) for when the run step failed."
    )
    completed_at: Optional[int] = Field(
        ...,
        description="The Unix timestamp (in seconds) for when the run step completed.",
    )
    metadata: Optional[Dict[str, Any]] = Field(
        ...,
        description="Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.\n",
    )
    usage: RunStepCompletionUsage


class Delta1(BaseModel):
    step_details: Optional[
        Union[
            RunStepDeltaStepDetailsMessageCreationObject,
            RunStepDeltaStepDetailsToolCallsObject,
        ]
    ] = Field(None, description="The details of the run step.")


class RunStepDeltaObject(BaseModel):
    id: str = Field(
        ...,
        description="The identifier of the run step, which can be referenced in API endpoints.",
    )
    object: Literal["thread.run.step.delta"] = Field(
        ..., description="The object type, which is always `thread.run.step.delta`."
    )
    delta: Delta1 = Field(
        ...,
        description="The delta containing the fields that have changed on the run step.",
    )


class ListRunStepsResponse(BaseModel):
    object: str = Field(..., examples=["list"])
    data: List[RunStepObject]
    first_id: str = Field(..., examples=["step_abc123"])
    last_id: str = Field(..., examples=["step_abc456"])
    has_more: bool = Field(..., examples=[False])


class RunStepStreamEvent1(BaseModel):
    event: Literal["thread.run.step.created"]
    data: RunStepObject


class RunStepStreamEvent2(BaseModel):
    event: Literal["thread.run.step.in_progress"]
    data: RunStepObject


class RunStepStreamEvent3(BaseModel):
    event: Literal["thread.run.step.delta"]
    data: RunStepDeltaObject


class RunStepStreamEvent4(BaseModel):
    event: Literal["thread.run.step.completed"]
    data: RunStepObject


class RunStepStreamEvent5(BaseModel):
    event: Literal["thread.run.step.failed"]
    data: RunStepObject


class RunStepStreamEvent6(BaseModel):
    event: Literal["thread.run.step.cancelled"]
    data: RunStepObject


class RunStepStreamEvent7(BaseModel):
    event: Literal["thread.run.step.expired"]
    data: RunStepObject


class AssistantStreamEvent(
    RootModel[
        Union[
            ErrorEvent,
            DoneEvent,
            ThreadStreamEvent1,
            Union[
                RunStreamEvent1,
                RunStreamEvent2,
                RunStreamEvent3,
                RunStreamEvent4,
                RunStreamEvent5,
                RunStreamEvent6,
                RunStreamEvent7,
                RunStreamEvent8,
                RunStreamEvent9,
            ],
            Union[
                RunStepStreamEvent1,
                RunStepStreamEvent2,
                RunStepStreamEvent3,
                RunStepStreamEvent4,
                RunStepStreamEvent5,
                RunStepStreamEvent6,
                RunStepStreamEvent7,
            ],
            Union[
                MessageStreamEvent1,
                MessageStreamEvent2,
                MessageStreamEvent3,
                MessageStreamEvent4,
                MessageStreamEvent5,
            ],
        ]
    ]
):
    root: Union[
        ErrorEvent,
        DoneEvent,
        ThreadStreamEvent1,
        Union[
            RunStreamEvent1,
            RunStreamEvent2,
            RunStreamEvent3,
            RunStreamEvent4,
            RunStreamEvent5,
            RunStreamEvent6,
            RunStreamEvent7,
            RunStreamEvent8,
            RunStreamEvent9,
        ],
        Union[
            RunStepStreamEvent1,
            RunStepStreamEvent2,
            RunStepStreamEvent3,
            RunStepStreamEvent4,
            RunStepStreamEvent5,
            RunStepStreamEvent6,
            RunStepStreamEvent7,
        ],
        Union[
            MessageStreamEvent1,
            MessageStreamEvent2,
            MessageStreamEvent3,
            MessageStreamEvent4,
            MessageStreamEvent5,
        ],
    ] = Field(
        ...,
        description='Represents an event emitted when streaming a Run.\n\nEach event in a server-sent events stream has an `event` and `data` property:\n\n```\nevent: thread.created\ndata: {"id": "thread_123", "object": "thread", ...}\n```\n\nWe emit events whenever a new object is created, transitions to a new state, or is being\nstreamed in parts (deltas). For example, we emit `thread.run.created` when a new run\nis created, `thread.run.completed` when a run completes, and so on. When an Assistant chooses\nto create a message during a run, we emit a `thread.message.created event`, a\n`thread.message.in_progress` event, many `thread.message.delta` events, and finally a\n`thread.message.completed` event.\n\nWe may add additional events over time, so we recommend handling unknown events gracefully\nin your code. See the [Assistants API quickstart](/docs/assistants/overview) to learn how to\nintegrate the Assistants API with streaming.\n',
    )
