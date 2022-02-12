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

import kserve
import joblib
import pathlib
from typing import Dict
from kserve.model import ModelMissingError, InferenceError

MODEL_BASENAME = "model"
MODEL_EXTENSIONS = [".joblib", ".pkl", ".pickle"]
ENV_PREDICT_PROBA = "PREDICT_PROBA"


class SKLearnModel(kserve.Model):  # pylint:disable=c-extension-no-member
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False

    def load(self) -> bool:
        model_path = pathlib.Path(kserve.Storage.download(self.model_dir))
        paths = [model_path / (MODEL_BASENAME + model_extension) for model_extension in MODEL_EXTENSIONS]
        existing_paths = [path for path in paths if path.exists()]
        if len(existing_paths) == 0:
            raise ModelMissingError(model_path)
        elif len(existing_paths) > 1:
            raise RuntimeError('More than one model file is detected, '
                               f'Only one is allowed within model_dir: {existing_paths}')
        self._model = joblib.load(existing_paths[0])
        self.ready = True
        return self.ready

    def predict(self, request: Dict) -> Dict:
        instances = request["instances"]
        try:
            if os.environ.get(ENV_PREDICT_PROBA, "false").lower() == "true" and \
                    hasattr(self._model, "predict_proba"):
                result = self._model.predict_proba(instances).tolist()
            else:
                result = self._model.predict(instances).tolist()
            return {"predictions": result}
        except Exception as e:
            raise InferenceError(str(e))
