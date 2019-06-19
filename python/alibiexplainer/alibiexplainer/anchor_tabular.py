from typing import Callable, List, Dict, Optional, Any
import kfserving
import logging
import os
import joblib
import alibi
from alibiexplainer.explainer_method import ExplainerMethodImpl
import numpy as np
import pandas as pd

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)


class AnchorTabular(ExplainerMethodImpl):

    def __init__(self, predict_fn: Callable):
        self.predict_fn = predict_fn
        self.cmap: Optional[Dict[Any, Any]] = None
        self.anchors_tabular: Optional[alibi.explainers.AnchorTabular] = None

    def validate(self, training_data_url: Optional[str]):
        if training_data_url is None:
            raise Exception("Anchor_tabular requires training data")

    def prepare(self, training_data_url: str):
        if not training_data_url is None:
            logging.info("Loading training file %s" % training_data_url)
            training_data_file = kfserving.Storage.download(training_data_url)
            training_data = joblib.load(training_data_file)
        else:
            raise Exception("Anchor_tabular requires training data")

        feature_names_url = os.environ.get("FEATURE_NAMES_URL")
        if not feature_names_url is None:
            logging.info("Loading feature names file %s" % feature_names_url)
            feature_names_file = kfserving.Storage.download(feature_names_url)
            feature_names = joblib.load(feature_names_file)
        else:
            feature_names = []

        categorical_map_url = os.environ.get("CATEGORICAL_MAP_URL")
        if not categorical_map_url is None:
            logging.info("Loading categorical map file %s" % categorical_map_url)
            categorical_map_file = kfserving.Storage.download(categorical_map_url)
            categorical_map = joblib.load(categorical_map_file)

            # reuse map for formatting output
            cmap = dict.fromkeys(categorical_map.keys())
            for key, val in categorical_map.items():
                cmap[key] = {i: v for i, v in enumerate(val)}
            self.cmap = cmap
        else:
            categorical_map = {}

        self.anchors_tabular = alibi.explainers.AnchorTabular(predict_fn=self.predict_fn,
                                                              feature_names=feature_names,
                                                              categorical_names=categorical_map)
        self.anchors_tabular.fit(training_data)

    def explain(self, inputs: List) -> Dict:
        if not self.anchors_tabular is None:
            arr = np.array(inputs)
            anchor_exp = self.anchors_tabular.explain(arr)
            if not self.cmap is None:
                # convert to interpretable raw features
                for i in range(len(anchor_exp['raw']['examples'])):
                    for key, arr in anchor_exp['raw']['examples'][i].items():
                        parr = pd.DataFrame(arr)
                        parr = parr.replace(self.cmap)
                        anchor_exp['raw']['examples'][i][key] = parr.values

                instance = anchor_exp['raw']['instance']
                anchor_exp['raw']['instance'] = pd.DataFrame(instance).replace(self.cmap).values
            return anchor_exp
        else:
            raise Exception("Explainer not initialized")
