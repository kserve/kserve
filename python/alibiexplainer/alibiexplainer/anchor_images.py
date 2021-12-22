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
import kserve
import logging
import numpy as np
import alibi
from alibi.api.interfaces import Explanation
from alibi.utils.wrappers import ArgmaxTransformer
from alibiexplainer.explainer_wrapper import ExplainerWrapper
from typing import Callable, List, Optional

logging.basicConfig(level=kserve.constants.KSERVE_LOGLEVEL)


class AnchorImages(ExplainerWrapper):
    def __init__(
        self,
        predict_fn: Callable,
        explainer: Optional[alibi.explainers.AnchorImage],
        **kwargs
    ):
        if explainer is None:
            raise Exception("Anchor images requires a built explainer")
        self.predict_fn = predict_fn
        self.anchors_image = explainer
        self.kwargs = kwargs

    def explain(self, inputs: List) -> Explanation:
        arr = np.array(inputs)
        # check if predictor returns predicted class or prediction probabilities for each class
        # if needed adjust predictor so it returns the predicted class
        if np.argmax(self.predict_fn(arr).shape) == 0:
            self.anchors_image.predictor = self.predict_fn
        else:
            self.anchors_image.predictor = ArgmaxTransformer(self.predict_fn)
        logging.info("Calling explain on image of shape %s", (arr.shape,))
        logging.info("anchor image call with %s", self.kwargs)
        anchor_exp = self.anchors_image.explain(arr[0], **self.kwargs)
        return anchor_exp
