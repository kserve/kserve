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

from kserve.protocol.rest.openai.types import Embedding
from server import RemoteOpenAIServer


MODEL = "intfloat/e5-small"
MODEL_NAME = "test-model"


@pytest.fixture(scope="module")
def server():  # noqa: F811
    args = [
        # use half precision for speed and memory savings in CI environment
        "--dtype",
        "bfloat16",
        "--enforce-eager",
    ]

    with RemoteOpenAIServer(MODEL, MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_embed_texts(server: RemoteOpenAIServer, model_name: str):
    inputs = [
        "Hello, how are you?",
        "The quick brown fox jumps over the lazy dog",
    ]

    embed_response = requests.post(
        server.url_for("openai/v1", "embeddings"),
        json={
            "model": model_name,
            "input": inputs,
            "encoding_format": "float",
        },
    )
    embed_response.raise_for_status()
    embedding = Embedding.model_validate(embed_response.json())

    assert embedding.object == "list"
    assert embedding.model is not None
    assert len(embedding.data) == 2
    for i, item in enumerate(embedding.data):
        assert item.index == i
        assert item.object == "embedding"
        assert isinstance(item.embedding, list)
        assert len(item.embedding) > 0
        assert all(isinstance(v, float) for v in item.embedding)
    assert embedding.usage.prompt_tokens > 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_embed_single_text(server: RemoteOpenAIServer, model_name: str):
    embed_response = requests.post(
        server.url_for("openai/v1", "embeddings"),
        json={
            "model": model_name,
            "input": "Hello, how are you?",
            "encoding_format": "float",
        },
    )
    embed_response.raise_for_status()
    embedding = Embedding.model_validate(embed_response.json())

    assert embedding.object == "list"
    assert len(embedding.data) == 1
    assert embedding.data[0].object == "embedding"
    assert isinstance(embedding.data[0].embedding, list)
    assert len(embedding.data[0].embedding) > 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_embed_max_model_len(server: RemoteOpenAIServer, model_name: str):
    # Sending a very long input should return a 400 error
    long_input = "Hello, how are you? " * 1000

    embed_response = requests.post(
        server.url_for("openai/v1", "embeddings"),
        json={
            "model": model_name,
            "input": long_input,
            "encoding_format": "float",
        },
    )
    assert embed_response.status_code == 400
