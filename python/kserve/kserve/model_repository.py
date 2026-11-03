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

from .model import BaseKServeModel

MODEL_MOUNT_DIRS = "/mnt/models"


class ModelRepository:
    """Model repository interface.

    It follows NVIDIA Triton's `Model Repository Extension`_.

    .. _Model Repository Extension:
        https://github.com/triton-inference-server/server/blob/main/docs/protocol/extension_model_repository.md
    """

    def __init__(self, models_dir: str = MODEL_MOUNT_DIRS):
        self.models: Dict[str, BaseKServeModel] = {}
        self.models_dir = models_dir

    def load_models(self):
        for name in os.listdir(self.models_dir):
            d = os.path.join(self.models_dir, name)
            if os.path.isdir(d):
                self.load_model(name)

    def set_models_dir(self, models_dir):  # used for unit tests
        self.models_dir = models_dir

    def get_model(self, name: str) -> Optional[BaseKServeModel]:
        return self.models.get(name, None)

    def get_models(self) -> Dict[str, BaseKServeModel]:
        return self.models

    async def is_model_ready(self, name: str):
        model = self.get_model(name)
        if not model:
            return False
        return await model.healthy()

    def update(self, model: BaseKServeModel, name: Optional[str] = None):
        """
        Update or add a model to the repository.
        Args:
            model (BaseKServeModel): The model to be added or updated in the repository.
            name (Optional[str], optional): The name to use for the model.
                If not provided, the model's own name attribute will be used. Defaults to None.
                This can be used to provide additional names for the same model.
        Returns:
            None
        """

        if name:
            self.models[name] = model
        else:
            self.models[model.name] = model

    def load(self, name: str) -> bool:
        pass

    def load_model(self, name: str) -> bool:
        pass

    def unload(self, name: str):
        if name in self.models:
            model = self.models[name]
            if callable(getattr(model, "stop", None)):
                model.stop()
            if model.engine:
                model.stop_engine()
            del self.models[name]
        else:
            raise KeyError(f"model with name {name} does not exist")
