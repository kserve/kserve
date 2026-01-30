import pytest
from kserve.errors import InvalidInput
from kserve.protocol.rest.openai.types import ChatCompletionRequest
from kserve.protocol.rest.openai.openai_chat_adapter_model import OpenAIChatAdapterModel, ChatPrompt



# Minimal concrete implementation
class DummyChatAdapter(OpenAIChatAdapterModel):
    def apply_chat_template(self, request: ChatCompletionRequest) -> ChatPrompt:
        pytest.fail("apply_chat_template should not be called")

    async def create_completion(self, *args, **kwargs):
        pytest.fail("create_completion should not be called")


@pytest.mark.asyncio
async def test_create_chat_completion_invalid_n():
    model = DummyChatAdapter(name="test-model")

    request = ChatCompletionRequest(
        model="test-model",
        messages=[],
        n=2,              # 👈 triggers the error
        stream=False,
    )

    with pytest.raises(InvalidInput) as exc:
        await model.create_chat_completion(request)

    assert str(exc.value) == "n != 1 is not supported"
