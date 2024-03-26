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

from kserve.protocol.infer_type import InferInput, InferRequest
import xgboost as xgb
import os
from sklearn.datasets import load_iris
from xgbserver import XGBoostModel

model_dir = os.path.join(os.path.dirname(__file__), "example_model", "model")
json_model_dir = os.path.join(os.path.dirname(__file__), "example_model", "json_model")
ubj_model_dir = os.path.join(os.path.dirname(__file__), "example_model", "ubj_model")
BST_FILE = "model.bst"
JSON_FILE = "model.json"
UBJ_FILE = "model.ubj"
NTHREAD = 1


def test_model():
    iris = load_iris()
    y = iris['target']
    X = iris['data']
    dtrain = xgb.DMatrix(X, label=y)
    param = {'max_depth': 6,
             'eta': 0.1,
             'silent': 1,
             'nthread': 4,
             'num_class': 10,
             'objective': 'multi:softmax'}
    xgb_model = xgb.train(params=param, dtrain=dtrain)
    model_file = os.path.join(model_dir, BST_FILE)
    xgb_model.save_model(model_file)
    model = XGBoostModel("model", model_dir, NTHREAD)
    model.load()
    request = [X[0].tolist()]
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]

    # test v2 infer call
    infer_input = InferInput(name="input-0", shape=[1, 4], datatype="FP32",
                             data=request)
    infer_request = InferRequest(model_name="model", infer_inputs=[infer_input])
    infer_response = model.predict(infer_request)
    assert infer_response.to_rest()["outputs"][0]["data"] == [0]


def test_json_model():
    iris = load_iris()
    y = iris['target']
    X = iris['data']
    dtrain = xgb.DMatrix(X, label=y)
    param = {'max_depth': 6,
             'eta': 0.1,
             'silent': 1,
             'nthread': 4,
             'num_class': 10,
             'objective': 'multi:softmax'}
    xgb_model = xgb.train(params=param, dtrain=dtrain)
    model_file = os.path.join(json_model_dir, JSON_FILE)
    xgb_model.save_model(model_file)
    model = XGBoostModel("model", json_model_dir, NTHREAD)
    model.load()
    request = [X[0].tolist()]
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]

    # test v2 infer call
    infer_input = InferInput(name="input-0", shape=[1, 4], datatype="FP32",
                             data=request)
    infer_request = InferRequest(model_name="model", infer_inputs=[infer_input])
    infer_response = model.predict(infer_request)
    assert infer_response.to_rest()["outputs"][0]["data"] == [0]


def test_ubj_model():
    iris = load_iris()
    y = iris['target']
    X = iris['data']
    dtrain = xgb.DMatrix(X, label=y)
    param = {'max_depth': 6,
             'eta': 0.1,
             'silent': 1,
             'nthread': 4,
             'num_class': 10,
             'objective': 'multi:softmax'}
    xgb_model = xgb.train(params=param, dtrain=dtrain)
    model_file = os.path.join(ubj_model_dir, UBJ_FILE)
    xgb_model.save_model(model_file)
    model = XGBoostModel("model", ubj_model_dir, NTHREAD)
    model.load()
    request = [X[0].tolist()]
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]

    # test v2 infer call
    infer_input = InferInput(name="input-0", shape=[1, 4], datatype="FP32",
                             data=request)
    infer_request = InferRequest(model_name="model", infer_inputs=[infer_input])
    infer_response = model.predict(infer_request)
    assert infer_response.to_rest()["outputs"][0]["data"] == [0]
