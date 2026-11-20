# Copyright 2026 The KServe Authors.
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
from unittest.mock import AsyncMock

from kserve import Model
from kserve.predictor_config import PredictorConfig

from trace_transformer import TraceTransformer


def test_transformer_uses_inherited_predictor_forwarding(monkeypatch):
    model = TraceTransformer("iris")
    monkeypatch.setattr(
        TraceTransformer,
        "predictor_config",
        property(lambda self: PredictorConfig("iris-predictor:8080")),
    )
    model._http_client_instance = AsyncMock()
    model._http_client_instance.infer.return_value = {"predictions": [1, 1]}
    payload = {"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]}

    result, _ = asyncio.run(model(payload))

    assert model.ready
    assert result == {"predictions": [1, 1]}
    assert TraceTransformer.preprocess is Model.preprocess
    assert TraceTransformer.predict is Model.predict
    assert TraceTransformer.postprocess is Model.postprocess
    model._http_client_instance.infer.assert_awaited_once_with(
        "http://iris-predictor:8080",
        model_name="iris",
        data=payload,
        headers={"Content-Type": "application/json"},
        response_headers=None,
    )
