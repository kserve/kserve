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
from typing import Dict, Union

import pandas as pd
from autogluon.tabular import TabularPredictor

from kserve import Model
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve_storage import Storage

ENV_PREDICT_PROBA = "PREDICT_PROBA"


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

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            instances = get_predict_input(payload)
            if isinstance(instances, pd.DataFrame):
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
