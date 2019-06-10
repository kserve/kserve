import kfserving
from enum import Enum
from typing import List, Any
import numpy as np
import alibi
import kfserving.protocols.seldon_http as seldon
from kfserving.protocols.seldon_http import SeldonRequestHandler
import requests
import pandas as pd
import json
import joblib
import logging

logging.basicConfig(level=kfserving.server.KFSERVER_LOGLEVEL)

class ExplainerAlgorithm(Enum):
    anchors = "anchors"

class ExplainerModelType(Enum):
    classification = "classification"

class ExplainerModelFeatures(Enum):
    tabular = "tabular"
    text = "text"
    image = "image"

class ModelProtocol(Enum):
    tensorflow_http = 'tensorflow.http'
    seldon_http = 'seldon.http'

class AlibiExplainer(kfserving.KFExplainer):
    def __init__(self,
                 model_url: str,
                 protocol: str,
                 algorithm: ExplainerAlgorithm,
                 model_type: ExplainerModelType,
                 model_features: ExplainerModelFeatures,
                 training_data_uri: str = None,
                 feature_names_uri: str = None,
                 categorical_map_uri: str = None):
        super().__init__()
        self.model_url = model_url
        self.protocol = protocol
        self.algorithm = algorithm
        self.model_type = model_type
        self.model_features = model_features
        self.training_data_uri = training_data_uri
        self.training_data = None
        self.feature_names_uri = feature_names_uri
        self.feature_names = None
        self.categorical_map_uri = categorical_map_uri
        self.categorical_map = None
        self.cmap = None
        self.anchors_tabular: alibi.explainers.AnchorTabular = None
        self.validate()

    def load(self):
        logging.info("Loading explainer")
        if not self.training_data_uri is None:
            logging.info("Loading training file %s" % self.training_data_uri)
            training_data_file = kfserving.Storage.download(self.training_data_uri)
            self.training_data = joblib.load(training_data_file)
        else:
            self.training_data = None
        if not self.feature_names_uri is None:
            logging.info("Loading feature names file %s" % self.feature_names_uri)
            feature_names_file = kfserving.Storage.download(self.feature_names_uri)
            self.feature_names = joblib.load(feature_names_file)
        else:
            self.feature_names = None
        if not self.categorical_map_uri is None:
            logging.info("Loading categorical map file %s" % self.categorical_map_uri)
            categorical_map_file = kfserving.Storage.download(self.categorical_map_uri)
            self.categorical_map = joblib.load(categorical_map_file)
            print(self.categorical_map)
            # reuse map for formatting output
            cmap = dict.fromkeys(self.categorical_map.keys())
            for key, val in self.categorical_map.items():
                cmap[key] = {i: v for i, v in enumerate(val)}
            self.cmap = cmap
        else:
            self.categorical_map = None

        if self.algorithm is ExplainerAlgorithm.anchors:
            self.anchors_tabular = alibi.explainers.AnchorTabular(predict_fn=self._predict_fn,
                                       feature_names=self.feature_names,
                                       categorical_names=self.categorical_map)
            if not self.training_data is None:
                self.anchors_tabular.fit(self.training_data)

        self.ready = True

    def _predict_fn(self, arr: np.ndarray) -> np.ndarray:
        if self.protocol == "seldon.http":
            payload = seldon.create_request(arr,seldon.SeldonPayload.NDARRAY)
            response_raw = requests.post(self.model_url,json=payload)
            if response_raw.status_code == 200:
                rh = SeldonRequestHandler(response_raw.json())
                response_list = rh.extract_request()
                return np.array(response_list)
            else:
                raise Exception("Failed to get response from model return_code:%d" % response_raw.status_code)
        else:
            raise NotImplementedError

    def validate(self):
        print(self.algorithm)
        print(self.model_type)

        if self.algorithm is ExplainerAlgorithm.anchors and \
                self.model_type is ExplainerModelType.classification and \
                self.model_features is ExplainerModelFeatures.tabular:
            return
        else:
            raise Exception("Invalid Alibi settings algorithm:%s modelType:%s modelFeatures:%s" % (self.algorithm, self.model_type, self.model_features))

    def explain(self, inputs: List) -> Any:
        if self.algorithm is ExplainerAlgorithm.anchors and \
                self.model_type is ExplainerModelType.classification and \
                self.model_features is ExplainerModelFeatures.tabular:
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

                # TODO string representation for now
                # convert arrays to lists for valid json
            return json.dumps(anchor_exp, cls=NumpyEncoder)
        else:
            raise NotImplementedError


class NumpyEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, (
        np.int_, np.intc, np.intp, np.int8, np.int16, np.int32, np.int64, np.uint8, np.uint16, np.uint32, np.uint64)):
            return int(obj)
        elif isinstance(obj, (np.float_, np.float16, np.float32, np.float64)):
            return float(obj)
        elif isinstance(obj, (np.ndarray,)):
            return obj.tolist()
        return json.JSONEncoder.default(self, obj)
