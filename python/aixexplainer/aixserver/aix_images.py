from aixserver.explainer_wrapper import ExplainerWrapper
from typing import Callable, Dict
import logging
import numpy as np
from aix360.algorithms.lime import LimeImageExplainer
from lime.wrappers.scikit_image import SegmentationAlgorithm
# import inspect


class LimeImage(ExplainerWrapper):
    def __init__(
        self,
        _predict: Callable,
        **kwargs
    ):
        self._predict = _predict
        self.kwargs = kwargs

    def explain(self, request: Dict) -> Dict:
        inputs = np.array(request['instances'][0])
        # if have custom parameter
        if len(request) > 1:
            self.kwargs = {**self.kwargs, **request}

        logging.info(
            "Calling explain on image of shape %s", (inputs.shape,))
        explainer = LimeImageExplainer(verbose=False)

        if "segmentation_algorithm" not in self.kwargs:
            self.kwargs["seg_algo"] = "quickshift"
        segmenter = SegmentationAlgorithm(self.kwargs["seg_algo"], kernel_size=1,
                                          max_dist=200, ratio=0.2)

        explanation = explainer.explain_instance(inputs,
                                                 classifier_fn=self._predict,
                                                 hide_color=0,
                                                 segmentation_fn=segmenter,
                                                 top_labels=self.kwargs["top_labels"],)

        temp = []
        masks = []
        for i in range(0, self.kwargs['top_labels']):
            temp, mask = explanation.get_image_and_mask(explanation.top_labels[i],
                                                        num_features=10,
                                                        hide_rest=False,
                                                        min_weight=self.kwargs['min_weight'])
            masks.append(mask.tolist())

        return {"explanations": {
            "temp": temp.tolist(),
            "masks": masks,
            "top_labels": np.array(explanation.top_labels).astype(np.int32).tolist()
        }}
