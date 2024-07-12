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

import time
import torch
import asyncio
from vllm.sampling_params import SamplingParams
from vllm.utils import random_uuid
from typing import (
    AsyncGenerator,
    AsyncIterator,
    Dict,
    Iterable,
    List,
    Optional,
    Tuple,
    Union,
)
from vllm.outputs import RequestOutput
from vllm.entrypoints.openai.serving_completion import (
    parse_prompt_format,
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
)
from kserve.errors import InvalidInput
from kserve.protocol.rest.openai.errors import OpenAIError
from kserve.protocol.rest.openai import ChatCompletionRequestMessage, CompletionRequest


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

    def __init__(self, engine: AsyncLLMEngine):
        self.engine = engine

        self.max_model_len = 0
        self.tokenizer = None

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
            prompt_is_tokens, prompts = parse_prompt_format(request.prompt)

            for i, prompt in enumerate(prompts):
                if prompt_is_tokens:
                    input_ids, prompt_text = self._validate_prompt_and_tokenize(
                        request, prompt_ids=prompt
                    )
                else:
                    input_ids, prompt_text = self._validate_prompt_and_tokenize(
                        request, prompt=prompt
                    )

                generators.append(
                    self.engine.generate(
                        {"prompt": prompt_text, "prompt_token_ids": input_ids},
                        sampling_params,
                        f"{request_id}-{i}",
                    )
                )
        except Exception as e:
            raise OpenAIError(str(e))

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
                        delta_token_ids = res.prompt_token_ids + output.token_ids
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
                    token_ids = prompt_token_ids + output.token_ids
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
    ):
        return self.tokenizer.apply_chat_template(
            conversation=messages, tokenize=False, add_generation_prompt=True
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

    def _validate_prompt_and_tokenize(
        self,
        request: Union[CreateCompletionRequest],
        prompt: Optional[str] = None,
        prompt_ids: Optional[List[int]] = None,
    ) -> Tuple[List[int], str]:
        if not (prompt or prompt_ids):
            raise InvalidInput("Either prompt or prompt_ids should be provided.")
        if prompt and prompt_ids:
            raise InvalidInput("Only one of prompt or prompt_ids should be provided.")

        input_ids = (
            prompt_ids if prompt_ids is not None else self.tokenizer(prompt).input_ids
        )
        token_num = len(input_ids)
        input_text = prompt if prompt is not None else self.tokenizer.decode(prompt_ids)

        if request.max_tokens is None:
            request.max_tokens = self.max_model_len - token_num

        if token_num + request.max_tokens > self.max_model_len:
            raise InvalidInput(
                f"This model's maximum context length is "
                f"{self.max_model_len} tokens. However, you requested "
                f"{request.max_tokens + token_num} tokens "
                f"({token_num} in the messages, "
                f"{request.max_tokens} in the completion). "
                f"Please reduce the length of the messages or completion.",
            )
        else:
            return input_ids, input_text

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
        out_text_offset: List[int] = []
        out_token_logprobs: List[Optional[float]] = []
        out_tokens: List[str] = []
        out_top_logprobs: List[Optional[Dict[str, float]]] = []

        last_token_len = 0

        for i, token_id in enumerate(token_ids):
            step_top_logprobs = top_logprobs[i]
            if step_top_logprobs is None:
                token = self.tokenizer.decode(token_id)
                out_tokens.append(token)
                out_token_logprobs.append(None)
                out_top_logprobs.append(None)
            else:
                token = self._get_decoded_token(step_top_logprobs[token_id], token_id)
                token_logprob = max(step_top_logprobs[token_id].logprob, -9999.0)
                out_tokens.append(token)
                out_token_logprobs.append(token_logprob)

                # makes sure to add the top num_output_top_logprobs + 1
                # logprobs, as defined in the openai API
                # (cf. https://github.com/openai/openai-openapi/blob/893ba52242dbd5387a97b96444ee1c742cfce9bd/openapi.yaml#L7153)
                out_top_logprobs.append(
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

            if len(out_text_offset) == 0:
                out_text_offset.append(initial_text_offset)
            else:
                out_text_offset.append(out_text_offset[-1] + last_token_len)
            last_token_len = len(token)

        return Logprobs(
            text_offset=out_text_offset,
            token_logprobs=out_token_logprobs,
            tokens=out_tokens,
            top_logprobs=out_top_logprobs,
        )
