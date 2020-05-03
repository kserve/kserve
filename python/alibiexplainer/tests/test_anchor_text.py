from alibiexplainer.anchor_text import AnchorText
import os
from .utils import Predictor
from sklearnserver.model import SKLearnModel
from alibi.datasets import fetch_movie_sentiment
from alibi.api.interfaces import Explanation
import json
import numpy as np

MOVIE_MODEL_URI = "gs://seldon-models/sklearn/moviesentiment"

def test_anchor_text():
    os.environ.clear()
    skmodel = SKLearnModel("adult", MOVIE_MODEL_URI)
    skmodel.load()
    predictor = Predictor(skmodel)
    anchor_text = AnchorText(predictor.predict_fn,None)
    movies = fetch_movie_sentiment()
    np.random.seed(0)
    explanation: Explanation = anchor_text.explain(movies.data[4:5])
    exp_json = json.loads(explanation.to_json())
    print(exp_json["data"]["anchor"])
