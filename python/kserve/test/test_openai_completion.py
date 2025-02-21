# Copyright 2023 The KServe Authors.
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

from contextlib import asynccontextmanager
from pathlib import Path
from typing import (
    AsyncGenerator,
    Callable,
    List,
    Optional,
    Tuple,
    Union,
    cast,
)
from unittest.mock import MagicMock, patch

import httpx
import pytest

from fastapi import Request

from kserve.protocol.rest.openai import (
    ChatPrompt,
    OpenAIChatAdapterModel,
    OpenAIProxyModel,
)

from kserve.protocol.rest.openai.types import (
    Completion,
    CompletionRequest,
    ChatCompletion,
    ChatCompletionChunk,
    ChatCompletionRequest,
    Error,
    ErrorResponse,
)
from kserve.protocol.rest.openai.errors import OpenAIError

FIXTURES_PATH = Path(__file__).parent / "fixtures" / "openai"


class ChunkIterator:
    """Yields chunks"""

    def __init__(self, chunks: List[Completion]):
        self.chunks = chunks
        self.curr_chunk = 0

    def __aiter__(self):
        return self

    async def __anext__(self):
        if self.curr_chunk >= len(self.chunks):
            raise StopAsyncIteration
        chunk = self.chunks[self.curr_chunk]
        self.curr_chunk += 1
        return chunk


class DummyModel(OpenAIChatAdapterModel):
    data: Tuple[Completion, Completion]
    num_chunks: int

    def __init__(self, data: Tuple[Completion, Completion], num_chunks: int = 5):
        self.data = data
        self.num_chunks = num_chunks

    async def create_completion(
        self,
        request: CompletionRequest,
        raw_request: Optional[Request] = None,
    ) -> Union[AsyncGenerator[str, None], Completion, ErrorResponse]:
        if request.stream:

            async def stream_results() -> AsyncGenerator[str, None]:
                async for partial_completion in ChunkIterator(
                    [self.data[1]] * self.num_chunks
                ):
                    yield f"data: {partial_completion.model_dump_json()}\n\n"
                yield "data: [DONE]\n\n"

            return stream_results()
        else:
            return self.data[0]

    def apply_chat_template(
        self,
        request: ChatCompletionRequest,
    ) -> ChatPrompt:
        return ChatPrompt(prompt="hello")


@pytest.fixture
def completion():
    with open(FIXTURES_PATH / "completion.json") as f:
        return Completion.model_validate_json(f.read())


@pytest.fixture
def completion_partial():
    with open(FIXTURES_PATH / "completion_partial.json") as f:
        return Completion.model_validate_json(f.read())


@pytest.fixture
def completion_chunk_stream():
    with open(FIXTURES_PATH / "completion_chunk_stream.txt", "rb") as f:
        return f.read()


@pytest.fixture
def chat_completion():
    with open(FIXTURES_PATH / "chat_completion.json") as f:
        return ChatCompletion.model_validate_json(f.read())


@pytest.fixture
def chat_completion_chunk():
    with open(FIXTURES_PATH / "chat_completion_chunk.json") as f:
        return ChatCompletionChunk.model_validate_json(f.read())


@pytest.fixture
def chat_completion_chunk_stream():
    with open(FIXTURES_PATH / "chat_completion_chunk_stream.txt", "rb") as f:
        return f.read()


@pytest.fixture
def completion_create_params():
    with open(FIXTURES_PATH / "completion_create_params.json") as f:
        return CompletionRequest.model_validate_json(f.read())


@pytest.fixture
def chat_completion_create_params():
    with open(FIXTURES_PATH / "chat_completion_create_params.json") as f:
        return ChatCompletionRequest.model_validate_json(f.read())


@pytest.fixture
def completion_request(completion_create_params: CompletionRequest):
    return completion_create_params


@pytest.fixture
def chat_completion_request(chat_completion_create_params: ChatCompletionRequest):
    return chat_completion_create_params


@pytest.fixture
def dummy_model(completion: Completion, completion_partial: Completion):
    return DummyModel((completion, completion_partial))


