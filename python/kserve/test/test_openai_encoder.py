# Copyright 2025 The KServe Authors.
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

from pathlib import Path
from typing import Tuple

import pytest

from kserve.protocol.rest.openai import (
    OpenAIEncoderModel,
)
from kserve.protocol.rest.openai.types import (
    Embedding,
    EmbeddingCompletionRequest as EmbeddingRequest,
    RerankRequest,
    Rerank,
)

FIXTURES_PATH = Path(__file__).parent / "fixtures" / "openai"


class DummyModel(OpenAIEncoderModel):
    embedding_data: Tuple[Embedding]
    rerank_data: Tuple[Rerank]

    def __init__(self, embedding_data: Tuple[Embedding], rerank_data: Tuple[Rerank]):
        super().__init__("dummy-model")
        self.embedding_data = embedding_data
        self.rerank_data = rerank_data

    async def create_embedding(self, request: EmbeddingRequest) -> Embedding:
        return self.embedding_data[0]

    async def create_rerank(self, request: RerankRequest) -> Rerank:
        return self.rerank_data[0]


@pytest.fixture
def embedding():
    with open(FIXTURES_PATH / "embedding.json") as f:
        return Embedding.model_validate_json(f.read())


@pytest.fixture
def embedding_create_params():
    with open(FIXTURES_PATH / "embedding_create_params.json") as f:
        return EmbeddingRequest.model_validate_json(f.read())


@pytest.fixture
def rerank():
    with open(FIXTURES_PATH / "rerank.json") as f:
        return Rerank.model_validate_json(f.read())


@pytest.fixture
def rerank_create_params():
    with open(FIXTURES_PATH / "rerank_create_params.json") as f:
        return RerankRequest.model_validate_json(f.read())


@pytest.fixture
def dummy_model_encoder(embedding, rerank):
    return DummyModel(embedding_data=(embedding,), rerank_data=(rerank,))


class TestOpenAICreateEmbedding:
    @pytest.mark.asyncio
    async def test_create_embedding(
        self,
        dummy_model_encoder: DummyModel,
        embedding: Embedding,
        embedding_create_params: EmbeddingRequest,
    ):
        c = await dummy_model_encoder.create_embedding(embedding_create_params)
        assert isinstance(c, Embedding)
        assert c.model_dump_json(indent=2) == embedding.model_dump_json(indent=2)

    @pytest.mark.asyncio
    async def test_create_rerank(
        self,
        dummy_model_encoder: DummyModel,
        rerank: Rerank,
        rerank_create_params: RerankRequest,
    ):
        c = await dummy_model_encoder.create_rerank(rerank_create_params)
        assert isinstance(c, Rerank)
        assert c.model_dump_json(indent=2) == rerank.model_dump_json(indent=2)
