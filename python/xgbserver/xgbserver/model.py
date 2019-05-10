import kfserving
import xgboost as xgb
from xgboost import XGBModel
import os
from kfserving.protocols.tensorflow_http import tensorflow_request_to_list, ndarray_to_tensorflow_response
from kfserving.protocols.seldon_http import seldon_request_to_ndarray, ndarray_to_seldon_response
import numpy as np
from typing import Dict, Any

BOOSTER_FILE = "model.bst"


class XGBoostModel(kfserving.KFModel):
    def __init__(self, name: str, model_dir: str, protocol: str, booster: XGBModel = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.protocol = protocol
        if not booster is None:
            self._booster = booster
            self.ready = True

    def load(self):
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), BOOSTER_FILE)
        self._booster = xgb.Booster(model_file=model_file)
        self.ready = True

    def _extract_data(self, body: Dict) -> Any:
        if self.protocol == kfserving.server.TFSERVING_HTTP_PROTOCOL:
            return tensorflow_request_to_list(body)
        elif self.protocol == kfserving.server.SELDON_HTTP_PROTOCOL:
            return seldon_request_to_ndarray(body)

    def _create_response(self, request: Dict, prediction: np.ndarray) -> Dict:
        if self.protocol == kfserving.server.TFSERVING_HTTP_PROTOCOL:
            return ndarray_to_tensorflow_response(prediction)
        elif self.protocol == kfserving.server.SELDON_HTTP_PROTOCOL:
            return ndarray_to_seldon_response(request, prediction)
        else:
            raise Exception("Invalid protocol %s" % self.protocol)

    def predict(self, body: Dict) -> Dict:
        data = self._extract_data(body)
        try:
            result = self._booster.predict(data)
            response = self._create_response(body, result)
            return response
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
