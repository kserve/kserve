from alibiexplainer.anchor_images import AnchorImages
import os
from tensorflow.keras.applications.inception_v3 import InceptionV3, preprocess_input
from alibi.datasets import fetch_imagenet
from alibi.api.interfaces import Explanation
import json
import numpy as np
import kfserving
import dill

IMAGENET_EXPLAINER_URI = "gs://seldon-models/tfserving/imagenet/alibi/0.4.0"
ADULT_MODEL_URI = "gs://seldon-models/sklearn/income/model"
EXPLAINER_FILENAME = "explainer.dill"

def test_anchor_images():
    os.environ.clear()
    alibi_model = os.path.join(kfserving.Storage.download(IMAGENET_EXPLAINER_URI), EXPLAINER_FILENAME)
    with open(alibi_model, 'rb') as f:
        model = InceptionV3(weights='imagenet')
        predictor = lambda x : model.predict(x)
        alibi_model = dill.load(f)
        anchor_images = AnchorImages(predictor,alibi_model)
        category = 'Persian cat'
        image_shape = (299, 299, 3)
        data, labels = fetch_imagenet(category, nb_images=10, target_size=image_shape[:2], seed=2, return_X_y=True)
        images = preprocess_input(data)
        print(images.shape)
        np.random.seed(0)
        explanation: Explanation = anchor_images.explain(images[0:1])
        exp_json = json.loads(explanation.to_json())
        assert exp_json["data"]["precision"] > 0.9