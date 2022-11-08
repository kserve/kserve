import inspect
from aixserver.explainer_wrapper import ExplainerWrapper
from aix360.algorithms.lime import LimeTabularExplainer
from typing import Callable, Dict
import numpy as np
import logging


class LimeTabular(ExplainerWrapper):
    def __init__(self, _predict_fn: Callable, **kwargs):
        self._predict = _predict_fn
        self.kwargs = kwargs

    def explain(self, request: Dict,) -> Dict:
        inputs = np.array(request['instances'][0])
        try:
            training_data = np.array(request["training_data"])
        except Exception as err:
            raise Exception("Failed to specify training set: %s", (err,))
        try:
            logging.info("Calling explain on tabular %s", (inputs.shape),)
            logging.info("Training data %s", (training_data.shape))
        except Exception as err:
            logging.info("Calling explain on tabular failed: %s", (err,))
        try:
            tabluar_args = {k: v for k, v in request.items()
                            if k in inspect.getfullargspec(LimeTabularExplainer).args}

            explainer = LimeTabularExplainer(training_data,
                                             discretize_continuous=True,
                                             **tabluar_args)
            exp_args = {k: v for k, v in request.items()
                        if k in inspect.getfullargspec(explainer.explain_instance).args}

            explanation = explainer.explain_instance(inputs,
                                                     predict_fn=self._predict,
                                                     **exp_args)
            m = explanation.as_map()
            exp = {str(k): explanation.as_list(int(k))
                   for k, _ in m.items()}
            html = str(explanation.as_html())
            return {"explanations": exp,
                    "html": html}
        except Exception as err:
            raise Exception("Failed to explain %s" % err)
