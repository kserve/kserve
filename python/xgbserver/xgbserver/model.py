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
            dmatrix = xgb.DMatrix(body)
            result: xgb.DMatrix = self._booster.predict(dmatrix)
            return result.tolist()
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
