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
from pytorchserver import PyTorchModel
import os

model_dir = "../../docs/samples/pytorch"
JOBLIB_FILE = "model.pt"

def test_model():
     server = PyTorchModel("pytorchmodel", "model_class_file", "model_class_name", model_dir)
     server.load()
     request = X[0:1].tolist()
     response = server.predict(request)
     assert response == [0]