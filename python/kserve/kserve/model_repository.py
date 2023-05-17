# Copyright 2022 The KServe Authors.
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
from typing import Dict, Optional

from .model import KServeModel

MODEL_MOUNT_DIRS = "/mnt/models"


class ModelRepository:
    """Model repository interface.

    It follows NVIDIA Triton's `Model Repository Extension`_.

    .. _Model Repository Extension:
        https://github.com/triton-inference-server/server/blob/main/docs/protocol/extension_model_repository.md
    """

    def __init__(self, models_dir: str = MODEL_MOUNT_DIRS):
        self.models: Dict[str, KServeModel] = {}
        self.models_dir = models_dir

    def load_models(self):
        for name in os.listdir(self.models_dir):
            d = os.path.join(self.models_dir, name)
            if os.path.isdir(d):
                self.load_model(name)

    def set_models_dir(self, models_dir):  # used for unit tests
        self.models_dir = models_dir

    def get_model(self, name: str) -> Optional[KServeModel]:
        return self.models.get(name, None)

    def get_models(self) -> Dict[str, KServeModel]:
        return self.models

    def is_model_ready(self, name: str):
        model = self.get_model(name)
        return False if model is None else model.ready

    def update(self, model: KServeModel):
        self.models[model.name] = model

    def load(self, name: str) -> bool:
        pass

    def load_model(self, name: str) -> bool:
        pass

    def unload(self, name: str):
        if name in self.models:
            del self.models[name]
        else:
            raise KeyError(f"model {name} does not exist")
