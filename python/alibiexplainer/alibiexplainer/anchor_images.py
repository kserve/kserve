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
from typing import Callable, List, Dict

import alibi
import kfserving
import numpy as np
from alibiexplainer.explainer_wrapper import ExplainerWrapper

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class AnchorImages(ExplainerWrapper):

    def __init__(self, predict_fn: Callable, explainer: alibi.explainers.AnchorImage, **kwargs):
        self.predict_fn = predict_fn
        self.anchors_image = explainer
        self.kwargs = kwargs

    def explain(self, inputs: List) -> Dict:
        if not self.anchors_image is None:
            arr = np.array(inputs)
            logging.info("Calling explain on image of shape %s", (arr.shape,))

            # set anchor_images predict function so it always returns predicted class
            # See anchor_images.__init__
            if np.argmax(self.predict_fn(arr).shape) == 0:
                self.anchors_image.predict_fn = self.predict_fn
            else:
                self.anchors_image.predict_fn = lambda x: np.argmax(self.predict_fn(x), axis=1)
            # We assume the input has batch dimension but Alibi explainers presently assume no batch
            np.random.seed(0)
            anchor_exp = self.anchors_image.explain(arr[0], **self.kwargs)
            return anchor_exp
        else:
            raise Exception("Explainer not initialized")
