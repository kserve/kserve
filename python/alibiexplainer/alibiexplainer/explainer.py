import kfserving
from enum import Enum
from typing import List, Any, Dict, Mapping, Optional
import numpy as np
import kfserving.protocols.seldon_http as seldon
from kfserving.protocols.seldon_http import SeldonRequestHandler
import requests
import json
import logging
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_images import AnchorImages
from kfserving.server import Protocol
from kfserving.protocols.util import NumpyEncoder


logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "anchor_tabular"
    anchor_images = "anchor_images"

    def __str__(self):
        return self.value


class AlibiExplainer(kfserving.KFModel):
    def __init__(self,
                 name: str,
                 predict_url: str,
                 protocol: Protocol,
                 method: ExplainerMethod,
                 config: Mapping,
                 explainer: object = None):
        super().__init__(name)
        self.predict_url = predict_url
        self.protocol = protocol
        self.method = method

        if self.method is ExplainerMethod.anchor_tabular:
            self.wrapper = AnchorTabular(self._predict_fn, explainer, **config)
        elif self.method is ExplainerMethod.anchor_images:
            self.wrapper = AnchorImages(self._predict_fn, explainer, **config)
        else:
            raise NotImplementedError

    def load(self):
        pass

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
        elif self.protocol == Protocol.tensorflow_http:
            logging.info("shape is %s" % (arr.shape,))
            data = []
            for req_data in arr:
                logging.info("Adding data shape %s" % (req_data.shape,))
                data.append(req_data.tolist())
            logging.info("Data length %s" % len(data))
            payload = {"instances": data}
            logging.info("Predict url is %s" % self.predict_url)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                j_resp = response_raw.json()
                return np.array(j_resp['predictions'])
            else:
                raise Exception(
                    "Failed to get response from model return_code:%d" % response_raw.status_code)
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> Any:
        if self.method is ExplainerMethod.anchor_tabular or self.method is ExplainerMethod.anchor_images:
            explaination = self.wrapper.explain(inputs)
            return json.loads(json.dumps(explaination, cls=NumpyEncoder))
        else:
            raise NotImplementedError
