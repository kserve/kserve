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
import numpy as np
import os
from catboost import CatBoostClassifier, CatBoostRegressor
from typing import Dict

MODEL_BASENAME = "model"


class CatBoostModel(kfserving.KFModel):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str, kind: str, nthread: int):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.kind = kind
        self.nthread = nthread
        self.ready = False

    def load(self) -> bool:
        model_path = kfserving.Storage.download(self.model_dir)
        path = os.path.join(model_path, MODEL_BASENAME)
        if os.path.exists(path):
            if self.kind == "classification":
                self._model = CatBoostClassifier()
                self._model.load_model(path)
                self.ready = True
            elif self.kind == "regerssion":
                self._model = CatBoostRegressor()
                self._model.load_model(path)
                self.ready = True
        return self.ready

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            inputs = np.array(instances)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, instances))
        try:
            result = self._model.predict(inputs, thread_count=self.nthread)
            return {"predictions": result}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
