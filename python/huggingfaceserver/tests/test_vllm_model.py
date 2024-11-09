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

from typing import AsyncIterator, AsyncGenerator
from unittest import mock
import json
from argparse import Namespace  # TODO: check whether installed

import pytest
from _pytest.monkeypatch import MonkeyPatch
from vllm.entrypoints.openai.serving_completion import (
    OpenAIServingCompletion,
    BaseModelPath,
)
from vllm.entrypoints.openai.serving_chat import OpenAIServingChat
from vllm.entrypoints.openai.serving_embedding import OpenAIServingEmbedding
from vllm.entrypoints.openai.serving_tokenization import OpenAIServingTokenization
from vllm import RequestOutput, AsyncLLMEngine
from vllm.config import ModelConfig

from huggingfaceserver.vllm.vllm_model import VLLMModel
from kserve.logging import logger
from kserve.protocol.rest.openai.errors import OpenAIError
from kserve.protocol.rest.openai.types import (
    ChatCompletionRequest,
    CompletionRequest,
    ChatCompletionChoice,
    UsageInfo,
    ChatMessage,
    ChatCompletionChoiceLogProbs,
    ChatCompletionLogProbsContent,
    CompletionChoice,
    Completion,
    CompletionLogProbs,
)
from kserve.protocol.rest.openai.types.openapi import (
    ChatCompletionResponse,
    ChatCompletionLogProb,
)

from vllm_mock_outputs import (
    opt_chat_cmpl_chunks,
    opt_chat_cmpl_chunks_with_logprobs,
    opt_cmpl_chunks,
    opt_cmpl_chunks_with_logprobs,
    opt_cmpl_chunks_with_two_prompts,
    opt_cmpl_chunks_with_two_prompts_log_probs,
    opt_cmpl_chunks_with_n_2,
    opt_cmpl_chunks_with_n_3,
    opt_cmpl_chunks_with_logit_bias,
    opt_chat_cmpl_chunks_with_logit_bias,
    opt_cmpl_chunks_with_echo_logprobs,
)  # TODO: can not import?


@pytest.fixture(scope="module")
def vllm_opt_model():
    model_id = "facebook/opt-125m"
    dtype = "float32"
    max_model_len = 512
    base_model_paths = [
        BaseModelPath(name=name, model_path=model_id) for name in [model_id]
    ]

    async def mock_get_model_config():
        return ModelConfig(
            model=model_id,
            tokenizer=model_id,
            tokenizer_mode="auto",
            trust_remote_code=False,
            seed=0,
            dtype=dtype,
            max_model_len=max_model_len,
        )

    engine_client = mock.AsyncMock(
        spec=AsyncLLMEngine
    )  # TODO: probably it's not good just fall back
    engine_client.get_model_config = mock_get_model_config

    def mock_load(self) -> bool:
        self.engine_client = engine_client

        self.openai_serving_chat = OpenAIServingChat(
            self.engine_client,
            engine_client.get_model_config(),
            base_model_paths,
            "assistant",
        )
        self.openai_serving_completion = OpenAIServingCompletion(
            self.engine_client,
            engine_client.get_model_config(),
            base_model_paths,
        )
        self.openai_serving_embedding = OpenAIServingEmbedding(
            self.engine_client,
            engine_client.get_model_config(),
            base_model_paths,
        )
        self.openai_serving_tokenization = OpenAIServingTokenization(
            self.engine_client,
            engine_client.get_model_config(),
            base_model_paths,
        )

        self.ready = True
        return self.ready

    mp = MonkeyPatch()
    mp.setattr(VLLMModel, "load", mock_load)

    args = Namespace(model_name="opt-125m", response_role="assistant")
    model = VLLMModel(
        args,
    )
    model.load()
    yield model, engine_client  # TODO: Why we need to yield engine_client?
    model.stop()
    mp.undo()


"""
Does not supprt  apply_chat_template for now
def compare_chatprompt_to_expected(actual, expected, fields_to_compare=None) -> bool:
    if fields_to_compare is None:
        fields_to_compare = [
            "response_role",
            "prompt",
        ]
    for field in fields_to_compare:
        if not getattr(actual, field) == getattr(expected, field):
            logger.error(
                "expected: %s\n  got: %s",
                getattr(expected, field),
                getattr(actual, field),
            )
            return False
    return True


@pytest.mark.asyncio()
class TestChatTemplate:
    async def test_vllm_chat_completion_tokenization_facebook_opt_model(
        self, vllm_opt_model
    ):
        opt_model, _ = vllm_opt_model

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        chat_template = (
            "{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}"
        )
        response = opt_model.apply_chat_template(messages, chat_template)

        expected = ChatPrompt(
            response_role="assistant",
            prompt="You are a friendly chatbot who always responds in the style of a pirate</s>How many helicopters can a human eat in one sitting?</s>",
        )
        assert compare_chatprompt_to_expected(response, expected) is True


    async def test_vllm_chat_completion_template_tools(self, vllm_opt_model):
        opt_model, _ = vllm_opt_model

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who will help users with weather queries.",
            },
            {
                "role": "user",
                "content": "What is the weather in Ithaca, NY?",
            },
        ]

        tool = {
            "type": "function",
            "function": {
                "name": "get_current_weather",
                "description": "Get the current weather",
                "parameters": {
                    "type": "dict",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "format": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The temperature unit to use. Infer this from the users location.",
                        },
                    },
                    "required": ["location", "format"],
                },
            },
        }

        tools = [ChatCompletionTool(**tool)]

        chat_template = (
            "{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% for tool in tools %}"
            "{% endfor %}{% endfor %}"
        )
        response = opt_model.apply_chat_template(messages, chat_template, tools)

        # Sanity check / usage example to ensure that no error is thrown
        assert response.prompt is not None
"""


def compare_response_to_expected(actual, expected, fields_to_compare=None) -> bool:
    if fields_to_compare is None:
        fields_to_compare = [
            # "id",
            "choices",
            "system_fingerprint",
            "object",
            "usage",
            "model",
            "created",
        ]
    for field in fields_to_compare:
        if field == "created":
            if not isinstance(getattr(actual, field), int):
                logger.error(
                    "expected: %s\n  got: %s", "int", type(getattr(actual, field))
                )
                return False
        elif not getattr(actual, field) == getattr(expected, field):
            logger.error(
                "expected: %s\n  got: %s",
                getattr(expected, field),
                getattr(actual, field),
            )
            return False
    return True


