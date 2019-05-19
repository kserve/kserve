# Copyright 2019 kubeflow.org.
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

import xgboost as xgb
from sklearn.datasets import load_digits
from xgbserver import XGBoostModel


def test_model():
    digits = load_digits(2)
    y = digits['target']
    X = digits['data']
    xgb_model = xgb.XGBClassifier(random_state=42).fit(X, y)
    server = XGBoostModel("xgbmodel", "", booster=xgb_model)
    request = [X[0].tolist()]
    response = server.predict(request)
    assert response == [0]
