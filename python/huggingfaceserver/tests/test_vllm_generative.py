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

import json
import re
from typing import Dict, List, Optional

import jsonschema
import torch
from openai import UnprocessableEntityError

from vllm.transformers_utils.tokenizer import get_tokenizer

from server import RemoteOpenAIServer


MODEL = "Qwen/Qwen2-1.5B-Instruct"
MODEL_NAME = "test-model"
GUIDED_DECODING_BACKENDS = ["outlines", "lm-format-enforcer", "xgrammar"]


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
    ]

    with RemoteOpenAIServer(MODEL, MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest_asyncio.fixture
async def client(server):
    async with server.get_async_client() as async_client:
        yield async_client


@pytest.fixture
def sample_guided_choice():
    return [
        "Python",
        "Java",
        "JavaScript",
        "C++",
        "C#",
        "PHP",
        "TypeScript",
        "Ruby",
        "Swift",
        "Kotlin",
    ]


@pytest.fixture
def sample_json_schema():
    return {
        "type": "object",
        "properties": {
            "name": {"type": "string"},
            "age": {"type": "integer"},
            "skills": {
                "type": "array",
                "items": {"type": "string", "maxLength": 20},
                "minItems": 3,
            },
            "work_history": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "company": {"type": "string"},
                        "duration": {"type": "number"},
                        "position": {"type": "string"},
                    },
                    "required": ["company", "position"],
                },
            },
        },
        "required": ["name", "age", "skills", "work_history"],
    }


@pytest.fixture
def sample_regex():
    return r"((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.){3}" r"(25[0-5]|(2[0-4]|1\d|[1-9]|)\d)"


@pytest.fixture
def sample_sql_statements():
    return """
start: select_statement
select_statement: "SELECT" column "from" table "where" condition
column: "col_1" | "col_2"
table: "table_1" | "table_2"
condition: column "=" number
number: "1" | "2"
"""


