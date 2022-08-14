import logging
from typing import Dict, Optional

from kserve.model_repository import ModelRepository
from ray.serve.api import RayServeHandle


class DataPlaneV1:

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry
        self._server_name = "kserve"  # TODO: get from variables
        self._server_version = "v0.8.0"  # TODO: get from variables

    def get_model_from_registry(self, name: str):
        model = self._model_registry.get_model(name)
        if model is None:
            # TODO: Handle if there is no model fond
            pass
        return model

    async def live(self):
        # response = {"status": "alive"}
        # return response
        return True

    async def metadata(self):
        return {
            "name": self._server_name,
            "version": self._server_version
        }

    async def model_metadata(self, model_name: str, model_version: str = None):
        model = self.get_model_from_registry(model_name)
        return await model.metadata()

    async def list(self):
        return {"models": list(self._model_registry.get_models().keys())}

    async def ready(self):
        models = self._model_registry.get_models().values()
        is_ready = all([model.ready for model in models])
        # return {"ready": is_ready}
        return is_ready

    async def model_ready(self, model_name: str):
        is_ready = self._model_registry.is_model_ready(model_name)
        # return {
        #     "name": model_name,
        #     "ready": is_ready
        # }
        return is_ready

    async def load(self, name):
        self._model_registry.load(name)
        return {
            "name": name,
            "load": True
        }

    async def unload(self, name):
        self._model_registry.unload(name)
        return {
            "name": name,
            "unload": True
        }

    async def infer(
        self,
        payload: dict,
        model_name: str,
    ):
        model_name = model_name.rstrip(model_name[-1*len(":predict"):])
        logging.info(model_name)
        model = self.get_model_from_registry(model_name)
        # TODO: Remove converting dict
        payload_dict = payload
        if (not isinstance(payload_dict, dict)):
            payload_dict = payload.__dict__
        if not isinstance(model, RayServeHandle):
            prediction = await model(payload_dict)
        else:
            prediction = (await model.remote(payload_dict))
        return prediction