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

from typing import Dict, Optional, Union
from kserve import KFModel
from ray.serve.api import RayServeHandle

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

    def get_model(self, name: str) -> Optional[Union[KFModel, RayServeHandle]]:
        return self.models.get(name, None)

    def get_models(self) -> Dict[str, Union[KFModel, RayServeHandle]]:
        return self.models

    def is_model_ready(self, name: str):
        model = self.get_model(name)
        if not model:
            return False
        if isinstance(model, KFModel):
            return False if model is None else model.ready
        else:
            # For Ray Serve, the models are guaranteed to be ready after the deploy call.
            return True

    def update(self, model: KFModel):
        self.models[model.name] = model

    def update_handle(self, name: str, model_handle: RayServeHandle):
        self.models[name] = model_handle

    def load(self, name: str) -> bool:
        pass

    def unload(self, name: str):
        if name in self.models:
            del self.models[name]
        else:
            raise KeyError(f"model {name} does not exist")
