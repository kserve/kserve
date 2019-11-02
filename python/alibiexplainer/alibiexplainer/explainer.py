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
from typing import List, Any, Mapping, Union, Dict
import argparse

import kfserving
import numpy as np
from alibiexplainer.anchor_images import AnchorImages
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_text import AnchorText
from kfserving.utils import NumpyEncoder

logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "AnchorTabular"
    anchor_images = "AnchorImages"
    anchor_text = "AnchorText"

    def __str__(self):
        return self.value


class AlibiExplainer(kfserving.KFModel):
    def __init__(self,
                 parser: argparse.ArgumentParser,
                 name: str,
                 predictor_host: str,
                 method: ExplainerMethod,
                 config: Dict,
                 explainer: object = None):
        super().__init__(name)
        self.parser = parser
        self.predictor_host = predictor_host
        logging.info("Predict URL set to %s", self.predictor_host)
        self.method = method
        self.config = config

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
        instances = []
        for req_data in arr:
            if isinstance(req_data, np.ndarray):
                instances.append(req_data.tolist())
            else:
                instances.append(req_data)
        resp = self.predict({"instances": instances})
        return np.array(resp["predictions"])

    def parse_headers(self, headers: List) -> Dict:
        argsStr = '--predictor_host x ' + str(self.method)
        for key,value in headers:
            if key.startswith("Alibi-"):
                argName = '_'.join(key.lower().split("-")[1:])
                argsStr += " --"+argName + " " + value
        try:
            args, _ = self.parser.parse_known_args(argsStr.split())
            argsDict = vars(args).copy()
            if 'explainer' in argsDict:
                extra = vars(args.explainer)
            else:
                extra = {}
            return extra
        except Exception as inst:
            logging.error("Failed to parse extra args: %s",inst)
            return {}

    def explain(self, request: Dict, headers: List = []) -> Any:
        requestArgs = self.parse_headers(headers)
        logging.info("Request Args: %s",requestArgs)
        if self.method is ExplainerMethod.anchor_tabular or self.method is ExplainerMethod.anchor_images or self.method is ExplainerMethod.anchor_text:
            explanation = self.wrapper.explain(request["instances"], requestArgs)
            logging.info("Explanation: %s", explanation)
            return json.loads(json.dumps(explanation, cls=NumpyEncoder))
        else:
            raise NotImplementedError
