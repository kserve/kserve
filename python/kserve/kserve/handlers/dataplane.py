import inspect

from kserve.model_repository import ModelRepository


class DataPlane:

    def __init__(self, model_registry: ModelRepository):
        self._model_registry = model_registry
        self._server_name = "kserve"  # TODO: get from variables
        self._server_version = "v0.9.0"  # TODO: get from variables

    def get_model_from_registry(self, name: str):
        model = self._model_registry.get_model(name)
        if model is None:
            # TODO: Handle if there is no model fond
            pass
        return model

    async def live(self):
        return True

    async def metadata(self):
        return {
            "name": self._server_name,
            "version": self._server_version
        }

    async def model_metadata(self, model_name: str):
        model = self.get_model_from_registry(model_name)
        return await model.metadata()

    async def list(self):
        return {"models": list(self._model_registry.get_models().keys())}

    async def ready(self):
        models = self._model_registry.get_models().values()
        return all([model.ready for model in models])

    async def model_ready(self, model_name: str):
        return self._model_registry.is_model_ready(model_name)

    async def load(self, name):
        await self._model_registry.load(name)
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

    async def infer(self, payload, headers, model_name: str):
        model = self.get_model_from_registry(model_name)
        result = (await model(payload, headers=headers)) if inspect.iscoroutinefunction(model.__call__) \
            else model(payload, headers=headers)
        return result
