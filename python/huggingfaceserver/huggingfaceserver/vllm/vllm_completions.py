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

import asyncio
import time
from typing import (
    AsyncGenerator,
    AsyncIterator,
    Dict,
    Iterable,
    List,
    Optional,
    Tuple,
    Union,
    Iterator,
)
from http import HTTPStatus

import torch
from vllm import PoolingParams
from vllm.entrypoints.logger import RequestLogger
from vllm.inputs.parse import parse_and_batch_prompt
from vllm.lora.request import LoRARequest
from vllm.prompt_adapter.request import PromptAdapterRequest
from vllm.sampling_params import SamplingParams
from vllm.utils import random_uuid

from vllm.outputs import RequestOutput
from vllm.entrypoints.openai.serving_completion import (
    merge_async_iterators,
)
from vllm.engine.async_llm_engine import AsyncLLMEngine
from vllm.transformers_utils.tokenizer import get_tokenizer
from vllm.sequence import Logprob

from kserve.protocol.rest.openai.types.openapi import (
    Choice as CompletionChoice,
    CompletionUsage,
    CreateCompletionRequest,
    CreateCompletionResponse as Completion,
    Logprobs,
    ChatCompletionTool
)
from kserve.protocol.rest.openai.errors import OpenAIError, create_error_response
from kserve.protocol.rest.openai import ChatCompletionRequestMessage, CompletionRequest, ChatCompletionRequest


def to_sampling_params(request: CreateCompletionRequest):
    echo_without_generation = request.echo and request.max_tokens == 0

    logits_processors = None
    if request.logit_bias:

        def logit_bias_logits_processor(
            token_ids: List[int], logits: torch.Tensor
        ) -> torch.Tensor:
            for token_id, bias in request.logit_bias.items():
                # Clamp the bias between -100 and 100 per OpenAI API spec
                bias = min(100, max(-100, bias))
                logits[int(token_id)] += bias
            return logits

        logits_processors = [logit_bias_logits_processor]

    return SamplingParams(
        n=request.n,
        best_of=request.best_of,
        presence_penalty=request.presence_penalty,
        frequency_penalty=request.frequency_penalty,
        temperature=request.temperature,
        top_p=request.top_p,
        seed=request.seed,
        stop=request.stop,
        logprobs=request.logprobs,
        max_tokens=request.max_tokens if not echo_without_generation else 1,
        logits_processors=logits_processors,
        prompt_logprobs=request.logprobs if request.echo else None,
    )


