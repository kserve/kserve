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
import pandas as pd
from alibiexplainer.explainer_wrapper import ExplainerWrapper

logging.basicConfig(level=kfserving.constants.KFSERVING_LOGLEVEL)


class AnchorTabular(ExplainerWrapper):

    def __init__(self, predict_fn: Callable, explainer=Optional[alibi.explainers.AnchorTabular],
                 **kwargs):
        self.predict_fn = predict_fn
        self.cmap: Optional[Dict[Any, Any]] = None
        self.anchors_tabular: Optional[alibi.explainers.AnchorTabular] = explainer
        if self.anchors_tabular is None:
            self.prepare(**kwargs)
        else:  # Overwrite predict_fn
            self._reuse_cat_map(self.anchors_tabular.categorical_names)
        self.kwargs = kwargs

    def _reuse_cat_map(self, categorical_map: Dict):
        # reuse map for formatting output
        cmap = dict.fromkeys(categorical_map.keys())
        for key, val in categorical_map.items():
            cmap[key] = {i: v for i, v in enumerate(val)}
        self.cmap = cmap

    def prepare(self, training_data_url=None, feature_names_url=None, categorical_map_url=None,
                **kwargs):
        if not training_data_url is None:
            logging.info("Loading training file %s" % training_data_url)
            training_data_file = kfserving.Storage.download(training_data_url)
            training_data = joblib.load(training_data_file)
        else:
            raise Exception("Anchor_tabular requires training data")

        if not feature_names_url is None:
            logging.info("Loading feature names file %s" % feature_names_url)
            feature_names_file = kfserving.Storage.download(feature_names_url)
            feature_names = joblib.load(feature_names_file)
        else:
            raise Exception("Anchor_tabular requires feature names")

        if not categorical_map_url is None:
            logging.info("Loading categorical map file %s" % categorical_map_url)
            categorical_map_file = kfserving.Storage.download(categorical_map_url)
            categorical_map = joblib.load(categorical_map_file)
            self._reuse_cat_map(categorical_map)
        else:
            categorical_map = {}

        logging.info("Creating AnchorTabular")
        self.anchors_tabular = alibi.explainers.AnchorTabular(predict_fn=self.predict_fn,
                                                              feature_names=feature_names,
                                                              categorical_names=categorical_map)
        logging.info("Fitting AnchorTabular")
        self.anchors_tabular.fit(training_data)

    def explain(self, inputs: List, **kwargs) -> Dict:
        if not self.anchors_tabular is None:
            arr = np.array(inputs)
            # set anchor_tabular predict function so it always returns predicted class
            # See anchor_tablular.__init__
            logging.info("Arr shape %s ", (arr.shape,))
            if np.argmax(self.predict_fn(arr).shape) == 0:
                self.anchors_tabular.predict_fn = self.predict_fn
            else:
                self.anchors_tabular.predict_fn = lambda x: np.argmax(self.predict_fn(x), axis=1)
            args = {**self.kwargs, **kwargs}
            logging.info("Calling AnchorTabular with parameters %s", args)
            # We assume the input has batch dimension but Alibi explainers presently assume no batch
            anchor_exp = self.anchors_tabular.explain(arr[0], **args)
            if not self.cmap is None:
                # convert to interpretable raw features
                for i in range(len(anchor_exp['raw']['examples'])):
                    for key, arr in anchor_exp['raw']['examples'][i].items():
                        parr = pd.DataFrame(arr)
                        parr = parr.replace(self.cmap)
                        anchor_exp['raw']['examples'][i][key] = parr.values

                instance = anchor_exp['raw']['instance']
                anchor_exp['raw']['instance'] = pd.DataFrame([instance]).replace(self.cmap).values.squeeze()
            return anchor_exp
        else:
            raise Exception("Explainer not initialized")
