import kfserving
import xgboost as xgb
import os
import logging

BOOSTER_FILE = "model.bst"


class XGBoostModel(kfserving.KFModel):
    def __init__(self, name, model_dir):
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), BOOSTER_FILE)
        self._booster = xgb.Booster(model_file=model_file)
        self.ready = True

    def preprocess(self, inputs):
        try:
            return xgb.DMatrix(inputs)
        except Exception as e:
            raise Exception(
                "Failed to initialize DMatrix from inputs: %s, %s" % (e, inputs))

    def postprocess(self, outputs):
        return list(outputs)

    def predict(self, inputs):
        try:
            result = self._booster.predict(inputs)
            return result
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