# --------------------------- CHAT COMPLETIONS --------------------------------
@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_no_logprobs_chat(client: openai.AsyncOpenAI, model_name: str):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {"role": "user", "content": "what is 1+1?"},
    ]

    chat_completion = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=5,
        temperature=0.0,
        logprobs=False,
    )

    assert chat_completion.object != "error"
    choice = chat_completion.choices[0]
    assert choice.logprobs is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_some_logprobs_chat(client: openai.AsyncOpenAI, model_name: str):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {"role": "user", "content": "what is 1+1?"},
    ]

    chat_completion = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=5,
        temperature=0.0,
        logprobs=True,
        top_logprobs=5,
    )

    choice = chat_completion.choices[0]
    assert choice.logprobs is not None
    assert choice.logprobs.content is not None
    assert len(choice.logprobs.content[0].top_logprobs) == 5


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_too_many_chat_logprobs(client: openai.AsyncOpenAI, model_name: str):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {"role": "user", "content": "what is 1+1?"},
    ]

    # Default max_logprobs is 20, so this should raise an error
    with pytest.raises((openai.BadRequestError, openai.APIError)):
        stream = await client.chat.completions.create(
            model=model_name,
            messages=messages,
            max_completion_tokens=10,
            logprobs=True,
            top_logprobs=21,
            stream=True,
        )
        async for chunk in stream:
            ...

    # the server should still work afterwards
    chat_completion = await client.chat.completions.create(
        model=model_name, messages=messages, max_completion_tokens=10, stream=False
    )
    message = chat_completion.choices[0].message
    assert message.content is not None and len(message.content) >= 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name, prompt_logprobs",
    [(MODEL_NAME, 1), (MODEL_NAME, 0), (MODEL_NAME, -1), (MODEL_NAME, None)],
)
async def test_prompt_logprobs_chat(
    client: openai.AsyncOpenAI, model_name: str, prompt_logprobs: Optional[int]
):
    params: Dict = {
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who won the world series in 2020?"},
            {
                "role": "assistant",
                "content": "The Los Angeles Dodgers won the World Series in 2020.",
            },
            {"role": "user", "content": "Where was it played?"},
        ],
        "model": model_name,
    }

    if prompt_logprobs is not None:
        params["extra_body"] = {"prompt_logprobs": prompt_logprobs}

    if prompt_logprobs is not None and prompt_logprobs < 0:
        with pytest.raises(UnprocessableEntityError):
            await client.chat.completions.create(**params)
    else:
        completion = await client.chat.completions.create(**params)
        if prompt_logprobs is not None:
            assert completion.prompt_logprobs is not None
            assert len(completion.prompt_logprobs) > 0
        else:
            assert completion.prompt_logprobs is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_more_than_one_prompt_logprobs_chat(
    client: openai.AsyncOpenAI, model_name: str
):
    params: Dict = {
        "messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who won the world series in 2020?"},
            {
                "role": "assistant",
                "content": "The Los Angeles Dodgers won the World Series in 2020.",
            },
            {"role": "user", "content": "Where was it played?"},
        ],
        "model": model_name,
        "extra_body": {"prompt_logprobs": 1},
    }

    completion_1 = await client.chat.completions.create(**params)

    params["extra_body"] = {"prompt_logprobs": 2}
    completion_2 = await client.chat.completions.create(**params)

    assert len(completion_1.prompt_logprobs[3]) == 2
    assert len(completion_2.prompt_logprobs[3]) == 3


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_single_chat_session(client: openai.AsyncOpenAI, model_name: str):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {"role": "user", "content": "what is 1+1?"},
    ]

    # test single completion
    chat_completion = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=5,
        logprobs=True,
        top_logprobs=5,
    )
    assert chat_completion.id is not None
    assert len(chat_completion.choices) == 1

    choice = chat_completion.choices[0]
    assert choice.finish_reason == "length"
    assert chat_completion.usage == openai.types.CompletionUsage(
        completion_tokens=5, prompt_tokens=25, total_tokens=30
    )

    message = choice.message
    assert message.content is not None and len(message.content) >= 5
    assert message.role == "assistant"
    messages.append({"role": "assistant", "content": message.content})

    # test multi-turn dialogue
    messages.append({"role": "user", "content": "express your result in json"})
    chat_completion = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
    )
    message = chat_completion.choices[0].message
    assert message.content is not None and len(message.content) >= 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_chat_streaming(client: openai.AsyncOpenAI, model_name: str):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {"role": "user", "content": "what is 1+1?"},
    ]

    # test single completion
    chat_completion = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.0,
    )
    output = chat_completion.choices[0].message.content
    stop_reason = chat_completion.choices[0].finish_reason

    # test streaming
    stream = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.0,
        stream=True,
    )
    chunks: List[str] = []
    finish_reason_count = 0
    async for chunk in stream:
        delta = chunk.choices[0].delta
        if delta.role:
            assert delta.role == "assistant"
        if delta.content:
            chunks.append(delta.content)
        if chunk.choices[0].finish_reason is not None:
            finish_reason_count += 1
    # finish reason should only return in last block
    assert finish_reason_count == 1
    assert chunk.choices[0].finish_reason == stop_reason
    # assert delta.content
    assert "".join(chunks) == output


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_chat_completion_stream_options(
    client: openai.AsyncOpenAI, model_name: str
):
    messages = [
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "What is the capital of France?"},
    ]

    # Test stream=True, stream_options={"include_usage": False}
    stream = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.0,
        stream=True,
        stream_options={"include_usage": False},
    )
    async for chunk in stream:
        assert chunk.usage is None

    # Test stream=True, stream_options={"include_usage": True,
    #                                   "continuous_usage_stats": False}}
    stream = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.0,
        stream=True,
        stream_options={"include_usage": True, "continuous_usage_stats": False},
    )

    async for chunk in stream:
        if chunk.choices[0].finish_reason is None:
            assert chunk.usage is None
        else:
            assert chunk.usage is None
            final_chunk = await stream.__anext__()
            assert final_chunk.usage is not None
            assert final_chunk.usage.prompt_tokens > 0
            assert final_chunk.usage.completion_tokens > 0
            assert final_chunk.usage.total_tokens == (
                final_chunk.usage.prompt_tokens + final_chunk.usage.completion_tokens
            )
            assert final_chunk.choices == []

    # Test stream=False, stream_options={"include_usage": None}
    with pytest.raises(UnprocessableEntityError):
        await client.chat.completions.create(
            model=model_name,
            messages=messages,
            max_completion_tokens=10,
            temperature=0.0,
            stream=False,
            stream_options={"include_usage": None},
        )

    # Test stream=False, stream_options={"include_usage": True}
    with pytest.raises(UnprocessableEntityError):
        await client.chat.completions.create(
            model=model_name,
            messages=messages,
            max_completion_tokens=10,
            temperature=0.0,
            stream=False,
            stream_options={"include_usage": True},
        )

    # Test stream=True, stream_options={"include_usage": True,
    #                           "continuous_usage_stats": True}
    stream = await client.chat.completions.create(
        model=model_name,
        messages=messages,
        max_completion_tokens=10,
        extra_body=dict(min_tokens=10),
        temperature=0.0,
        stream=True,
        stream_options={
            "include_usage": True,
            "continuous_usage_stats": True,
        },
    )
    last_completion_tokens = 0
    async for chunk in stream:
        assert chunk.usage.prompt_tokens >= 0
        assert (
            last_completion_tokens == 0
            or chunk.usage.completion_tokens > last_completion_tokens
            or (
                not chunk.choices
                and chunk.usage.completion_tokens == last_completion_tokens
            )
        )
        assert chunk.usage.total_tokens == (
            chunk.usage.prompt_tokens + chunk.usage.completion_tokens
        )
        last_completion_tokens = chunk.usage.completion_tokens

    assert last_completion_tokens == 10


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", GUIDED_DECODING_BACKENDS)
async def test_guided_choice_chat(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_guided_choice
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": "The best language for type-safe systems programming is ",
        },
    ]
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.7,
        extra_body=dict(
            guided_choice=sample_guided_choice,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )
    choice1 = chat_completion.choices[0].message.content
    assert choice1 in sample_guided_choice

    messages.append({"role": "assistant", "content": choice1})
    messages.append({"role": "user", "content": "I disagree, pick another one"})
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=10,
        temperature=0.7,
        extra_body=dict(
            guided_choice=sample_guided_choice,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )
    choice2 = chat_completion.choices[0].message.content
    assert choice2 in sample_guided_choice
    assert choice1 != choice2


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", GUIDED_DECODING_BACKENDS)
async def test_guided_json_chat(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_json_schema
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": f"Give an example JSON for an employee profile that "
            f"fits this schema: {sample_json_schema}",
        },
    ]
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=1000,
        extra_body=dict(
            guided_json=sample_json_schema,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )
    message = chat_completion.choices[0].message
    assert message.content is not None
    json1 = json.loads(message.content)
    jsonschema.validate(instance=json1, schema=sample_json_schema)

    messages.append({"role": "assistant", "content": message.content})
    messages.append(
        {"role": "user", "content": "Give me another one with a different name and age"}
    )
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=1000,
        extra_body=dict(
            guided_json=sample_json_schema,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )
    message = chat_completion.choices[0].message
    assert message.content is not None
    json2 = json.loads(message.content)
    jsonschema.validate(instance=json2, schema=sample_json_schema)
    assert json1["name"] != json2["name"]
    assert json1["age"] != json2["age"]


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", GUIDED_DECODING_BACKENDS)
async def test_guided_regex_chat(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_regex
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": f"Give an example IP address with this regex: {sample_regex}",
        },
    ]
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=20,
        extra_body=dict(
            guided_regex=sample_regex, guided_decoding_backend=guided_decoding_backend
        ),
    )
    ip1 = chat_completion.choices[0].message.content
    assert ip1 is not None
    assert re.fullmatch(sample_regex, ip1) is not None

    messages.append({"role": "assistant", "content": ip1})
    messages.append({"role": "user", "content": "Give me a different one"})
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=20,
        extra_body=dict(
            guided_regex=sample_regex, guided_decoding_backend=guided_decoding_backend
        ),
    )
    ip2 = chat_completion.choices[0].message.content
    assert ip2 is not None
    assert re.fullmatch(sample_regex, ip2) is not None
    assert ip1 != ip2


