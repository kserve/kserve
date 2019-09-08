# Copyright 2019 kubeflow.org.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging
from typing import List
import requests
import numpy as np
from .server import Protocol
from .server import KFModel
from .server import KFSERVER_LOGLEVEL
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
        self.ready = False

    def load(self):
        if not self.ready:
            self.ready = True

    # subclass of Transformer should implement preprocess
    def preprocess(self, inputs: List) -> List:
        raise NotImplementedError

    # subclass of Transformer should implment postprocess
    def postprocess(self, inputs: List) -> List:
        raise NotImplementedError

    def predict(self, inputs: List) -> List:
        if self.protocol == Protocol.seldon_http:
            payload = seldon.create_request(np.array(inputs), seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                rh = SeldonRequestHandler(response_raw.json())
                response_list = rh.extract_request()
                return response_list
            else:
                raise Exception("Failed to get response from model return_code:%d" %
                                response_raw.status_code)
        elif self.protocol == Protocol.tensorflow_http:
            payload = {"instances": inputs}
            logging.info(payload.items())
            logging.info(self.predict_url)
            response_raw = requests.post(self.predict_url, json=payload)
            if response_raw.status_code == 200:
                logging.info(response_raw.json())
                return response_raw.json()["predictions"]
            else:
                raise Exception("Failed to get response from model return_code:%d" %
                                response_raw.status_code)
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> List:
        raise NotImplementedError

    def detect_outlier(self, inputs: List):
        raise NotImplementedError
