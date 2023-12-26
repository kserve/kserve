import asyncio

from fastapi import FastAPI

from kserve import Model, ModelServer

from kserve.protocol.rest.server import UvicornServer


class FakeModel(Model):
    def __init__(self, name, sleep_millisecond=0.05):
        super().__init__(name)
        self.name = name
        self.ready = True
        self.sleep_millisecond = sleep_millisecond

    async def predict(self, request, headers=None):
        await asyncio.sleep(self.sleep_millisecond)
        return {"outputs": [
            {"data": [0.1, 0.9]},
            {"data": [0.2, 0.8]}
        ]}


class MockPredictor:

    async def run(self):
        model = FakeModel("TestModel")
        model.load()
        server = ModelServer()
        server.register_model(model)
        _rest_server = UvicornServer(9000, [],
                                          server.dataplane, server.model_repository_extension,
                                          False)
        await _rest_server.run()


if __name__ == "__main__":
    asyncio.run(MockPredictor().run())
