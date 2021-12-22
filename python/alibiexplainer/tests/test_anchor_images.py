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

from alibiexplainer.anchor_images import AnchorImages
import os
import tensorflow as tf
import json
import numpy as np
import kserve
import dill


CIFAR10_EXPLAINER_URI = "gs://seldon-models/tfserving/cifar10/explainer-py36-0.5.2"
EXPLAINER_FILENAME = "explainer.dill"


def test_cifar10_images():  # pylint: disable-msg=too-many-locals
    os.environ.clear()
    alibi_model = os.path.join(
        kserve.Storage.download(CIFAR10_EXPLAINER_URI), EXPLAINER_FILENAME
    )
    with open(alibi_model, "rb") as f:
        alibi_model = dill.load(f)
        url = "https://storage.googleapis.com/seldon-models/alibi-detect/classifier/"
        path_model = os.path.join(url, "cifar10", "resnet32", "model.h5")
        save_path = tf.keras.utils.get_file("resnet32", path_model)
        model = tf.keras.models.load_model(save_path)
        _, test = tf.keras.datasets.cifar10.load_data()
        X_test, _ = test
        X_test = X_test.astype("float32") / 255
        idx = 12
        test_example = X_test[idx: idx + 1]
        anchor_images = AnchorImages(
            lambda x: model.predict(x), alibi_model)  # pylint: disable-msg=unnecessary-lambda
        np.random.seed(0)
        explanation = anchor_images.explain(test_example)
        exp_json = json.loads(explanation.to_json())
        assert exp_json["data"]["precision"] > 0.9
