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

import kfserving
import numpy as np
import os
from typing import List, Any
import torch
from torch.autograd import Variable
import importlib
import shutil
import sys

PYTORCH_FILE = "model.pt"

class PyTorchModel(kfserving.KFModel):
    def __init__(self, name: str, model_class_name: str, model_class_file: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_class_name = model_class_name
        self.model_class_file = model_class_file
        self.model_dir = model_dir
        self.ready = False

    def load(self):
        model_file = os.path.join(
            kfserving.Storage.download(self.model_dir), PYTORCH_FILE)
        model_class_file = os.path.join(
            kfserving.Storage.download(self.model_dir), self.model_class_file)
        model_class_name = self.model_class_name

        # Load the python class into memory
        sys.path.append(os.path.dirname(model_class_file))
        modulename = os.path.basename(model_class_file).split('.')[0].replace('-', '_')
        model_class = getattr(importlib.import_module(modulename), model_class_name)

        # Make sure the model weight is transform with the right device in this machine
        device = torch.device('cuda:0' if torch.cuda.is_available() else 'cpu')
        self._pytorch = model_class().to(device)
        self._pytorch.load_state_dict(torch.load(model_file, map_location=device))
        self._pytorch.eval()
        self.ready = True

    def predict(self, body: List) -> List:
        try:
            inputs = torch.FloatTensor(body)
        except Exception as e:
            raise Exception(
                "Failed to initialize Torch Tensor from inputs: %s, %s" % (e, inputs))
        try:
            result = self._pytorch(inputs).tolist()
            return result
        except Exception as e:
            raise Exception("Failed to predict %s" % e)