class OpenAIServingCompletion:

    def __init__(self, engine: AsyncLLMEngine, request_logger: RequestLogger = None):
        self.engine = engine

        self.max_model_len = 0
        self.tokenizer = None
        self.request_logger = request_logger

        try:
            event_loop = asyncio.get_running_loop()
        except RuntimeError:
            event_loop = None

        if (
            event_loop is not None and event_loop.is_running()
        ):  # If the current is instanced by Ray Serve, there is already a running event loop
            event_loop.create_task(self._post_init())
        else:  # When using single vLLM without engine_use_ray
            loop = asyncio.new_event_loop()
            loop.run_until_complete(self._post_init())
            loop.close()

    async def create_completion(self, completion_request: CompletionRequest):
        """Completion API similar to OpenAI's API.

        See https://platform.openai.com/docs/api-reference/completions/create
        for the API specification. This API mimics the OpenAI Completion API.

        NOTE: Currently we do not support the following feature:
            - suffix (the language models we currently support do not support
            suffix)
        """
        # Return error for unsupported features.
        request = completion_request.params

        if request.suffix is not None:
            raise OpenAIError("suffix is not currently supported")

        model_name = request.model
        request_id = (
            completion_request.request_id
            if completion_request.request_id
            else f"cmpl-{random_uuid()}"
        )
        created_time = int(time.time())

        # Schedule the request and get the result generator.
        generators = []
        try:
            sampling_params = to_sampling_params(request)
            prompts = list(
                self._tokenize_prompt_input_or_inputs(
                    request,
                    request.prompt,
                    # TODO: Introduce vLLM specific sampling params
                    # truncate_prompt_tokens=sampling_params.truncate_prompt_tokens,
                    # add_special_tokens=request.add_special_tokens,
                )
            )

            for i, prompt_inputs in enumerate(prompts):
                self._log_inputs(request_id, prompt_inputs, sampling_params)
                generators.append(
                    self.engine.generate(
                        {"prompt_token_ids": prompt_inputs[0]},
                        sampling_params,
                        f"{request_id}-{i}",
                    )
                )
        except Exception as e:
            raise e if isinstance(e, OpenAIError) else OpenAIError(str(e))

        result_generator: AsyncIterator[Tuple[int, RequestOutput]] = (
            merge_async_iterators(*generators)
        )

        # Similar to the OpenAI API, when n != best_of, we do not stream
        # the results.
        stream = request.stream and (
            request.best_of is None or request.n == request.best_of
        )

        # Streaming response
        if stream:
            return self.completion_stream_generator(
                request,
                prompts,
                result_generator,
                request_id,
                created_time,
                model_name,
                num_prompts=len(prompts),
            )

        # Non-streaming response
        final_res_batch: List[RequestOutput] = [None] * len(prompts)
        try:
            async for i, res in result_generator:
                if res.prompt is None:
                    res.prompt = prompts[i][1]
                final_res_batch[i] = res
            response = self.request_output_to_completion_response(
                final_res_batch, request, request_id, created_time, model_name
            )
        except Exception as e:
            raise OpenAIError(str(e))

        return response

    async def completion_stream_generator(
        self,
        request: CreateCompletionRequest,
        prompts: List[Tuple[List[int], str]],
        result_generator: AsyncIterator[Tuple[int, RequestOutput]],
        request_id: str,
        created_time: int,
        model_name: str,
        num_prompts: int,
    ) -> AsyncGenerator[Completion, None]:
        previous_texts = [""] * request.n * num_prompts
        previous_num_tokens = [0] * request.n * num_prompts
        has_echoed = [False] * request.n * num_prompts

        try:
            async for prompt_idx, res in result_generator:
                if res.prompt is None:
                    res.prompt = prompts[prompt_idx][1]

                for output in res.outputs:
                    i = output.index + prompt_idx * request.n

                    if request.echo and request.max_tokens == 0:
                        # only return the prompt
                        delta_text = res.prompt
                        delta_token_ids = res.prompt_token_ids
                        top_logprobs = res.prompt_logprobs
                        has_echoed[i] = True
                    elif request.echo and request.max_tokens > 0 and not has_echoed[i]:
                        # echo the prompt and first token
                        delta_text = res.prompt + output.text
                        delta_token_ids = res.prompt_token_ids + list(output.token_ids)
                        top_logprobs = (res.prompt_logprobs or []) + (
                            output.logprobs or []
                        )
                        has_echoed[i] = True
                    else:
                        # return just the delta
                        delta_text = output.text[len(previous_texts[i]) :]
                        delta_token_ids = output.token_ids[previous_num_tokens[i] :]
                        top_logprobs = (
                            output.logprobs[previous_num_tokens[i] :]
                            if output.logprobs
                            else None
                        )

                    if request.logprobs is not None:
                        logprobs = self._create_logprobs(
                            token_ids=delta_token_ids,
                            top_logprobs=top_logprobs,
                            num_output_top_logprobs=request.logprobs,
                            initial_text_offset=len(previous_texts[i]),
                        )
                    else:
                        logprobs = None

                    previous_texts[i] = output.text
                    previous_num_tokens[i] = len(output.token_ids)
                    finish_reason = output.finish_reason
                    if output.finish_reason is not None:  # return final usage
                        prompt_tokens = len(res.prompt_token_ids)
                        completion_tokens = len(output.token_ids)
                        final_usage = CompletionUsage(
                            prompt_tokens=prompt_tokens,
                            completion_tokens=completion_tokens,
                            total_tokens=prompt_tokens + completion_tokens,
                        )
                    else:
                        final_usage = None
                    response_json = Completion(
                        id=request_id,
                        created=created_time,
                        model=model_name,
                        object="text_completion",
                        choices=[
                            CompletionChoice(
                                index=i,
                                finish_reason=finish_reason,
                                text=delta_text,
                                logprobs=logprobs,
                            )
                        ],
                        usage=final_usage,
                    )
                    yield response_json
        except ValueError as e:
            raise OpenAIError(str(e))

    def request_output_to_completion_response(
        self,
        final_res_batch: List[RequestOutput],
        request: CreateCompletionRequest,
        request_id: str,
        created_time: int,
        model_name: str,
    ) -> Completion:
        choices = []
        num_prompt_tokens = 0
        num_generated_tokens = 0
        for final_res in final_res_batch:
            assert final_res is not None
            prompt_token_ids = final_res.prompt_token_ids
            prompt_logprobs = final_res.prompt_logprobs
            prompt_text = final_res.prompt

            for output in final_res.outputs:
                if request.echo and request.max_tokens == 0:
                    token_ids = prompt_token_ids
                    top_logprobs = prompt_logprobs
                    output_text = prompt_text
                elif request.echo and request.max_tokens > 0:
                    token_ids = prompt_token_ids + list(output.token_ids)
                    top_logprobs = output.logprobs or prompt_logprobs
                    if output.logprobs and prompt_logprobs:
                        top_logprobs = prompt_logprobs + output.logprobs
                    output_text = prompt_text + output.text
                else:
                    token_ids = output.token_ids
                    top_logprobs = output.logprobs
                    output_text = output.text

                if request.logprobs is not None:
                    logprobs = self._create_logprobs(
                        token_ids=token_ids,
                        top_logprobs=top_logprobs,
                        num_output_top_logprobs=request.logprobs,
                    )
                else:
                    logprobs = None

                choice_data = CompletionChoice(
                    index=len(choices),
                    text=output_text,
                    logprobs=logprobs,
                    finish_reason=output.finish_reason,
                )
                choices.append(choice_data)

            num_prompt_tokens += len(prompt_token_ids)
            num_generated_tokens += sum(
                len(output.token_ids) for output in final_res.outputs
            )

        usage = CompletionUsage(
            prompt_tokens=num_prompt_tokens,
            completion_tokens=num_generated_tokens,
            total_tokens=num_prompt_tokens + num_generated_tokens,
        )

        return Completion(
            id=request_id,
            created=created_time,
            model=model_name,
            object="text_completion",
            choices=choices,
            usage=usage,
        )

    def apply_chat_template(
        self,
        messages: Iterable[ChatCompletionRequestMessage,],
        chat_template: Optional[str] = None,
        tools: Optional[list[ChatCompletionTool]] = None
    ):
        return self.tokenizer.apply_chat_template(
            conversation=messages,
            chat_template=chat_template,
            tokenize=False,
            add_generation_prompt=True,
            tools=tools
        )

    async def _post_init(self):
        engine_model_config = await self.engine.get_model_config()
        self.max_model_len = engine_model_config.max_model_len

        # A separate tokenizer to map token IDs to strings.
        self.tokenizer = get_tokenizer(
            engine_model_config.tokenizer,
            tokenizer_mode=engine_model_config.tokenizer_mode,
            trust_remote_code=engine_model_config.trust_remote_code,
            revision=engine_model_config.tokenizer_revision,
        )

    def _validate_input(
        self,
        request: CreateCompletionRequest,
        input_ids: List[int],
        input_text: str,
    ) -> Tuple[List[int], str]:
        token_num = len(input_ids)

        if request.max_tokens is None:
            request.max_tokens = self.max_model_len - token_num

        if token_num + request.max_tokens > self.max_model_len:
            raise OpenAIError(
                response=create_error_response(
                    f"This model's maximum context length is "
                    f"{self.max_model_len} tokens. However, you requested "
                    f"{request.max_tokens + token_num} tokens "
                    f"({token_num} in the messages, "
                    f"{request.max_tokens} in the completion). "
                    f"Please reduce the length of the messages or completion.",
                    err_type="BadRequest",
                    status_code=HTTPStatus.BAD_REQUEST,
                )
            )
        else:
            return input_ids, input_text

    def _tokenize_prompt_input_or_inputs(
        self,
        request: CreateCompletionRequest,
        input_or_inputs: Union[str, List[str], List[int], List[List[int]]],
        #  TODO: Introduce vLLM specific sampling params
        # truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]] = None,
        add_special_tokens: bool = True,
    ) -> Iterator[tuple[list[int], str]]:
        """
        Tokenize/detokenize depending on the input format.

        According to `OpenAI API <https://platform.openai.com/docs/api-reference/embeddings/create>`_
        , each input can be a string or array of tokens. Note that each request
        can pass one or more inputs.
        """
        for prompt_input in parse_and_batch_prompt(input_or_inputs):
            # Although our type checking is based on mypy,
            # VSCode Pyright extension should still work properly
            # "is True" is required for Pyright to perform type narrowing
            # See: https://github.com/microsoft/pyright/issues/7672
            if prompt_input["is_tokens"] is False:
                yield self._normalize_prompt_text_to_input(
                    request,
                    prompt=prompt_input["content"],
                    # truncate_prompt_tokens=truncate_prompt_tokens,
                    add_special_tokens=add_special_tokens,
                )
            else:
                yield self._normalize_prompt_tokens_to_input(
                    request,
                    prompt_ids=prompt_input["content"],
                    # truncate_prompt_tokens=truncate_prompt_tokens,
                )

    def _normalize_prompt_text_to_input(
        self,
        request: CreateCompletionRequest,
        prompt: str,
        #  TODO: Introduce vLLM specific sampling params
        # truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]],
        add_special_tokens: bool,
    ) -> tuple[list[int], str]:
        encoded = self.tokenizer(prompt, add_special_tokens=add_special_tokens)
        # if truncate_prompt_tokens is not None:
        #     encoded = self.tokenizer(prompt,
        #                         add_special_tokens=add_special_tokens,
        #                         truncation=True,
        #                         max_length=truncate_prompt_tokens)

        input_ids = encoded.input_ids
        input_text = prompt
        return self._validate_input(request, input_ids, input_text)

    def _normalize_prompt_tokens_to_input(
        self,
        request: CreateCompletionRequest,
        prompt_ids: List[int],
        #  TODO: Introduce vLLM specific sampling params
        # truncate_prompt_tokens: Optional[Annotated[int, Field(ge=1)]],
    ) -> tuple[list[int], str]:
        input_ids = prompt_ids
        # if truncate_prompt_tokens is not None:
        #     input_ids = prompt_ids[-truncate_prompt_tokens:]

        input_text = self.tokenizer.decode(input_ids)
        return self._validate_input(request, input_ids, input_text)

    def _get_decoded_token(self, logprob: Logprob, token_id: int) -> str:
        if logprob.decoded_token is not None:
            return logprob.decoded_token
        return self.tokenizer.decode(token_id)

    def _create_logprobs(
        self,
        token_ids: List[int],
        top_logprobs: List[Optional[Dict[int, Logprob]]],
        num_output_top_logprobs: int,
        initial_text_offset: int = 0,
    ) -> Logprobs:
        """Create OpenAI-style logprobs."""
        logprobs = Logprobs(
            text_offset=[],
            token_logprobs=[],
            tokens=[],
            top_logprobs=[],
        )

        last_token_len = 0

        for i, token_id in enumerate(token_ids):
            step_top_logprobs = top_logprobs[i]
            if step_top_logprobs is None:
                token = self.tokenizer.decode(token_id)
                logprobs.tokens.append(token)
                logprobs.token_logprobs.append(None)
                logprobs.top_logprobs.append(None)
            else:
                token = self._get_decoded_token(step_top_logprobs[token_id], token_id)
                token_logprob = max(step_top_logprobs[token_id].logprob, -9999.0)
                logprobs.tokens.append(token)
                logprobs.token_logprobs.append(token_logprob)

                # makes sure to add the top num_output_top_logprobs + 1
                # logprobs, as defined in the openai API
                # (cf. https://github.com/openai/openai-openapi/blob/893ba52242dbd5387a97b96444ee1c742cfce9bd/openapi.yaml#L7153)
                logprobs.top_logprobs.append(
                    {
                        # Convert float("-inf") to the
                        # JSON-serializable float that OpenAI uses
                        self._get_decoded_token(top_lp[1], top_lp[0]): max(
                            top_lp[1].logprob, -9999.0
                        )
                        for i, top_lp in enumerate(step_top_logprobs.items())
                        if num_output_top_logprobs >= i
                    }
                )

            if len(logprobs.text_offset) == 0:
                logprobs.text_offset.append(initial_text_offset)
            else:
                logprobs.text_offset.append(logprobs.text_offset[-1] + last_token_len)
            last_token_len = len(token)

        return logprobs

    def _log_inputs(
        self,
        request_id: str,
        input: Tuple[List[int], str],
        params: Optional[Union[SamplingParams, PoolingParams]] = None,
        lora_request: Optional[LoRARequest] = None,
        prompt_adapter_request: Optional[PromptAdapterRequest] = None,
    ):
        if self.request_logger is None:
            return
        prompt_token_ids, prompt = input
        max_log_len = self.request_logger.max_log_len
        if max_log_len is not None:
            prompt = prompt[:max_log_len]
            prompt_token_ids = prompt_token_ids[:max_log_len]
        self.request_logger.log_inputs(
            request_id,
            prompt,
            prompt_token_ids,
            params,
            lora_request,
            prompt_adapter_request,
        )
