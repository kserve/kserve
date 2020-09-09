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
from typing import Dict
from kfserving.kfmodels.kfmodel_types import KFModelTypes, MODEL_EXTENSIONS

MODEL_BASENAME = "model"
MODEL_EXTENSIONS = MODEL_EXTENSIONS[KFModelTypes.Sklearn]


class SKLearnModel(kfserving.KFModel):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str, full_model_path: str = ""):
        super().__init__(name)
        self.name = name
        self.full_model_path = full_model_path
        self.model_dir = model_dir
        self.ready = False
        self._model = None

    def load_from_model_dir(self):
        model_path = kfserving.Storage.download(self.model_dir)
        paths = [os.path.join(model_path, MODEL_BASENAME + model_extension)
                 for model_extension in MODEL_EXTENSIONS]
        model_file = next(path for path in paths if os.path.exists(path))
        self._model = joblib.load(model_file)  # pylint:disable=attribute-defined-outside-init
        self.ready = True

    def load_from_full_model_path(self):
        self._model = joblib.load(self.full_model_path)
        self.ready = True

    async def load(self):
        if len(self.full_model_path) != 0:
            self.load_from_full_model_path()
        else:
            self.load_from_model_dir()

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
