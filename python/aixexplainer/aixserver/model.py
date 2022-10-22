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
from typing import Dict

import asyncio
import logging
import kserve
import numpy as np
from aix360.algorithms.lime import LimeImageExplainer
from lime.wrappers.scikit_image import SegmentationAlgorithm
from aix360.algorithms.lime import LimeTextExplainer
import nest_asyncio
nest_asyncio.apply()


class AIXModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, predictor_host: str, segm_alg: str, num_samples: str,
                 top_labels: str, min_weight: str, positive_only: str, explainer_type: str):
        super().__init__(name)
        self.name = name
        self.top_labels = int(top_labels)
        self.num_samples = int(num_samples)
        self.segmentation_alg = segm_alg
        self.predictor_host = predictor_host
        self.min_weight = float(min_weight)
        self.positive_only = (positive_only.lower() == "true") | (
            positive_only.lower() == "t")
        if str.lower(explainer_type) != "limeimages" and str.lower(explainer_type) != "limetexts":
            raise Exception("Invalid explainer type: %s" % explainer_type)
        self.explainer_type = explainer_type
        self.ready = False

    def load(self) -> bool:
        self.ready = True
        return self.ready

    def _predict(self, input_im):
        scoring_data = {'instances': input_im.tolist()if type(
            input_im) != list else input_im}

        loop = asyncio.get_running_loop()
        resp = loop.run_until_complete(self.predict(scoring_data))
        return np.array(resp["predictions"])

    def explain(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        instances = payload["instances"]
        try:
            top_labels = (int(payload["top_labels"])
                          if "top_labels" in payload else
                          self.top_labels)
            segmentation_alg = (payload["segmentation_alg"]
                                if "segmentation_alg" in payload else
                                self.segmentation_alg)
            num_samples = (int(payload["num_samples"])
                           if "num_samples" in payload else
                           self.num_samples)
            positive_only = ((payload["positive_only"].lower() == "true") | (payload["positive_only"].lower() == "t")
                             if "positive_only" in payload else
                             self.positive_only)
            min_weight = (float(payload['min_weight'])
                          if "min_weight" in payload else
                          self.min_weight)
        except Exception as err:
            raise Exception("Failed to specify parameters: %s", (err,))

        try:
            if str.lower(self.explainer_type) == "limeimages":
                inputs = np.array(instances[0])
                logging.info(
                    "Calling explain on image of shape %s", (inputs.shape,))
            elif str.lower(self.explainer_type) == "limetexts":
                inputs = str(instances[0])
                logging.info("Calling explain on text %s", (len(inputs),))
        except Exception as err:
            raise Exception(
                "Failed to initialize NumPy array from inputs: %s, %s" % (err, instances))
        try:
            if str.lower(self.explainer_type) == "limeimages":
                explainer = LimeImageExplainer(verbose=False)
                segmenter = SegmentationAlgorithm(segmentation_alg, kernel_size=1,
                                                  max_dist=200, ratio=0.2)
                explanation = explainer.explain_instance(inputs,
                                                         classifier_fn=self._predict,
                                                         top_labels=top_labels,
                                                         hide_color=0,
                                                         num_samples=num_samples,
                                                         segmentation_fn=segmenter)

                temp = []
                masks = []
                for i in range(0, top_labels):
                    temp, mask = explanation.get_image_and_mask(explanation.top_labels[i],
                                                                positive_only=positive_only,
                                                                num_features=10,
                                                                hide_rest=False,
                                                                min_weight=min_weight)
                    masks.append(mask.tolist())

                return {"explanations": {
                    "temp": temp.tolist(),
                    "masks": masks,
                    "top_labels": np.array(explanation.top_labels).astype(np.int32).tolist()
                }}
            elif str.lower(self.explainer_type) == "limetexts":
                explainer = LimeTextExplainer(verbose=False)
                explaination = explainer.explain_instance(inputs,
                                                          classifier_fn=self._predict,
                                                          top_labels=top_labels)
                m = explaination.as_map()
                exp = {str(k): explaination.as_list(int(k))
                       for k, _ in m.items()}

                return {"explanations": exp}

        except Exception as err:
            raise Exception("Failed to explain %s" % err)
