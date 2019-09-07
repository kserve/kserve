import logging
from typing import List
import requests
import numpy as np

from kfserving.server import Protocol
from kfserving.server import KFModel
from kfserving.server import KFSERVER_LOGLEVEL
import kfserving.protocols.seldon_http as seldon
from kfserving.protocols.seldon_http import SeldonRequestHandler

logging.basicConfig(level=KFSERVER_LOGLEVEL)


class Transformer(KFModel):
    def __init__(self, name: str,
                 predict_url: str,
                 protocol: Protocol):
        super().__init__(name)
        self.predict_url = predict_url
        self.protocol = protocol

    def load(self):
        if not self.ready:
            logging.info("Loading transformer")
            self.ready = True

    def preprocess(self, inputs: List) -> List:
        return inputs

    def postprocess(self, inputs: List) -> List:
        return inputs

    def predict(self, inputs: List) -> List:
        if self.protocol == Protocol.seldon_http:
            payload = seldon.create_request(np.array(inputs), seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                rh = SeldonRequestHandler(response_raw.json())
                response_list = rh.extract_request()
                return response_list
            else:
                raise Exception("Failed to get response from model return_code:%d" % response_raw.status_code)
        elif self.protocol == Protocol.tensorflow_http:
            payload = {"instances": inputs}
            logging.info(payload.items())
            logging.info(self.predict_url)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                logging.info(response_raw.json())
                return response_raw.json()["predictions"]
            else:
                raise Exception("Failed to get response from model return_code:%d" % response_raw.status_code)
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> List:
        raise NotImplementedError

    def detect_outlier(self, inputs: List):
        raise NotImplementedError
