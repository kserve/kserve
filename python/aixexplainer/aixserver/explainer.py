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

from enum import Enum
import logging
from typing import Dict, Mapping

import asyncio
import kserve
import numpy as np
from aixserver.explainer_wrapper import ExplainerWrapper
from aixserver.aix_images import LimeImage
from aixserver.aix_text import LimeText
from aixserver.aix_tabular import LimeTabular
import nest_asyncio
nest_asyncio.apply()

expMethod = {"LimeImages": LimeImage,
             "LimeTexts": LimeText, "LimeTabular": LimeTabular}


class ExplainerMethod(Enum):
    lime_tabular = "LimeTabular"
    lime_images = "LimeImages"
    lime_text = "LimeTexts"

    def __str__(self):
        return self.value


class AIXModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(
        self,
        name: str,
        predictor_host: str,
        method: ExplainerMethod,
        config: Mapping
    ):
        super().__init__(name)
        self.name = name
        self.predictor_host = predictor_host
        self.ready = False
        self.method = method
        if method in ExplainerMethod:
            self.wrapper: ExplainerWrapper = expMethod[str(method)](
                self._predict, **config)
        else:
            raise Exception("Invalid explainer type")

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def _predict(self, input_im):
        scoring_data = {'instances': input_im.tolist()if type(
            input_im) != list else input_im}

        loop = asyncio.get_running_loop()
        resp = loop.run_until_complete(self.predict(scoring_data))
        logging.info("perdict: %s", resp["predictions"])
        return np.array(resp["predictions"])

    def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        explanation = self.wrapper.explain(payload)
        return explanation
