import kfserving
from joblib import load
import numpy as np
import os


class SKLearnModel(kfserving.KFModel):
    def __init__(self, name, model_dir):
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = kfserving.Storage.download(self.model_dir)
        self._model = load(model_file)
        self.ready = True

    def preprocess(self, inputs):
        try:
            return np.array(inputs)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, inputs))

    def postprocess(self, outputs):
        return outputs.tolist()

    def predict(self, inputs):
        try:
            result = self._model.predict(inputs)
            return result
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
