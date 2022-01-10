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
from .model_repository import ModelRepository, MODEL_MOUNT_DIRS
from sklearnserver import SKLearnModel


class SKLearnModelRepository(ModelRepository):

    def __init__(self, model_dir: str = MODEL_MOUNT_DIRS):
        super().__init__(model_dir)

    async def load(self, name: str) -> bool:
        model = SKLearnModel(name, os.path.join(self.models_dir, name))
        if model.load():
            self.update(model)
        return model.ready
