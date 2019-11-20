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

from sklearn import svm
from sklearn import datasets
from sklearnserver import SKLearnModel
import joblib
import pickle
import os

model_dir = os.path.join(os.path.dirname(__file__), "example_models")
joblib_dir = os.path.join(model_dir, "joblib")
pickle_dir = os.path.join(model_dir, "pickle")
JOBLIB_FILE = "model.joblib"
PICKLE_FILE = "model.pkl"


def _train_sample_model():
    iris = datasets.load_iris()
    X, y = iris.data, iris.target
    sklearn_model = svm.SVC(gamma='scale')
    sklearn_model.fit(X, y)
    return sklearn_model, X


def test_model_joblib():
    sklearn_model, data = _train_sample_model()
    model_file = os.path.join(joblib_dir, JOBLIB_FILE)
    joblib.dump(value=sklearn_model, filename=model_file)
    model = SKLearnModel("sklearnmodel", joblib_dir)
    model.load()
    request = data[0:1].tolist()
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]


def test_model_pickle():
    sklearn_model, data = _train_sample_model()
    model_file = os.path.join(pickle_dir, PICKLE_FILE)
    pickle.dump(sklearn_model, open(model_file, 'wb'))
    model = SKLearnModel("sklearnmodel", pickle_dir)
    model.load()
    request = data[0:1].tolist()
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]
