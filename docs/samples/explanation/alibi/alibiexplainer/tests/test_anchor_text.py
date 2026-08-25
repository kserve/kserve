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

from alibiexplainer.anchor_text import AnchorText
import os
from .utils import Predictor
from sklearnserver.model import SKLearnModel
from alibi.datasets import fetch_movie_sentiment
import json
import numpy as np

MOVIE_MODEL_URI = "gs://kfserving-examples/models/sklearn/1.0/moviesentiment/model"


def test_anchor_text():
    os.environ.clear()
    skmodel = SKLearnModel("movie", MOVIE_MODEL_URI)
    skmodel.load()
    predictor = Predictor(skmodel)
    anchor_text = AnchorText(predictor.predict_fn, None)
    movies = fetch_movie_sentiment()
    np.random.seed(0)
    explanation = anchor_text.explain(movies.data[4:5])
    exp_json = json.loads(explanation.to_json())
    print(exp_json["data"]["anchor"])