@pytest.mark.asyncio
async def test_guided_decoding_type_error_chat(client: openai.AsyncOpenAI):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": "The best language for type-safe systems programming is ",
        },
    ]

    with pytest.raises(UnprocessableEntityError):
        _ = await client.chat.completions.create(
            model=MODEL_NAME,
            messages=messages,
            extra_body=dict(guided_regex={1: "Python", 2: "C++"}),
        )


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", GUIDED_DECODING_BACKENDS)
async def test_guided_choice_chat_logprobs(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_guided_choice
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": "The best language for type-safe systems programming is ",
        },
    ]
    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=10,
        logprobs=True,
        top_logprobs=5,
        extra_body=dict(
            guided_choice=sample_guided_choice,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )

    assert chat_completion.choices[0].logprobs is not None
    assert chat_completion.choices[0].logprobs.content is not None
    top_logprobs = chat_completion.choices[0].logprobs.content[0].top_logprobs

    # -9999.0 is the minimum logprob returned by OpenAI
    for item in top_logprobs:
        assert item.logprob >= -9999.0, f"Failed (top_logprobs={top_logprobs})"


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", GUIDED_DECODING_BACKENDS)
async def test_named_tool_use(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_json_schema
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": f"Give an example JSON for an employee profile that "
            f"fits this schema: {sample_json_schema}",
        },
    ]

    # non-streaming

    chat_completion = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=1000,
        tools=[
            {
                "type": "function",
                "function": {
                    "name": "dummy_function_name",
                    "description": "This is a dummy function",
                    "parameters": sample_json_schema,
                },
            }
        ],
        tool_choice={"type": "function", "function": {"name": "dummy_function_name"}},
        extra_body=dict(guided_decoding_backend=guided_decoding_backend),
    )
    message = chat_completion.choices[0].message
    assert len(message.content) == 0
    json_string = message.tool_calls[0].function.arguments
    json1 = json.loads(json_string)
    jsonschema.validate(instance=json1, schema=sample_json_schema)

    messages.append({"role": "assistant", "content": json_string})
    messages.append(
        {"role": "user", "content": "Give me another one with a different name and age"}
    )

    # streaming

    stream = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=messages,
        max_completion_tokens=1000,
        tools=[
            {
                "type": "function",
                "function": {
                    "name": "dummy_function_name",
                    "description": "This is a dummy function",
                    "parameters": sample_json_schema,
                },
            }
        ],
        tool_choice={"type": "function", "function": {"name": "dummy_function_name"}},
        extra_body=dict(guided_decoding_backend=guided_decoding_backend),
        stream=True,
    )

    output = []
    finish_reason_count = 0
    async for chunk in stream:
        delta = chunk.choices[0].delta
        if delta.role:
            assert delta.role == "assistant"
        assert delta.content is None or len(delta.content) == 0
        if delta.tool_calls:
            output.append(delta.tool_calls[0].function.arguments)
        if chunk.choices[0].finish_reason is not None:
            finish_reason_count += 1
    # finish reason should only return in last block
    assert finish_reason_count == 1
    json2 = json.loads("".join(output))
    jsonschema.validate(instance=json2, schema=sample_json_schema)
    assert json1["name"] != json2["name"]
    assert json1["age"] != json2["age"]


