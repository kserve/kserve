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

import catboost as cb
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response

from kserve import Model
from kserve_storage import Storage

MODEL_EXTENSIONS = (".cbm", ".bin")


class CatBoostModel(Model):
    def __init__(self, name: str, model_dir: str, model: cb.CatBoost = None):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        if model is not None:
            self._model = model
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

        # If multiple model files exist, prefer .cbm over .bin
        if len(model_files) > 1:
            cbm_files = [f for f in model_files if f.endswith(".cbm")]
            if cbm_files:
                model_files = [cbm_files[0]]  # Use first .cbm file
            else:
                model_files = [model_files[0]]  # Use first available file

        self._model = cb.CatBoost()
        self._model.load_model(model_files[0])
        self.ready = True
        return self.ready

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            instances = get_predict_input(payload)
            result = self._model.predict(instances)
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
