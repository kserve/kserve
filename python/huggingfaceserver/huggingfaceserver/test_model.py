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
from pytest_httpx import HTTPXMock

from kserve.model import PredictorConfig
from kserve.protocol.rest.v2_datamodels import GenerateRequest
from .task import MLTask
from .model import HuggingfaceModel


def test_t5():
    model = HuggingfaceModel("t5-small", {"model_id": "t5-small"})
    model.load()

    request = "translate this to germany"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {
        "predictions": ["Das ist für Deutschland", "Das ist für Deutschland"]
    }


def test_bert():
    model = HuggingfaceModel(
        "bert-base-uncased",
        {"model_id": "bert-base-uncased", "disable_lower_case": False},
    )
    model.load()

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


def test_bert_predictor_host(httpx_mock: HTTPXMock):
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
        {
            "model_id": "bert-base-uncased",
            "tensor_input_names": "input_ids",
            "disable_lower_case": False,
        },
        predictor_config=PredictorConfig(
            predictor_host="localhost:8081", predictor_protocol="v2"
        ),
    )
    model.load()

    response = asyncio.run(
        model({"instances": ["The capital of France is [MASK]."]}, headers={})
    )
    assert response == {"predictions": ["[PAD]"]}


def test_bert_sequence_classification():
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        {
            "model_id": "textattack/bert-base-uncased-yelp-polarity",
            "task": MLTask.sequence_classification.value,
        },
    )
    model.load()

    request = "Hello, my dog is cute."
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {"predictions": [1, 1]}


def test_bert_token_classification():
    model = HuggingfaceModel(
        "bert-large-cased-finetuned-conll03-english",
        {
            "model_id": "dbmdz/bert-large-cased-finetuned-conll03-english",
            "disable_special_tokens": True,
        },
    )
    model.load()

    request = "HuggingFace is a company based in Paris and New York"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {
        "predictions": [
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
            [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
        ]
    }


def test_bloom():
    model = HuggingfaceModel(
        "bloom-560m",
        {"model_id": "bigscience/bloom-560m", "disable_special_tokens": True},
    )
    model.load()

    request = "Hello, my dog is cute"
    response = asyncio.run(
        model.generate(generate_request=GenerateRequest(text_input=request), headers={})
    )
    assert (
        response.text_output
        == "Hello, my dog is cute.\n- Hey, my dog is cute.\n- Hey, my dog"
    )


def test_input_padding():
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        {
            "model_id": "textattack/bert-base-uncased-yelp-polarity",
            "task": MLTask.sequence_classification.value,
        },
    )
    model.load()

    # inputs with different lengths will throw an error
    # unless we set padding=True in the tokenizer
    request_one = "Hello, my dog is cute."
    request_two = "Hello there, my dog is cute."
    response = asyncio.run(model({"instances": [request_one, request_two]}, headers={}))
    assert response == {"predictions": [1, 1]}


def test_input_truncation():
    model = HuggingfaceModel(
        "bert-base-uncased-yelp-polarity",
        {
            "model_id": "textattack/bert-base-uncased-yelp-polarity",
            "task": MLTask.sequence_classification.value,
        },
    )
    model.load()

    # bert-base-uncased has a max length of 512 (tokenizer.model_max_length).
    # this request exceeds that, so it will throw an error
    # unless we set truncation=True in the tokenizer
    request = "good " * 600
    response = asyncio.run(model({"instances": [request]}, headers={}))
    assert response == {"predictions": [1]}


if __name__ == "__main__":
    unittest.main()
