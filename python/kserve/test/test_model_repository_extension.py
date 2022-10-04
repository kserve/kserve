import pytest
from kserve.errors import ModelNotFound
from kserve.handlers.model_repository_extension import ModelRepositoryExtension, ModelNotReady
from kserve.model_repository import ModelRepository
from test.test_server import DummyModel, DummyModelRepository


class TestModelRepositoryExtension:

    MODEL_NAME = "TestModel"

    @pytest.fixture()
    def model_repo_ext(self) -> ModelRepositoryExtension:
        model_repo_ext = ModelRepositoryExtension(model_registry=ModelRepository())
        model = DummyModel(self.MODEL_NAME)
        model.load()
        model_repo_ext._model_registry.update(model)
        return model_repo_ext

    def test_index(self, model_repo_ext):

        assert model_repo_ext.index() == [
            {
                "name": self.MODEL_NAME,
                "reason": "",
                "state": "Ready"
            }
        ]

        # Deploy another model

        model = DummyModel("TestModel_2")
        # model.load()  # TestModel_2 is not loaded i.e. NotReady
        model_repo_ext._model_registry.update(model)
        assert model_repo_ext.index() == [
            {
                "name": self.MODEL_NAME,
                "reason": "",
                "state": "Ready"
            },
            {
                "name": "TestModel_2",
                "reason": "",
                "state": "NotReady"
            }
        ]

        # List only ready models
        assert model_repo_ext.index(filter_ready=True) == [
            {
                "name": self.MODEL_NAME,
                "reason": "",
                "state": "Ready"
            }
        ]

    async def test_load(self):
        model_repo_ext = ModelRepositoryExtension(
            model_registry=DummyModelRepository(test_load_success=True)
        )
        await model_repo_ext.load(self.MODEL_NAME)
        model = model_repo_ext._model_registry.get_model(self.MODEL_NAME)
        assert model.name == self.MODEL_NAME

    async def test_load_fail(self):
        model_repo_ext = ModelRepositoryExtension(
            model_registry=DummyModelRepository(test_load_success=False)
        )
        with pytest.raises(ModelNotReady) as e:
            await model_repo_ext.load(self.MODEL_NAME)
        assert e.value.model_name == self.MODEL_NAME
        assert e.value.error_msg == f"Model with name {self.MODEL_NAME} is not ready."

    async def test_load_fail_with_exception(self):
        model_repo_ext = ModelRepositoryExtension(
            model_registry=DummyModelRepository(test_load_success=False, fail_with_exception=True)
        )
        with pytest.raises(ModelNotReady) as e:
            await model_repo_ext.load(self.MODEL_NAME)
        assert e.value.model_name == self.MODEL_NAME
        assert e.value.error_msg == f"Model with name {self.MODEL_NAME} is not ready. " \
                                    f"Error type: <class 'Exception'> error " \
                                    f"msg: Could not load model {self.MODEL_NAME}."

    def test_unload(self, model_repo_ext):
        model_repo_ext.unload(self.MODEL_NAME)
        assert model_repo_ext._model_registry.get_models() == {}

    def test_unload_fail(self, model_repo_ext):
        with pytest.raises(ModelNotFound) as e:
            model_repo_ext.unload("FAKE_NAME")
        assert e.value.reason == f"Model FAKE_NAME not found."

