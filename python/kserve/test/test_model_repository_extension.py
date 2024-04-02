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

import pytest
from kserve.errors import ModelNotFound, ModelNotReady
from kserve.protocol.model_repository_extension import ModelRepositoryExtension
from kserve.model_repository import ModelRepository
from test.test_server import DummyModel, DummyModelRepository


@pytest.mark.asyncio
class TestModelRepositoryExtension:
    MODEL_NAME = "TestModel"

    @pytest.fixture()
    def model_repo_ext(self) -> ModelRepositoryExtension:
        model_repo_ext = ModelRepositoryExtension(model_registry=ModelRepository())
        model = DummyModel(self.MODEL_NAME)
        model.load()
        model_repo_ext._model_registry.update(model)
        return model_repo_ext

    async def test_index(self, model_repo_ext):
        assert model_repo_ext.index() == [
            {"name": self.MODEL_NAME, "reason": "", "state": "Ready"}
        ]

        # Deploy another model

        model = DummyModel("TestModel_2")
        # model.load()  # TestModel_2 is not loaded i.e. NotReady
        model_repo_ext._model_registry.update(model)
        assert model_repo_ext.index() == [
            {"name": self.MODEL_NAME, "reason": "", "state": "Ready"},
            {"name": "TestModel_2", "reason": "", "state": "NotReady"},
        ]

        # List only ready models
        assert model_repo_ext.index(filter_ready=True) == [
            {"name": self.MODEL_NAME, "reason": "", "state": "Ready"}
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
            model_registry=DummyModelRepository(
                test_load_success=False, fail_with_exception=True
            )
        )
        with pytest.raises(ModelNotReady) as e:
            await model_repo_ext.load(self.MODEL_NAME)
        assert e.value.model_name == self.MODEL_NAME
        assert (
            e.value.error_msg == f"Model with name {self.MODEL_NAME} is not ready. "
            f"Error type: <class 'Exception'> error "
            f"msg: Could not load model {self.MODEL_NAME}."
        )

    async def test_unload(self, model_repo_ext):
        await model_repo_ext.unload(self.MODEL_NAME)
        assert model_repo_ext._model_registry.get_models() == {}

    async def test_unload_fail(self, model_repo_ext):
        with pytest.raises(ModelNotFound) as e:
            await model_repo_ext.unload("FAKE_NAME")
        assert e.value.reason == "Model with name FAKE_NAME does not exist."