@pytest.mark.asyncio()
class TestChatCompletions:
    async def test_vllm_chat_completion_facebook_opt_model_without_request_id(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = None

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            nonlocal request_id
            request_id = args[2]
            request_id = request_id.rsplit("-", 1)[0]
            for cmpl_chunk in opt_chat_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=False,
            max_tokens=10,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response = await opt_model.create_chat_completion(params)
        expected = ChatCompletionResponse(
            id=request_id,
            choices=[
                ChatCompletionChoice(
                    finish_reason="length",
                    index=0,
                    message=ChatMessage(
                        content="Most redditors know the tiny difference between Frogling",
                        tool_calls=None,
                        role="assistant",
                        function_call=None,
                    ),
                    logprobs=None,
                )
            ],
            created=1719498299,
            model="opt-125m",
            system_fingerprint=None,
            object="chat.completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=29, total_tokens=39),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_chat_completion_facebook_opt_model_with_max_token(
        self, vllm_opt_model
    ):
        opt_model, engine_client = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        engine_client.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=False,
            max_tokens=10,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response = await opt_model.create_chat_completion(params)
        expected = ChatCompletionResponse(
            id=request_id,
            choices=[
                ChatCompletionChoice(
                    finish_reason="length",
                    index=0,
                    message=ChatMessage(
                        content="Most redditors know the tiny difference between Frogling",
                        tool_calls=None,
                        role="assistant",
                        function_call=None,
                    ),
                    logprobs=None,
                )
            ],
            created=1719498299,
            model="opt-125m",
            system_fingerprint=None,
            object="chat.completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=29, total_tokens=39),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_chat_completion_facebook_opt_model_should_set_correct_max_tokens(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        max_tokens_arg = None

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            nonlocal max_tokens_arg
            # sampling_params is the second argument to generate()
            max_tokens_arg = args[1].max_tokens
            for cmpl_chunk in opt_chat_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]

        # max_tokens unset
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=False,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        await opt_model.create_chat_completion(params)
        assert max_tokens_arg is not None

        # max_tokens set
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=False,
            max_tokens=15,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        await opt_model.create_chat_completion(params)
        assert max_tokens_arg == 15

    async def test_vllm_chat_completion_facebook_opt_model_with_max_token_stream(
        self, vllm_opt_model
    ):
        model_name = "opt-125m"
        opt_model, mock_vllm_engine = vllm_opt_model

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model=model_name,
            messages=messages,
            stream=True,
            max_tokens=10,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response_iterator = await opt_model.create_chat_completion(params)

        completion = ""
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            completion += resp.choices[0].delta.content
            assert resp.choices[0].logprobs is None
            assert resp.model == model_name
            assert isinstance(resp.id, str)
            assert resp.object == "chat.completion.chunk"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)

        assert completion == "Most redditors know the tiny difference between Frogling"

    async def test_vllm_chat_completion_facebook_opt_model_with_logprobs(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks_with_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model=model_name,
            messages=messages,
            stream=False,
            max_tokens=10,
            log_probs=True,
            top_logprobs=2,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response = await opt_model.create_chat_completion(params)
        expected = ChatCompletionResponse(
            id=request_id,
            choices=[
                ChatCompletionChoice(
                    finish_reason="length",
                    index=0,
                    message=ChatMessage(
                        content="Most redditors know the tiny difference between Frogling",
                        tool_calls=None,
                        role="assistant",
                        function_call=None,
                    ),
                    logprobs=ChatCompletionChoiceLogProbs(
                        content=[
                            ChatCompletionLogProbsContent(
                                token="Most",
                                logprob=-6.909554481506348,
                                bytes=[77, 111, 115, 116],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token="Most",
                                        logprob=-6.909554481506348,
                                        bytes=[77, 111, 115, 116],
                                    ),
                                    ChatCompletionLogProb(
                                        token="I",
                                        logprob=-2.197445869445801,
                                        bytes=[73],
                                    ),
                                    ChatCompletionLogProb(
                                        token="The",
                                        logprob=-3.4867753982543945,
                                        bytes=[84, 104, 101],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" redd",
                                logprob=-7.630484580993652,
                                bytes=[32, 114, 101, 100, 100],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" redd",
                                        logprob=-7.630484580993652,
                                        bytes=[32, 114, 101, 100, 100],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" of",
                                        logprob=-1.8084166049957275,
                                        bytes=[32, 111, 102],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" people",
                                        logprob=-2.3389289379119873,
                                        bytes=[32, 112, 101, 111, 112, 108, 101],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token="itors",
                                logprob=-0.039746206253767014,
                                bytes=[105, 116, 111, 114, 115],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token="itors",
                                        logprob=-0.039746206253767014,
                                        bytes=[105, 116, 111, 114, 115],
                                    ),
                                    ChatCompletionLogProb(
                                        token="itor",
                                        logprob=-4.065564155578613,
                                        bytes=[105, 116, 111, 114],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" know",
                                logprob=-4.415658473968506,
                                bytes=[32, 107, 110, 111, 119],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" know",
                                        logprob=-4.415658473968506,
                                        bytes=[32, 107, 110, 111, 119],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" are",
                                        logprob=-1.5063375234603882,
                                        bytes=[32, 97, 114, 101],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" don",
                                        logprob=-2.7589268684387207,
                                        bytes=[32, 100, 111, 110],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" the",
                                logprob=-2.7328412532806396,
                                bytes=[32, 116, 104, 101],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" the",
                                        logprob=-2.7328412532806396,
                                        bytes=[32, 116, 104, 101],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" that",
                                        logprob=-1.2675859928131104,
                                        bytes=[32, 116, 104, 97, 116],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" this",
                                        logprob=-2.295158624649048,
                                        bytes=[32, 116, 104, 105, 115],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" tiny",
                                logprob=-9.554351806640625,
                                bytes=[32, 116, 105, 110, 121],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" tiny",
                                        logprob=-9.554351806640625,
                                        bytes=[32, 116, 105, 110, 121],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" answer",
                                        logprob=-1.7232582569122314,
                                        bytes=[32, 97, 110, 115, 119, 101, 114],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" difference",
                                        logprob=-3.347280740737915,
                                        bytes=[
                                            32,
                                            100,
                                            105,
                                            102,
                                            102,
                                            101,
                                            114,
                                            101,
                                            110,
                                            99,
                                            101,
                                        ],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" difference",
                                logprob=-4.9500274658203125,
                                bytes=[
                                    32,
                                    100,
                                    105,
                                    102,
                                    102,
                                    101,
                                    114,
                                    101,
                                    110,
                                    99,
                                    101,
                                ],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" difference",
                                        logprob=-4.9500274658203125,
                                        bytes=[
                                            32,
                                            100,
                                            105,
                                            102,
                                            102,
                                            101,
                                            114,
                                            101,
                                            110,
                                            99,
                                            101,
                                        ],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" amount",
                                        logprob=-3.1549720764160156,
                                        bytes=[32, 97, 109, 111, 117, 110, 116],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" little",
                                        logprob=-3.626887798309326,
                                        bytes=[32, 108, 105, 116, 116, 108, 101],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" between",
                                logprob=-0.08497463166713715,
                                bytes=[32, 98, 101, 116, 119, 101, 101, 110],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" between",
                                        logprob=-0.08497463166713715,
                                        bytes=[32, 98, 101, 116, 119, 101, 101, 110],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" in",
                                        logprob=-3.210397958755493,
                                        bytes=[32, 105, 110],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token=" Frog",
                                logprob=-12.07158374786377,
                                bytes=[32, 70, 114, 111, 103],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token=" Frog",
                                        logprob=-12.07158374786377,
                                        bytes=[32, 70, 114, 111, 103],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" a",
                                        logprob=-1.4436050653457642,
                                        bytes=[32, 97],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" the",
                                        logprob=-2.731874942779541,
                                        bytes=[32, 116, 104, 101],
                                    ),
                                ],
                            ),
                            ChatCompletionLogProbsContent(
                                token="ling",
                                logprob=-6.787796497344971,
                                bytes=[108, 105, 110, 103],
                                top_logprobs=[
                                    ChatCompletionLogProb(
                                        token="ling",
                                        logprob=-6.787796497344971,
                                        bytes=[108, 105, 110, 103],
                                    ),
                                    ChatCompletionLogProb(
                                        token=" and",
                                        logprob=-1.6513729095458984,
                                        bytes=[32, 97, 110, 100],
                                    ),
                                    ChatCompletionLogProb(
                                        token="s",
                                        logprob=-1.7453670501708984,
                                        bytes=[115],
                                    ),
                                ],
                            ),
                        ]
                    ),
                )
            ],
            created=1719498299,
            model=model_name,
            system_fingerprint=None,
            object="chat.completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=29, total_tokens=39),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_chat_completion_facebook_opt_model_with_logprobs_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks_with_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model=model_name,
            messages=messages,
            stream=True,
            max_tokens=10,
            log_probs=True,
            top_logprobs=2,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response_iterator = await opt_model.create_chat_completion(params)
        completion = ""
        log_probs = ChatCompletionChoiceLogProbs(
            content=[],
        )
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            completion += resp.choices[0].delta.content
            assert resp.choices[0].logprobs is not None
            log_probs.content += resp.choices[0].logprobs.content
            assert resp.model == model_name
            assert isinstance(resp.id, str)
            assert resp.object == "chat.completion.chunk"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)

        assert completion == "Most redditors know the tiny difference between Frogling"
        assert log_probs == ChatCompletionChoiceLogProbs(
            content=[
                ChatCompletionLogProbsContent(
                    token="Most",
                    logprob=-6.909554481506348,
                    bytes=[77, 111, 115, 116],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token="Most",
                            logprob=-6.909554481506348,
                            bytes=[77, 111, 115, 116],
                        ),
                        ChatCompletionLogProb(
                            token="I", logprob=-2.197445869445801, bytes=[73]
                        ),
                        ChatCompletionLogProb(
                            token="The",
                            logprob=-3.4867753982543945,
                            bytes=[84, 104, 101],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" redd",
                    logprob=-7.630484580993652,
                    bytes=[32, 114, 101, 100, 100],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" redd",
                            logprob=-7.630484580993652,
                            bytes=[32, 114, 101, 100, 100],
                        ),
                        ChatCompletionLogProb(
                            token=" of",
                            logprob=-1.8084166049957275,
                            bytes=[32, 111, 102],
                        ),
                        ChatCompletionLogProb(
                            token=" people",
                            logprob=-2.3389289379119873,
                            bytes=[32, 112, 101, 111, 112, 108, 101],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token="itors",
                    logprob=-0.039746206253767014,
                    bytes=[105, 116, 111, 114, 115],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token="itors",
                            logprob=-0.039746206253767014,
                            bytes=[105, 116, 111, 114, 115],
                        ),
                        ChatCompletionLogProb(
                            token="itor",
                            logprob=-4.065564155578613,
                            bytes=[105, 116, 111, 114],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" know",
                    logprob=-4.415658473968506,
                    bytes=[32, 107, 110, 111, 119],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" know",
                            logprob=-4.415658473968506,
                            bytes=[32, 107, 110, 111, 119],
                        ),
                        ChatCompletionLogProb(
                            token=" are",
                            logprob=-1.5063375234603882,
                            bytes=[32, 97, 114, 101],
                        ),
                        ChatCompletionLogProb(
                            token=" don",
                            logprob=-2.7589268684387207,
                            bytes=[32, 100, 111, 110],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" the",
                    logprob=-2.7328412532806396,
                    bytes=[32, 116, 104, 101],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" the",
                            logprob=-2.7328412532806396,
                            bytes=[32, 116, 104, 101],
                        ),
                        ChatCompletionLogProb(
                            token=" that",
                            logprob=-1.2675859928131104,
                            bytes=[32, 116, 104, 97, 116],
                        ),
                        ChatCompletionLogProb(
                            token=" this",
                            logprob=-2.295158624649048,
                            bytes=[32, 116, 104, 105, 115],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" tiny",
                    logprob=-9.554351806640625,
                    bytes=[32, 116, 105, 110, 121],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" tiny",
                            logprob=-9.554351806640625,
                            bytes=[32, 116, 105, 110, 121],
                        ),
                        ChatCompletionLogProb(
                            token=" answer",
                            logprob=-1.7232582569122314,
                            bytes=[32, 97, 110, 115, 119, 101, 114],
                        ),
                        ChatCompletionLogProb(
                            token=" difference",
                            logprob=-3.347280740737915,
                            bytes=[32, 100, 105, 102, 102, 101, 114, 101, 110, 99, 101],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" difference",
                    logprob=-4.9500274658203125,
                    bytes=[32, 100, 105, 102, 102, 101, 114, 101, 110, 99, 101],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" difference",
                            logprob=-4.9500274658203125,
                            bytes=[32, 100, 105, 102, 102, 101, 114, 101, 110, 99, 101],
                        ),
                        ChatCompletionLogProb(
                            token=" amount",
                            logprob=-3.1549720764160156,
                            bytes=[32, 97, 109, 111, 117, 110, 116],
                        ),
                        ChatCompletionLogProb(
                            token=" little",
                            logprob=-3.626887798309326,
                            bytes=[32, 108, 105, 116, 116, 108, 101],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" between",
                    logprob=-0.08497463166713715,
                    bytes=[32, 98, 101, 116, 119, 101, 101, 110],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" between",
                            logprob=-0.08497463166713715,
                            bytes=[32, 98, 101, 116, 119, 101, 101, 110],
                        ),
                        ChatCompletionLogProb(
                            token=" in",
                            logprob=-3.210397958755493,
                            bytes=[32, 105, 110],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token=" Frog",
                    logprob=-12.07158374786377,
                    bytes=[32, 70, 114, 111, 103],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token=" Frog",
                            logprob=-12.07158374786377,
                            bytes=[32, 70, 114, 111, 103],
                        ),
                        ChatCompletionLogProb(
                            token=" a", logprob=-1.4436050653457642, bytes=[32, 97]
                        ),
                        ChatCompletionLogProb(
                            token=" the",
                            logprob=-2.731874942779541,
                            bytes=[32, 116, 104, 101],
                        ),
                    ],
                ),
                ChatCompletionLogProbsContent(
                    token="ling",
                    logprob=-6.787796497344971,
                    bytes=[108, 105, 110, 103],
                    top_logprobs=[
                        ChatCompletionLogProb(
                            token="ling",
                            logprob=-6.787796497344971,
                            bytes=[108, 105, 110, 103],
                        ),
                        ChatCompletionLogProb(
                            token=" and",
                            logprob=-1.6513729095458984,
                            bytes=[32, 97, 110, 100],
                        ),
                        ChatCompletionLogProb(
                            token="s", logprob=-1.7453670501708984, bytes=[115]
                        ),
                    ],
                ),
            ]
        )

    async def test_vllm_chat_completion_facebook_opt_model_with_max_tokens_exceed_model_len(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks:
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=True,
            max_tokens=2048,
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        with pytest.raises(OpenAIError):
            await opt_model.create_chat_completion(params)

    async def test_vllm_chat_completion_facebook_opt_model_with_logit_bias(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_chat_cmpl_chunks_with_logit_bias:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        messages = [
            {
                "role": "system",
                "content": "You are a friendly chatbot who always responds in the style of a pirate",
            },
            {
                "role": "user",
                "content": "How many helicopters can a human eat in one sitting?",
            },
        ]
        params = ChatCompletionRequest(
            model="opt-125m",
            messages=messages,
            stream=False,
            max_tokens=10,
            logit_bias={"1527": 50, "27449": 100},
            chat_template="{% for message in messages %}"
            "{{ message.content }}{{ eos_token }}"
            "{% endfor %}",
        )
        response = await opt_model.create_chat_completion(params)
        expected = ChatCompletionResponse(
            id=request_id,
            choices=[
                ChatCompletionChoice(
                    finish_reason="length",
                    index=0,
                    message=ChatMessage(
                        content=" Frog Frog Frog Frog Frog Frog Frog Frog Frog Frog",
                        tool_calls=None,
                        role="assistant",
                        function_call=None,
                    ),
                    logprobs=None,
                )
            ],
            created=1719660998,
            model="opt-125m",
            system_fingerprint=None,
            object="chat.completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=29, total_tokens=39),
        )
        assert compare_response_to_expected(response, expected) is True


@pytest.mark.asyncio()
class TestCompletions:
    async def test_vllm_completion_facebook_opt_model_without_request_id(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = None

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            nonlocal request_id
            request_id = args[2]
            request_id = request_id.rsplit("-", 1)[0]
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719569921,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_max_token(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719569921,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_max_token_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
        )
        response_iterator = await opt_model.create_completion(params)
        completion = ""
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            completion += resp.choices[0].text
            assert resp.choices[0].logprobs is None
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completion == "- Labrador! He has tiny ears with fluffy white"

    async def test_vllm_completion_facebook_opt_model_with_logprobs(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
            logprobs=2,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=CompletionLogProbs(
                        text_offset=[0, 1, 10, 11, 14, 18, 23, 28, 33, 40],
                        token_logprobs=[
                            -5.968788146972656,
                            -11.009231567382812,
                            -3.1531941890716553,
                            -1.4167277812957764,
                            -2.766524314880371,
                            -6.9396467208862305,
                            -1.3619931936264038,
                            -4.619960308074951,
                            -5.6248779296875,
                            -2.2152767181396484,
                        ],
                        tokens=[
                            "-",
                            " Labrador",
                            "!",
                            " He",
                            " has",
                            " tiny",
                            " ears",
                            " with",
                            " fluffy",
                            " white",
                        ],
                        top_logprobs=[
                            {
                                "-": -5.968788146972656,
                                ".": -1.4537553787231445,
                                ",": -1.8416948318481445,
                            },
                            {
                                " Labrador": -11.009231567382812,
                                " I": -1.754422903060913,
                                " she": -3.075488328933716,
                            },
                            {
                                "!": -3.1531941890716553,
                                " mix": -1.0394361019134521,
                                ".": -2.1872146129608154,
                            },
                            {" He": -1.4167277812957764, "\n": -2.0672662258148193},
                            {
                                " has": -2.766524314880371,
                                "'s": -1.0847474336624146,
                                " is": -1.547521710395813,
                            },
                            {
                                " tiny": -6.9396467208862305,
                                " a": -1.3877270221710205,
                                " been": -2.3109371662139893,
                            },
                            {
                                " ears": -1.3619931936264038,
                                " paws": -2.2743258476257324,
                            },
                            {
                                " with": -4.619960308074951,
                                " and": -0.805719792842865,
                                ",": -1.6155686378479004,
                            },
                            {
                                " fluffy": -5.6248779296875,
                                " a": -1.4977400302886963,
                                " tiny": -3.006150484085083,
                            },
                            {
                                " white": -2.2152767181396484,
                                " fur": -1.9012728929519653,
                            },
                        ],
                    ),
                    text="- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719569921,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )

        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_logprobs_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
            logprobs=2,
        )
        response_iterator = await opt_model.create_completion(params)
        completion = ""
        log_probs = CompletionLogProbs(
            text_offset=[], token_logprobs=[], tokens=[], top_logprobs=[]
        )
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            completion += resp.choices[0].text
            assert resp.choices[0].logprobs is not None
            log_probs.text_offset += resp.choices[0].logprobs.text_offset
            log_probs.token_logprobs += resp.choices[0].logprobs.token_logprobs
            log_probs.tokens += resp.choices[0].logprobs.tokens
            log_probs.top_logprobs += resp.choices[0].logprobs.top_logprobs
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completion == "- Labrador! He has tiny ears with fluffy white"
        assert log_probs == CompletionLogProbs(
            text_offset=[0, 1, 10, 11, 14, 18, 23, 28, 33, 40],
            token_logprobs=[
                -5.968788146972656,
                -11.009231567382812,
                -3.1531941890716553,
                -1.4167277812957764,
                -2.766524314880371,
                -6.9396467208862305,
                -1.3619931936264038,
                -4.619960308074951,
                -5.6248779296875,
                -2.2152767181396484,
            ],
            tokens=[
                "-",
                " Labrador",
                "!",
                " He",
                " has",
                " tiny",
                " ears",
                " with",
                " fluffy",
                " white",
            ],
            top_logprobs=[
                {
                    "-": -5.968788146972656,
                    ".": -1.4537553787231445,
                    ",": -1.8416948318481445,
                },
                {
                    " Labrador": -11.009231567382812,
                    " I": -1.754422903060913,
                    " she": -3.075488328933716,
                },
                {
                    "!": -3.1531941890716553,
                    " mix": -1.0394361019134521,
                    ".": -2.1872146129608154,
                },
                {" He": -1.4167277812957764, "\n": -2.0672662258148193},
                {
                    " has": -2.766524314880371,
                    "'s": -1.0847474336624146,
                    " is": -1.547521710395813,
                },
                {
                    " tiny": -6.9396467208862305,
                    " a": -1.3877270221710205,
                    " been": -2.3109371662139893,
                },
                {" ears": -1.3619931936264038, " paws": -2.2743258476257324},
                {
                    " with": -4.619960308074951,
                    " and": -0.805719792842865,
                    ",": -1.6155686378479004,
                },
                {
                    " fluffy": -5.6248779296875,
                    " a": -1.4977400302886963,
                    " tiny": -3.006150484085083,
                },
                {" white": -2.2152767181396484, " fur": -1.9012728929519653},
            ],
        )

    async def test_vllm_completion_facebook_opt_model_with_echo(self, vllm_opt_model):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
            echo=True,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719569921,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_echo_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
            echo=True,
        )
        response_iterator = await opt_model.create_completion(params)
        completion = ""
        async for resp in response_iterator:
            resp = json.loads(resp)
            completion += resp.choices[0].text
            assert len(resp.choices) == 1
            assert resp.choices[0].logprobs is None
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert (
            completion
            == "Hi, I love my cat- Labrador! He has tiny ears with fluffy white"
        )

    async def test_vllm_completion_facebook_opt_model_with_echo_and_logprobs(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_echo_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
            echo=True,
            logprobs=2,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=CompletionLogProbs(
                        text_offset=[
                            0,
                            4,
                            6,
                            7,
                            9,
                            14,
                            17,
                            21,
                            22,
                            31,
                            32,
                            35,
                            39,
                            44,
                            49,
                            54,
                            61,
                        ],
                        token_logprobs=None,
                        tokens=[
                            "</s>",
                            "Hi",
                            ",",
                            " I",
                            " love",
                            " my",
                            " cat",
                            "-",
                            " Labrador",
                            "!",
                            " He",
                            " has",
                            " tiny",
                            " ears",
                            " with",
                            " fluffy",
                            " white",
                        ],
                        top_logprobs=None,
                    ),
                    text="Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719815937,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        # FixMe: pydantic does not allows adding None to the token_logrobs list. We should fix the type definition.
        expected.choices[0].logprobs.token_logprobs = [
            None,
            -9.352765083312988,
            -1.4278249740600586,
            -0.976689338684082,
            -5.6148481369018555,
            -4.214991569519043,
            -4.99854040145874,
            -5.968787670135498,
            -11.009231567382812,
            -3.1531941890716553,
            -1.4167277812957764,
            -2.766524314880371,
            -6.9396467208862305,
            -1.3619931936264038,
            -4.619960308074951,
            -5.6248779296875,
            -2.2152767181396484,
        ]
        # FixMe: pydantic does not allows adding None to the top_logrobs list. We should fix the type definition.
        expected.choices[0].logprobs.top_logprobs = [
            None,
            {
                "Hi": -9.352765083312988,
                "I": -1.4278708696365356,
                "The": -2.4365129470825195,
            },
            {",": -1.4278249740600586, "!": -1.934173583984375},
            {" I": -0.976689338684082, " ": -2.723400115966797},
            {
                " love": -5.6148481369018555,
                "'m": -1.015452265739441,
                " have": -1.9374703168869019,
            },
            {
                " my": -4.214991569519043,
                " your": -1.7619359493255615,
                " the": -1.999145269393921,
            },
            {
                " cat": -4.99854040145874,
                " new": -3.4642574787139893,
                " old": -4.73804235458374,
            },
            {"-": -5.968787670135498, ".": -1.453755497932434, ",": -1.841694951057434},
            {
                " Labrador": -11.009231567382812,
                " I": -1.754422903060913,
                " she": -3.075488328933716,
            },
            {
                "!": -3.1531941890716553,
                " mix": -1.0394361019134521,
                ".": -2.1872146129608154,
            },
            {" He": -1.4167277812957764, "\n": -2.0672662258148193},
            {
                " has": -2.766524314880371,
                "'s": -1.0847474336624146,
                " is": -1.547521710395813,
            },
            {
                " tiny": -6.9396467208862305,
                " a": -1.3877270221710205,
                " been": -2.3109371662139893,
            },
            {" ears": -1.3619931936264038, " paws": -2.2743258476257324},
            {
                " with": -4.619960308074951,
                " and": -0.805719792842865,
                ",": -1.6155686378479004,
            },
            {
                " fluffy": -5.6248779296875,
                " a": -1.4977400302886963,
                " tiny": -3.006150484085083,
            },
            {" white": -2.2152767181396484, " fur": -1.9012728929519653},
        ]
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_echo_and_logprobs_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_echo_logprobs:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
            echo=True,
            logprobs=2,
        )
        response_iterator = await opt_model.create_completion(params)
        completion = ""
        log_probs = CompletionLogProbs(
            text_offset=[], token_logprobs=[], tokens=[], top_logprobs=[]
        )
        expected_logprobs = CompletionLogProbs(
            # FixMe: text_offset resets for generated tokens if echo is True and stream is True. vLLM also behaves
            #  this way. Is this the expected behavior?
            text_offset=[0, 4, 6, 7, 9, 14, 17, 21, 1, 10, 11, 14, 18, 23, 28, 33, 40],
            token_logprobs=None,
            tokens=[
                "</s>",
                "Hi",
                ",",
                " I",
                " love",
                " my",
                " cat",
                "-",
                " Labrador",
                "!",
                " He",
                " has",
                " tiny",
                " ears",
                " with",
                " fluffy",
                " white",
            ],
            top_logprobs=None,
        )
        # FixMe: pydantic does not allows adding None to the token_logrobs list. We should fix the type definition.
        expected_logprobs.token_logprobs = [
            None,
            -9.352765083312988,
            -1.4278249740600586,
            -0.976689338684082,
            -5.6148481369018555,
            -4.214991569519043,
            -4.99854040145874,
            -5.968787670135498,
            -11.009231567382812,
            -3.1531941890716553,
            -1.4167277812957764,
            -2.766524314880371,
            -6.9396467208862305,
            -1.3619931936264038,
            -4.619960308074951,
            -5.6248779296875,
            -2.2152767181396484,
        ]
        # FixMe: pydantic does not allows adding None to the top_logrobs list. We should fix the type definition.
        expected_logprobs.top_logprobs = [
            None,
            {
                "Hi": -9.352765083312988,
                "I": -1.4278708696365356,
                "The": -2.4365129470825195,
            },
            {",": -1.4278249740600586, "!": -1.934173583984375},
            {" I": -0.976689338684082, " ": -2.723400115966797},
            {
                " love": -5.6148481369018555,
                "'m": -1.015452265739441,
                " have": -1.9374703168869019,
            },
            {
                " my": -4.214991569519043,
                " your": -1.7619359493255615,
                " the": -1.999145269393921,
            },
            {
                " cat": -4.99854040145874,
                " new": -3.4642574787139893,
                " old": -4.73804235458374,
            },
            {"-": -5.968787670135498, ".": -1.453755497932434, ",": -1.841694951057434},
            {
                " Labrador": -11.009231567382812,
                " I": -1.754422903060913,
                " she": -3.075488328933716,
            },
            {
                "!": -3.1531941890716553,
                " mix": -1.0394361019134521,
                ".": -2.1872146129608154,
            },
            {" He": -1.4167277812957764, "\n": -2.0672662258148193},
            {
                " has": -2.766524314880371,
                "'s": -1.0847474336624146,
                " is": -1.547521710395813,
            },
            {
                " tiny": -6.9396467208862305,
                " a": -1.3877270221710205,
                " been": -2.3109371662139893,
            },
            {" ears": -1.3619931936264038, " paws": -2.2743258476257324},
            {
                " with": -4.619960308074951,
                " and": -0.805719792842865,
                ",": -1.6155686378479004,
            },
            {
                " fluffy": -5.6248779296875,
                " a": -1.4977400302886963,
                " tiny": -3.006150484085083,
            },
            {" white": -2.2152767181396484, " fur": -1.9012728929519653},
        ]
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            completion += resp.choices[0].text
            assert resp.choices[0].logprobs is not None
            log_probs.text_offset += resp.choices[0].logprobs.text_offset
            log_probs.token_logprobs += resp.choices[0].logprobs.token_logprobs
            log_probs.tokens += resp.choices[0].logprobs.tokens
            log_probs.top_logprobs += resp.choices[0].logprobs.top_logprobs
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert (
            completion
            == "Hi, I love my cat- Labrador! He has tiny ears with fluffy white"
        )
        assert log_probs == expected_logprobs

    async def test_vllm_completion_facebook_opt_model_with_two_prompts(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model="opt-125m",
            prompt=prompts,
            stream=False,
            max_tokens=10,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with fluffy white",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text=" and no one is going to notice. You don",
                ),
            ],
            created=1719584168,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=12, total_tokens=32),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_two_prompts_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model=model_name,
            prompt=prompts,
            stream=True,
            max_tokens=10,
        )
        response_iterator = await opt_model.create_completion(params)
        completions = [""] * 2
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            assert resp.choices[0].logprobs is None
            if resp.choices[0].index == 0:
                completions[0] += resp.choices[0].text

            elif resp.choices[0].index == 1:
                completions[1] += resp.choices[0].text
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completions == [
            "- Labrador! He has tiny ears with fluffy white",
            " and no one is going to notice. You don",
        ]

    async def test_vllm_completion_facebook_opt_model_with_two_prompts_echo(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model="opt-125m",
            prompt=prompts,
            stream=False,
            max_tokens=10,
            echo=True,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text="The sky is blue and no one is going to notice. You don",
                ),
            ],
            created=1719584168,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=12, total_tokens=32),
        )
        assert compare_response_to_expected(response, expected) is True

    # FixMe: completion with echo true and stream fails
    async def test_vllm_completion_facebook_opt_model_with_two_prompts_echo_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model=model_name,
            prompt=prompts,
            stream=True,
            max_tokens=10,
            echo=True,
        )
        response_iterator = await opt_model.create_completion(params)
        completions = [""] * 2
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            assert resp.choices[0].logprobs is None
            if resp.choices[0].index == 0:
                completions[0] += resp.choices[0].text

            elif resp.choices[0].index == 1:
                completions[1] += resp.choices[0].text
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completions == [
            "Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
            "The sky is blue and no one is going to notice. You don",
        ]

    async def test_vllm_completion_facebook_opt_model_with_two_prompts_logprobs(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        model_name = "opt-125m"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model=model_name,
            prompt=prompts,
            stream=False,
            max_tokens=10,
            logprobs=2,
        )

        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=CompletionLogProbs(
                        text_offset=[0, 1, 10, 11, 14, 18, 23, 28, 33, 40],
                        token_logprobs=[
                            -5.968789577484131,
                            -11.009230613708496,
                            -3.1531925201416016,
                            -1.4167288541793823,
                            -2.7665247917175293,
                            -6.939647674560547,
                            -1.3619911670684814,
                            -4.619960784912109,
                            -5.624878883361816,
                            -2.215275764465332,
                        ],
                        tokens=[
                            "-",
                            " Labrador",
                            "!",
                            " He",
                            " has",
                            " tiny",
                            " ears",
                            " with",
                            " fluffy",
                            " white",
                        ],
                        top_logprobs=[
                            {
                                "-": -5.968789577484131,
                                ".": -1.4537543058395386,
                                ",": -1.8416975736618042,
                            },
                            {
                                " Labrador": -11.009230613708496,
                                " I": -1.7544232606887817,
                                " she": -3.0754880905151367,
                            },
                            {
                                "!": -3.1531925201416016,
                                " mix": -1.0394372940063477,
                                ".": -2.187213897705078,
                            },
                            {" He": -1.4167288541793823, "\n": -2.067265510559082},
                            {
                                " has": -2.7665247917175293,
                                "'s": -1.0847479104995728,
                                " is": -1.5475212335586548,
                            },
                            {
                                " tiny": -6.939647674560547,
                                " a": -1.3877274990081787,
                                " been": -2.3109357357025146,
                            },
                            {
                                " ears": -1.3619911670684814,
                                " paws": -2.2743265628814697,
                            },
                            {
                                " with": -4.619960784912109,
                                " and": -0.8057191371917725,
                                ",": -1.615569829940796,
                            },
                            {
                                " fluffy": -5.624878883361816,
                                " a": -1.4977388381958008,
                                " tiny": -3.0061492919921875,
                            },
                            {" white": -2.215275764465332, " fur": -1.901274561882019},
                        ],
                    ),
                    text="- Labrador! He has tiny ears with fluffy white",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=CompletionLogProbs(
                        text_offset=[0, 4, 7, 11, 14, 20, 23, 30, 31, 35],
                        token_logprobs=[
                            -2.4368906021118164,
                            -5.254100799560547,
                            -0.42237523198127747,
                            -1.1646629571914673,
                            -2.8208425045013428,
                            -0.08860369771718979,
                            -2.0382237434387207,
                            -1.2939914464950562,
                            -4.5231032371521,
                            -3.299767017364502,
                        ],
                        tokens=[
                            " and",
                            " no",
                            " one",
                            " is",
                            " going",
                            " to",
                            " notice",
                            ".",
                            " You",
                            " don",
                        ],
                        top_logprobs=[
                            {
                                " and": -2.4368906021118164,
                                ",": -1.4933549165725708,
                                ".": -1.4948359727859497,
                            },
                            {
                                " no": -5.254100799560547,
                                " the": -1.2720444202423096,
                                " I": -2.4218027591705322,
                            },
                            {
                                " one": -0.42237523198127747,
                                " clouds": -4.17390775680542,
                            },
                            {" is": -1.1646629571914673, " can": -2.5355124473571777},
                            {
                                " going": -2.8208425045013428,
                                " watching": -2.0994670391082764,
                                " looking": -2.5881574153900146,
                            },
                            {
                                " to": -0.08860369771718979,
                                " anywhere": -3.895568609237671,
                            },
                            {
                                " notice": -2.0382237434387207,
                                " see": -2.475170612335205,
                            },
                            {".": -1.2939914464950562, " it": -1.670294165611267},
                            {
                                " You": -4.5231032371521,
                                "\n": -0.5480296015739441,
                                " ": -2.24289870262146,
                            },
                            {
                                " don": -3.299767017364502,
                                " can": -2.143829822540283,
                                "'re": -2.1697640419006348,
                            },
                        ],
                    ),
                    text=" and no one is going to notice. You don",
                ),
            ],
            created=1719589967,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=12, total_tokens=32),
        )

        response = await opt_model.create_completion(params)

        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_two_prompts_logprobs_stream(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "opt-125m"
        prompts = ["Hi, I love my cat", "The sky is blue"]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == [2, 30086, 6, 38, 657, 127, 4758]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model=model_name,
            prompt=prompts,
            stream=True,
            max_tokens=10,
            logprobs=2,
        )
        response_iterator = await opt_model.create_completion(params)
        completions = [""] * 2
        log_probs_list = [
            CompletionLogProbs(
                text_offset=[], token_logprobs=[], tokens=[], top_logprobs=[]
            ),
            CompletionLogProbs(
                text_offset=[], token_logprobs=[], tokens=[], top_logprobs=[]
            ),
        ]
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            if resp.choices[0].index == 0:
                completions[0] += resp.choices[0].text
                assert resp.choices[0].logprobs is not None
                log_probs_list[0].text_offset += resp.choices[0].logprobs.text_offset
                log_probs_list[0].token_logprobs += resp.choices[
                    0
                ].logprobs.token_logprobs
                log_probs_list[0].tokens += resp.choices[0].logprobs.tokens
                log_probs_list[0].top_logprobs += resp.choices[0].logprobs.top_logprobs
            elif resp.choices[0].index == 1:
                completions[1] += resp.choices[0].text
                assert resp.choices[0].logprobs is not None
                log_probs_list[1].text_offset += resp.choices[0].logprobs.text_offset
                log_probs_list[1].token_logprobs += resp.choices[
                    0
                ].logprobs.token_logprobs
                log_probs_list[1].tokens += resp.choices[0].logprobs.tokens
                log_probs_list[1].top_logprobs += resp.choices[0].logprobs.top_logprobs
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completions == [
            "- Labrador! He has tiny ears with fluffy white",
            " and no one is going to notice. You don",
        ]
        assert log_probs_list == [
            CompletionLogProbs(
                text_offset=[0, 1, 10, 11, 14, 18, 23, 28, 33, 40],
                token_logprobs=[
                    -5.968789577484131,
                    -11.009230613708496,
                    -3.1531925201416016,
                    -1.4167288541793823,
                    -2.7665247917175293,
                    -6.939647674560547,
                    -1.3619911670684814,
                    -4.619960784912109,
                    -5.624878883361816,
                    -2.215275764465332,
                ],
                tokens=[
                    "-",
                    " Labrador",
                    "!",
                    " He",
                    " has",
                    " tiny",
                    " ears",
                    " with",
                    " fluffy",
                    " white",
                ],
                top_logprobs=[
                    {
                        "-": -5.968789577484131,
                        ".": -1.4537543058395386,
                        ",": -1.8416975736618042,
                    },
                    {
                        " Labrador": -11.009230613708496,
                        " I": -1.7544232606887817,
                        " she": -3.0754880905151367,
                    },
                    {
                        "!": -3.1531925201416016,
                        " mix": -1.0394372940063477,
                        ".": -2.187213897705078,
                    },
                    {" He": -1.4167288541793823, "\n": -2.067265510559082},
                    {
                        " has": -2.7665247917175293,
                        "'s": -1.0847479104995728,
                        " is": -1.5475212335586548,
                    },
                    {
                        " tiny": -6.939647674560547,
                        " a": -1.3877274990081787,
                        " been": -2.3109357357025146,
                    },
                    {" ears": -1.3619911670684814, " paws": -2.2743265628814697},
                    {
                        " with": -4.619960784912109,
                        " and": -0.8057191371917725,
                        ",": -1.615569829940796,
                    },
                    {
                        " fluffy": -5.624878883361816,
                        " a": -1.4977388381958008,
                        " tiny": -3.0061492919921875,
                    },
                    {" white": -2.215275764465332, " fur": -1.901274561882019},
                ],
            ),
            CompletionLogProbs(
                text_offset=[0, 4, 7, 11, 14, 20, 23, 30, 31, 35],
                token_logprobs=[
                    -2.4368906021118164,
                    -5.254100799560547,
                    -0.42237523198127747,
                    -1.1646629571914673,
                    -2.8208425045013428,
                    -0.08860369771718979,
                    -2.0382237434387207,
                    -1.2939914464950562,
                    -4.5231032371521,
                    -3.299767017364502,
                ],
                tokens=[
                    " and",
                    " no",
                    " one",
                    " is",
                    " going",
                    " to",
                    " notice",
                    ".",
                    " You",
                    " don",
                ],
                top_logprobs=[
                    {
                        " and": -2.4368906021118164,
                        ",": -1.4933549165725708,
                        ".": -1.4948359727859497,
                    },
                    {
                        " no": -5.254100799560547,
                        " the": -1.2720444202423096,
                        " I": -2.4218027591705322,
                    },
                    {" one": -0.42237523198127747, " clouds": -4.17390775680542},
                    {" is": -1.1646629571914673, " can": -2.5355124473571777},
                    {
                        " going": -2.8208425045013428,
                        " watching": -2.0994670391082764,
                        " looking": -2.5881574153900146,
                    },
                    {" to": -0.08860369771718979, " anywhere": -3.895568609237671},
                    {" notice": -2.0382237434387207, " see": -2.475170612335205},
                    {".": -1.2939914464950562, " it": -1.670294165611267},
                    {
                        " You": -4.5231032371521,
                        "\n": -0.5480296015739441,
                        " ": -2.24289870262146,
                    },
                    {
                        " don": -3.299767017364502,
                        " can": -2.143829822540283,
                        "'re": -2.1697640419006348,
                    },
                ],
            ),
        ]

    async def test_vllm_completion_facebook_opt_model_with_suffix(self, vllm_opt_model):
        opt_model, mock_vllm_engine = vllm_opt_model

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
            suffix="Thank You!",
        )
        with pytest.raises(OpenAIError, match="suffix is not currently supported"):
            await opt_model.create_completion(params)

    async def test_vllm_completion_facebook_opt_model_with_best_of_and_n_not_equal(
        self, vllm_opt_model
    ):
        """
        When best_of != n, the result should not be streamed even if stream=True is set
        """
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_n_2:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=True,
            max_tokens=10,
            best_of=3,
            n=2,
        )
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text=", so I know how much you guys are needing",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text=" and myself.  Sometimes I try to pick my",
                ),
            ],
            created=1719640772,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=7, total_tokens=27),
        )
        response = await opt_model.create_completion(params)

        assert not isinstance(response, AsyncGenerator)
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_best_of_and_n_equal(
        self, vllm_opt_model
    ):
        """
        When best_of == n, the result can be streamed when stream=True is set
        """
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "ot-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_n_3:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
            best_of=3,
            n=3,
        )
        response_iterator = await opt_model.create_completion(params)
        assert isinstance(response_iterator, AsyncGenerator)
        completions = [""] * 3
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            if resp.choices[0].index == 0:
                completions[0] += resp.choices[0].text
            elif resp.choices[0].index == 1:
                completions[1] += resp.choices[0].text
            elif resp.choices[0].index == 2:
                completions[2] += resp.choices[0].text
            assert len(resp.choices) == 1
            assert resp.choices[0].logprobs is None
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completions == [
            ", so I know how much you guys are needing",
            "-newbie and don't generally seek it out",
            " and myself.  Sometimes I try to pick my",
        ]

    async def test_vllm_completion_facebook_opt_model_with_best_of_and_n_and_echo(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_n_2:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            echo=True,
            max_tokens=10,
            best_of=3,
            n=2,
        )
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="Hi, I love my cat, so I know how much you guys are needing",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text="Hi, I love my cat and myself.  Sometimes I try to pick my",
                ),
            ],
            created=1719640772,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=7, total_tokens=27),
        )
        response = await opt_model.create_completion(params)

        assert not isinstance(response, AsyncGenerator)
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_best_of_and_n_and_echo_stream(
        self, vllm_opt_model
    ):
        """
        When best_of == n, the result can be streamed when stream=True is set
        """
        opt_model, mock_vllm_engine = vllm_opt_model
        model_name = "ot-125m"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_n_3:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model=model_name,
            prompt=prompt,
            stream=True,
            max_tokens=10,
            best_of=3,
            n=3,
            echo=True,
        )
        response_iterator = await opt_model.create_completion(params)
        assert isinstance(response_iterator, AsyncGenerator)
        completions = [""] * 3
        async for resp in response_iterator:
            resp = json.loads(resp)
            assert len(resp.choices) == 1
            if resp.choices[0].index == 0:
                completions[0] += resp.choices[0].text
            elif resp.choices[0].index == 1:
                completions[1] += resp.choices[0].text
            elif resp.choices[0].index == 2:
                completions[2] += resp.choices[0].text
            assert len(resp.choices) == 1
            assert resp.choices[0].logprobs is None
            assert resp.model == model_name
            assert resp.object == "text_completion"
            assert resp.system_fingerprint is None
            assert isinstance(resp.created, int)
        assert completions == [
            "Hi, I love my cat, so I know how much you guys are needing",
            "Hi, I love my cat-newbie and don't generally seek it out",
            "Hi, I love my cat and myself.  Sometimes I try to pick my",
        ]

    async def test_vllm_completion_facebook_opt_model_with_max_tokens_exceed_model_len(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=2048,
        )
        with pytest.raises(OpenAIError):
            await opt_model.create_completion(params)

    async def test_vllm_completion_facebook_opt_model_with_token_ids(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        token_ids = [2, 30086, 6, 38, 657, 127, 4758]
        params = CompletionRequest(
            model="opt-125m",
            prompt=token_ids,
            stream=False,
            max_tokens=10,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719646396,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )

        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_token_ids_and_echo(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        token_ids = [2, 30086, 6, 38, 657, 127, 4758]
        params = CompletionRequest(
            model="opt-125m",
            prompt=token_ids,
            stream=False,
            max_tokens=10,
            echo=True,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
                )
            ],
            created=1719646396,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        assert response.choices == expected.choices
        assert response.system_fingerprint == expected.system_fingerprint
        assert response.object == expected.object
        assert response.usage == expected.usage
        assert response.model == expected.model
        assert isinstance(response.created, int)

    async def test_vllm_completion_facebook_opt_model_with_batch_token_ids(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        token_ids = [[2, 30086, 6, 38, 657, 127, 4758], [2, 133, 6360, 16, 2440]]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == token_ids[0]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model="opt-125m",
            prompt=token_ids,
            stream=False,
            max_tokens=10,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with fluffy white",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text=" and no one is going to notice. You don",
                ),
            ],
            created=1719646549,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=12, total_tokens=32),
        )
        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_batch_token_ids_with_echo(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"
        token_ids = [[2, 30086, 6, 38, 657, 127, 4758], [2, 133, 6360, 16, 2440]]

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            if args[0]["prompt_token_ids"] == token_ids[0]:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[0]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk
            else:
                for cmpl_chunk in opt_cmpl_chunks_with_two_prompts_log_probs[1]:
                    cmpl_chunk.request_id = args[2]
                    yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        params = CompletionRequest(
            model="opt-125m",
            prompt=token_ids,
            stream=False,
            max_tokens=10,
            echo=True,
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="Hi, I love my cat- Labrador! He has tiny ears with fluffy white",
                ),
                CompletionChoice(
                    finish_reason="length",
                    index=1,
                    logprobs=None,
                    text="The sky is blue and no one is going to notice. You don",
                ),
            ],
            created=1719646549,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=20, prompt_tokens=12, total_tokens=32),
        )

        assert compare_response_to_expected(response, expected) is True

    async def test_vllm_completion_facebook_opt_model_with_logit_bias(
        self, vllm_opt_model
    ):
        opt_model, mock_vllm_engine = vllm_opt_model
        request_id = "cmpl-d771287a234c44498e345f0a429d6691"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            for cmpl_chunk in opt_cmpl_chunks_with_logit_bias:
                cmpl_chunk.request_id = args[2]
                yield cmpl_chunk

        mock_vllm_engine.generate = mock_generate

        prompt = "Hi, I love my cat"
        params = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            stream=False,
            max_tokens=10,
            logit_bias={"33564": -50},
        )
        response = await opt_model.create_completion(params)
        expected = Completion(
            id=request_id,
            choices=[
                CompletionChoice(
                    finish_reason="length",
                    index=0,
                    logprobs=None,
                    text="- Labrador! He has tiny ears with red hair",
                )
            ],
            created=1719659778,
            model="opt-125m",
            system_fingerprint=None,
            object="text_completion",
            usage=UsageInfo(completion_tokens=10, prompt_tokens=7, total_tokens=17),
        )
        assert compare_response_to_expected(response, expected) is True


class TestOpenAIServingCompletion:
    def test_validate_input_with_max_tokens_exceeding_model_limit(self, vllm_opt_model):
        opt_model, mock_vllm_engine = vllm_opt_model
        prompt = "Hi, I love my cat"

        async def mock_generate(*args, **kwargs) -> AsyncIterator[RequestOutput]:
            pass

        mock_vllm_engine.generate = mock_generate
        request = CompletionRequest(
            model="opt-125m",
            prompt=prompt,
            max_tokens=opt_model.openai_serving_completion.max_model_len + 1,
        )
        with pytest.raises(OpenAIError):  # TODO: should be value error?
            opt_model.openai_serving_completion._validate_input(
                request,
                input_text=prompt,
                input_ids=[2, 30086, 6, 38, 657, 127, 4758],
            )
