# Copyright 2024 The KServe Authors.
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
import torch
import json
from kserve.model import PredictorConfig
from kserve.protocol.rest.openai.types import (
    ChatCompletionRequest,
    CompletionRequest,
)
from kserve.protocol.rest.openai.errors import OpenAIError
from pytest_httpx import HTTPXMock
from transformers import AutoConfig
from pytest import approx

from huggingfaceserver.task import infer_task_from_model_architecture
from huggingfaceserver.encoder_model import HuggingfaceEncoderModel
from huggingfaceserver.generative_model import HuggingfaceGenerativeModel
from huggingfaceserver.task import MLTask
from test_output import bert_token_classification_return_prob_expected_output
import torch.nn.functional as F


@pytest.fixture(scope="module")
def bloom_model():
    model = HuggingfaceGenerativeModel(
        "bloom-560m",
        model_id_or_path="bigscience/bloom-560m",
        max_length=512,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def t5_model():
    model = HuggingfaceGenerativeModel(
        "t5-small",
        model_id_or_path="google-t5/t5-small",
        max_length=512,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def bert_base_model():
    model = HuggingfaceEncoderModel(
        "google-bert/bert-base-uncased",
        model_id_or_path="bert-base-uncased",
        do_lower_case=True,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def bert_base_yelp_polarity():
    model = HuggingfaceEncoderModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def bert_base_return_prob():
    model = HuggingfaceEncoderModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
        return_probabilities=True,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def bert_token_classification_return_prob():
    model = HuggingfaceEncoderModel(
        "bert-large-cased-finetuned-conll03-english",
        model_id_or_path="dbmdz/bert-large-cased-finetuned-conll03-english",
        do_lower_case=True,
        add_special_tokens=False,
        return_probabilities=True,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def bert_token_classification():
    model = HuggingfaceEncoderModel(
        "bert-large-cased-finetuned-conll03-english",
        model_id_or_path="dbmdz/bert-large-cased-finetuned-conll03-english",
        do_lower_case=True,
        add_special_tokens=False,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def openai_gpt_model():
    model = HuggingfaceGenerativeModel(
        "openai-gpt",
        model_id_or_path="openai-community/openai-gpt",
        task=MLTask.text_generation,
        max_length=512,
        dtype=torch.float32,
    )
    model.load()
    yield model
    model.stop()


@pytest.fixture(scope="module")
def text_embedding():
    model = HuggingfaceEncoderModel(
        "mxbai-embed-large-v1",
        model_id_or_path="mixedbread-ai/mxbai-embed-large-v1",
        task=MLTask.text_embedding,
    )
    model.load()
    yield model
    model.stop()


def test_unsupported_model():
    config = AutoConfig.from_pretrained("google/tapas-base-finetuned-wtq")
    with pytest.raises(ValueError) as err_info:
        infer_task_from_model_architecture(config)
    assert "Task table_question_answering is not supported" in err_info.value.args[0]


@pytest.mark.asyncio
async def test_t5(t5_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        stream=False,
    )
    response = await t5_model.create_completion(request)
    assert response.choices[0].text == "wir setzen Worte"
    assert response.usage.completion_tokens == 7


@pytest.mark.asyncio
async def test_t5_stopping_criteria(t5_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        stop=["setzen "],
        stream=False,
    )
    response = await t5_model.create_completion(request)
    assert response.choices[0].text == "wir setzen"


@pytest.mark.asyncio
async def test_t5_bad_params(t5_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        echo=True,
        stream=False,
    )
    with pytest.raises(OpenAIError) as err_info:
        await t5_model.create_completion(request)
    assert err_info.value.args[0] == "'echo' is not supported by encoder-decoder models"


@pytest.mark.asyncio
async def test_bert(bert_base_model: HuggingfaceEncoderModel):
    response, _ = await bert_base_model(
        {
            "instances": [
                "The capital of France is [MASK].",
                "The capital of [MASK] is paris.",
            ]
        },
        headers={},
    )
    assert response == {"predictions": ["paris", "france"]}


@pytest.mark.asyncio
async def test_model_revision(request: HuggingfaceEncoderModel):
    # https://huggingface.co/google-bert/bert-base-uncased
    commit = "86b5e0934494bd15c9632b12f734a8a67f723594"
    model = HuggingfaceEncoderModel(
        "google-bert/bert-base-uncased",
        model_id_or_path="bert-base-uncased",
        model_revision=commit,
        tokenizer_revision=commit,
        do_lower_case=True,
    )
    model.load()
    request.addfinalizer(model.stop)

    response, _ = await model(
        {
            "instances": [
                "The capital of France is [MASK].",
                "The capital of [MASK] is paris.",
            ]
        },
        headers={},
    )
    assert response == {"predictions": ["paris", "france"]}


@pytest.mark.asyncio
async def test_bert_predictor_host(request, httpx_mock: HTTPXMock):
    model_name = "bert"
    httpx_mock.add_response(
        json={
            "model_name": model_name,
            "outputs": [
                {
                    "name": "OUTPUT__0",
                    "shape": [1, 9, 758],
                    "data": [1] * 9 * 758,
                    "datatype": "INT64",
                }
            ],
        }
    )

    model = HuggingfaceEncoderModel(
        model_name,
        model_id_or_path="google-bert/bert-base-uncased",
        tensor_input_names="input_ids",
        predictor_config=PredictorConfig(
            predictor_host="localhost:8081", predictor_protocol="v2"
        ),
    )
    model.load()
    request.addfinalizer(model.stop)

    response, _ = await model(
        {"instances": ["The capital of France is [MASK]."]}, headers={}
    )
    assert response == {"predictions": ["[PAD]"]}


@pytest.mark.asyncio
async def test_bert_sequence_classification(bert_base_yelp_polarity):
    request = "Hello, my dog is cute."
    response, _ = await bert_base_yelp_polarity(
        {"instances": [request, request]}, headers={}
    )
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_bert_sequence_classification_return_probabilities(bert_base_return_prob):
    request = "Hello, my dog is cute."
    response, _ = await bert_base_return_prob(
        {"instances": [request, request]}, headers={}
    )

    assert response == {
        "predictions": [
            {0: approx(-3.1508713), 1: approx(3.5892851)},
            {0: approx(-3.1508713), 1: approx(3.589285)},
        ]
    }


@pytest.mark.asyncio
async def test_bert_token_classification_return_prob(
    bert_token_classification_return_prob,
):
    request = "Hello, my dog is cute."

    response, _ = await bert_token_classification_return_prob(
        {"instances": [request, request]}, headers={}
    )
    assert response == bert_token_classification_return_prob_expected_output


@pytest.mark.asyncio
async def test_bert_token_classification(bert_token_classification):
    request = "HuggingFace is a company based in Paris and New York"
    response, _ = await bert_token_classification(
        {"instances": [request, request]}, headers={}
    )
    assert response == {
        "predictions": [
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
        ]
    }


@pytest.mark.asyncio
async def test_bloom_completion(bloom_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=False,
        echo=True,
    )
    response = await bloom_model.create_completion(request)
    assert (
        response.choices[0].text
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute"
    )


@pytest.mark.asyncio
async def test_bloom_completion_max_tokens(bloom_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=False,
        echo=True,
        max_tokens=100,
        # bloom doesn't have any field specifying context length. Our implementation would default to 2048. Testing with something longer than HF's default max_length of 20
    )
    response = await bloom_model.create_completion(request)
    assert (
        response.choices[0].text
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey,"
    )


@pytest.mark.asyncio
async def test_bloom_completion_streaming(bloom_model: HuggingfaceGenerativeModel):
    request = CompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=True,
        echo=False,
    )
    response = await bloom_model.create_completion(request)
    output = ""
    async for chunk in response:
        chunk = chunk.removeprefix("data: ")
        chunk = chunk.removesuffix("\n\n")
        if chunk == "[DONE]":
            break
        chunk = json.loads(chunk)
        output += chunk["choices"][0]["text"]
    assert output == ".\n- Hey, my dog is cute.\n- Hey, my dog is cute"


@pytest.mark.asyncio
async def test_bloom_chat_completion(bloom_model: HuggingfaceGenerativeModel):
    messages = [
        {
            "role": "system",
            "content": "You are a friendly chatbot who always responds in the style of a pirate",
        },
        {
            "role": "user",
            "content": "How many helicopters can a human eat in one sitting?",
        },
    ]
    request = ChatCompletionRequest(
        model="bloom-560m",
        messages=messages,
        stream=False,
        max_tokens=20,
        chat_template="{% for message in messages %}"
        "{{ message.content }}{{ eos_token }}"
        "{% endfor %}",
    )
    response = await bloom_model.create_chat_completion(request)
    assert (
        response.choices[0].message.content
        == "The first thing you need to do is to get a good idea of what you are looking for."
    )
    assert response.usage.completion_tokens == 20


@pytest.mark.asyncio
async def test_bloom_chat_completion_streaming(bloom_model: HuggingfaceGenerativeModel):
    messages = [
        {
            "role": "system",
            "content": "You are a friendly chatbot who always responds in the style of a pirate",
        },
        {
            "role": "user",
            "content": "How many helicopters can a human eat in one sitting?",
        },
    ]
    request = ChatCompletionRequest(
        model="bloom-560m",
        messages=messages,
        stream=True,
        max_tokens=20,
        chat_template="{% for message in messages %}"
        "{{ message.content }}{{ eos_token }}"
        "{% endfor %}",
    )
    response = await bloom_model.create_chat_completion(request)
    output = ""
    async for chunk in response:
        chunk = chunk.removeprefix("data: ")
        chunk = chunk.removesuffix("\n\n")
        if chunk == "[DONE]":
            break
        chunk = json.loads(chunk)
        output += chunk["choices"][0]["delta"]["content"]
    assert (
        output
        == "The first thing you need to do is to get a good idea of what you are looking for."
    )


@pytest.mark.asyncio
async def test_text_embedding(text_embedding):
    def cosine_similarity(a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        if len(a.shape) == 1:
            a = a.unsqueeze(0)

        if len(b.shape) == 1:
            b = b.unsqueeze(0)

        a_norm = F.normalize(a, p=2, dim=1)
        b_norm = F.normalize(b, p=2, dim=1)
        return torch.mm(a_norm, b_norm.transpose(0, 1))

    requests = ["I'm happy", "I'm full of happiness", "They were at the park."]
    response, _ = await text_embedding({"instances": requests}, headers={})
    predictions = response["predictions"]

    # The first two requests are semantically similar, so the cosine similarity should be high
    assert (
        cosine_similarity(torch.tensor(predictions[0]), torch.tensor(predictions[1]))[0]
        > 0.9
    )
    # The third request is semantically different, so the cosine similarity should be low
    assert (
        cosine_similarity(torch.tensor(predictions[0]), torch.tensor(predictions[2]))[0]
        < 0.55
    )


@pytest.mark.asyncio
async def test_input_padding(bert_base_yelp_polarity: HuggingfaceEncoderModel):
    # inputs with different lengths will throw an error
    # unless we set padding=True in the tokenizer
    request_one = "Hello, my dog is cute."
    request_two = "Hello there, my dog is cute."
    response, _ = await bert_base_yelp_polarity(
        {"instances": [request_one, request_two]}, headers={}
    )
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_input_truncation(bert_base_yelp_polarity: HuggingfaceEncoderModel):
    # bert-base-uncased has a max length of 512 (tokenizer.model_max_length).
    # this request exceeds that, so it will throw an error
    # unless we set truncation=True in the tokenizer
    request = "good " * 600
    response, _ = await bert_base_yelp_polarity({"instances": [request]}, headers={})
    assert response == {"predictions": [1]}


@pytest.mark.asyncio
async def test_input_padding_with_pad_token_not_specified(
    openai_gpt_model: HuggingfaceGenerativeModel,
):
    # inputs with different lengths will throw an error
    # unless padding token is configured.
    # openai-gpt model does not specify the pad token, so the fallback pad token should be added.
    assert openai_gpt_model._tokenizer.pad_token == "[PAD]"
    assert openai_gpt_model._tokenizer.pad_token_id is not None
    request = CompletionRequest(
        model="openai-gpt",
        prompt=["Sun rises in the east, sets in the", "My name is Teven and I am"],
        stream=False,
        temperature=0,
    )
    response = await openai_gpt_model.create_completion(request)
    assert (
        response.choices[0].text
        == "west , and the sun sets in the west . \n the sun rises in the"
    )
    assert "a member of the royal family ." in response.choices[1].text


@pytest.mark.asyncio
async def test_tools_chat_completion(bloom_model: HuggingfaceGenerativeModel):
    messages = [
        {
            "role": "system",
            "content": "You are a friendly chatbot whose purpose is to tell me what the weather is.",
        },
        {
            "role": "user",
            "content": "weather in Ithaca, NY",
        },
    ]

    tools = [
        {
            "type": "function",
            "function": {
                "name": "get_current_weather",
                "description": "Get the current weather",
                "parameters": {
                    "type": "dict",
                    "properties": {
                        "location": {
                            "type": "string",
                            "description": "The city and state, e.g. San Francisco, CA",
                        },
                        "format": {
                            "type": "string",
                            "enum": ["celsius", "fahrenheit"],
                            "description": "The temperature unit to use. Infer this from the users location.",
                        },
                    },
                    "required": ["location", "format"],
                },
            },
        }
    ]
    request = ChatCompletionRequest(
        model="bloom-560m",
        messages=messages,
        stream=False,
        max_tokens=100,
        tools=tools,
        tool_choice="auto",
        chat_template="{% for message in messages %}"
        "{{ message.content }} You have these tools: {% for tool in tools %} {{ eos_token }}"
        "{% endfor %}{% endfor %}",
    )
    response = await bloom_model.create_chat_completion(request)

    assert response.choices[0].message.content


@pytest.mark.asyncio
async def test_trust_remote_code_encoder_model():
    model = HuggingfaceEncoderModel(
        "nomic-embed-text",
        model_id_or_path="nomic-ai/nomic-embed-text-v1.5",
        max_length=1024,
        dtype=torch.float32,
        trust_remote_code=True,
        task=MLTask.text_embedding,
    )
    model.load()
    model.stop()
