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

    monkeypatch.setattr(
        "autogluonserver.autogluon_model_repository.AutoGluonModel", FakeAutoGluonModel
    )

    repo = AutoGluonModelRepository(str(tmp_path))
    await repo.load(model_name)
    assert repo.get_model(model_name) is not None
    assert repo.is_model_ready(model_name)


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

    monkeypatch.setattr(
        "autogluonserver.autogluon_model_repository.AutoGluonModel", FakeAutoGluonModel
    )

    repo = AutoGluonModelRepository(str(tmp_path))
    with pytest.raises(FileNotFoundError, match="model directory not found"):
        await repo.load(model_name)

    assert repo.get_model(model_name) is None
    assert not repo.is_model_ready(model_name)
