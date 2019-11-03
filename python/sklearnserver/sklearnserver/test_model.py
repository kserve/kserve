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
import os

model_dir = model_dir = os.path.join(os.path.dirname(__file__), "example_model")
JOBLIB_FILE = "model.joblib"

def test_model():
    iris = datasets.load_iris()
    X, y = iris.data, iris.target
    sklearn_model = svm.SVC(gamma='scale')
    sklearn_model.fit(X, y)
    model_file = os.path.join((model_dir), JOBLIB_FILE)
    joblib.dump(value=sklearn_model, filename=model_file)
    model = SKLearnModel("sklearnmodel", model_dir)
    model.load()
    request = X[0:1].tolist()
    response = model.predict({"instances": request})
    assert response["predictions"] == [0]
