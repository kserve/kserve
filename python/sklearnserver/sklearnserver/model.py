import kfserving
import joblib 
import numpy as np
import os

class SKLearnModel(kfserving.KFModel):
    def __init__(self, name, model_dir):
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = kfserving.Storage.download(self.model_dir)
        self._joblib = joblib.load(model_file)
        self.ready = True

    def predict(self, inputs):
        try:
            inputs = np.array(inputs)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, inputs))
        try:
            result = self._joblib.predict(inputs).tolist()
            return result
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
