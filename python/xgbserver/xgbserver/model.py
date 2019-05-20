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
import xgboost as xgb
from xgboost import XGBModel
import os
import numpy as np
from typing import List, Any

BOOSTER_FILE = "model.bst"

class XGBoostModel(kfserving.KFModel):
    def __init__(self, name: str, model_dir: str, booster: XGBModel = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        if not booster is None:
            self._booster = booster
            self.ready = True

    def load(self):
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), BOOSTER_FILE)
        self._booster = xgb.Booster(model_file=model_file)
        self.ready = True

    def predict(self, body: List) -> List:
        try:
            # Use of list as input is deprecated see https://github.com/dmlc/xgboost/pull/3970
            arr = np.array(body)
            result: np.array = self._booster.predict(arr)
            return result.tolist()
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
