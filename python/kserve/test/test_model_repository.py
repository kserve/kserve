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
import pytest

from kserve import ModelRepository, Model
from kserve.protocol.rest.openai import CompletionRequest, OpenAIModel
from unittest.mock import patch
from kserve.protocol.rest.openai.types.openapi import (
    CreateChatCompletionResponse as ChatCompletion,
    CreateChatCompletionStreamResponse as ChatCompletionChunk,
    CreateCompletionResponse as Completion,
)
from typing import AsyncIterator, Union


class DummyOpenAIModel(OpenAIModel):
    async def create_completion(
        self, params: CompletionRequest
    ) -> Union[Completion, AsyncIterator[Completion]]:
        pass

    async def create_chat_completion(
        self, params: CompletionRequest
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        pass


def test_adding_kserve_model():
    repo = ModelRepository()
    repo.update(Model(name="kserve-model"))

    actual = repo.get_model("kserve-model")

    assert actual is not None
    assert isinstance(actual, Model)
    assert actual.name == "kserve-model"


def test_adding_openai_model():
    repo = ModelRepository()
    repo.update(DummyOpenAIModel(name="openai-model"))

    actual = repo.get_model("openai-model")

    assert actual is not None
    assert isinstance(actual, OpenAIModel)
    assert actual.name == "openai-model"


@pytest.mark.asyncio
async def test_is_model_ready_nonexistent_model():
    repo = ModelRepository()
    actual = await repo.is_model_ready("none-model")
    assert actual is False


@pytest.mark.asyncio
async def test_is_model_ready_kserve_model():
    repo = ModelRepository()
    model = Model(name="kserve-model")
    repo.update(model)
    with patch.object(model, "healthy"):
        model.healthy.side_effect = lambda: model.ready
        actual = await repo.is_model_ready("kserve-model")
        assert actual is False
        model.load()
        actual = await repo.is_model_ready("kserve-model")
        assert actual is True
        assert len(model.healthy.call_args) == 2


@pytest.mark.asyncio
async def test_is_model_ready_openai_model():
    repo = ModelRepository()
    model = DummyOpenAIModel(name="openai-model")
    repo.update(model)

    actual = await repo.is_model_ready("openai-model")
    assert actual is True
