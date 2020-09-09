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
from typing import Dict

BOOSTER_FILE = "model.bst"
DEFAULT_NTHREAD = 1


class XGBoostModel(kfserving.KFModel):
    def __init__(self, name: str, model_dir: str, full_model_path: str = "",
                 nthread: int = DEFAULT_NTHREAD, booster: XGBModel = None):
        super().__init__(name)
        self.name = name
        self.full_model_path = full_model_path
        self.model_dir = model_dir
        self.nthread = nthread
        if not booster is None:
            self._booster = booster
            self.ready = True

    def load_from_model_dir(self):
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), BOOSTER_FILE)
        self._booster = xgb.Booster(params={"nthread": self.nthread},
                                    model_file=model_file)
        self.ready = True

    def load_from_full_model_path(self):
        self._booster = xgb.Booster(params={"nthread": self.nthread},
                                    model_file=self.full_model_path)
        self.ready = True

    async def load(self):
        if len(self.full_model_path) != 0:
            self.load_from_full_model_path()
        else:
            self.load_from_model_dir()

    def predict(self, request: Dict) -> Dict:
        try:
            # Use of list as input is deprecated see https://github.com/dmlc/xgboost/pull/3970
            dmatrix = xgb.DMatrix(request["instances"], nthread=self.nthread)
            result: xgb.DMatrix = self._booster.predict(dmatrix)
            return {"predictions": result.tolist()}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
