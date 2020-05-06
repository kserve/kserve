import kfserving
from typing import List, Union
import numpy as np


class Predictor(): # pylint:disable=too-few-public-methods
    def __init__(self, clf: kfserving.KFModel):
        self.clf = clf

    def predict_fn(self, arr: Union[np.ndarray, List]) -> np.ndarray:
        instances = []
        for req_data in arr:
            if isinstance(req_data, np.ndarray):
                instances.append(req_data.tolist())
            else:
                instances.append(req_data)
        resp = self.clf.predict({"instances": instances})
        return np.array(resp["predictions"])
