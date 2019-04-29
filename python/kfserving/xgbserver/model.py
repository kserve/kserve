from kfserving.kfserver import KFModel
from kfserving.kfserver import Storage
import xgboost as xgb
import pandas as pd


class XGBoostModel(KFModel):
    def __init__(self, name, model_file):
        self.name = name
        self.model_file = model_file
        self.ready = False

    def load(self):
        self._booster = xgb.Booster(model_file=self.model_file)
        self.ready = True

    def preprocess(self, inputs):
        try:
            return xgb.DMatrix(inputs)
        except Exception as e:
            raise Exception(
                "Failed to initialize DMatrix from inputs: %s, %s" % (e, inputs))

    def predict(self, inputs):
        try:
            return self._booster.predict(inputs)
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
