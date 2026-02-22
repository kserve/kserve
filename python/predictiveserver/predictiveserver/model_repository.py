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

from kserve.model_repository import MODEL_MOUNT_DIRS, ModelRepository

from .model import PredictiveServerModel


class PredictiveServerModelRepository(ModelRepository):
    """
    Model repository for multi-model serving with Predictive Server.

    Supports dynamic loading of models from different ML frameworks.
    """

    def __init__(
        self,
        model_dir: str = MODEL_MOUNT_DIRS,
        framework: str = "sklearn",
        nthread: int = 1,
    ):
        """
        Initialize the Predictive Server model repository.

        Args:
            model_dir: Base directory containing models
            framework: Default ML framework to use (sklearn, xgboost, lightgbm)
            nthread: Number of threads for XGBoost and LightGBM (default: 1)
        """
        super().__init__(model_dir)
        self.framework = framework
        self.nthread = nthread
        self.load_models()

    async def load(self, name: str) -> bool:
        """
        Load a model asynchronously.

        Args:
            name: Model name

        Returns:
            True if model loaded successfully
        """
        return self.load_model(name)

    def load_model(self, name: str) -> bool:
        """
        Load a model synchronously.

        Args:
            name: Model name

        Returns:
            True if model loaded successfully
        """
        model = PredictiveServerModel(
            name, os.path.join(self.models_dir, name), self.framework, self.nthread
        )
        if model.load():
            self.update(model)
        return model.ready
