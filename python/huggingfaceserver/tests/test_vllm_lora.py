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

import openai
import pytest
import pytest_asyncio
from huggingface_hub import snapshot_download

from server import RemoteOpenAIServer


MODEL = "Qwen/Qwen2-1.5B-Instruct"
MODEL_NAME = "Qwen2"
LORA_NAME = "teacher-persona-ai"


@pytest.fixture(scope="module")
def qwen2_lora_files():
    return snapshot_download(repo_id="RyZhangHason/Teacher-Persona")


@pytest.fixture(scope="module")
def lora_server(qwen2_lora_files):
    args = [
        # use half precision for speed and memory savings in CI environment
        "--dtype",
        "bfloat16",
        "--max-model-len",
        "2048",
        "--trust_remote_code",
        "--enforce-eager",
        "--enable-lora",
        "--lora-modules",
        '{"name": "%s", "path": "%s", "base_model_name": "%s"}'
        % (LORA_NAME, qwen2_lora_files, MODEL),
    ]

    with RemoteOpenAIServer(MODEL, MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest_asyncio.fixture
async def client(lora_server):
    async with lora_server.get_async_client() as async_client:
        yield async_client


@pytest.mark.vllm_cpu
@pytest.mark.asyncio
async def test_lora_chat(client: openai.AsyncOpenAI):

    chat_completion = await client.chat.completions.create(
        model=LORA_NAME,
        messages=[
            {
                "role": "system",
                "content": "You are an AI assistant with the qualities of an ideal teacher—empathetic, warm, and trustworthy. Your goal is to educate and support learners in a human-like manner.",
            },
            {
                "role": "user",
                "content": "Can you explain the Pythagorean theorem in a way that makes it easy to understand?",
            },
        ],
        temperature=0.0,
        seed=42,
    )

    message = chat_completion.choices[0].message
    assert message.role == "assistant"
    assert message.content is not None and len(message.content) >= 0


@pytest.mark.vllm_cpu
@pytest.mark.asyncio
async def test_lora_chat_stream(client: openai.AsyncOpenAI):
    stream = await client.chat.completions.create(
        model=LORA_NAME,
        messages=[
            {
                "role": "system",
                "content": "You are an AI assistant with the qualities of an ideal teacher—empathetic, warm, and trustworthy. Your goal is to educate and support learners in a human-like manner.",
            },
            {
                "role": "user",
                "content": "Can you explain the Pythagorean theorem in a way that makes it easy to understand?",
            },
        ],
        temperature=0.0,
        seed=42,
        stream=True,
    )

    collected_chunks = []
    complete_content = ""

    async for chunk in stream:
        collected_chunks.append(chunk)
        if chunk.choices[0].delta.content is not None:
            complete_content += chunk.choices[0].delta.content

    assert len(collected_chunks) > 0
    assert complete_content is not None and len(complete_content) > 0

    # Check first chunk has assistant role
    first_chunk = collected_chunks[0]
    assert first_chunk.choices[0].delta.role == "assistant"