@pytest.mark.asyncio
async def test_inconsistent_tool_choice_and_tools(
    client: openai.AsyncOpenAI, sample_json_schema
):
    messages = [
        {"role": "system", "content": "you are a helpful assistant"},
        {
            "role": "user",
            "content": f"Give an example JSON for an employee profile that "
            f"fits this schema: {sample_json_schema}",
        },
    ]

    with pytest.raises(UnprocessableEntityError):
        await client.chat.completions.create(
            model=MODEL_NAME,
            messages=messages,
            max_completion_tokens=1000,
            tool_choice={
                "type": "function",
                "function": {"name": "dummy_function_name"},
            },
        )

    with pytest.raises(UnprocessableEntityError):
        await client.chat.completions.create(
            model=MODEL_NAME,
            messages=messages,
            max_completion_tokens=1000,
            tools=[
                {
                    "type": "function",
                    "function": {
                        "name": "dummy_function_name",
                        "description": "This is a dummy function",
                        "parameters": sample_json_schema,
                    },
                }
            ],
            tool_choice={
                "type": "function",
                "function": {"name": "nondefined_function_name"},
            },
        )
    with pytest.raises(UnprocessableEntityError):
        await client.chat.completions.create(
            model=MODEL_NAME,
            messages=messages,
            max_completion_tokens=1000,
            tools=[
                {
                    "type": "function",
                    "function": {
                        "name": "dummy_function_name",
                        "description": "This is a dummy function",
                        "parameters": sample_json_schema,
                    },
                }
            ],
            tool_choice={},
        )


@pytest.mark.asyncio
async def test_response_format_json_object(client: openai.AsyncOpenAI):
    for _ in range(2):
        resp = await client.chat.completions.create(
            model=MODEL_NAME,
            messages=[
                {
                    "role": "user",
                    "content": (
                        "what is 1+1? please respond with a JSON object, "
                        'the format is {"result": 2}'
                    ),
                }
            ],
            response_format={"type": "json_object"},
        )

        content = resp.choices[0].message.content
        assert content is not None

        loaded = json.loads(content)
        assert loaded == {"result": 2}, loaded


@pytest.mark.asyncio
async def test_response_format_json_schema(client: openai.AsyncOpenAI):
    prompt = 'what is 1+1? The format is "result": 2'
    # Check that this prompt cannot lead to a valid JSON without json_schema
    for _ in range(2):
        resp = await client.chat.completions.create(
            model=MODEL_NAME,
            messages=[{"role": "user", "content": prompt}],
        )
        content = resp.choices[0].message.content
        assert content is not None
        with pytest.raises((json.JSONDecodeError, AssertionError)):
            loaded = json.loads(content)
            assert loaded == {"result": 2}, loaded

    for _ in range(2):
        resp = await client.chat.completions.create(
            model=MODEL_NAME,
            messages=[{"role": "user", "content": prompt}],
            response_format={
                "type": "json_schema",
                "json_schema": {
                    "name": "foo_test",
                    "schema": {
                        "type": "object",
                        "properties": {
                            "result": {"type": "integer"},
                        },
                    },
                },
            },
        )

        content = resp.choices[0].message.content
        assert content is not None

        loaded = json.loads(content)
        assert "result" in loaded, loaded


@pytest.mark.asyncio
async def test_extra_fields_allowed(client: openai.AsyncOpenAI):
    resp = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=[
            {
                "role": "user",
                "content": "what is 1+1?",
                "extra_field": "0",
            }
        ],  # type: ignore
        temperature=0,
        seed=0,
    )

    content = resp.choices[0].message.content
    assert content is not None


