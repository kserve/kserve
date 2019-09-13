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
from kfserving.server import Protocol
from kfserving.protocols.util import NumpyEncoder


logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "anchor_tabular"

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
            self.wrapper = AnchorTabular(self._predict_fn,explainer,**config)
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
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> Any:
        if self.method is ExplainerMethod.anchor_tabular:
            explaination = self.wrapper.explain(inputs)
            return json.loads(json.dumps(explaination, cls=NumpyEncoder))
        else:
            raise NotImplementedError
