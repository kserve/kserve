# Copyright 2021 The KServe Authors.
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
import io
import json
from pathlib import Path
from unittest import mock
from openai.types import Completion, CompletionCreateParams
from openai.types.completion_create_params import CompletionCreateParamsNonStreaming
from openai.types.chat import (
    ChatCompletion,
    ChatCompletionChunk,
    CompletionCreateParams as ChatCompletionCreateParams,
)
from openai.types.chat.completion_create_params import (
    CompletionCreateParamsNonStreaming as ChatCompletionCreateParamsNonStreaming,
)
from kserve.protocol.rest.openai_model import OpenAIChatAdapterModel

import pytest

FIXTURES_PATH = Path(__file__).parent / "fixtures" / "openai"


class TestOpenAICompletionConversion:
    @pytest.fixture
    def completion(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "completion.json") as f:
            return Completion.model_validate_json(f.read())

    @pytest.fixture
    def chat_completion(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "chat_completion.json") as f:
            return ChatCompletion.model_validate_json(f.read())

    @pytest.fixture
    def chat_completion_chunk(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "chat_completion_chunk.json") as f:
            return ChatCompletionChunk.model_validate_json(f.read())

    @pytest.fixture
    def completion_partial(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "completion_partial.json") as f:
            return Completion.model_validate_json(f.read())

    def test_completion_to_chat_completion(
        self, completion: Completion, chat_completion: ChatCompletion
    ):
        converted_chat_completion = OpenAIChatAdapterModel.completion_to_chat_completion(
            completion, "assistant"
        )
        assert converted_chat_completion.model_dump_json() == chat_completion.model_dump_json()

    def test_completion_to_chat_completion_chunk(
        self, completion_partial: Completion, chat_completion_chunk: ChatCompletionChunk
    ):
        converted_chat_completion_chunk = (
            OpenAIChatAdapterModel.completion_to_chat_completion_chunk(
                completion_partial, "assistant"
            )
        )
        assert converted_chat_completion_chunk.model_dump_json(
            indent=2
        ) == chat_completion_chunk.model_dump_json(indent=2)


class TestOpenAIParamsConversion:
    @pytest.fixture
    def completion_create_params(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "completion_create_params.json") as f:
            return CompletionCreateParamsNonStreaming(**json.load(f))

    @pytest.fixture
    def chat_completion_create_params(self):  # pylint: disable=no-self-use
        with open(FIXTURES_PATH / "chat_completion_create_params.json") as f:
            return ChatCompletionCreateParamsNonStreaming(**json.load(f))

    def test_convert_params(
        self,
        chat_completion_create_params: ChatCompletionCreateParamsNonStreaming,
        completion_create_params: CompletionCreateParams,
    ):
        converted_params = OpenAIChatAdapterModel.chat_completion_params_to_completion_params(
            chat_completion_create_params,
            prompt=chat_completion_create_params["messages"][0]["content"],
        )
        print(converted_params)
        assert converted_params == completion_create_params
