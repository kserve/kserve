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
from kserve.model_repository import ModelRepository, MODEL_MOUNT_DIRS
from lgbserver import LightGBMModel


class LightGBMModelRepository(ModelRepository):
    def __init__(self, model_dir: str = MODEL_MOUNT_DIRS, nthread: int = 1):
        super().__init__(model_dir)
        self.nthread = nthread
        self.load_models()

    async def load(self, name: str) -> bool:
        return self.load_model(name)

    def load_model(self, name: str) -> bool:
        model = LightGBMModel(name, os.path.join(self.models_dir, name), self.nthread)
        if model.load():
            self.update(model)
        return model.ready
