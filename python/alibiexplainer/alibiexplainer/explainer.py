# Copyright 2021 The KServe Authors.
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
import asyncio
from enum import Enum
from typing import List, Any, Mapping, Union, Dict

import kserve
import numpy as np
from alibiexplainer.anchor_images import AnchorImages
from alibiexplainer.anchor_tabular import AnchorTabular
from alibiexplainer.anchor_text import AnchorText
from alibiexplainer.explainer_wrapper import ExplainerWrapper

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


class ExplainerMethod(Enum):
    anchor_tabular = "AnchorTabular"
    anchor_images = "AnchorImages"
    anchor_text = "AnchorText"

    def __str__(self):
        return self.value


class AlibiExplainer(kserve.KFModel):
    def __init__(  # pylint:disable=too-many-arguments
        self,
        name: str,
        predictor_host: str,
        method: ExplainerMethod,
        config: Mapping,
        explainer: object = None,
    ):
        super().__init__(name)
        self.predictor_host = predictor_host
        logging.info("Predict URL set to %s", self.predictor_host)
        self.method = method

        if self.method is ExplainerMethod.anchor_tabular:
            self.wrapper: ExplainerWrapper = AnchorTabular(
                self._predict_fn, explainer, **config
            )
        elif self.method is ExplainerMethod.anchor_images:
            self.wrapper = AnchorImages(self._predict_fn, explainer, **config)
        elif self.method is ExplainerMethod.anchor_text:
            self.wrapper = AnchorText(self._predict_fn, explainer, **config)
        else:
            raise NotImplementedError

    def load(self) -> bool:
        pass

    def _predict_fn(self, arr: Union[np.ndarray, List]) -> np.ndarray:
        instances = []
        for req_data in arr:
            if isinstance(req_data, np.ndarray):
                instances.append(req_data.tolist())
            else:
                instances.append(req_data)
        loop = asyncio.get_running_loop()  # type: ignore
        resp = loop.run_until_complete(self.predict({"instances": instances}))
        return np.array(resp["predictions"])

    def explain(self, request: Dict) -> Any:
        if (
            self.method is ExplainerMethod.anchor_tabular
            or self.method is ExplainerMethod.anchor_images
            or self.method is ExplainerMethod.anchor_text
        ):
            explanation = self.wrapper.explain(request["instances"])
            explanationAsJsonStr = explanation.to_json()
            logging.info("Explanation: %s", explanationAsJsonStr)
            return json.loads(explanationAsJsonStr)

        raise NotImplementedError
