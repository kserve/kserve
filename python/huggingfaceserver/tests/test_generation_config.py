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

"""Fast, model-free tests for HuggingfaceGenerativeModel.build_generation_config.

These guard the contract that a request temperature of 0 (OpenAI's convention for
deterministic/greedy decoding) never reaches transformers as a non-positive
temperature. transformers >= 5 raises ``ValueError`` when a
``TemperatureLogitsWarper`` is built with temperature <= 0, which previously
crashed the background generation thread and hung the server on every subsequent
request.
"""

from types import SimpleNamespace

import pytest

from kserve.protocol.rest.openai.types import CompletionRequest

from huggingfaceserver.generative_model import HuggingfaceGenerativeModel


def _build_config(temperature):
    # build_generation_config only reads self._tokenizer.pad_token_id, so a stub
    # self avoids loading a real model/tokenizer and keeps the test fast.
    stub_self = SimpleNamespace(_tokenizer=SimpleNamespace(pad_token_id=0))
    request = CompletionRequest(
        model="test-model",
        prompt="hello",
        stream=False,
        max_tokens=8,
        temperature=temperature,
    )
    return HuggingfaceGenerativeModel.build_generation_config(stub_self, request)


@pytest.mark.parametrize("temperature", [0, 0.0])
def test_zero_temperature_maps_to_greedy_decoding(temperature):
    config = _build_config(temperature)
    # do_sample defaults to False (greedy) and no non-positive temperature is emitted,
    # so transformers will not construct an invalid TemperatureLogitsWarper.
    assert config.do_sample is False
    assert config.temperature is None or config.temperature > 0.0


def test_positive_temperature_is_forwarded():
    config = _build_config(0.7)
    assert config.temperature == pytest.approx(0.7)
