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
from openai.types import Completion, CompletionChoice, CompletionUsage
from vllm.sampling_params import SamplingParams
from vllm.utils import random_uuid
from typing import AsyncGenerator, AsyncIterator, List, Tuple
from fastapi import Request
from vllm.outputs import RequestOutput
from vllm.entrypoints.openai.serving_completion import (
    parse_prompt_format,
    merge_async_iterators,
)
from .completions_utils import OpenAIServing
from vllm.engine.async_llm_engine import AsyncLLMEngine
from kserve.protocol.rest.openai.types.openapi import CreateCompletionRequest


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


class OpenAIServingCompletion(OpenAIServing):

    def __init__(self, engine: AsyncLLMEngine, served_model: str):
        super().__init__(engine=engine, served_model=served_model)

    async def create_completion(
        self, request: CreateCompletionRequest, raw_request: Request
    ):
        """Completion API similar to OpenAI's API.

        See https://platform.openai.com/docs/api-reference/completions/create
        for the API specification. This API mimics the OpenAI Completion API.

        NOTE: Currently we do not support the following feature:
            - suffix (the language models we currently support do not support
            suffix)
        """
        error_check_ret = await self._check_model(request)
        if error_check_ret is not None:
            return error_check_ret

        # Return error for unsupported features.
        if request.suffix is not None:
            return self.create_error_response("suffix is not currently supported")

        model_name = request.model
        request_id = f"cmpl-{random_uuid()}"
        created_time = int(time.time())

        # Schedule the request and get the result generator.
        generators = []
        try:
            sampling_params = to_sampling_params(request)
            prompt_is_tokens, prompts = parse_prompt_format(request.prompt)

            for i, prompt in enumerate(prompts):
                if prompt_is_tokens:
                    input_ids = self._validate_prompt_and_tokenize(
                        request, prompt_ids=prompt
                    )
                else:
                    input_ids = self._validate_prompt_and_tokenize(
                        request, prompt=prompt
                    )

                generators.append(
                    self.engine.generate(
                        prompt,
                        sampling_params,
                        f"{request_id}-{i}",
                        prompt_token_ids=input_ids,
                    )
                )
        except ValueError as e:
            return self.create_error_response(str(e))

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
                raw_request,
                result_generator,
                request_id,
                created_time,
                model_name,
                num_prompts=len(prompts),
            )

        # Non-streaming response
        final_res_batch: RequestOutput = [None] * len(prompts)
        try:
            async for i, res in result_generator:
                if await raw_request.is_disconnected():
                    # Abort the request if the client disconnects.
                    await self.engine.abort(f"{request_id}-{i}")
                    return self.create_error_response("Client disconnected")
                final_res_batch[i] = res
            response = self.request_output_to_completion_response(
                final_res_batch, request, request_id, created_time, model_name
            )
        except ValueError as e:
            return self.create_error_response(str(e))

        return response

    async def completion_stream_generator(
        self,
        request: CreateCompletionRequest,
        raw_request: Request,
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

                # Abort the request if the client disconnects.
                if await raw_request.is_disconnected():
                    await self.engine.abort(f"{request_id}-{prompt_idx}")
                    raise StopAsyncIteration()

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
                        top_logprobs = res.prompt_logprobs + (output.logprobs or [])
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
                                finish_reason=(
                                    finish_reason if finish_reason else "length"
                                ),  # finish_reason validation expects it be one of Literal["stop", "length", "content_filter"]
                                text=delta_text,
                                logprobs=logprobs,
                            )
                        ],
                        usage=final_usage,
                    )
                    yield response_json
        except ValueError as e:
            data = self.create_streaming_error_response(str(e))
            yield data

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
                    stop_reason=output.stop_reason,
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
