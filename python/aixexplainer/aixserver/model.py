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
from typing import Dict

import asyncio
import logging
import kfserving
import numpy as np
from aix360.algorithms.lime import LimeImageExplainer
from lime.wrappers.scikit_image import SegmentationAlgorithm


class AIXModel(kfserving.KFModel):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, predictor_host: str, segm_alg: str, num_samples: str,
                 top_labels: str, min_weight: str, positive_only: str, explainer_type: str):
        super().__init__(name)
        self.name = name
        self.top_labels = int(top_labels)
        self.num_samples = int(num_samples)
        self.segmentation_alg = segm_alg
        self.predictor_host = predictor_host
        self.min_weight = float(min_weight)
        self.positive_only = (positive_only.lower() == "true") | (positive_only.lower() == "t")
        if str.lower(explainer_type) != "limeimages":
            raise Exception("Invalid explainer type: %s" % explainer_type)
        self.explainer_type = explainer_type
        self.ready = False

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def _predict(self, input_im):
        scoring_data = {'instances': input_im.tolist()}

        loop = asyncio.get_running_loop()
        resp = loop.run_until_complete(self.predict(scoring_data))
        return np.array(resp["predictions"])

    def explain(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            inputs = np.array(instances[0])
            logging.info("Calling explain on image of shape %s", (inputs.shape,))
        except Exception as err:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (err, instances))
        try:
            if str.lower(self.explainer_type) == "limeimages":
                explainer = LimeImageExplainer(verbose=False)
                segmenter = SegmentationAlgorithm(self.segmentation_alg, kernel_size=1,
                                                  max_dist=200, ratio=0.2)
                explanation = explainer.explain_instance(inputs,
                                                         classifier_fn=self._predict,
                                                         top_labels=self.top_labels,
                                                         hide_color=0,
                                                         num_samples=self.num_samples,
                                                         segmentation_fn=segmenter)

                temp = []
                masks = []
                for i in range(0, self.top_labels):
                    temp, mask = explanation.get_image_and_mask(explanation.top_labels[i],
                                                                positive_only=self.positive_only,
                                                                num_features=10,
                                                                hide_rest=False,
                                                                min_weight=self.min_weight)
                    masks.append(mask.tolist())

                return {"explanations": {
                    "temp": temp.tolist(),
                    "masks": masks,
                    "top_labels": np.array(explanation.top_labels).astype(np.int32).tolist()
                }}

        except Exception as err:
            raise Exception("Failed to explain %s" % err)
