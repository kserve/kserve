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

from kserve.model import PredictorConfig
from openai.types.completion_create_params import CompletionCreateParamsNonStreaming
from pytest_httpx import HTTPXMock

from .model import HuggingfaceModel
from .task import MLTask


def test_t5(request):
    model = HuggingfaceModel("t5-small", model_id_or_path="google-t5/t5-small")
    model.load()
    request.addfinalizer(model.unload)

    request = "translate this to germany"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {
        "predictions": ["Das ist für Deutschland", "Das ist für Deutschland"]
    }


def test_bert(request):
    model = HuggingfaceModel(
        "google-bert/bert-base-uncased",
        model_id_or_path="bert-base-uncased",
        do_lower_case=True,
    )
    model.load()
    request.addfinalizer(model.unload)

    response = asyncio.run(
        model(
            {
                "instances": [
                    "The capital of France is [MASK].",
                    "The capital of [MASK] is paris.",
                ]
            },
            headers={},
        )
    )
    assert response == {"predictions": ["paris", "france"]}


def test_model_revision(request):
    # https://huggingface.co/google-bert/bert-base-uncased
    commit = "86b5e0934494bd15c9632b12f734a8a67f723594"
    model = HuggingfaceModel(
        "google-bert/bert-base-uncased",
        model_id_or_path="bert-base-uncased",
        model_revision=commit,
        tokenizer_revision=commit,
        do_lower_case=True,
    )
    model.load()
    request.addfinalizer(model.unload)

    response = asyncio.run(
        model(
            {
                "instances": [
                    "The capital of France is [MASK].",
                    "The capital of [MASK] is paris.",
                ]
            },
            headers={},
        )
    )
    assert response == {"predictions": ["paris", "france"]}


def test_bert_predictor_host(request, httpx_mock: HTTPXMock):
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

    model = HuggingfaceModel(
        "bert",
        model_id_or_path="google-bert/bert-base-uncased",
        tensor_input_names="input_ids",
        predictor_config=PredictorConfig(
            predictor_host="localhost:8081", predictor_protocol="v2"
        ),
    )
    model.load()
    request.addfinalizer(model.unload)

    response = asyncio.run(
        model({"instances": ["The capital of France is [MASK]."]}, headers={})
    )
    assert response == {"predictions": ["[PAD]"]}


def test_bert_sequence_classification(request):
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.unload)

    request = "Hello, my dog is cute."
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {"predictions": [1, 1]}


def test_bert_token_classification(request):
    model = HuggingfaceModel(
        "bert-large-cased-finetuned-conll03-english",
        model_id_or_path="dbmdz/bert-large-cased-finetuned-conll03-english",
        do_lower_case=True,
        add_special_tokens=False,
    )
    model.load()
    request.addfinalizer(model.unload)

    request = "HuggingFace is a company based in Paris and New York"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {
        "predictions": [
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
        ]
    }


def test_bloom(request):
    model = HuggingfaceModel(
        "bloom-560m",
        model_id_or_path="bigscience/bloom-560m",
        add_special_tokens=False,
    )
    model.load()
    request.addfinalizer(model.unload)

    params = CompletionCreateParamsNonStreaming(
        model="bloom-560m",
        prompt="Hello, my dog is cute",
        stream=False,
        echo=True,
    )
    response = asyncio.run(model.create_completion(params))
    assert (
        response.choices[0].text
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog is cute.\n- Hey"
    )


def test_input_padding(request):
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.unload)

    # inputs with different lengths will throw an error
    # unless we set padding=True in the tokenizer
    request_one = "Hello, my dog is cute."
    request_two = "Hello there, my dog is cute."
    response = asyncio.run(model({"instances": [request_one, request_two]}, headers={}))
    assert response == {"predictions": [1, 1]}


def test_input_truncation(request):
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        model_id_or_path="textattack/bert-base-uncased-yelp-polarity",
        task=MLTask.sequence_classification,
    )
    model.load()
    request.addfinalizer(model.unload)

    # bert-base-uncased has a max length of 512 (tokenizer.model_max_length).
    # this request exceeds that, so it will throw an error
    # unless we set truncation=True in the tokenizer
    request = "good " * 600
    response = asyncio.run(model({"instances": [request]}, headers={}))
    assert response == {"predictions": [1]}


if __name__ == "__main__":
    unittest.main()
