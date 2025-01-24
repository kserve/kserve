import pytest
import openai
import pytest_asyncio

from server import RemoteOpenAIServer


MODEL_NAME = "meta-llama/Llama-3.2-1B-Instruct"


@pytest.fixture(scope="module")
def server():  # noqa: F811
    args = [
        # use half precision for speed and memory savings in CI environment
        "--dtype",
        "bfloat16",
        # "--max-model-len",
        # "8192",
        "--trust-remote-code",
        "--enforce-eager",
    ]

    with RemoteOpenAIServer(MODEL_NAME, args) as remote_server:
        yield remote_server


@pytest_asyncio.fixture
async def client(server):
    async with server.get_async_client() as async_client:
        yield async_client


@pytest.mark.asyncio
@pytest.mark.parametrize(
    # first test base model, then test loras
    "model_name",
    ["test-model"],
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

    choice = chat_completion.choices[0]
    assert choice.logprobs is None
