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


class AnchorTabular(ExplainerWrapper):
    def __init__(
        self,
        predict_fn: Callable,
        explainer=Optional[alibi.explainers.AnchorTabular],
        **kwargs
    ):
        if explainer is None:
            raise Exception("Anchor images requires a built explainer")
        self.predict_fn = predict_fn
        self.anchors_tabular: alibi.explainers.AnchorTabular = explainer
        self.anchors_tabular = explainer
        self.kwargs = kwargs

    def explain(self, inputs: List) -> Explanation:
        arr = np.array(inputs)
        # set anchor_tabular predict function so it always returns predicted class
        # See anchor_tabular.__init__
        logging.info("Arr shape %s ", (arr.shape,))

        # check if predictor returns predicted class or prediction probabilities for each class
        # if needed adjust predictor so it returns the predicted class
        if np.argmax(self.predict_fn(arr).shape) == 0:
            self.anchors_tabular.predictor = self.predict_fn
            self.anchors_tabular.samplers[0].predictor = self.predict_fn
        else:
            self.anchors_tabular.predictor = ArgmaxTransformer(self.predict_fn)
            self.anchors_tabular.samplers[0].predictor = ArgmaxTransformer(
                self.predict_fn
            )

        # We assume the input has batch dimension but Alibi explainers presently assume no batch
        anchor_exp = self.anchors_tabular.explain(arr[0], **self.kwargs)
        return anchor_exp
