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
import lightgbm as lgb
from lightgbm import Booster
import os
from typing import Dict
import pandas as pd

BOOSTER_FILE = "model.bst"


class LightGBMModel(kfserving.KFModel):
    def __init__(self, name: str, model_dir: str, nthread: int,
                 booster: Booster = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.nthread = nthread
        if not booster is None:
            self._booster = booster
            self.ready = True

    def load(self) -> bool:
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), BOOSTER_FILE)
        self._booster = lgb.Booster(params={"nthread": self.nthread},
                                    model_file=model_file)
        self.ready = True
        return self.ready

    def predict(self, request: Dict) -> Dict:
        try:
            dfs = []
            for input in request['inputs']:
                dfs.append(pd.DataFrame(input, columns=self._booster.feature_name()))
            inputs = pd.concat(dfs, axis=0)

            result = self._booster.predict(inputs)
            return {"predictions": result.tolist()}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
