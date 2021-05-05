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

from catboost import CatBoostClassifier
import os
import time
import tempfile
from pathlib import Path
from catboostserver import CatBoostModel

MODEL_FILE = "model"
NTHREAD = 1


def test_model():
    with tempfile.TemporaryDirectory() as tmpdir:
        save_model(tmpdir)
        model = CatBoostModel("model", tmpdir, "classification", NTHREAD)
        model.load()
    response = model.predict({"instances": [1, 3]})
    assert response["predictions"] == [1]

def save_model(tmpdirname :str):
    train_data = [[1, 3],
              [0, 4],
              [1, 7]]
    train_labels = [1, 0, 1]
    cat_boost_model = CatBoostClassifier(learning_rate=0.03)
    cat_boost_model.fit(train_data,
                        train_labels,
                        verbose=False)
    cat_boost_model_file = os.path.join(tmpdirname, "model")
    cat_boost_model.save_model(cat_boost_model_file)
