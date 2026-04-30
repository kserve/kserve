# Copyright 2026 The KServe Authors.
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

from autogluonserver import AutoGluonModelRepository


@pytest.mark.asyncio
async def test_load_updates_repository_with_ready_model(monkeypatch, tmp_path):
    model_name = "autogluon-model"
    (tmp_path / model_name).mkdir()

    class FakeAutoGluonModel:
        def __init__(self, name, model_dir):
            self.name = name
            self.model_dir = model_dir
            self.ready = False

        def load(self):
            self.ready = True
            return True

        async def healthy(self):
            return self.ready

    monkeypatch.setattr(
        "autogluonserver.autogluon_model_repository.create_autogluon_model",
        lambda n, d: FakeAutoGluonModel(n, d),
    )

    repo = AutoGluonModelRepository(str(tmp_path))
    await repo.load(model_name)
    assert repo.get_model(model_name) is not None
    assert await repo.is_model_ready(model_name)


@pytest.mark.asyncio
async def test_load_failure_does_not_register_model(monkeypatch, tmp_path):
    model_name = "missing-model"

    class FakeAutoGluonModel:
        def __init__(self, name, model_dir):
            self.name = name
            self.model_dir = model_dir
            self.ready = False

        def load(self):
            raise FileNotFoundError("model directory not found")

        async def healthy(self):
            return self.ready

    monkeypatch.setattr(
        "autogluonserver.autogluon_model_repository.create_autogluon_model",
        lambda n, d: FakeAutoGluonModel(n, d),
    )

    repo = AutoGluonModelRepository(str(tmp_path))
    with pytest.raises(FileNotFoundError, match="model directory not found"):
        await repo.load(model_name)

    assert repo.get_model(model_name) is None
    assert not await repo.is_model_ready(model_name)
