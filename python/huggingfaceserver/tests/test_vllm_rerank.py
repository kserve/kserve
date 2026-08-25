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


import pytest
import requests

from kserve.protocol.rest.openai.types import Rerank
from server import RemoteOpenAIServer


MODEL = "BAAI/bge-reranker-base"
MODEL_NAME = "test-model"


@pytest.fixture(scope="module")
def server():  # noqa: F811
    args = [
        # use half precision for speed and memory savings in CI environment
        "--dtype",
        "bfloat16",
        "--max-model-len",
        "100",
        "--enforce-eager",
    ]

    with RemoteOpenAIServer(MODEL, MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_rerank_texts(server: RemoteOpenAIServer, model_name: str):
    query = "What is the capital of France?"
    documents = [
        "The capital of Brazil is Brasilia.",
        "The capital of France is Paris.",
    ]

    rerank_response = requests.post(
        server.url_for("openai/v1", "rerank"),
        json={
            "model": model_name,
            "query": query,
            "documents": documents,
        },
    )
    rerank_response.raise_for_status()
    rerank = Rerank.model_validate(rerank_response.json())

    assert rerank.id is not None
    assert rerank.results is not None
    assert len(rerank.results) == 2
    assert rerank.results[0].relevance_score >= 0.9
    assert rerank.results[1].relevance_score <= 0.01


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_top_n(server: RemoteOpenAIServer, model_name: str):
    query = "What is the capital of France?"
    documents = [
        "The capital of Brazil is Brasilia.",
        "The capital of France is Paris.",
        "Cross-encoder models are neat",
    ]

    rerank_response = requests.post(
        server.url_for("openai/v1", "rerank"),
        json={"model": model_name, "query": query, "documents": documents, "top_n": 2},
    )
    rerank_response.raise_for_status()
    rerank = Rerank.model_validate(rerank_response.json())

    assert rerank.id is not None
    assert rerank.results is not None
    assert len(rerank.results) == 2
    assert rerank.results[0].relevance_score >= 0.9
    assert rerank.results[1].relevance_score <= 0.01


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_rerank_max_model_len(server: RemoteOpenAIServer, model_name: str):
    query = "What is the capital of France?" * 100
    documents = [
        "The capital of Brazil is Brasilia.",
        "The capital of France is Paris.",
    ]

    rerank_response = requests.post(
        server.url_for("openai/v1", "rerank"),
        json={"model": model_name, "query": query, "documents": documents},
    )
    assert rerank_response.status_code == 400
    # Assert just a small fragments of the response
    assert "Please reduce the length of the input." in rerank_response.text
