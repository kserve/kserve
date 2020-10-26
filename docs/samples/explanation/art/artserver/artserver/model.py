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

import os
from typing import Dict

import logging
import kfserving
import numpy as np
from keras.models import model_from_json
from art.attacks import FastGradientMethod
from art.classifiers import KerasClassifier

MODEL_FILE = "model.h5"
MODEL_JSON = "model.json"

class ARTModel(kfserving.KFModel): #pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]

        try:
            inputs = np.asarray(instances)
            logging.info("Calling predict on image of shape %s", (inputs.shape,))
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, instances))

        try:
            model_weights_file = os.path.join(self.model_dir, MODEL_FILE)
            model_json_file = os.path.join(self.model_dir, MODEL_JSON)

            # read model json file
            with open(model_json_file, 'r') as f:
                mnist_model = model_from_json(f.read())
            # read model weights file
            mnist_model.load_weights(model_weights_file)

            # wrap mnist_model into a framework independent class structure
            mymodel = KerasClassifier(mnist_model)
            preds = np.argmax(mymodel.predict(inputs), axis=1)
            return {"predictions" : preds.tolist()}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
