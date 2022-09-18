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
from typing import Dict

import kserve
from jpmml_evaluator import make_evaluator
from jpmml_evaluator.py4j import launch_gateway, Py4JBackend

from kserve.model import ModelMissingError

MODEL_EXTENSIONS = ('.pmml')


class PmmlModel(kserve.Model):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.ready = False
        self.evaluator = None
        self.input_fields = []
        self._gateway = None
        self._backend = None

    def load(self) -> bool:
        model_path = kserve.Storage.download(self.model_dir)
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
        self._gateway = launch_gateway()
        self._backend = Py4JBackend(self._gateway)
        self.evaluator = make_evaluator(
            self._backend, model_files[0]).verify()
        self.input_fields = [inputField.getName()
                             for inputField in self.evaluator.getInputFields()]
        self.ready = True
        return self.ready

    def predict(self, payload: Dict, headers: Dict[str, str] = None) -> Dict:
        instances = payload["instances"]
        try:
            result = [self.evaluator.evaluate(dict(zip(self.input_fields, instance))) for instance in instances]
            return {"predictions": result}
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
