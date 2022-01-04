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

import logging
from typing import Dict
import pickle

import kserve
import numpy as np


class PipeStep(object):
    """
    Wrapper for turning functions into pipeline transforms (no-fitting)
    """
    def __init__(self, step_func):
        self._step_func = step_func

    def fit(self, *args):
        return self

    def transform(self, X):
        return self._step_func(X)


class RFModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str):
        super().__init__(name)
        self.name = name
        self.ready = False

    def load(self) -> bool:

        with open('../rfmodel.pickle', 'rb') as f:
            rf = pickle.load(f)
        self.model = rf

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

            n_samples, a, b, c = inputs.shape
            inputs = np.reshape(inputs, (n_samples, 2352))

            predictions = self.model.predict_proba(inputs)

            class_preds = [[] for x in range(0, len(predictions[0]))]
            for j in range(0, len(predictions[0])):
                for i in range(0, len(predictions)):
                    class_preds[j].append(predictions[i][j][1])

            return {"predictions": class_preds}
        except Exception as e:
            raise Exception("Failed to predict: %s" % e)
