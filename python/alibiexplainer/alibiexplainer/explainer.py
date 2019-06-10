import kfserving
from enum import Enum
from typing import List, Any
import numpy as np
import kfserving.protocols.seldon_http as seldon
from kfserving.protocols.seldon_http import SeldonRequestHandler
import requests
import json
import logging
from alibiexplainer.anchor_tabular import AnchorTabular

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "anchor_tabular"


class AlibiExplainer(kfserving.KFExplainer):
    def __init__(self,
                 model_url: str,
                 protocol: str,
                 method: ExplainerMethod,
                 training_data_url: str = None):
        super().__init__()
        self.model_url = model_url
        self.protocol = protocol
        self.method = method
        self.training_data_uri = training_data_url
        self.training_data = None
        if self.method is ExplainerMethod.anchor_tabular:
            self.explainer = AnchorTabular(self._predict_fn)
        else:
            raise NotImplementedError

    def load(self):
        logging.info("Loading explainer")
        self.explainer.prepare()
        self.ready = True

    def _predict_fn(self, arr: np.ndarray) -> np.ndarray:
        if self.protocol == "seldon.http":
            payload = seldon.create_request(arr, seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.model_url, json=payload)
            if response_raw.status_code == 200:
                rh = SeldonRequestHandler(response_raw.json())
                response_list = rh.extract_request()
                return np.array(response_list)
            else:
                raise Exception("Failed to get response from model return_code:%d" % response_raw.status_code)
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> Any:
        if self.method is ExplainerMethod.anchor_tabular:
            explaination = self.explainer.explain(inputs)
            return json.loads(json.dumps(explaination, cls=NumpyEncoder))
        else:
            raise NotImplementedError


class NumpyEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, (
                np.int_, np.intc, np.intp, np.int8, np.int16, np.int32, np.int64, np.uint8, np.uint16, np.uint32,
                np.uint64)):
            return int(obj)
        elif isinstance(obj, (np.float_, np.float16, np.float32, np.float64)):
            return float(obj)
        elif isinstance(obj, (np.ndarray,)):
            return obj.tolist()
        return json.JSONEncoder.default(self, obj)