@asynccontextmanager
async def mocked_openai_proxy_model(handler: Callable):
    transport = httpx.MockTransport(handler=handler)
    http_client = httpx.AsyncClient(transport=transport)
    try:
        with patch.object(
            OpenAIProxyModel, "preprocess_completion_request"
        ), patch.object(OpenAIProxyModel, "postprocess_completion"), patch.object(
            OpenAIProxyModel, "postprocess_completion_chunk"
        ), patch.object(
            OpenAIProxyModel, "preprocess_chat_completion_request"
        ), patch.object(
            OpenAIProxyModel, "postprocess_chat_completion"
        ), patch.object(
            OpenAIProxyModel, "postprocess_chat_completion_chunk"
        ):
            yield OpenAIProxyModel(
                name="test-model",
                predictor_url="http://example.com/",
                http_client=http_client,
            )
    finally:
        await http_client.aclose()


class TestOpenAICreateCompletion:
    def test_completion_to_chat_completion(
        self, completion: Completion, chat_completion: ChatCompletion
    ):
        converted_chat_completion = (
            OpenAIChatAdapterModel.completion_to_chat_completion(
                completion, "assistant"
            )
        )
        assert (
            converted_chat_completion.model_dump_json()
            == chat_completion.model_dump_json()
        )

    @pytest.mark.asyncio
    async def test_create_completion_not_streaming(
        self,
        dummy_model: DummyModel,
        completion: Completion,
        completion_create_params: CompletionRequest,
    ):
        c = await dummy_model.create_completion(completion_create_params)
        assert isinstance(c, Completion)
        assert c.model_dump_json(indent=2) == completion.model_dump_json(indent=2)

    @pytest.mark.asyncio
    async def test_create_completion_streaming(
        self,
        dummy_model: DummyModel,
        completion_partial: Completion,
        completion_create_params: CompletionRequest,
    ):
        completion_create_params.stream = True
        c = await dummy_model.create_completion(completion_create_params)
        assert isinstance(c, AsyncGenerator)
        num_chunks_consumed = 0
        async for chunk in c:
            chunk = chunk.removeprefix("data: ")
            if chunk == "[DONE]\n\n":
                return
            assert (
                chunk == completion_partial.model_dump_json() + "\n\n"
            )  # not using indent but using two new lines
            num_chunks_consumed += 1
        assert num_chunks_consumed == dummy_model.num_chunks


class TestOpenAICreateChatCompletion:
    def test_completion_to_chat_completion(
        self, completion: Completion, chat_completion: ChatCompletion
    ):
        converted_chat_completion = (
            OpenAIChatAdapterModel.completion_to_chat_completion(
                completion, "assistant"
            )
        )
        assert (
            converted_chat_completion.model_dump_json()
            == chat_completion.model_dump_json()
        )

    @pytest.mark.asyncio
    async def test_create_chat_completion_not_streaming(
        self,
        dummy_model: DummyModel,
        chat_completion: ChatCompletion,
        chat_completion_create_params: ChatCompletionRequest,
    ):
        c = await dummy_model.create_chat_completion(chat_completion_create_params)
        assert isinstance(c, ChatCompletion)
        assert c.model_dump_json(indent=2) == chat_completion.model_dump_json(indent=2)

    @pytest.mark.asyncio
    async def test_create_chat_completion_streaming(
        self,
        dummy_model: DummyModel,
        chat_completion_chunk: ChatCompletionChunk,
        chat_completion_create_params: ChatCompletionRequest,
    ):
        chat_completion_create_params.stream = True
        c = await dummy_model.create_chat_completion(chat_completion_create_params)
        assert isinstance(c, AsyncGenerator)
        num_chunks_consumed = 0
        async for chunk in c:
            chunk = chunk.removeprefix("data: ")
            if chunk == "[DONE]\n\n":
                return
            assert (
                chunk == chat_completion_chunk.model_dump_json() + "\n\n"
            )  # not using indent but using two new lines
            num_chunks_consumed += 1
        assert num_chunks_consumed == dummy_model.num_chunks


