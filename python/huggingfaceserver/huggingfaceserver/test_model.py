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

import asyncio
import unittest
import pytest

from kserve.model import PredictorConfig
from kserve.protocol.rest.openai import CompletionRequest
from kserve.protocol.rest.openai.types import CreateCompletionRequest
from pytest_httpx import HTTPXMock

from .encoder_model import HuggingfaceEncoderModel
from .generative_model import HuggingfaceGenerativeModel
from .task import MLTask


@pytest.mark.asyncio
async def test_t5(request):
    model = HuggingfaceGenerativeModel(
        "t5-small",
        model_id_or_path="google-t5/t5-small",
        max_length=512,
    )
    model.load()
    request.addfinalizer(model.stop)

    params = CreateCompletionRequest(
        model="t5-small",
        prompt="translate from English to German: we are making words",
        stream=False,
    )
    request = CompletionRequest(params=params)
    response = await model.create_completion(request)
    assert response.choices[0].text == "wir setzen Worte"


@pytest.mark.asyncio
async def test_bert(request):
    model = HuggingfaceEncoderModel(
        "google-bert/bert-base-uncased",
        model_id_or_path="bert-base-uncased",
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
async def test_model_revision(request):
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
async def test_bert_sequence_classification(request):
    model = HuggingfaceEncoderModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.stop)

    request = "Hello, my dog is cute."
    response = await model({"instances": [request, request]}, headers={})
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_bert_token_classification(request):
    model = HuggingfaceEncoderModel(
        "bert-large-cased-finetuned-conll03-english",
        model_id_or_path="dbmdz/bert-large-cased-finetuned-conll03-english",
        do_lower_case=True,
        add_special_tokens=False,
    )
    model.load()
    request.addfinalizer(model.stop)

    request = "HuggingFace is a company based in Paris and New York"
    response = await model({"instances": [request, request]}, headers={})
    assert response == {
        "predictions": [
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
        ]
    }


@pytest.mark.asyncio
async def test_bloom(request):
    model = HuggingfaceGenerativeModel(
        "bloom-560m",
        model_id_or_path="bigscience/bloom-560m",
    )
    model.load()
    request.addfinalizer(model.stop)

    params = CreateCompletionRequest(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=False,
        echo=True,
    )
    request = CompletionRequest(params=params)
    response = await model.create_completion(request)
    assert (
        response.choices[0].text
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute"
    )


@pytest.mark.asyncio
async def test_input_padding(request):
    model = HuggingfaceEncoderModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.stop)

    # inputs with different lengths will throw an error
    # unless we set padding=True in the tokenizer
    request_one = "Hello, my dog is cute."
    request_two = "Hello there, my dog is cute."
    response = await model({"instances": [request_one, request_two]}, headers={})
    assert response == {"predictions": [1, 1]}


@pytest.mark.asyncio
async def test_input_truncation(request):
    model = HuggingfaceEncoderModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.stop)

    # bert-base-uncased has a max length of 512 (tokenizer.model_max_length).
    # this request exceeds that, so it will throw an error
    # unless we set truncation=True in the tokenizer
    request = "good " * 600
    response = await model({"instances": [request]}, headers={})
    assert response == {"predictions": [1]}


if __name__ == "__main__":
    unittest.main()
