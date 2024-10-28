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

import inspect
import sys
from typing import Dict, List, Optional

from ..errors import ModelNotFound, ModelNotReady
from ..model_repository import ModelRepository


class ModelRepositoryExtension:
    """
    This is a class implements the 'Model Repository Extension' to the kserve Protocol V2 as
    described in 'https://github.com/triton-inference-server/server/blob/main/docs/protocol
    /extension_model_repository.md#model-repository-extension'

    Attributes:
        model_registry (ModelRepository): Backing model store
    """

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry

    async def index(self, filter_ready: Optional[bool] = False) -> List[Dict[str, str]]:
        """Returns information about every model available in a model repository.

        Args:
            filter_ready: When set True, the function returns only the models that are ready

        Returns:
            List[Dict[str, str]]: list with metadata for models as below:

                {
                    name: model_name,
                    state: "Ready" or "NotReady"
                    reason: ""
                }
        """
        model_list = []
        for model_name in self._model_registry.get_models().keys():
            model_ready = await self._model_registry.is_model_ready(model_name)
            if model_ready or not filter_ready:
                # If model is ready or filter_ready is set to False
                model_list.append(
                    {
                        "name": model_name,
                        "state": ("Ready" if model_ready else "NotReady"),
                        "reason": "",
                    }
                )

        return model_list

    async def load(self, model_name: str) -> None:
        """Loads the specified model.

        Args:
            model_name (str): name of the model to load.

        Returns: None

        Raises:
            ModelNotReady: Exception if model loading fails.
        """
        try:
            # For backward compatibility, the synchronous `load` has been kept here.
            if inspect.iscoroutinefunction(self._model_registry.load):
                await self._model_registry.load(model_name)
            else:
                self._model_registry.load(model_name)
        except Exception:
            ex_type, ex_value, ex_traceback = sys.exc_info()
            raise ModelNotReady(
                model_name, f"Error type: {ex_type} error msg: {ex_value}"
            )
        is_ready = await self._model_registry.is_model_ready(model_name)
        if not is_ready:
            raise ModelNotReady(model_name)

    async def unload(self, model_name: str) -> None:
        """Unload the specified model.

        Args:
            model_name (str): Name of the model to unload.

        Returns: None

        Raises:
            ModelNotFound: Exception if the requested model is not found.
        """
        try:
            self._model_registry.unload(model_name)
        except KeyError:
            raise ModelNotFound(model_name)
