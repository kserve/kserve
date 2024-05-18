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

from kserve.model import PredictorConfig
from kserve.protocol.rest.openai import ChatCompletionRequest, CompletionRequest
from kserve.protocol.rest.openai.types import (
    CreateChatCompletionRequest,
    CreateCompletionRequest,
)
from pytest_httpx import HTTPXMock
from transformers import AutoConfig

from .task import infer_task_from_model_architecture
from .encoder_model import HuggingfaceEncoderModel
from .generative_model import HuggingfaceGenerativeModel
from .task import MLTask
import torch


@pytest.fixture(scope="module")
def bloom_model():
    model = HuggingfaceGenerativeModel(
        "bloom-560m",
        model_id_or_path="bigscience/bloom-560m",
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


def test_unsupported_model():
    config = AutoConfig.from_pretrained("google/tapas-base-finetuned-wtq")
    with pytest.raises(ValueError) as err_info:
        infer_task_from_model_architecture(config)
    assert "Task table_question_answering is not supported" in err_info.value.args[0]


@pytest.mark.asyncio
async def test_t5(t5_model: HuggingfaceGenerativeModel):
    params = CreateCompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        stream=False,
    )
    request = CompletionRequest(params=params)
    response = await t5_model.create_completion(request)
    assert response.choices[0].text == "wir setzen Worte"


@pytest.mark.asyncio
async def test_t5_stopping_criteria(t5_model: HuggingfaceGenerativeModel):
    params = CreateCompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        stop=["setzen "],
        stream=False,
    )
    request = CompletionRequest(params=params)
    response = await t5_model.create_completion(request)
    assert response.choices[0].text == "wir setzen"


@pytest.mark.asyncio
async def test_t5_bad_params(t5_model: HuggingfaceGenerativeModel):
    params = CreateCompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        echo=True,
        stream=False,
    )
    request = CompletionRequest(params=params)
    with pytest.raises(ValueError) as err_info:
        await t5_model.create_completion(request)
    assert err_info.value.args[0] == "'echo' is not supported by encoder-decoder models"


@pytest.mark.asyncio
async def test_bert(bert_base_model: HuggingfaceEncoderModel):
    response = await bert_base_model(
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

    response = await model(
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
    httpx_mock.add_response(
        json={
            "outputs": [
                {
                    "name": "OUTPUT__0",
                    "shape": [1, 9, 758],
                    "data": [1] * 9 * 758,
                    "datatype": "INT64",
                }
            ]
        }
    )

    model = HuggingfaceEncoderModel(
        "bert",
        model_id_or_path="google-bert/bert-base-uncased",
        tensor_input_names="input_ids",
        predictor_config=PredictorConfig(
            predictor_host="localhost:8081", predictor_protocol="v2"
        ),
    )
    model.load()
    request.addfinalizer(model.stop)

    response = await model(
        {"instances": ["The capital of France is [MASK]."]}, headers={}
    )
    assert response == {"predictions": ["[PAD]"]}


@pytest.mark.asyncio
async def test_bert_sequence_classification(bert_base_yelp_polarity):
    request = "Hello, my dog is cute."
    response = await bert_base_yelp_polarity(
        {"instances": [request, request]}, headers={}
    )
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_bert_token_classification(bert_token_classification):
    request = "HuggingFace is a company based in Paris and New York"
    response = await bert_token_classification(
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
    params = CreateCompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=False,
        echo=True,
    )
    request = CompletionRequest(params=params)
    response = await bloom_model.create_completion(request)
    assert (
        response.choices[0].text
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute"
    )


@pytest.mark.asyncio
async def test_bloom_completion_streaming(bloom_model: HuggingfaceGenerativeModel):
    params = CreateCompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=True,
        echo=False,
    )
    request = CompletionRequest(params=params)
    response = await bloom_model.create_completion(request)
    output = ""
    async for chunk in response:
        output += chunk.choices[0].text
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
    params = CreateChatCompletionRequest(
        model="bloom-560m",
        messages=messages,
        stream=False,
    )
    request = ChatCompletionRequest(params=params)
    response = await bloom_model.create_chat_completion(request)
    assert (
        response.choices[0].message.content
        == "The first thing you need to do is to get a good idea of what you are looking for"
    )


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
    params = CreateChatCompletionRequest(
        model="bloom-560m",
        messages=messages,
        stream=True,
    )
    request = ChatCompletionRequest(params=params)
    response = await bloom_model.create_chat_completion(request)
    output = ""
    async for chunk in response:
        output += chunk.choices[0].delta.content
    assert (
        output
        == "The first thing you need to do is to get a good idea of what you are looking for"
    )


@pytest.mark.asyncio
async def test_input_padding(bert_base_yelp_polarity: HuggingfaceEncoderModel):
    # inputs with different lengths will throw an error
    # unless we set padding=True in the tokenizer
    request_one = "Hello, my dog is cute."
    request_two = "Hello there, my dog is cute."
    response = await bert_base_yelp_polarity(
        {"instances": [request_one, request_two]}, headers={}
    )
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_input_truncation(bert_base_yelp_polarity: HuggingfaceEncoderModel):
    # bert-base-uncased has a max length of 512 (tokenizer.model_max_length).
    # this request exceeds that, so it will throw an error
    # unless we set truncation=True in the tokenizer
    request = "good " * 600
    response = await bert_base_yelp_polarity({"instances": [request]}, headers={})
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
    params = CreateCompletionRequest(
        model="openai-gpt",
        prompt=["Sun rises in the east, sets in the", "My name is Teven and I am"],
        stream=False,
        temperature=0,
    )
    request = CompletionRequest(params=params)
    response = await openai_gpt_model.create_completion(request)
    assert (
        response.choices[0].text
        == "west, and the sun sets in the west. \n the sun rises in the"
    )
    assert "a member of the royal family." in response.choices[1].text
