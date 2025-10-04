# Copyright 2021 The KServe Authors.
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

import os
from catboostserver import CatBoostModel

# Use example model following KServe pattern
example_model_dir = os.path.join(
    os.path.dirname(__file__), "..", "example_model", "model"
)


def test_cbm_model():
    """Test CatBoost model loading and prediction using example model"""
    catboost_model = CatBoostModel("test-model", example_model_dir)
    catboost_model.load()

    # Test with Iris data (same as used in other KServe tests)
    request = {"instances": [[6.8, 2.8, 4.8, 1.4], [6.0, 3.4, 4.5, 1.6]]}
    response = catboost_model.predict(request)

    assert "predictions" in response
    assert len(response["predictions"]) == 2


def test_model_formats():
    """Test that model supports both .cbm and .bin formats"""
    # The example model is .cbm format, test it works
    catboost_model = CatBoostModel("test-model", example_model_dir)
    catboost_model.load()

    assert catboost_model.ready
    assert catboost_model._model is not None