@pytest.mark.asyncio
async def test_complex_message_content(client: openai.AsyncOpenAI):
    resp = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=[
            {
                "role": "user",
                "content": [
                    {
                        "type": "text",
                        "text": "what is 1+1? please provide the result without any other text.",
                    }
                ],
            }
        ],
        temperature=0,
        seed=0,
    )
    content = resp.choices[0].message.content
    assert content is not None
    assert "2" in content


@pytest.mark.asyncio
async def test_custom_role(client: openai.AsyncOpenAI):
    # Not sure how the model handles custom roles so we just check that
    # both string and complex message content are handled in the same way

    resp1 = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=[
            {
                "role": "my-custom-role",
                "content": "what is 1+1?",
            }
        ],  # type: ignore
        temperature=0,
        seed=0,
    )

    resp2 = await client.chat.completions.create(
        model=MODEL_NAME,
        messages=[
            {
                "role": "my-custom-role",
                "content": [{"type": "text", "text": "what is 1+1?"}],
            }
        ],  # type: ignore
        temperature=0,
        seed=0,
    )

    content1 = resp1.choices[0].message.content
    content2 = resp2.choices[0].message.content
    print(content1, content2)
    assert content1 == content2


@pytest.mark.asyncio
async def test_long_seed(client: openai.AsyncOpenAI):
    for seed in [torch.iinfo(torch.long).min - 1, torch.iinfo(torch.long).max + 1]:
        with pytest.raises(UnprocessableEntityError) as exc_info:
            await client.chat.completions.create(
                model=MODEL_NAME,
                messages=[
                    {
                        "role": "system",
                        "content": "You are a helpful assistant.",
                    }
                ],
                temperature=0,
                seed=seed,
            )

        assert (
            "greater_than_equal" in exc_info.value.message
            or "less_than_equal" in exc_info.value.message
        )


