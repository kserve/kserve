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

import kserve
import joblib
import numpy as np
import os
from typing import Dict

MODEL_BASENAME = "model"
MODEL_EXTENSIONS = [".joblib", ".pkl", ".pickle"]


class SKLearnModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self) -> bool:
        model_path = kserve.Storage.download(self.model_dir)
        paths = [os.path.join(model_path, MODEL_BASENAME + model_extension)
                 for model_extension in MODEL_EXTENSIONS]
        for path in paths:
            if os.path.exists(path):
                self._model = joblib.load(path)
                self.ready = True
                break
        return self.ready

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            inputs = np.array(instances)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, instances))
        try:
            result = self._model.predict(inputs).tolist()
            return {"predictions": result}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
