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
import openai
import pytest_asyncio

from server import RemoteOpenAIServer


MODEL = "deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B"
MODEL_NAME = "test-model"


@pytest.fixture(scope="module")
def server():  # noqa: F811
    args = [
        # use half precision for speed and memory savings in CI environment
        "--dtype",
        "bfloat16",
        "--max-model-len",
        "2048",
        "--trust_remote_code",
        "--enable-reasoning",
        "--reasoning-parser",
        "deepseek_r1",
    ]

    with RemoteOpenAIServer(MODEL, MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest_asyncio.fixture
async def client(server):
    async with server.get_async_client() as async_client:
        yield async_client


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_reasoning(client: openai.AsyncOpenAI, model_name: str):
    # Round 1
    messages = [{"role": "user", "content": "9.11 and 9.8, which is greater?"}]

    chat_completion = await client.chat.completions.create(
        model=model_name, messages=messages
    )

    assert chat_completion.object != "error"
    reasoning_content = chat_completion.choices[0].message.reasoning_content
    content = chat_completion.choices[0].message.content
    assert reasoning_content is not None
    assert content is not None

    print("reasoning_content for Round 1:", reasoning_content)
    print("content for Round 1:", content)

    # Round 2
    messages.append({"role": "assistant", "content": content})
    messages.append(
        {
            "role": "user",
            "content": "How many Rs are there in the word 'strawberry'?",
        }
    )
    chat_completion = await client.chat.completions.create(
        model=model_name, messages=messages
    )

    assert chat_completion.object != "error"
    reasoning_content = chat_completion.choices[0].message.reasoning_content
    content = chat_completion.choices[0].message.content
    assert reasoning_content is not None
    assert content is not None

    print("reasoning_content for Round 2:", reasoning_content)
    print("content for Round 2:", content)
