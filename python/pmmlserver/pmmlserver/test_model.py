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

from pmmlserver import PmmlModel

model_dir = model_dir = os.path.join(os.path.dirname(__file__), "example_model", "model")


def test_model():
    server = PmmlModel("model", model_dir)
    server.load()

    request = {"instances": [[5.1, 3.5, 1.4, 0.2]]}
    response = server.predict(request)
    expect_result = {'Species': 'setosa',
                     'Probability_setosa': 1.0,
                     'Probability_versicolor': 0.0,
                     'Probability_virginica': 0.0,
                     'Node_Id': '2'}

    assert isinstance(response["predictions"][0], dict)
    assert response["predictions"][0] == expect_result
