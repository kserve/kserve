from kfserving.kfserver import KFModel
from kfserving.kfserver import Storage
import xgboost as xgb


class XGBoostModel(KFModel):
    def __init__(self, name, model_file):
        self.name = name
        self.model_file = model_file
        self.ready = False

    def load(self):
        self._booster = xgb.Booster(model_file=self.model_file)
        self.ready = True

    def predict(self, inputs):
        try:
            dmatrix = xgb.DMatrix(inputs)
        except Exception as e:
            raise Exception(
                "Failed to initialize DMatrix from inputs: {}, {}".format(inputs, e))
        try:
            return self._booster.predict(dmatrix)
        except Exception as e:
            raise Exception("Failed to predict {}".format(e))
