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

import kfserving
import joblib
import numpy as np
import os
import pickle
from typing import List, Dict

JOBLIB_FILE = "model.joblib"
PICKEL_FILE = "model.pkl"

class SKLearnModel(kfserving.KFModel): #pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_path = kfserving.Storage.download(self.model_dir)
        joblib_path = os.path.join(model_path, JOBLIB_FILE)
        pickle_path = os.path.join(model_path, PICKEL_FILE)
        if os.path.exists(joblib_path):
            self._model = joblib.load(joblib_path) #pylint:disable=attribute-defined-outside-init
        elif os.path.exists(pickle_path):
            self._model = pickle.load(open(pickle_path, 'rb'))
        else:
            raise Exception("Model file " + JOBLIB_FILE + " or " + PICKEL_FILE + " is not found")
        self.ready = True

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            inputs = np.array(instances)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, instances))
        try:
            result = self._model.predict(inputs).tolist()
            return { "predictions" : result }
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
