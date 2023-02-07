# Copyright 2021 The KServe Authors.
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

import lightgbm as lgb
import pandas as pd
from kserve.errors import InferenceError, ModelMissingError
from kserve.storage import Storage

from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from lightgbm import Booster

import kserve

MODEL_EXTENSIONS = (".bst")


class LightGBMModel(kserve.Model):
    def __init__(self, name: str, model_dir: str, nthread: int,
                 booster: Booster = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.nthread = nthread
        if booster is not None:
            self._booster = booster
            self.ready = True

    def load(self) -> bool:
        model_path = Storage.download(self.model_dir)
        model_files = []
        for file in os.listdir(model_path):
            file_path = os.path.join(model_path, file)
            if os.path.isfile(file_path) and file.endswith(MODEL_EXTENSIONS):
                model_files.append(file_path)
        if len(model_files) == 0:
            raise ModelMissingError(model_path)
        elif len(model_files) > 1:
            raise RuntimeError('More than one model file is detected, '
                               f'Only one is allowed within model_dir: {model_files}')
        self._booster = lgb.Booster(params={"nthread": self.nthread},
                                    model_file=model_files[0])
        self.ready = True
        return self.ready

    def predict(self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None) -> Union[Dict, InferResponse]:
        try:
            dfs = []
            results = []
            instances = get_predict_input(payload)
            # NOTE: mixed type input needs to be dataframe conversion for lightgbm prediction
            if isinstance(instances[0][0], Dict):
                dfs = [pd.DataFrame(
                        input, columns=self._booster.feature_name()) for input in instances[0]]
                inputs = pd.concat(dfs, axis=0)
                results = [self._booster.predict(inputs)]
            else:
                results = [self._booster.predict(instance) for instance in instances]
            return get_predict_response(payload, results, self.name)

        except Exception as e:
            raise InferenceError(str(e))
