import kfserving
import joblib 
import numpy as np
import os
from typing import List, Any

JOBLIB_FILE = "model.joblib"

class SKLearnModel(kfserving.KFModel):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = os.path.join(
        kfserving.Storage.download(self.model_dir),JOBLIB_FILE)
        self._joblib = joblib.load(model_file)
        self.ready = True

    def predict(self, body: List) -> List:
        try:
            inputs = np.array(body)
        except Exception as e:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (e, inputs))
        try:
            result = self._joblib.predict(inputs).tolist()
            return result
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
