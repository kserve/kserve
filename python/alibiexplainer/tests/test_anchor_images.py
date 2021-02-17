# Copyright 2020 kubeflow.org.
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

from alibiexplainer.anchor_images import AnchorImages
import os
from tensorflow.keras.applications.inception_v3 import InceptionV3, preprocess_input
from alibi.datasets import fetch_imagenet
import json
import numpy as np
import kfserving
import dill

IMAGENET_EXPLAINER_URI = "gs://seldon-models/tfserving/imagenet/explainer-py36-0.5.2"
ADULT_MODEL_URI = "gs://seldon-models/sklearn/income/model"
EXPLAINER_FILENAME = "explainer.dill"


def test_anchor_images():
    os.environ.clear()
    alibi_model = os.path.join(
        kfserving.Storage.download(IMAGENET_EXPLAINER_URI), EXPLAINER_FILENAME
    )
    with open(alibi_model, "rb") as f:
        model = InceptionV3(weights="imagenet")
        predictor = lambda x: model.predict(x) # pylint:disable=unnecessary-lambda
        alibi_model = dill.load(f)
        anchor_images = AnchorImages(
            predictor, alibi_model, batch_size=25, stop_on_first=True
        )
        category = "Persian cat"
        image_shape = (299, 299, 3)
        data, _ = fetch_imagenet(
            category, nb_images=10, target_size=image_shape[:2], seed=2, return_X_y=True
        )
        images = preprocess_input(data)
        print(images.shape)
        np.random.seed(0)
        explanation = anchor_images.explain(images[0:1])
        exp_json = json.loads(explanation.to_json())
        assert exp_json["data"]["precision"] > 0.9
