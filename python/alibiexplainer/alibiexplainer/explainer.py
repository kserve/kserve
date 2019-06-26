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
from kfserving.server import Protocol
from kfserving.protocols.util import NumpyEncoder
from alibiexplainer.explainer_method import ExplainerMethodImpl

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "anchor_tabular"


class AlibiExplainer(kfserving.KFModel):
    def __init__(self,
                 name: str,
                 predict_url: str,
                 protocol: Protocol,
                 method: ExplainerMethod,
                 training_data_url: str = None):
        super().__init__(name)
        self.predict_url = predict_url
        self.protocol = protocol
        self.method = method
        self.training_data_url = training_data_url
        if self.method is ExplainerMethod.anchor_tabular:
            self.explainer: ExplainerMethodImpl = AnchorTabular(self._predict_fn)
            self.explainer.validate(self.training_data_url)
        else:
            raise NotImplementedError

    def load(self):
        if not self.ready:
            logging.info("Loading explainer")
            self.explainer.prepare(self.training_data_url)
            self.ready = True

    def _predict_fn(self, arr: np.ndarray) -> np.ndarray:
        if self.protocol == Protocol.seldon_http:
            payload = seldon.create_request(arr, seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.predict_url, json=payload)
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
