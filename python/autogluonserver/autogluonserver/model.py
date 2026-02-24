# Copyright 2025 The KServe Authors.
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

import os
from typing import Dict, List, Union

import numpy as np
import pandas as pd
from autogluon.tabular import TabularPredictor

from kserve import Model
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve_storage import Storage

ENV_PREDICT_PROBA = "PREDICT_PROBA"


def _tensor_to_dataframe(instances, predictor) -> pd.DataFrame:
    """Build a DataFrame with model feature names from v2 tensor input (ndarray or DataFrame with integer columns)."""
    if isinstance(instances, np.ndarray):
        if not hasattr(predictor, "features") or not predictor.features:
            return pd.DataFrame(instances)
        arr = instances
        if arr.ndim == 1:
            arr = arr.reshape(1, -1)
        n_features = len(predictor.features)
        if arr.shape[1] != n_features:
            raise InferenceError(
                f"v2 tensor has {arr.shape[1]} columns but model expects {n_features} features "
                f"(order: {predictor.features}). Send data in row-major order matching GET /v2/models/{{name}} inputs."
            )
        return pd.DataFrame(arr, columns=predictor.features)

    if isinstance(instances, pd.DataFrame):
        cols = instances.columns.tolist()
        # Integer column names 0,1,...,n-1 from v2 path without column semantics
        if (
            hasattr(predictor, "features")
            and predictor.features
            and len(cols) == len(predictor.features)
            and all(isinstance(c, (int, np.integer)) for c in cols)
            and cols == list(range(len(cols)))
        ):
            df = instances.copy()
            df.columns = predictor.features
            return df
        return instances

    return pd.DataFrame(instances)


class AutoGluonModel(Model):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self) -> bool:
        model_path = Storage.download(self.model_dir)
        if not os.path.isdir(model_path):
            raise ModelMissingError(model_path)
        self._predictor = TabularPredictor.load(model_path)
        self.ready = True
        return self.ready

    def get_input_types(self) -> List[Dict]:
        """Return v2 model metadata inputs: one tensor per feature, in predictor.features order."""
        predictor = getattr(self, "_predictor", None)
        if predictor is None or not getattr(predictor, "features", None):
            return []
        # One entry per feature so clients know column order for v2 tensor payloads
        return [
            {"name": name, "datatype": "FP64", "shape": [-1]}
            for name in predictor.features
        ]

    def get_output_types(self) -> List[Dict]:
        """Return v2 model metadata outputs: single 'predictions' tensor (variable batch)."""
        predictor = getattr(self, "_predictor", None)
        if predictor is None:
            return []
        return [{"name": "predictions", "datatype": "BYTES", "shape": [-1]}]

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            instances = get_predict_input(payload)
            # v2 tensor input (ndarray or DataFrame with 0,1,2,... columns) -> map to model feature names
            if isinstance(instances, (np.ndarray, pd.DataFrame)) and getattr(
                self, "_predictor", None
            ):
                df = _tensor_to_dataframe(instances, self._predictor)
            elif isinstance(instances, pd.DataFrame):
                df = instances
            else:
                df = pd.DataFrame(instances)

            if (
                os.environ.get(ENV_PREDICT_PROBA, "false").lower() == "true"
                and hasattr(self._predictor, "predict_proba")
            ):
                result = self._predictor.predict_proba(df)
            else:
                result = self._predictor.predict(df)

            if isinstance(result, pd.Series):
                result = result.tolist()
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
