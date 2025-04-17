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
import requests
import json

from server import RemoteOpenAIServer


MODEL = "Qwen/Qwen2.5-1.5B-Instruct"
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
        "--enforce-eager",
        "--enable-auto-tool-choice",
        "--tool-call-parser",
        "hermes",
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
async def test_function_calling(client: openai.AsyncOpenAI, model_name: str):
    tools = [
        {
            "type": "function",
            "function": {
                "name": "get_weather",
                "description": "Get current temperature for provided coordinates in celsius.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "latitude": {"type": "number"},
                        "longitude": {"type": "number"},
                    },
                    "required": ["latitude", "longitude"],
                    "additionalProperties": False,
                },
                "strict": True,
            },
        }
    ]

    messages = [{"role": "user", "content": "What's the weather like in Paris today?"}]

    chat_completion = await client.chat.completions.create(
        model=model_name, messages=messages, tools=tools
    )

    assert chat_completion.object != "error"
    choice = chat_completion.choices[0]
    assert choice.message.tool_calls is not None
    assert len(choice.message.tool_calls) == 1
    tool_call = choice.message.tool_calls[0]
    assert tool_call.function.name == "get_weather"

    def get_weather(latitude, longitude):
        response = requests.get(
            f"https://api.open-meteo.com/v1/forecast?latitude={latitude}&longitude={longitude}&current=temperature_2m,wind_speed_10m&hourly=temperature_2m,relative_humidity_2m,wind_speed_10m"
        )
        data = response.json()
        return data["current"]["temperature_2m"]

    args = json.loads(tool_call.function.arguments)

    result = get_weather(args["latitude"], args["longitude"])

    messages.append(chat_completion.choices[0].message)
    messages.append(
        {
            "role": "tool",
            "tool_call_id": tool_call.id,
            "content": str(result),
        }
    )

    chat_completion_2 = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        tools=tools,
    )

    assert chat_completion.object != "error"
    assert chat_completion_2.choices[0].message.content is not None
    print(chat_completion_2.choices[0].message.content)