class TestOpenAICompletionConversion:
    def test_completion_to_chat_completion(
        self, completion: Completion, chat_completion: ChatCompletion
    ):
        converted_chat_completion = (
            OpenAIChatAdapterModel.completion_to_chat_completion(
                completion, "assistant"
            )
        )
        assert (
            converted_chat_completion.model_dump_json()
            == chat_completion.model_dump_json()
        )

    def test_completion_to_chat_completion_chunk(
        self, completion_partial: Completion, chat_completion_chunk: ChatCompletionChunk
    ):
        converted_chat_completion_chunk = (
            OpenAIChatAdapterModel.completion_to_chat_completion_chunk(
                completion_partial, "assistant"
            )
        )
        assert converted_chat_completion_chunk.model_dump_json(
            indent=2
        ) == chat_completion_chunk.model_dump_json(indent=2)


class TestOpenAIParamsConversion:
    def test_convert_params(
        self,
        chat_completion_create_params: ChatCompletionRequest,
        completion_create_params: CompletionRequest,
    ):
        converted_params = (
            OpenAIChatAdapterModel.chat_completion_params_to_completion_params(
                chat_completion_create_params,
                prompt=chat_completion_create_params.messages[0]["content"],
            )
        )
        assert converted_params == completion_create_params


class TestOpenAIProxyModelCompletion:

    @pytest.mark.asyncio
    async def test_completion_upstream_connection_error(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        def handler(request):
            raise httpx.ConnectError(
                "[Errno 8] nodename nor servname provided, or not known"
            )

        async with mocked_openai_proxy_model(handler) as model:
            with pytest.raises(OpenAIError) as err_info:
                result = await model.create_completion(completion_request)
                assert result == completion
        assert (
            "Failed to communicate with upstream: [Errno 8] nodename nor servname provided"
            in str(err_info.value)
        )

    @pytest.mark.asyncio
    async def test_completion_upstream_status_code_error_invalid_body(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        def handler(request):
            return httpx.Response(status_code=400, content="Junk response")

        async with mocked_openai_proxy_model(handler) as model:
            with pytest.raises(OpenAIError) as err_info:
                result = await model.create_completion(completion_request)
                assert result == completion
        assert "Received invalid response from upstream: Junk response" in str(
            err_info.value
        )

    @pytest.mark.asyncio
    async def test_completion_upstream_status_code_error_valid_body(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        res = ErrorResponse(
            error=Error(
                code="400", message="Bad request", type="BadRequest", param=None
            )
        )

        def handler(request):
            return httpx.Response(status_code=400, content=res.model_dump_json())

        async with mocked_openai_proxy_model(handler) as model:
            with pytest.raises(OpenAIError) as err_info:
                result = await model.create_completion(completion_request)
                assert result == completion
        assert err_info.value.response == res

    @pytest.mark.asyncio
    async def test_completion_upstream_timeout(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        def handler(request):
            raise httpx.ReadTimeout("Read timed out", request=request)

        async with mocked_openai_proxy_model(handler) as model:
            with pytest.raises(OpenAIError) as err_info:
                result = await model.create_completion(completion_request)
                assert result == completion
        assert (
            str(err_info.value)
            == "Timed out when communicating with upstream: Read timed out"
        )

    @pytest.mark.asyncio
    async def test_completion_upstream_request_error(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        def handler(request):
            raise httpx.RequestError("Some error", request=request)

        async with mocked_openai_proxy_model(handler) as model:
            with pytest.raises(OpenAIError) as err_info:
                result = await model.create_completion(completion_request)
                assert result == completion
        assert str(err_info.value) == "Upstream request failed: Some error"

    @pytest.mark.asyncio
    async def test_completion_non_streamed(
        self,
        completion_request: CompletionRequest,
        completion: Completion,
    ):
        def handler(request):
            return httpx.Response(
                200,
                headers={"Content-type": "application/json"},
                content=completion.model_dump_json(),
            )

        async with mocked_openai_proxy_model(handler) as model:
            result = await model.create_completion(completion_request)
            assert result == completion
            cast(
                MagicMock, OpenAIProxyModel.preprocess_completion_request
            ).assert_called_once_with(completion_request, None)
            cast(
                MagicMock, OpenAIProxyModel.postprocess_completion
            ).assert_called_once_with(completion, completion_request, None)
            cast(
                MagicMock, OpenAIProxyModel.preprocess_chat_completion_request
            ).assert_not_called()
            cast(
                MagicMock, OpenAIProxyModel.postprocess_chat_completion
            ).assert_not_called()

    @pytest.mark.asyncio
    async def test_completion_streamed(
        self,
        completion_request: CompletionRequest,
        completion_chunk_stream: bytes,
    ):
        completion_request.stream = True

        def handler(request):
            return httpx.Response(
                200,
                headers={"Content-type": "text/event-stream"},
                content=completion_chunk_stream,
            )

        async with mocked_openai_proxy_model(handler) as model:
            async for completion_chunk in await model.create_completion(
                completion_request
            ):
                cast(
                    MagicMock, OpenAIProxyModel.postprocess_completion_chunk
                ).assert_called_with(completion_chunk, completion_request, None)
            cast(
                MagicMock, OpenAIProxyModel.preprocess_completion_request
            ).assert_called_once_with(completion_request, None)
            cast(MagicMock, OpenAIProxyModel.postprocess_completion).assert_not_called()
            cast(
                MagicMock, OpenAIProxyModel.preprocess_chat_completion_request
            ).assert_not_called()
            cast(
                MagicMock, OpenAIProxyModel.postprocess_chat_completion
            ).assert_not_called()

    @pytest.mark.asyncio
    async def test_chat_completion_non_streamed(
        self,
        chat_completion_request: ChatCompletionRequest,
        chat_completion: ChatCompletion,
    ):
        def handler(request):
            return httpx.Response(
                200,
                headers={"Content-type": "application/json"},
                content=chat_completion.model_dump_json(),
            )

        async with mocked_openai_proxy_model(handler) as model:
            result = await model.create_chat_completion(chat_completion_request)
            assert result == chat_completion
            cast(
                MagicMock, OpenAIProxyModel.preprocess_chat_completion_request
            ).assert_called_once_with(chat_completion_request, None)
            cast(
                MagicMock, OpenAIProxyModel.postprocess_chat_completion
            ).assert_called_once_with(chat_completion, chat_completion_request, None)
            cast(
                MagicMock, OpenAIProxyModel.preprocess_completion_request
            ).assert_not_called()
            cast(MagicMock, OpenAIProxyModel.postprocess_completion).assert_not_called()

    @pytest.mark.asyncio
    async def test_chat_completion_streamed(
        self,
        chat_completion_request: ChatCompletionRequest,
        chat_completion_chunk_stream: bytes,
    ):
        chat_completion_request.stream = True

        def handler(request):
            return httpx.Response(
                200,
                headers={"Content-type": "text/event-stream"},
                content=chat_completion_chunk_stream,
            )

        async with mocked_openai_proxy_model(handler) as model:
            async for chat_completion_chunk in await model.create_chat_completion(
                chat_completion_request
            ):
                cast(
                    MagicMock, OpenAIProxyModel.postprocess_chat_completion_chunk
                ).assert_called_with(
                    chat_completion_chunk, chat_completion_request, None
                )
            cast(
                MagicMock, OpenAIProxyModel.preprocess_chat_completion_request
            ).assert_called_once_with(chat_completion_request, None)
            cast(MagicMock, OpenAIProxyModel.postprocess_completion).assert_not_called()
            cast(
                MagicMock, OpenAIProxyModel.preprocess_completion_request
            ).assert_not_called()
            cast(
                MagicMock, OpenAIProxyModel.postprocess_chat_completion
            ).assert_not_called()

    @pytest.mark.asyncio
    async def test_healthcheck_success(self):
        def handler(request):
            if request.url.path == "/health":
                return httpx.Response(
                    200,
                )
            return httpx.Response(
                503,
            )

        async with mocked_openai_proxy_model(handler) as model:
            result = await model.healthy()
            assert result is True

    @pytest.mark.asyncio
    async def test_healthcheck_failure(self):
        def handler(request):
            if request.url.path == "/health":
                return httpx.Response(
                    503,
                )
            return httpx.Response(
                200,
            )

        async with mocked_openai_proxy_model(handler) as model:
            result = await model.healthy()
            assert result is False
