# Copyright 2020 kubeflow.org.
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

from typing import List, Optional
from kfserving import KFModel

MODEL_MOUNT_DIRS = "/mnt/models"


class KFModelRepository:
    """
    Model repository interface, follows NVIDIA Triton's `model-repository`
    extension.
    """

    def __init__(self, models_dir: str = MODEL_MOUNT_DIRS):
        self.models = {}
        self.models_dir = models_dir

    def set_models_dir(self, models_dir):  # used for unit tests
        self.models_dir = models_dir

    def get_model(self, name: str) -> Optional[KFModel]:
        return self.models.get(name, None)

    def get_models(self) -> List[KFModel]:
        return list(self.models.values())

    def is_model_ready(self, name: str):
        model = self.get_model(name)
        return False if model is None else model.ready

    def update(self, model: KFModel):
        self.models[model.name] = model

    def load(self, name: str) -> bool:
        pass

    def unload(self, name: str):
        if name in self.models:
            del self.models[name]
        else:
            raise KeyError(f"model {name} does not exist")
