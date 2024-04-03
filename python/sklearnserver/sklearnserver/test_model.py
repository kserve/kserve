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
from sklearn import svm
from sklearn import datasets
from sklearnserver import SKLearnModel
import joblib
import pickle
import os
from kserve.errors import ModelMissingError

import pytest

_MODEL_DIR = os.path.join(os.path.dirname(__file__), "example_models")
JOBLIB_FILE = [os.path.join(_MODEL_DIR, "joblib", "model"), "model.joblib"]
PICKLE_FILES = [
    [os.path.join(_MODEL_DIR, "pkl", "model"), "model.pkl"],
    [os.path.join(_MODEL_DIR, "pickle", "model"), "model.pickle"],
]
MULTI_DIR = os.path.join(_MODEL_DIR, "multi", "model")
MIXEDTYPE_DIR = os.path.join(_MODEL_DIR, "mixedtype", "model")


def create_v2_request(request, model_name=None):
    infer_inputs = []
    parameters = {}
    if len(request) > 0 and isinstance(request[0], dict):
        parameters["content_type"] = "pd"
        dataframe = request[0]
        for key, val in dataframe.items():
            infer_input = InferInput(
                name=key,
                shape=[len(val)],
                datatype=(
                    "INT32" if len(val) > 0 and isinstance(val[0], int) else "BYTES"
                ),
                data=val,
            )

            infer_inputs.append(infer_input)

    infer_request = InferRequest(
        model_name=model_name, infer_inputs=infer_inputs, parameters=parameters
    )
    return infer_request


def _train_sample_model():
    iris = datasets.load_iris()
    X, y = iris.data, iris.target
    sklearn_model = svm.SVC(gamma="scale", probability=True)
    sklearn_model.fit(X, y)
    return sklearn_model, X


def _run_pickle_model(model_dir, model_name):
    sklearn_model, data = _train_sample_model()
    model_file = os.path.join(model_dir, model_name)
    pickle.dump(sklearn_model, open(model_file, "wb"))
    model = SKLearnModel("model", model_dir)
    model.load()
    request = data[0:1].tolist()
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]


def test_model_joblib():
    sklearn_model, data = _train_sample_model()
    model_file = os.path.join(JOBLIB_FILE[0], JOBLIB_FILE[1])
    joblib.dump(value=sklearn_model, filename=model_file)
    model = SKLearnModel("model", JOBLIB_FILE[0])
    model.load()
    request = data[0:1].tolist()
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]

    # test v2 infer call
    infer_input = InferInput(
        name="input-0", shape=[1, 4], datatype="FP32", data=request
    )
    infer_request = InferRequest(model_name="model", infer_inputs=[infer_input])
    infer_response = model.predict(infer_request)
    assert infer_response.to_rest()["outputs"][0]["data"] == [0]


def test_mixedtype_model_joblib():
    model = SKLearnModel("model", MIXEDTYPE_DIR)
    model.load()
    request = [
        {
            "MSZoning": ["RL"],
            "LotArea": [8450],
            "LotShape": ["Reg"],
            "Utilities": ["AllPub"],
            "YrSold": [2008],
            "Neighborhood": ["CollgCr"],
            "OverallQual": [7],
            "YearBuilt": [2003],
            "SaleType": ["WD"],
            "GarageArea": [548],
        }
    ]
    response = model.predict({"instances": request})
    assert response["predictions"] == [12.202832815138274]

    # test v2 infer call for mixed type input
    infer_request = create_v2_request(request=request, model_name="model")
    infer_response = model.predict(infer_request)
    assert infer_response.to_rest()["outputs"][0]["data"] == [12.202832815138274]


def test_model_pickle():
    for pickle_file in PICKLE_FILES:
        _run_pickle_model(pickle_file[0], pickle_file[1])


def test_dir_with_no_model():
    model = SKLearnModel("model", _MODEL_DIR)
    with pytest.raises(ModelMissingError):
        model.load()


def test_dir_with_incompatible_model():
    model = SKLearnModel("model", _MODEL_DIR + "/pkl")
    with pytest.raises(ModuleNotFoundError) as e:
        model.load()
    assert "No module named" in str(e.value)


def test_dir_with_two_models():
    model = SKLearnModel("model", MULTI_DIR)
    with pytest.raises(RuntimeError) as e:
        model.load()
    assert "More than one model file is detected" in str(e.value)
