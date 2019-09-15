import logging
from typing import Callable, List, Dict, Mapping, Tuple

import alibi
import kfserving
import numpy as np
import spacy
from alibiexplainer.explainer_wrapper import ExplainerWrapper
from alibi.utils.download import spacy_model

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)

SPACY_LM = 'spacy_language_model'
THRESHOLD = 'threshold'
DELTA = 'delta'
TAU = 'tau'
BATCH_SIZE = 'batch_size'
TOPN = 'top_n'
DESIRED_LABEL = 'desired_label'
USE_SIMILARITY_PROBA = 'use_similarity_proba'
USE_UNK = 'use_unk'
SAMPLE_PROBA = 'sample_proba'
TEMPERATURE = 'temperature'

class AnchorText(ExplainerWrapper):

    def __init__(self, predict_fn: Callable, explainer: alibi.explainers.AnchorText,
                 config: Mapping):
        (self.args_clazz,self.args_explain)= self.prepare_args(config)
        logging.info("Anchor Text clazz args %s",self.args_clazz)
        logging.info("Anchor Text explain args: %s",self.args_explain)
        self.predict_fn = predict_fn
        if explainer is None:
            logging.info("Loading Spacy Language model for %s", self.args_clazz[SPACY_LM])
            spacy_model(model=self.args_clazz[SPACY_LM])
            self.nlp = spacy.load(self.args_clazz[SPACY_LM])
            logging.info("Language model loaded")
        self.anchors_text = explainer

    def prepare_args(self,config: Mapping) -> Tuple[Dict,Dict]:
        args_clazz = {SPACY_LM: 'en_core_web_md'}
        args_explain = {}
        if SPACY_LM in config:
            args_clazz[SPACY_LM] = config[SPACY_LM]
        if USE_UNK in config:
            args_explain[USE_UNK] = config[USE_UNK] == 'true'
        if THRESHOLD in config:
            args_explain[THRESHOLD] = float(config[THRESHOLD])
        if DELTA in config:
            args_explain[DELTA] = float(config[DELTA])
        if TAU in config:
            args_explain[TAU] = float(config[TAU])
        if BATCH_SIZE in config:
            args_explain[BATCH_SIZE] = int(config[BATCH_SIZE])
        if TOPN in config:
            args_explain[TOPN] = int(config[TOPN])
        if DESIRED_LABEL in config:
            args_explain[DESIRED_LABEL] = int(config[DESIRED_LABEL])
        if USE_SIMILARITY_PROBA in config:
            args_explain[USE_SIMILARITY_PROBA] = config[USE_SIMILARITY_PROBA] == 'true'
        if SAMPLE_PROBA in config:
            args_explain[SAMPLE_PROBA] = float(config[SAMPLE_PROBA])
        if TEMPERATURE in config:
            args_explain[TEMPERATURE] = float(config[TEMPERATURE])

        return args_clazz,args_explain

    def explain(self, inputs: List) -> Dict:
        if self.anchors_text is None:
            self.anchors_text = alibi.explainers.AnchorText(self.nlp, self.predict_fn)
        # We assume the input has batch dimension but Alibi explainers presently assume no batch
        np.random.seed(0)
        anchor_exp = self.anchors_text.explain(inputs[0],**self.args_explain)
        return anchor_exp
