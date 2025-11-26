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
import pathlib
from typing import Dict, Union

from kserve.errors import InferenceError, ModelMissingError
from kserve.storage import Storage

import joblib
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_input, get_predict_response
from kserve import Model

MODEL_EXTENSIONS = (".joblib", ".pkl", ".pickle")
ENV_PREDICT_PROBA = "PREDICT_PROBA"


class SKLearnModel(Model):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self) -> bool:
        model_path = pathlib.Path(Storage.download(self.model_dir))
        model_files = []
        for file in os.listdir(model_path):
            file_path = os.path.join(model_path, file)
            if os.path.isfile(file_path) and file.endswith(MODEL_EXTENSIONS):
                model_files.append(model_path / file)
        if len(model_files) == 0:
            raise ModelMissingError(model_path)
        elif len(model_files) > 1:
            raise RuntimeError(
                "More than one model file is detected, "
                f"Only one is allowed within model_dir: {model_files}"
            )
        self._model = joblib.load(model_files[0])
        self.ready = True
        return self.ready

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            instances = get_predict_input(payload)
            if os.environ.get(ENV_PREDICT_PROBA, "false").lower() == "true" and hasattr(
                self._model, "predict_proba"
            ):
                result = self._model.predict_proba(instances)
            else:
                result = self._model.predict(instances)
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