# --------------------------- COMPLETIONS TESTS ---------------------------
@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name,num_virtual_tokens",
    [(MODEL_NAME, 0)],
)
async def test_single_completion(
    client: openai.AsyncOpenAI, model_name: str, num_virtual_tokens: int
):
    completion = await client.completions.create(
        model=model_name, prompt="Hello, my name is", max_tokens=5, temperature=0.0
    )

    assert completion.id is not None
    assert completion.choices is not None and len(completion.choices) == 1

    choice = completion.choices[0]
    assert len(choice.text) >= 5
    assert choice.finish_reason == "length"
    assert completion.usage == openai.types.CompletionUsage(
        completion_tokens=5,
        prompt_tokens=5 + num_virtual_tokens,
        total_tokens=10 + num_virtual_tokens,
    )

    # test using token IDs
    completion = await client.completions.create(
        model=model_name,
        prompt=[0, 0, 0, 0, 0],
        max_tokens=5,
        temperature=0.0,
    )
    assert len(completion.choices[0].text) >= 1
    assert completion.choices[0].prompt_logprobs is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_no_logprobs(client: openai.AsyncOpenAI, model_name: str):
    # test using token IDs
    completion = await client.completions.create(
        model=model_name,
        prompt=[0, 0, 0, 0, 0],
        max_tokens=5,
        temperature=0.0,
        logprobs=None,
    )
    choice = completion.choices[0]
    assert choice.logprobs is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_zero_logprobs(client: openai.AsyncOpenAI, model_name: str):
    # test using token IDs
    completion = await client.completions.create(
        model=model_name,
        prompt=[0, 0, 0, 0, 0],
        max_tokens=5,
        temperature=0.0,
        logprobs=0,
    )
    choice = completion.choices[0]
    assert choice.logprobs is not None
    assert choice.logprobs.token_logprobs is not None
    assert choice.logprobs.top_logprobs is not None
    assert len(choice.logprobs.top_logprobs[0]) == 1


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_some_logprobs(client: openai.AsyncOpenAI, model_name: str):
    # test using token IDs
    completion = await client.completions.create(
        model=model_name,
        prompt=[0, 0, 0, 0, 0],
        max_tokens=5,
        temperature=0.0,
        logprobs=5,
    )
    choice = completion.choices[0]
    assert choice.logprobs is not None
    assert choice.logprobs.token_logprobs is not None
    assert choice.logprobs.top_logprobs is not None
    assert 5 <= len(choice.logprobs.top_logprobs[0]) <= 6


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_too_many_completion_logprobs(
    client: openai.AsyncOpenAI, model_name: str
):

    with pytest.raises(
        (openai.BadRequestError, openai.APIError)
    ):  # test using token IDs
        stream = await client.completions.create(
            model=model_name,
            prompt=[0, 0, 0, 0, 0],
            max_tokens=5,
            temperature=0.0,
            # vLLM has higher default max_logprobs (20 instead of 5) to support
            # both Completion API and Chat Completion API
            logprobs=30,
            stream=True,
        )
        async for chunk in stream:
            ...

    # the server should still work afterwards
    completion = await client.completions.create(
        model=model_name,
        prompt=[0, 0, 0, 0, 0],
        max_tokens=5,
        temperature=0.0,
    )
    assert len(completion.choices[0].text) >= 0


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name, prompt_logprobs",
    [(MODEL_NAME, -1), (MODEL_NAME, 0), (MODEL_NAME, 1), (MODEL_NAME, None)],
)
async def test_prompt_logprobs_completion(
    client: openai.AsyncOpenAI, model_name: str, prompt_logprobs: Optional[int]
):
    params: Dict = {
        "prompt": ["A robot may not injure another robot", "My name is"],
        "model": model_name,
    }
    if prompt_logprobs is not None:
        params["extra_body"] = {"prompt_logprobs": prompt_logprobs}

    if prompt_logprobs is not None and prompt_logprobs < 0:
        with pytest.raises(UnprocessableEntityError):
            await client.completions.create(**params)
    else:
        completion = await client.completions.create(**params)
        if prompt_logprobs is not None:
            assert completion.choices[0].prompt_logprobs is not None
            assert len(completion.choices[0].prompt_logprobs) > 0

            assert completion.choices[1].prompt_logprobs is not None
            assert len(completion.choices[1].prompt_logprobs) > 0

        else:
            assert completion.choices[0].prompt_logprobs is None


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_completion_streaming(client: openai.AsyncOpenAI, model_name: str):
    prompt = "What is an LLM?"

    single_completion = await client.completions.create(
        model=model_name,
        prompt=prompt,
        max_tokens=5,
        temperature=0.0,
    )
    single_output = single_completion.choices[0].text
    stream = await client.completions.create(
        model=model_name, prompt=prompt, max_tokens=5, temperature=0.0, stream=True
    )
    chunks: List[str] = []
    finish_reason_count = 0
    async for chunk in stream:
        chunks.append(chunk.choices[0].text)
        if chunk.choices[0].finish_reason is not None:
            finish_reason_count += 1
    # finish reason should only return in last block
    assert finish_reason_count == 1
    assert chunk.choices[0].finish_reason == "length"
    assert chunk.choices[0].text
    assert "".join(chunks) == single_output


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_parallel_streaming(client: openai.AsyncOpenAI, model_name: str):
    """Streaming for parallel sampling.
    The tokens from multiple samples, are flattened into a single stream,
    with an index to indicate which sample the token belongs to.
    """

    prompt = "What is an LLM?"
    n = 3
    max_tokens = 5

    stream = await client.completions.create(
        model=model_name, prompt=prompt, max_tokens=max_tokens, n=n, stream=True
    )
    chunks: List[List[str]] = [[] for i in range(n)]
    finish_reason_count = 0
    async for chunk in stream:
        index = chunk.choices[0].index
        text = chunk.choices[0].text
        chunks[index].append(text)
        if chunk.choices[0].finish_reason is not None:
            finish_reason_count += 1
    assert finish_reason_count == n
    for chunk in chunks:
        assert len(chunk) == max_tokens
        print("".join(chunk))


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_completion_stream_options(client: openai.AsyncOpenAI, model_name: str):
    prompt = "What is the capital of France?"

    # Test stream=True, stream_options=
    #     {"include_usage": False, "continuous_usage_stats": False}
    stream = await client.completions.create(
        model=model_name,
        prompt=prompt,
        max_tokens=5,
        temperature=0.0,
        stream=True,
        stream_options={
            "include_usage": False,
            "continuous_usage_stats": False,
        },
    )

    async for chunk in stream:
        assert chunk.usage is None

    # Test stream=True, stream_options=
    #     {"include_usage": False, "continuous_usage_stats": True}
    stream = await client.completions.create(
        model=model_name,
        prompt=prompt,
        max_tokens=5,
        temperature=0.0,
        stream=True,
        stream_options={
            "include_usage": False,
            "continuous_usage_stats": True,
        },
    )
    async for chunk in stream:
        assert chunk.usage is None

    # Test stream=True, stream_options=
    #     {"include_usage": True, "continuous_usage_stats": False}
    stream = await client.completions.create(
        model=model_name,
        prompt=prompt,
        max_tokens=5,
        temperature=0.0,
        stream=True,
        stream_options={
            "include_usage": True,
            "continuous_usage_stats": False,
        },
    )
    async for chunk in stream:
        if chunk.choices[0].finish_reason is None:
            assert chunk.usage is None
        else:
            assert chunk.usage is None
            final_chunk = await stream.__anext__()
            assert final_chunk.usage is not None
            assert final_chunk.usage.prompt_tokens > 0
            assert final_chunk.usage.completion_tokens > 0
            assert final_chunk.usage.total_tokens == (
                final_chunk.usage.prompt_tokens + final_chunk.usage.completion_tokens
            )
            assert final_chunk.choices == []

    # Test stream=True, stream_options=
    #     {"include_usage": True, "continuous_usage_stats": True}
    stream = await client.completions.create(
        model=model_name,
        prompt=prompt,
        max_tokens=5,
        temperature=0.0,
        stream=True,
        stream_options={
            "include_usage": True,
            "continuous_usage_stats": True,
        },
    )
    async for chunk in stream:
        assert chunk.usage is not None
        assert chunk.usage.prompt_tokens > 0
        assert chunk.usage.completion_tokens > 0
        assert chunk.usage.total_tokens == (
            chunk.usage.prompt_tokens + chunk.usage.completion_tokens
        )
        if chunk.choices[0].finish_reason is not None:
            final_chunk = await stream.__anext__()
            assert final_chunk.usage is not None
            assert final_chunk.usage.prompt_tokens > 0
            assert final_chunk.usage.completion_tokens > 0
            assert final_chunk.usage.total_tokens == (
                final_chunk.usage.prompt_tokens + final_chunk.usage.completion_tokens
            )
            assert final_chunk.choices == []

    # Test stream=False, stream_options=
    #     {"include_usage": None}
    with pytest.raises(UnprocessableEntityError):
        await client.completions.create(
            model=model_name,
            prompt=prompt,
            max_tokens=5,
            temperature=0.0,
            stream=False,
            stream_options={"include_usage": None},
        )

    # Test stream=False, stream_options=
    #    {"include_usage": True}
    with pytest.raises(UnprocessableEntityError):
        await client.completions.create(
            model=model_name,
            prompt=prompt,
            max_tokens=5,
            temperature=0.0,
            stream=False,
            stream_options={"include_usage": True},
        )

    # Test stream=False, stream_options=
    #     {"continuous_usage_stats": None}
    with pytest.raises(UnprocessableEntityError):
        await client.completions.create(
            model=model_name,
            prompt=prompt,
            max_tokens=5,
            temperature=0.0,
            stream=False,
            stream_options={"continuous_usage_stats": None},
        )

    # Test stream=False, stream_options=
    #    {"continuous_usage_stats": True}
    with pytest.raises(UnprocessableEntityError):
        await client.completions.create(
            model=model_name,
            prompt=prompt,
            max_tokens=5,
            temperature=0.0,
            stream=False,
            stream_options={"continuous_usage_stats": True},
        )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
