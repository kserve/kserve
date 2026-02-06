import pytest
from unittest.mock import AsyncMock, MagicMock
from http import HTTPStatus

from fastapi import Request, Response
from starlette.datastructures import Headers

from kserve.protocol.rest.openai.types import (
    CompletionRequest,
    ChatCompletionRequest,
    EmbeddingRequest,
    RerankRequest,
    ErrorResponse,
)

from kserve.protocol.rest.openai.dataplane import OpenAIDataPlane
from kserve.protocol.rest.openai.openai_model import (
    OpenAIModel,
    OpenAIGenerativeModel,
    OpenAIEncoderModel,
)


# ----------------------------------------------------------------------
# Fixtures
# ----------------------------------------------------------------------

@pytest.fixture
def model_registry():
    return MagicMock()


@pytest.fixture
def dataplane(model_registry):
    """
    DataPlane requires model_registry in __init__
    """
    dp = OpenAIDataPlane(model_registry=model_registry)
    dp.get_model = AsyncMock()
    return dp


@pytest.fixture
def headers():
    return Headers({"x-test": "true"})


@pytest.fixture
def response():
    return Response()


@pytest.fixture
def raw_request():
    return MagicMock(spec=Request)


# ----------------------------------------------------------------------
# create_completion
# ----------------------------------------------------------------------

