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
import json
import numpy as np
import kfserving
import dill
import PIL
import random
import requests
from requests import RequestException
from io import BytesIO

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
        predictor = lambda x: model.predict(x)  # pylint:disable=unnecessary-lambda
        alibi_model = dill.load(f)
        anchor_images = AnchorImages(
            predictor, alibi_model, batch_size=25, stop_on_first=True
        )
        image_shape = (299, 299, 3)
        # the image downloader comes from seldonio/alibi
        # https://github.com/SeldonIO/alibi/blob/76e6192b6d78848dd47c11ba6f6348ca94c424c6/alibi/datasets.py#L104-L125
        img_urls = json.load(open('alibiexplainer/tests/persian_cat.json'))
        seed = 2
        random.seed(seed)
        random.shuffle(img_urls)
        data = []
        nb = 0
        nb_images = 10
        target_size = image_shape[:2]
        min_std = 10.
        for img_url in img_urls:
            try:
                resp = requests.get(img_url, timeout=2)
                resp.raise_for_status()
            except RequestException:
                continue
            try:
                image = PIL.Image.open(BytesIO(resp.content)).convert('RGB')
            except OSError:
                continue
            image = np.expand_dims(image.resize(target_size), axis=0)
            if np.std(image) < min_std:  # do not include empty images
                continue
            data.append(image)
            nb += 1
            if nb == nb_images:
                break
        data = np.concatenate(data, axis=0)
        images = preprocess_input(data)
        print(images.shape)
        np.random.seed(0)
        explanation = anchor_images.explain(images[0:1])
        exp_json = json.loads(explanation.to_json())
        assert exp_json["data"]["precision"] > 0.9
