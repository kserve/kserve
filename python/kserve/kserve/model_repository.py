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
from typing import Dict, Optional, Union

from ray.serve.handle import DeploymentHandle

from .model import BaseKServeModel

MODEL_MOUNT_DIRS = "/mnt/models"


class ModelRepository:
    """Model repository interface.

    It follows NVIDIA Triton's `Model Repository Extension`_.

    .. _Model Repository Extension:
        https://github.com/triton-inference-server/server/blob/main/docs/protocol/extension_model_repository.md
    """

    def __init__(self, models_dir: str = MODEL_MOUNT_DIRS):
        self.models: Dict[str, Union[BaseKServeModel, DeploymentHandle]] = {}
        self.models_dir = models_dir

    def load_models(self):
        for name in os.listdir(self.models_dir):
            d = os.path.join(self.models_dir, name)
            if os.path.isdir(d):
                self.load_model(name)

    def set_models_dir(self, models_dir):  # used for unit tests
        self.models_dir = models_dir

    def get_model(
        self, name: str
    ) -> Optional[Union[BaseKServeModel, DeploymentHandle]]:
        return self.models.get(name, None)

    def get_models(self) -> Dict[str, Union[BaseKServeModel, DeploymentHandle]]:
        return self.models

    def is_model_ready(self, name: str):
        model = self.get_model(name)
        if not model:
            return False
        if isinstance(model, BaseKServeModel):
            return model.healthy()
        else:
            # For Ray Serve, the models are guaranteed to be ready after deploying the model.
            return True

    def update(self, model: BaseKServeModel):
        self.models[model.name] = model

    def update_handle(self, name: str, model_handle: DeploymentHandle):
        self.models[name] = model_handle

    def load(self, name: str) -> bool:
        pass

    def load_model(self, name: str) -> bool:
        pass

    def unload(self, name: str):
        if name in self.models:
            model = self.models[name]
            if callable(getattr(model, "stop", None)):
                model.stop()
            del self.models[name]
        else:
            raise KeyError(f"model {name} does not exist")