@pytest.mark.asyncio
async def test_create_completion_invalid_model(
    dataplane, raw_request, headers, response
):
    dataplane.get_model.return_value = MagicMock(spec=OpenAIEncoderModel)

    result = await dataplane.create_completion(
        model_name="bad-model",
        request=CompletionRequest(prompt="hi"),
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    assert isinstance(result, ErrorResponse)
    assert result.error.message == (
        "Model bad-model does not support Completions API"
    )
    assert result.error.code == "400"

@pytest.mark.asyncio
async def test_create_completion_success(
    dataplane, raw_request, headers, response
):
    model = MagicMock(spec=OpenAIGenerativeModel)
    model.create_completion = AsyncMock(return_value="OK")
    dataplane.get_model.return_value = model

    req = CompletionRequest(prompt="hello")

    result = await dataplane.create_completion(
        model_name="good-model",
        request=req,
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    # Arrow-marked lines are executed here
    model.create_completion.assert_awaited_once()

    _, kwargs = model.create_completion.call_args
    assert kwargs["request"] == req
    assert kwargs["raw_request"] == raw_request
    assert kwargs["context"]["headers"] == dict(headers)
    assert kwargs["context"]["response"] == response

    assert result == "OK"

@pytest.mark.asyncio
async def test_create_chat_completion_success(
    dataplane, raw_request, headers, response
):
    model = MagicMock(spec=OpenAIGenerativeModel)
    model.create_chat_completion = AsyncMock(return_value="OK")
    dataplane.get_model.return_value = model

    request = {"messages": [{"role": "user", "content": "hello"}], "model": "test"}

    result = await dataplane.create_chat_completion(
        model_name="good-model",
        request=request,
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    # Arrow-marked lines are executed here
    model.create_chat_completion.assert_awaited_once()

    _, kwargs = model.create_chat_completion.call_args
    assert kwargs["request"] == request
    assert kwargs["raw_request"] == raw_request
    assert kwargs["context"]["headers"] == dict(headers)
    assert kwargs["context"]["response"] == response

    assert result == "OK"


# @pytest.mark.asyncio
# async def test_create_completion_success(
#     dataplane, raw_request, headers, response
# ):
#     model = MagicMock(spec=OpenAIGenerativeModel)
#     model.create_completion = AsyncMock(return_value="OK")
#     dataplane.get_model.return_value = model

#     req = CompletionRequest(prompt="hello")

#     result = await dataplane.create_completion(
#         model_name="good-model",
#         request=req,
#         raw_request=raw_request,
#         headers=headers,
#         response=response,
#     )

#     model.create_completion.assert_awaited_once()
#     _, kwargs = model.create_completion.call_args

#     assert kwargs["request"] == req
#     assert kwargs["raw_request"] == raw_request
#     assert kwargs["context"]["headers"] == dict(headers)
#     assert kwargs["context"]["response"] == response
#     assert result == "OK"


# ----------------------------------------------------------------------
# create_chat_completion
# ----------------------------------------------------------------------

@pytest.mark.asyncio
async def test_create_chat_completion_invalid_model(
    dataplane, raw_request, headers, response
):
    dataplane.get_model.return_value = MagicMock(spec=OpenAIEncoderModel)

    result = await dataplane.create_chat_completion(
        model_name="bad-model",
        request=ChatCompletionRequest(messages=[]),
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    assert isinstance(result, ErrorResponse)
    assert "does not support Chat Completion API" in result.error.message


# ----------------------------------------------------------------------
# create_embedding
# ----------------------------------------------------------------------

@pytest.mark.asyncio
async def test_create_embedding_invalid_model(
    dataplane, raw_request, headers, response
):
    dataplane.get_model.return_value = MagicMock(spec=OpenAIGenerativeModel)

    result = await dataplane.create_embedding(
        model_name="bad-model",
        request={"input": "hello", "model": "test"},
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    assert isinstance(result, ErrorResponse)
    assert "does not support Embeddings API" in result.error.message
    assert result.error.code == "400"


@pytest.mark.asyncio
async def test_create_embedding_success(
    dataplane, raw_request, headers, response
):
    model = MagicMock(spec=OpenAIEncoderModel)
    model.create_embedding = AsyncMock(return_value="OK")
    dataplane.get_model.return_value = model

    request = {"input": "hello", "model": "test"}

    result = await dataplane.create_embedding(
        model_name="good-model",
        request=request,
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    # Arrow-marked lines are executed here
    model.create_embedding.assert_awaited_once()

    _, kwargs = model.create_embedding.call_args
    assert kwargs["request"] == request
    assert kwargs["raw_request"] == raw_request
    assert kwargs["context"]["headers"] == dict(headers)
    assert kwargs["context"]["response"] == response

    assert result == "OK"



# ----------------------------------------------------------------------
# create_rerank
# ----------------------------------------------------------------------

@pytest.mark.asyncio
async def test_create_rerank_success(
    dataplane, raw_request, headers, response
):
    model = MagicMock(spec=OpenAIEncoderModel)
    model.create_rerank = AsyncMock(return_value="OK")
    dataplane.get_model.return_value = model

    request = {"query": "q", "documents": ["a", "b"], "model": "test"}

    result = await dataplane.create_rerank(
        model_name="good-model",
        request=request,
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    # Arrow-marked lines are now executed
    model.create_rerank.assert_awaited_once()

    _, kwargs = model.create_rerank.call_args
    assert kwargs["request"] == request
    assert kwargs["raw_request"] == raw_request
    assert kwargs["context"]["headers"] == dict(headers)
    assert kwargs["context"]["response"] == response

    assert result == "OK"

@pytest.mark.asyncio
async def test_create_rerank_invalid_model(
    dataplane, raw_request, headers, response
):
    # Model does NOT support Rerank
    dataplane.get_model.return_value = MagicMock()

    result = await dataplane.create_rerank(
        model_name="bad-model",
        request={"query": "q", "documents": ["a", "b"], "model": "test"},
        raw_request=raw_request,
        headers=headers,
        response=response,
    )

    # Arrow-marked line is executed here
    assert isinstance(result, ErrorResponse)
    assert result.error.message == "Model bad-model does not support Rerank API"
    assert result.error.code == "400"


# ----------------------------------------------------------------------
# models()
# ----------------------------------------------------------------------

@pytest.mark.asyncio
async def test_models_filters_openai_models(dataplane, model_registry):
    openai_model = MagicMock(spec=OpenAIModel)
    non_openai_model = MagicMock()

    model_registry.get_models.return_value = {
        "openai": openai_model,
        "other": non_openai_model,
    }

    result = await dataplane.models()

    assert result == [openai_model]
