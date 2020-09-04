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

import inspect
from typing import List, Optional
from kfserving import KFModel
from kfserving.kfmodel_factory import KFModelFactory
from kfserving.kfmodels.kfmodel_types import get_kfmodel_type

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

    def update(self, model: KFModel):
        self.models[model.name] = model

    async def load(self, name: str) -> bool:
        model_type, model_full_path = get_kfmodel_type(name, self.models_dir)

        model = KFModelFactory.create_model(name, self.models_dir, model_type)
        model.set_full_model_path(model_full_path)

        if inspect.iscoroutinefunction(model.load):
            await model.load()
        else:
            model.load()

        self.update(model)
        return model.ready

    def unload(self, name: str):
        if name in self.models:
            del self.models[name]
        else:
            raise KeyError(f"model {name} does not exist")