async def test_batch_completions(client: openai.AsyncOpenAI, model_name: str):
    # test both text and token IDs
    for prompts in (["Hello, my name is"] * 2, [[0, 0, 0, 0, 0]] * 2):
        # test simple list
        batch = await client.completions.create(
            model=model_name,
            prompt=prompts,
            max_tokens=5,
            temperature=0.0,
        )
        assert len(batch.choices) == 2
        assert batch.choices[0].text == batch.choices[1].text

        # test n = 2
        batch = await client.completions.create(
            model=model_name,
            prompt=prompts,
            n=2,
            max_tokens=5,
            temperature=0.0,
            extra_body=dict(
                # NOTE: this has to be true for n > 1 in vLLM, but
                # not necessary for official client.
                use_beam_search=True
            ),
        )
        assert len(batch.choices) == 4
        assert (
            batch.choices[0].text != batch.choices[1].text
        ), "beam search should be different"
        assert (
            batch.choices[0].text == batch.choices[2].text
        ), "two copies of the same prompt should be the same"
        assert (
            batch.choices[1].text == batch.choices[3].text
        ), "two copies of the same prompt should be the same"

        # test streaming
        batch = await client.completions.create(
            model=model_name,
            prompt=prompts,
            max_tokens=5,
            temperature=0.0,
            stream=True,
        )
        texts = [""] * 2
        async for chunk in batch:
            assert len(chunk.choices) == 1
            choice = chunk.choices[0]
            texts[choice.index] += choice.text
        assert texts[0] == texts[1]


