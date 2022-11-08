from aixserver.explainer_wrapper import ExplainerWrapper
from typing import Callable, Dict
from aix360.algorithms.lime import LimeTextExplainer
import logging


class LimeText(ExplainerWrapper):
    def __init__(
        self,
        _predict: Callable,
        **kwargs

    ):
        self._predict = _predict
        self.kwargs = kwargs

    def explain(self, request: Dict,) -> Dict:
        inputs = str(request['instances'][0])
        # if use custom parameter
        if len(request) > 1:
            request.pop('instances')
            self.kwargs = {**self.kwargs, **request}
        logging.info("Calling explain on text %s", (len(inputs),))
        explainer = LimeTextExplainer(verbose=False)
        explaination = explainer.explain_instance(inputs,
                                                  classifier_fn=self._predict,
                                                  **self.kwargs)
        m = explaination.as_map()
        exp = {str(k): explaination.as_list(int(k))
               for k, _ in m.items()}

        return {"explanations": exp}
