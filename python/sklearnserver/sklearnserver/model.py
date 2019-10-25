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
from typing import List

JOBLIB_FILE = "model.joblib"

class SKLearnModel(kfserving.KFModel): #pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = os.path.join(kfserving.Storage.download(self.model_dir), JOBLIB_FILE) #pylint:disable=c-extension-no-member
        self._joblib = joblib.load(model_file) #pylint:disable=attribute-defined-outside-init
        self.ready = True

    def predict(self, request: Dict) -> Dict:
        try:
            inputs = np.array(request["instances"])
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, inputs))
        try:
            result = self._joblib.predict(inputs).tolist()
            return { "predictions" : result }
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