@pytest.mark.asyncio
async def test_logits_bias(client: openai.AsyncOpenAI):
    prompt = "Hello, my name is"
    max_tokens = 5
    tokenizer = get_tokenizer(tokenizer_name=MODEL)

    # Test exclusive selection
    token_id = 1000
    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt=prompt,
        max_tokens=max_tokens,
        temperature=0.0,
        logit_bias={str(token_id): 100},
        seed=42,
    )
    assert len(completion.choices[0].text) >= 5
    response_tokens = tokenizer(completion.choices[0].text, add_special_tokens=False)[
        "input_ids"
    ]
    expected_tokens = tokenizer(
        tokenizer.decode([token_id] * 5), add_special_tokens=False
    )["input_ids"]
    assert all(
        [
            response == expected
            for response, expected in zip(response_tokens, expected_tokens)
        ]
    )

    # Test ban
    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt=prompt,
        max_tokens=max_tokens,
        temperature=0.0,
    )
    response_tokens = tokenizer(completion.choices[0].text, add_special_tokens=False)[
        "input_ids"
    ]
    first_response = completion.choices[0].text
    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt=prompt,
        max_tokens=max_tokens,
        temperature=0.0,
        logit_bias={str(token): -100 for token in response_tokens},
    )
    assert first_response != completion.choices[0].text


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", ["outlines", "lm-format-enforcer"])
async def test_guided_regex_completion(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_regex
):
    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt=f"Give an example IPv4 address with this regex: {sample_regex}",
        n=3,
        temperature=1.0,
        max_tokens=20,
        extra_body=dict(
            guided_regex=sample_regex, guided_decoding_backend=guided_decoding_backend
        ),
    )

    assert completion.id is not None
    assert len(completion.choices) == 3
    for i in range(3):
        assert re.fullmatch(sample_regex, completion.choices[i].text) is not None


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", ["outlines", "lm-format-enforcer"])
async def test_guided_choice_completion(
    client: openai.AsyncOpenAI, guided_decoding_backend: str, sample_guided_choice
):
    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt="The best language for type-safe systems programming is ",
        n=2,
        temperature=1.0,
        max_tokens=10,
        extra_body=dict(
            guided_choice=sample_guided_choice,
            guided_decoding_backend=guided_decoding_backend,
        ),
    )

    assert completion.id is not None
    assert len(completion.choices) == 2
    for i in range(2):
        assert completion.choices[i].text in sample_guided_choice


@pytest.mark.asyncio
async def test_guided_grammar(client: openai.AsyncOpenAI, sample_sql_statements):

    completion = await client.completions.create(
        model=MODEL_NAME,
        prompt=(
            "Generate a sql state that select col_1 from "
            "table_1 where it is equals to 1"
        ),
        temperature=1.0,
        max_tokens=500,
        extra_body=dict(guided_grammar=sample_sql_statements),
    )

    content = completion.choices[0].text

    # use Lark to parse the output, and make sure it's a valid parse tree
    from lark import Lark

    parser = Lark(sample_sql_statements)
    parser.parse(content)

    # remove spaces for comparison b/c we removed them in the grammar
    ground_truth = "SELECT col_1 from table_1 where col_1 = 1".replace(" ", "")

    assert content.strip() == ground_truth


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "model_name",
    [MODEL_NAME],
)
@pytest.mark.parametrize("logprobs_arg", [1, 0])
async def test_echo_logprob_completion(
    client: openai.AsyncOpenAI, model_name: str, logprobs_arg: int
):
    tokenizer = get_tokenizer(tokenizer_name=MODEL)
    # test using text and token IDs
    for prompt in ("Hello, my name is", [0, 0, 0, 0, 0]):
        completion = await client.completions.create(
            model=model_name,
            prompt=prompt,
            max_tokens=5,
            temperature=0.0,
            echo=True,
            logprobs=logprobs_arg,
        )

        prompt_text = tokenizer.decode(prompt) if isinstance(prompt, list) else prompt
        assert re.search(r"^" + prompt_text, completion.choices[0].text)
        logprobs = completion.choices[0].logprobs
        assert logprobs is not None
        assert len(logprobs.text_offset) > 5
        assert len(logprobs.token_logprobs) > 5 and logprobs.token_logprobs[0] is None
        assert len(logprobs.top_logprobs) > 5 and logprobs.top_logprobs[0] is None
        for top_logprobs in logprobs.top_logprobs[1:]:
            assert max(logprobs_arg, 1) <= len(top_logprobs) <= logprobs_arg + 1
        assert len(logprobs.tokens) > 5


@pytest.mark.asyncio
@pytest.mark.parametrize("guided_decoding_backend", ["outlines", "lm-format-enforcer"])
async def test_guided_decoding_type_error(
    client: openai.AsyncOpenAI,
    guided_decoding_backend: str,
    sample_json_schema,
    sample_regex,
):
    with pytest.raises(openai.UnprocessableEntityError):
        _ = await client.completions.create(
            model=MODEL_NAME,
            prompt="Give an example JSON that fits this schema: 42",
            extra_body=dict(
                guided_json=42, guided_decoding_backend=guided_decoding_backend
            ),
        )

    with pytest.raises(openai.UnprocessableEntityError):
        _ = await client.completions.create(
            model=MODEL_NAME,
            prompt="Give an example string that fits this regex",
            extra_body=dict(guided_regex=sample_regex, guided_json=sample_json_schema),
        )
