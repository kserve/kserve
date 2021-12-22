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

from sklearn import svm
from sklearn import datasets
from sklearnserver import SKLearnModel
import joblib
import pickle
import os

import pytest

_MODEL_DIR = os.path.join(os.path.dirname(__file__), "example_models")
JOBLIB_FILE = [os.path.join(_MODEL_DIR, "joblib", "model"), "model.joblib"]
PICKLE_FILES = [[os.path.join(_MODEL_DIR, "pkl", "model"), "model.pkl"],
                [os.path.join(_MODEL_DIR, "pickle", "model"), "model.pickle"]]
MULTI_DIR = os.path.join(_MODEL_DIR, "multi", "model")


def _train_sample_model():
    iris = datasets.load_iris()
    X, y = iris.data, iris.target
    sklearn_model = svm.SVC(gamma='scale')
    sklearn_model.fit(X, y)
    return sklearn_model, X


def _run_pickle_model(model_dir, model_name):
    sklearn_model, data = _train_sample_model()
    model_file = os.path.join(model_dir, model_name)
    pickle.dump(sklearn_model, open(model_file, 'wb'))
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


def test_model_pickle():
    for pickle_file in PICKLE_FILES:
        _run_pickle_model(pickle_file[0], pickle_file[1])


def test_dir_with_no_model():
    model = SKLearnModel("model", _MODEL_DIR)
    with pytest.raises(RuntimeError) as e:
        model.load()
    assert 'Missing Model File' in str(e.value)


def test_dir_with_two_models():
    model = SKLearnModel("model", MULTI_DIR)
    with pytest.raises(RuntimeError) as e:
        model.load()
    assert 'More than one model file is detected' in str(e.value)
