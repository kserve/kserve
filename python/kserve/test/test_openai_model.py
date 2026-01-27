from typing import AsyncGenerator, Union

import pytest
from unittest.mock import Mock

from kserve.protocol.rest.openai.openai_model import AsyncMappingIterator, OpenAIGenerativeModel, OpenAIEncoderModel

class FailingAsyncIterator:
    def __aiter__(self):
        return self

    async def __anext__(self):
        raise RuntimeError("boom")


@pytest.mark.asyncio
async def test_async_mapping_iterator_calls_sync_close_on_exception():
    close_mock = Mock()

    iterator = AsyncMappingIterator(
        iterator=FailingAsyncIterator(),
        mapper=lambda x: x,
        close=close_mock,
    )

    with pytest.raises(RuntimeError, match="boom"):
        await iterator.__anext__()

    # ✅ verify the arrow-marked line was executed
    close_mock.assert_called_once()
