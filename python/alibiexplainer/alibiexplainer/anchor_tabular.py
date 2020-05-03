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
from typing import Callable, List, Dict, Optional, Any

import alibi
import joblib
import kfserving
import numpy as np
from alibiexplainer.explainer_wrapper import ExplainerWrapper
from alibi.api.interfaces import Explanation
from alibi.utils.wrappers import ArgmaxTransformer

logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)

class AnchorTabular(ExplainerWrapper):

    def __init__(self, predict_fn: Callable, explainer=Optional[alibi.explainers.AnchorTabular],
                 **kwargs):
        if explainer is None:
            raise Exception("Anchor images requires a built explainer")
        self.predict_fn = predict_fn
        self.cmap: Optional[Dict[Any, Any]] = None
        self.anchors_tabular: alibi.explainers.AnchorTabular = explainer
        self.anchors_tabular = explainer
        #self._reuse_cat_map(self.anchors_tabular.categorical_names)
        self.kwargs = kwargs

    def _reuse_cat_map(self, categorical_map: Dict):
        # reuse map for formatting output
        cmap = dict.fromkeys(categorical_map.keys())
        for key, val in categorical_map.items():
            cmap[key] = {i: v for i, v in enumerate(val)}
        self.cmap = cmap

    def explain(self, inputs: List) -> Explanation:
        arr = np.array(inputs)
        # set anchor_tabular predict function so it always returns predicted class
        # See anchor_tablular.__init__
        logging.info("Arr shape %s ", (arr.shape,))

        # check if predictor returns predicted class or prediction probabilities for each class
        # if needed adjust predictor so it returns the predicted class
        if np.argmax(self.predict_fn(arr).shape) == 0:
            self.anchors_tabular.predictor = self.predict_fn
            self.anchors_tabular.samplers[0].predictor = self.predict_fn
        else:
            self.anchors_tabular.predictor = ArgmaxTransformer(self.predict_fn)
            self.anchors_tabular.samplers[0].predictor = ArgmaxTransformer(self.predict_fn)

        # We assume the input has batch dimension but Alibi explainers presently assume no batch
        anchor_exp = self.anchors_tabular.explain(arr[0], **self.kwargs)
        return anchor_exp

