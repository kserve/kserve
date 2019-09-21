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
import json
import logging
from enum import Enum
from typing import List, Any, Mapping, Union

import kfserving
import numpy as np
from alibiexplainer.anchor_images import AnchorImages
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_text import AnchorText
from kfserving.protocols.seldon_http import SeldonRequestHandler
from kfserving.protocols.tensorflow_http import TensorflowRequestHandler
from kfserving.protocols.util import NumpyEncoder
from kfserving.server import Protocol, PREDICTOR_URL_FORMAT

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)

SELDON_PREDICTOR_URL_FORMAT = "http://{0}/api/v0.1/predictions"


class ExplainerMethod(Enum):
    anchor_tabular = "AnchorTabular"
    anchor_images = "AnchorImages"
    anchor_text = "AnchorText"

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
            self.predict_url = PREDICTOR_URL_FORMAT.format(predictor_host, name)
        else:
            self.predict_url = SELDON_PREDICTOR_URL_FORMAT.format(predictor_host)
        logging.info("Predict URL set to %s", self.predict_url)
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
            resp = SeldonRequestHandler.predict(arr, self.predict_url)
            return np.array(resp)
        elif self.protocol == Protocol.tensorflow_http:
            inputs = []
            for req_data in arr:
                if isinstance(req_data, np.ndarray):
                    inputs.append(req_data.tolist())
                else:
                    inputs.append(str(req_data))
            resp = TensorflowRequestHandler.predict(inputs, self.predict_url)
            return np.array(resp)
        else:
            raise NotImplementedError

    def explain(self, inputs: List) -> Any:
        if self.method is ExplainerMethod.anchor_tabular or self.method is ExplainerMethod.anchor_images or self.method is ExplainerMethod.anchor_text:
            explanation = self.wrapper.explain(inputs)
            logging.info("Explanation: %s", explanation)
            return json.loads(json.dumps(explanation, cls=NumpyEncoder))
        else:
            raise NotImplementedError
