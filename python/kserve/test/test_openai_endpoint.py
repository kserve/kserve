import pytest
from unittest.mock import AsyncMock, MagicMock
from typing import AsyncGenerator
from types import SimpleNamespace

from fastapi import FastAPI, Request, Response
from fastapi.responses import ORJSONResponse
from starlette.responses import StreamingResponse, Response as StarletteResponse
from starlette.datastructures import Headers

from kserve.protocol.rest.openai.types import (
    CompletionRequest,
    ChatCompletionRequest,
    EmbeddingRequest,
    RerankRequest,
    ErrorResponse,
)

from kserve.protocol.rest.openai.endpoints import (
    OpenAIEndpoints,
    register_openai_endpoints,
)
from kserve.errors import ModelNotReady

@pytest.fixture
def dataplane():
    dp = MagicMock()
    dp.model_ready = AsyncMock(return_value=True)
    dp.create_completion = AsyncMock()
    dp.create_chat_completion = AsyncMock()
    dp.create_embedding = AsyncMock()
    dp.create_rerank = AsyncMock()
    dp.models = AsyncMock(return_value=[])
    return dp


@pytest.fixture
def endpoints(dataplane):
    return OpenAIEndpoints(dataplane)


@pytest.fixture
def raw_request():
    req = MagicMock(spec=Request)
    req.headers = Headers({"x-test": "true"})
    return req


@pytest.fixture
def response():
    return Response()

async def fake_stream() -> AsyncGenerator[str, None]:
    yield "data"

@pytest.mark.asyncio
async def test_create_chat_completion_success(endpoints, dataplane, raw_request, response):
    dataplane.create_chat_completion.return_value = {"ok": True}

    req = ChatCompletionRequest(
        model="test-model",
        messages=[{"role": "user", "content": "hi"}],
    )

    result = await endpoints.create_chat_completion(req, raw_request, response)

    dataplane.create_chat_completion.assert_awaited_once()
    assert result == {"ok": True}

@pytest.mark.asyncio
async def test_create_chat_completion_error(endpoints, dataplane, raw_request, response):
    dataplane.create_chat_completion.return_value = ErrorResponse(
        error={
            "code": "400",
            "message": "bad request",
            "type": "BadRequestError",
            "param": None,
        }
    )

    req = ChatCompletionRequest(
        model="test-model",
        messages=[{"role": "user", "content": "hi"}],
    )

    result = await endpoints.create_chat_completion(req, raw_request, response)

    assert isinstance(result, ORJSONResponse)
    assert result.status_code == 400

@pytest.mark.asyncio
async def test_create_chat_completion_streaming(endpoints, dataplane, raw_request, response):
    dataplane.create_chat_completion.return_value = fake_stream()

    req = ChatCompletionRequest(
        model="test-model",
        messages=[{"role": "user", "content": "hi"}],
    )

    result = await endpoints.create_chat_completion(req, raw_request, response)

    assert isinstance(result, StreamingResponse)

@pytest.mark.asyncio
async def test_create_chat_completion_model_not_ready(endpoints, dataplane, raw_request, response):
    dataplane.model_ready.return_value = False

    req = ChatCompletionRequest(
        model="test-model",
        messages=[{"role": "user", "content": "hi"}],
    )

    with pytest.raises(ModelNotReady):
        await endpoints.create_chat_completion(req, raw_request, response)

@pytest.mark.asyncio
async def test_create_rerank_success(endpoints, dataplane, raw_request, response):
    dataplane.create_rerank.return_value = {"rerank": True}

    req = RerankRequest(
        model="test-model",
        query="q",
        documents=["a", "b"],
    )

    result = await endpoints.create_rerank(raw_request, req, response)

    assert result == {"rerank": True}

@pytest.mark.asyncio
async def test_models_endpoint(endpoints, dataplane):
    model = MagicMock()
    model.name = "test-model"
    dataplane.models.return_value = [model]

    result = await endpoints.models()

    assert result.object == "list"
    assert result.data[0].id == "test-model"

@pytest.mark.asyncio
async def test_health_success(endpoints, dataplane):
    dataplane.model_ready.return_value = True
    await endpoints.health("test-model")


@pytest.mark.asyncio
async def test_health_failure(endpoints, dataplane):
    dataplane.model_ready.return_value = False
    with pytest.raises(ModelNotReady):
        await endpoints.health("test-model")

@pytest.mark.asyncio
async def test_health_exception_from_dataplane(endpoints, dataplane):
    dataplane.model_ready.side_effect = RuntimeError("boom")

    with pytest.raises(ModelNotReady) as exc:
        await endpoints.health("test-model")

    assert isinstance(exc.value.__cause__, RuntimeError)

def test_register_openai_endpoints():
    app = FastAPI()
    dataplane = MagicMock()

    register_openai_endpoints(app, dataplane)

    routes = {route.path for route in app.router.routes}

    assert "/openai/v1/completions" in routes
    assert "/openai/v1/chat/completions" in routes
    assert "/openai/v1/embeddings" in routes
    assert "/openai/v1/rerank" in routes
    assert "/openai/v1/models" in routes
