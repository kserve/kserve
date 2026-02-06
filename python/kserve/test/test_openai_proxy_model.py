import pytest
from unittest.mock import Mock, AsyncMock, patch

import httpx
import orjson

from kserve.protocol.rest.openai.openai_proxy_model import OpenAIProxyModel
from kserve.protocol.rest.openai.types import (
    Completion,
    CompletionChunk,
    ChatCompletion,
    ChatCompletionChunk,
)


# ---------------------------------------------------------------------------
# 1. pass hooks (pre/post methods should be callable and no-op)
# ---------------------------------------------------------------------------

def test_pre_and_post_hooks_are_noop():
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=Mock(),
    )

    model.preprocess_completion_request(Mock(), None)
    model.postprocess_completion(Mock(), Mock(), None)
    model.postprocess_completion_chunk(Mock(), Mock(), None)
    model.preprocess_chat_completion_request(Mock(), None)
    model.postprocess_chat_completion(Mock(), Mock(), None)
    model.postprocess_chat_completion_chunk(Mock(), Mock(), None)


# ---------------------------------------------------------------------------
# 2. _handle_completion_chunk skip_upstream_validation branch
# ---------------------------------------------------------------------------

def test_handle_completion_chunk_skip_validation():
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=Mock(),
        skip_upstream_validation=True,
    )

    data = {"id": "abc", "choices": []}
    raw_chunk = f"data: {orjson.dumps(data).decode()}"

    with patch.object(
        CompletionChunk, "model_construct", return_value="chunk"
    ) as mc:
        result = model._handle_completion_chunk(raw_chunk, Mock(), None)

    assert result == "chunk"
    mc.assert_called_once()


# ---------------------------------------------------------------------------
# 3. _handle_chat_completion_chunk empty chunk early return
# ---------------------------------------------------------------------------

def test_handle_chat_completion_chunk_empty_returns_none():
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=Mock(),
    )

    assert model._handle_chat_completion_chunk("", Mock(), None) is None


# ---------------------------------------------------------------------------
# 4. _handle_chat_completion_chunk skip_upstream_validation branch
# ---------------------------------------------------------------------------

def test_handle_chat_completion_chunk_skip_validation():
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=Mock(),
        skip_upstream_validation=True,
    )

    data = {"id": "abc", "choices": []}
    raw_chunk = f"data: {orjson.dumps(data).decode()}"

    with patch.object(
        ChatCompletionChunk, "model_construct", return_value="chunk"
    ) as mc:
        result = model._handle_chat_completion_chunk(raw_chunk, Mock(), None)

    assert result == "chunk"
    mc.assert_called_once()


# ---------------------------------------------------------------------------
# 5. _build_request uses upstream_headers
# ---------------------------------------------------------------------------

def test_build_request_with_upstream_headers():
    client = httpx.AsyncClient()
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=client,
    )

    raw_request = {
        "upstream_headers": {
            "Authorization": "Bearer TOKEN"
        }
    }

    request = Mock()
    request.model_dump_json.return_value = "{}"

    req = model._build_request("http://test", request, raw_request)

    assert req.headers["Authorization"] == "Bearer TOKEN"


# ---------------------------------------------------------------------------
# 6. generate_completion skip_upstream_validation branch
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_generate_completion_skip_validation():
    response = Mock()
    response.json.return_value = {"id": "abc", "choices": []}
    response.raise_for_status = Mock()

    client = AsyncMock()
    client.send.return_value = response

    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=client,
        skip_upstream_validation=True,
    )

    with patch.object(
        Completion, "model_construct", return_value="completion"
    ) as mc:
        result = await model.generate_completion(Mock())

    assert result == "completion"
    mc.assert_called_once()



# ---------------------------------------------------------------------------
# 7. generate_chat_completion skip_upstream_validation branch
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_generate_chat_completion_skip_validation():
    response = Mock()
    response.json.return_value = {"id": "abc", "choices": []}
    response.raise_for_status = Mock()

    client = AsyncMock()
    client.send.return_value = response

    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=client,
        skip_upstream_validation=True,
    )

    with patch.object(
        ChatCompletion, "model_construct", return_value="chat"
    ) as mc:
        result = await model.generate_chat_completion(Mock())

    assert result == "chat"
    mc.assert_called_once()



# ---------------------------------------------------------------------------
# 8. healthy() fallback to super().healthy()
# ---------------------------------------------------------------------------

@pytest.mark.asyncio
async def test_healthy_fallback_to_super():
    model = OpenAIProxyModel(
        name="test",
        predictor_url="http://test",
        http_client=Mock(),
        health_endpoint=None,
    )

    with patch(
        "kserve.model.BaseKServeModel.healthy",
        new_callable=AsyncMock,
        return_value=True,
    ):
        coroutine = await model.healthy()
        result = await coroutine

    assert result is True



