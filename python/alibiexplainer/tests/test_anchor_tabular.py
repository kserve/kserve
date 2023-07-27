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

from alibiexplainer.anchor_tabular import AnchorTabular
import os
import dill
from sklearnserver.model import SKLearnModel
from alibi.datasets import fetch_adult
import numpy as np
import json
from .utils import Predictor
from kserve.storage import Storage

ADULT_EXPLAINER_URI = "gs://kfserving-examples/models/sklearn/1.3/income/explainer"
ADULT_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.3/income/model"
EXPLAINER_FILENAME = "explainer.dill"


def test_anchor_tabular():
    os.environ.clear()
    alibi_model = os.path.join(
        Storage.download(ADULT_EXPLAINER_URI), EXPLAINER_FILENAME
    )
    with open(alibi_model, "rb") as f:
        skmodel = SKLearnModel("adult", ADULT_MODEL_URI)
        skmodel.load()
        predictor = Predictor(skmodel)
        alibi_model = dill.load(f)
        anchor_tabular = AnchorTabular(predictor.predict_fn, alibi_model)
        adult = fetch_adult()
        X_test = adult.data[30001:, :]
        np.random.seed(0)
        explanation = anchor_tabular.explain(X_test[0:1].tolist())
        exp_json = json.loads(explanation.to_json())
        assert exp_json["data"]["anchor"][0] == "Relationship = Own-child" or \
               exp_json["data"]["anchor"][0] == "Age <= 28.00"
