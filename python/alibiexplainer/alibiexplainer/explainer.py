import json
import logging
from enum import Enum
from typing import List, Any, Mapping, Union

import kfserving
import kfserving.protocols.seldon_http as seldon
import numpy as np
import requests
from alibiexplainer.anchor_images import AnchorImages
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_text import AnchorText
from kfserving.protocols.seldon_http import SeldonRequestHandler
from kfserving.protocols.util import NumpyEncoder
from kfserving.server import Protocol, TENSORFLOW_PREDICTOR_URL_FORMAT, SELDON_PREDICTOR_URL_FORMAT

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "anchor_tabular"
    anchor_images = "anchor_images"
    anchor_text = "anchor_text"

    def __str__(self):
        return self.value


class AlibiExplainer(kfserving.KFModel):
    def __init__(self,
                 name: str,
                 predictor_host: str,
                 protocol: Protocol,
                 method: ExplainerMethod,
                 config: Mapping,
                 explainer: object = None):
        super().__init__(name)
        self.protocol = protocol
        if self.protocol == Protocol.tensorflow_http:
            self.predict_url = TENSORFLOW_PREDICTOR_URL_FORMAT.format(predictor_host, name)
        else:
            self.predict_url = SELDON_PREDICTOR_URL_FORMAT.format(predictor_host)
        logging.info("Predict URL set to %s",self.predict_url)
        self.method = method

        if self.method is ExplainerMethod.anchor_tabular:
            self.wrapper = AnchorTabular(self._predict_fn, explainer, **config)
        elif self.method is ExplainerMethod.anchor_images:
            self.wrapper = AnchorImages(self._predict_fn, explainer, **config)
        elif self.method is ExplainerMethod.anchor_text:
            self.wrapper = AnchorText(self._predict_fn, explainer, **config)
        else:
            raise NotImplementedError

    def load(self):
        pass

    def _predict_fn(self, arr: Union[np.ndarray, List]) -> np.ndarray:
        if self.protocol == Protocol.seldon_http:
            payload = seldon.create_request(arr, seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                rh = SeldonRequestHandler(response_raw.json())
                response_list = rh.extract_request()
                return np.array(response_list)
            else:
                raise Exception(
                    "Failed to get response from model return_code:%d" % response_raw.status_code)
        elif self.protocol == Protocol.tensorflow_http:
            data = []
            for req_data in arr:
                if isinstance(req_data, np.ndarray):
                    data.append(req_data.tolist())
                else:
                    data.append(str(req_data))
            payload = {"instances": data}
            logging.info("Calling model with %d instances",len(data))
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
        if self.method is ExplainerMethod.anchor_tabular or self.method is ExplainerMethod.anchor_images or self.method is ExplainerMethod.anchor_text:
            explanation = self.wrapper.explain(inputs)
            logging.info("Explanation: %s",explanation)
            return json.loads(json.dumps(explanation, cls=NumpyEncoder))
        else:
            raise NotImplementedError
