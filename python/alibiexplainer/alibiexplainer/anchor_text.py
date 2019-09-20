import logging
from typing import Callable, List, Dict

import kfserving
import numpy as np
import spacy
import alibi
from alibi.utils.download import spacy_model
from alibiexplainer.explainer_wrapper import ExplainerWrapper

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class AnchorText(ExplainerWrapper):

    def __init__(self, predict_fn: Callable, explainer: alibi.explainers.AnchorText,
                 spacy_language_model: str = 'en_core_web_md', **kwargs):
        self.predict_fn = predict_fn
        self.kwargs = kwargs
        logging.info("Anchor Text args %s", self.kwargs)
        if explainer is None:
            logging.info("Loading Spacy Language model for %s", spacy_language_model)
            spacy_model(model=spacy_language_model)
            self.nlp = spacy.load(spacy_language_model)
            logging.info("Language model loaded")
        self.anchors_text = explainer

    def explain(self, inputs: List) -> Dict:
        if self.anchors_text is None:
            self.anchors_text = alibi.explainers.AnchorText(self.nlp, self.predict_fn)
        # We assume the input has batch dimension but Alibi explainers presently assume no batch
        np.random.seed(0)
        anchor_exp = self.anchors_text.explain(inputs[0], **self.kwargs)
        return anchor_exp
