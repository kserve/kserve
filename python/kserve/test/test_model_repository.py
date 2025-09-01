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
from kserve.protocol.rest.openai import (
    CompletionRequest,
    OpenAIGenerativeModel,
    OpenAIEncoderModel,
)
from unittest.mock import patch
from kserve.protocol.rest.openai.types import (
    ChatCompletion,
    ChatCompletionChunk,
    Completion,
    Embedding,
    EmbeddingRequest,
    Rerank,
    RerankRequest,
)
from typing import AsyncIterator, Union


class DummyOpenAIGenerativeModel(OpenAIGenerativeModel):
    async def create_completion(
        self, params: CompletionRequest
    ) -> Union[Completion, AsyncIterator[Completion]]:
        pass

    async def create_chat_completion(
        self, params: CompletionRequest
    ) -> Union[ChatCompletion, AsyncIterator[ChatCompletionChunk]]:
        pass


class DummyOpenAIEncoderModel(OpenAIEncoderModel):
    async def create_embedding(self, params: EmbeddingRequest) -> Embedding:
        pass

    async def create_rerank(self, params: RerankRequest) -> Rerank:
        pass


def test_adding_kserve_model():
    repo = ModelRepository()
    model = Model(name="kserve-model")
    repo.update(model)

    actual = repo.get_model("kserve-model")

    assert actual is not None
    assert isinstance(actual, Model)
    assert actual.name == "kserve-model"

    repo.update(model, name="additional-model-name")
    actual = repo.get_model("additional-model-name")
    assert actual is not None
    assert isinstance(actual, Model)
    assert actual is model


def test_adding_openai_completion_model():
    repo = ModelRepository()
    repo.update(DummyOpenAIGenerativeModel(name="openai-completion-model"))

    actual = repo.get_model("openai-completion-model")

    assert actual is not None
    assert isinstance(actual, OpenAIGenerativeModel)
    assert actual.name == "openai-completion-model"


def test_adding_openai_embedding_model():
    repo = ModelRepository()
    repo.update(DummyOpenAIEncoderModel(name="openai-embedding-model"))

    actual = repo.get_model("openai-embedding-model")

    assert actual is not None
    assert isinstance(actual, OpenAIEncoderModel)
    assert actual.name == "openai-embedding-model"


def test_adding_rerank_model():
    repo = ModelRepository()
    repo.update(DummyOpenAIEncoderModel(name="rerank-model"))

    actual = repo.get_model("rerank-model")

    assert actual is not None
    assert isinstance(actual, OpenAIEncoderModel)
    assert actual.name == "rerank-model"


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
    model = DummyOpenAIGenerativeModel(name="openai-model")
    repo.update(model)

    actual = await repo.is_model_ready("openai-model")
    assert actual is True
