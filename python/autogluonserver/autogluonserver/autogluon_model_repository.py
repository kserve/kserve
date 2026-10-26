# Copyright 2026 The KServe Authors.
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
from kserve.model_repository import ModelRepository, MODEL_MOUNT_DIRS

from autogluonserver.predictor_factory import create_autogluon_model


class AutoGluonModelRepository(ModelRepository):
    def __init__(self, model_dir: str = MODEL_MOUNT_DIRS):
        super().__init__(model_dir)
        self.load_models()

    async def load(self, name: str) -> bool:
        return self.load_model(name)

    def _validate_model_path(self, name: str) -> str:
        """Validate model name to prevent path traversal attacks."""
        if not name or not name.strip():
            raise ValueError("Model name cannot be empty")

        if os.path.isabs(name):
            raise ValueError(f"Model name cannot be an absolute path: {name}")

        models_dir_real = os.path.realpath(self.models_dir)
        candidate_path = os.path.join(models_dir_real, name)
        candidate_path_real = os.path.realpath(candidate_path)

        try:
            common_path = os.path.commonpath([models_dir_real, candidate_path_real])
        except ValueError:
            raise ValueError(f"Model path traversal detected: {name}")

        if common_path != models_dir_real:
            raise ValueError(f"Model path traversal detected: {name}")

        return candidate_path_real

    def load_model(self, name: str) -> bool:
        model_path = self._validate_model_path(name)
        model = create_autogluon_model(name, model_path)
        if model.load():
            self.update(model)
        return model.ready
