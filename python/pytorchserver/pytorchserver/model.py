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

import kserve
import os
from typing import Dict
import torch
import importlib
import sys

PYTORCH_FILE = "model.pt"


class PyTorchModel(kserve.Model):
    def __init__(self, name: str, model_class_name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_class_name = model_class_name
        self.model_dir = model_dir
        self.ready = False
        self.model = None
        self.device = torch.device('cuda:0' if torch.cuda.is_available() else 'cpu')

    def load(self) -> bool:
        model_file_dir = kserve.Storage.download(self.model_dir, self.name)
        model_file = os.path.join(model_file_dir, PYTORCH_FILE)
        py_files = []
        for filename in os.listdir(model_file_dir):
            if filename.endswith('.py'):
                py_files.append(filename)
        if len(py_files) == 1:
            model_class_file = os.path.join(model_file_dir, py_files[0])
        elif len(py_files) == 0:
            raise Exception('Missing PyTorch Model Class File.')
        else:
            raise Exception('More than one Python file is detected',
                            'Only one Python file is allowed within model_dir.')
        model_class_name = self.model_class_name

        # Load the python class into memory
        sys.path.append(os.path.dirname(model_class_file))
        modulename = os.path.basename(model_class_file).split('.')[0].replace('-', '_')
        model_class = getattr(importlib.import_module(modulename), model_class_name)

        # Make sure the model weight is transform with the right device in this machine
        self.model = model_class().to(self.device)
        self.model.load_state_dict(torch.load(model_file, map_location=self.device))
        self.model.eval()
        self.ready = True
        return self.ready

    def predict(self, request: Dict) -> Dict:
        inputs = []
        with torch.no_grad():
            try:
                inputs = torch.tensor(request["instances"]).to(self.device)
            except Exception as e:
                raise TypeError(
                    "Failed to initialize Torch Tensor from inputs: %s, %s" % (e, inputs))
            try:
                return {"predictions":  self.model(inputs).tolist()}
            except Exception as e:
                raise Exception("Failed to predict %s" % e)
