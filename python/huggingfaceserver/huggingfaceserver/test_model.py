import asyncio
import unittest
from huggingfaceserver import HuggingfaceModel


def test_t5():
    model = HuggingfaceModel("t5-small", {"model_id": "t5-small"})
    model.load()

    request = "translate this to germany"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {"predictions": ['Das ist für Deutschland', 'Das ist für Deutschland']}


def test_bert():
    model = HuggingfaceModel("bert-base-uncased", {"model_id": "bert-base-uncased", "do_lower_case": True})
    model.load()

    response = asyncio.run(model({"instances": ["The capital of France is [MASK].",
                                                "The capital of [MASK] is paris."]}, headers={}))
    assert response == {"predictions": ["paris", "france"]}


def test_bert_sequence_classification():
    model = HuggingfaceModel("bert-base-uncased-yelp-polarity",
                             {"model_id": "textattack/bert-base-uncased-yelp-polarity",
                              "task": "sequence-classification"})
    model.load()

    request = "Hello, my dog is cute."
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {"predictions": [1, 1]}


def test_bert_token_classification():
    model = HuggingfaceModel("bert-large-cased-finetuned-conll03-english",
                             {"model_id": "dbmdz/bert-large-cased-finetuned-conll03-english",
                              "add_special_tokens": False})
    model.load()

    request = "HuggingFace is a company based in Paris and New York"
    response = asyncio.run(model({"instances": [request, request]}, headers={}))
    assert response == {"predictions": [[[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]],
                                        [[0, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]]]}


if __name__ == '__main__':
    unittest.main()
