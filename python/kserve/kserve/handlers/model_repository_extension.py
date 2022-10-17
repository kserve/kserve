import inspect
import sys
from typing import Dict, List

from kserve.errors import ModelNotFound, ModelNotReady
from kserve import ModelRepository


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

    def index(self, filter_ready=False) -> List[Dict[str, str]]:
        """
        Returns information about every model available in a model repository.

        Args:
            filter_ready: When set True, the function returns only the models that are ready

        Returns:
            A list with metadata for models as below
            {
                name: model_name,
                state: "Ready" or "NotReady"
                reason: ""
            }

        """
        model_list = []
        for model_name in self._model_registry.get_models().keys():
            model_ready = self._model_registry.is_model_ready(model_name)
            if model_ready or not filter_ready:
                # If model is ready or filter_ready is set to False
                model_list.append({
                    "name": model_name,
                    "state": (
                        "Ready" if self._model_registry.is_model_ready(model_name) else "NotReady"
                    ),
                    "reason": ""
                })

        return model_list

    async def load(self, model_name: str) -> None:
        """
        Loads the Specified model.

        Args:
            model_name (str): name of the model to load

        Returns: None
        Raises: 'ModelNotReady' exception if model loading fails

        """
        try:
            if inspect.iscoroutinefunction(self._model_registry.load):
                await self._model_registry.load(model_name)
            else:
                self._model_registry.load(model_name)
        except Exception:
            ex_type, ex_value, ex_traceback = sys.exc_info()
            raise ModelNotReady(model_name, f"Error type: {ex_type} error msg: {ex_value}")

        if not self._model_registry.is_model_ready(model_name):
            raise ModelNotReady(model_name)

    def unload(self, model_name: str) -> None:
        """
        Unloads the specified model.

        Args:
            model_name (str): Name of the model to unload
        Returns: None
        Raises: 'ModelNotFound' exception if the requested model is not found.
        """
        try:
            self._model_registry.unload(model_name)
        except KeyError:
            raise ModelNotFound(model_name)
