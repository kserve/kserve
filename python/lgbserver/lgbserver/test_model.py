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

import lightgbm as lgb
import os
from sklearn.datasets import load_iris
from lgbserver import LightGBMModel
import pandas as pd

model_dir = os.path.join(os.path.dirname(__file__), "example_model", "model")
BST_FILE = "model.bst"
NTHREAD = 1


def test_model():
    iris = load_iris()
    y = iris['target']
    X = pd.DataFrame(iris['data'], columns=iris['feature_names'])
    dtrain = lgb.Dataset(X, label=y)

    params = {
        'objective': 'multiclass',
        'metric': 'softmax',
        'num_class': 3
    }
    lgb_model = lgb.train(params=params, train_set=dtrain)
    model_file = os.path.join(model_dir, BST_FILE)
    lgb_model.save_model(model_file)
    model = LightGBMModel("model", model_dir, NTHREAD)
    model.load()

    request = {"x": {0: 1.1}, 'sepal_width_(cm)': {0: 3.5}, 'petal_length_(cm)': {0: 1.4},
               'petal_width_(cm)': {0: 0.2}, 'sepal_length_(cm)': {0: 5.1}}

    response = model.predict({"inputs": [request, request]})
    import numpy
    assert numpy.argmax(response["predictions"][0]) == 0
