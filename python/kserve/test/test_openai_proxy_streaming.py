# Copyright 2026 The KServe Authors.
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

import json
from pathlib import Path
from unittest.mock import patch

import httpx
import pytest
from fastapi import FastAPI
from pydantic import ValidationError

from kserve.model_repository import ModelRepository
from kserve.protocol.rest.openai.dataplane import OpenAIDataPlane
from kserve.protocol.rest.openai.endpoints import (
    OPENAI_ROUTE_PREFIX,
    register_openai_endpoints,
)
from kserve.protocol.rest.openai.openai_proxy_model import OpenAIProxyModel
from kserve.protocol.rest.openai.types import (
    CompletionRequest,
    ChatCompletionRequest,
    CompletionChunk,
    ChatCompletionChunk,
)

FIXTURES_PATH = Path(__file__).parent / "fixtures" / "openai"


class UpstreamStream(httpx.AsyncByteStream):
    """Keep the upstream body unread so tests exercise streaming and cleanup."""

    def __init__(self, body):
        self.body = body
        self.closed = False

    async def __aiter__(self):
        yield self.body

    async def aclose(self):
        self.closed = True


@pytest.mark.asyncio
@pytest.mark.parametrize("stream", [False, True], ids=["json", "sse"])
@pytest.mark.parametrize(
    "skip_upstream_validation", [False, True], ids=["validate", "skip-validation"]
)
@pytest.mark.parametrize(
    "path,payload,fixture_name",
    [
        ("/v1/completions", {"prompt": "Hello"}, "completion"),
        (
            "/v1/chat/completions",
            {"messages": [{"role": "user", "content": "Hello"}]},
            "chat_completion",
        ),
    ],
    ids=["completion", "chat-completion"],
)
async def test_proxy_streaming(
    path, payload, fixture_name, stream, skip_upstream_validation
):
    suffix = "_chunk_stream.txt" if stream else ".json"
    body = (FIXTURES_PATH / (fixture_name + suffix)).read_bytes()
    upstream_body = UpstreamStream(body)

    def upstream(request):
        if request.url.path == "/health":
            return httpx.Response(200)
        return httpx.Response(
            200,
            headers={
                "content-type": "text/event-stream" if stream else "application/json"
            },
            stream=upstream_body,
        )

    async with httpx.AsyncClient(transport=httpx.MockTransport(upstream)) as backend:
        model = OpenAIProxyModel(
            "test-model",
            "http://backend",
            http_client=backend,
            skip_upstream_validation=skip_upstream_validation,
        )
        registry = ModelRepository()
        registry.update(model)
        app = FastAPI()
        register_openai_endpoints(app, OpenAIDataPlane(registry))

        def postprocess(chunk, request, raw_request):
            chunk.model = "postprocessed-model"

        hook = (
            "postprocess_completion_chunk"
            if fixture_name == "completion"
            else "postprocess_chat_completion_chunk"
        )
        with patch.object(model, hook, side_effect=postprocess) as postprocess_hook:
            async with httpx.AsyncClient(
                transport=httpx.ASGITransport(app=app, raise_app_exceptions=False),
                base_url="http://kserve",
            ) as client:
                response = await client.post(
                    OPENAI_ROUTE_PREFIX + path,
                    json={"model": "test-model", "stream": stream, **payload},
                )

        assert response.status_code == 200, response.text
        if stream:
            assert response.headers["content-type"].startswith("text/event-stream")
            events = response.text.split("\n\n")
            assert events[-2:] == ["data: [DONE]", ""]
            assert all(event.startswith("data: ") for event in events[:-2])
            chunks = [json.loads(event.removeprefix("data: ")) for event in events[:-2]]
            expected = [
                json.loads(line.removeprefix("data: "))
                for line in body.decode().splitlines()
                if line.startswith("data: ") and line != "data: [DONE]"
            ]
            assert len(chunks) == len(expected) == postprocess_hook.call_count
            chunk_type = (
                CompletionChunk if fixture_name == "completion" else ChatCompletionChunk
            )
            for actual, original in zip(chunks, expected, strict=True):
                original["model"] = "postprocessed-model"
                assert chunk_type.model_validate(actual) == chunk_type.model_validate(
                    original
                )
        else:
            assert response.json()["choices"]
            postprocess_hook.assert_not_called()
        assert upstream_body.closed


@pytest.mark.asyncio
@pytest.mark.parametrize("chat", [False, True], ids=["completion", "chat-completion"])
@pytest.mark.parametrize("invalid", [False, True], ids=["early-close", "invalid-json"])
async def test_proxy_stream_closes_upstream(chat, invalid):
    fixture = "chat_completion" if chat else "completion"
    body = (
        b"data: invalid-json\n\n"
        if invalid
        else (FIXTURES_PATH / (fixture + "_chunk_stream.txt")).read_bytes()
    )
    upstream_body = UpstreamStream(body)

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda _: httpx.Response(200, stream=upstream_body)
        )
    ) as backend:
        model = OpenAIProxyModel("test-model", "http://backend", http_client=backend)
        if chat:
            stream = await model.create_chat_completion(
                ChatCompletionRequest(
                    model="test-model",
                    messages=[{"role": "user", "content": "Hello"}],
                    stream=True,
                )
            )
        else:
            stream = await model.create_completion(
                CompletionRequest(
                    model="test-model",
                    prompt="Hello",
                    stream=True,
                )
            )
        if invalid:
            with pytest.raises(ValidationError):
                await anext(stream)
        else:
            await anext(stream)
            assert not upstream_body.closed
            await stream.aclose()
        assert upstream_body.closed
