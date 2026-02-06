import pytest
from unittest.mock import MagicMock, AsyncMock
from kserve.model_repository import ModelRepository  # adjust import path


# ---------------------------
# Test set_models_dir
# ---------------------------
def test_set_models_dir_updates_models_dir():
    repo = ModelRepository()

    # Initial value should be default
    default_dir = repo.models_dir

    # Set a new models_dir
    new_dir = "/tmp/new_models"
    repo.set_models_dir(new_dir)

    assert repo.models_dir == new_dir
    assert repo.models_dir != default_dir


# ---------------------------
# Test load_models()
# ---------------------------
def test_load_models_calls_load_model_for_directories(tmp_path):
    models_dir = tmp_path
    (models_dir / "modelA").mkdir()  # directory
    (models_dir / "file.txt").write_text("abc")  # file

    repo = ModelRepository(models_dir=str(models_dir))
    repo.load_model = MagicMock()

    repo.load_models()

    repo.load_model.assert_called_once_with("modelA")


# ---------------------------
# Test update()
# ---------------------------
def test_update_uses_model_name_when_no_custom_name():
    repo = ModelRepository()
    model = MagicMock()
    model.name = "my_model"

    repo.update(model)

    assert repo.get_model("my_model") == model


def test_update_uses_custom_name_if_provided():
    repo = ModelRepository()
    model = MagicMock()
    model.name = "original"

    repo.update(model, name="alias")

    assert repo.get_model("alias") == model


# ---------------------------
# Test get_model / get_models
# ---------------------------
def test_get_model_returns_none_if_missing():
    repo = ModelRepository()
    assert repo.get_model("unknown") is None


def test_get_models_returns_dictionary():
    repo = ModelRepository()
    model = MagicMock()
    model.name = "abc"
    repo.update(model)
    assert "abc" in repo.get_models()


# ---------------------------
# Test is_model_ready (async)
# ---------------------------
@pytest.mark.asyncio
async def test_is_model_ready_returns_true_if_healthy():
    repo = ModelRepository()
    model = MagicMock()
    model.name = "m1"
    model.healthy = AsyncMock(return_value=True)

    repo.update(model)

    assert await repo.is_model_ready("m1") is True


@pytest.mark.asyncio
async def test_is_model_ready_returns_false_if_model_missing():
    repo = ModelRepository()
    assert await repo.is_model_ready("missing") is False


# ---------------------------
# Test load() and load_model() — currently pass
# ---------------------------
def test_load_returns_none():
    repo = ModelRepository()
    assert repo.load("anything") is None


def test_load_model_returns_none():
    repo = ModelRepository()
    assert repo.load_model("anything") is None


# ---------------------------
# Test unload()
# ---------------------------
def test_unload_calls_stop_and_stop_engine_and_removes_model():
    repo = ModelRepository()

    model = MagicMock()
    model.name = "m1"

    # model.stop exists
    model.stop = MagicMock()
    # engine is truthy → stop_engine must be called
    model.engine = True
    model.stop_engine = MagicMock()

    repo.update(model, name="m1")

    repo.unload("m1")

    model.stop.assert_called_once()
    model.stop_engine.assert_called_once()
    assert "m1" not in repo.get_models()


def test_unload_calls_only_stop_when_engine_false():
    repo = ModelRepository()

    model = MagicMock()
    model.name = "m1"
    model.stop = MagicMock()
    model.engine = False
    model.stop_engine = MagicMock()

    repo.update(model, name="m1")

    repo.unload("m1")

    model.stop.assert_called_once()
    model.stop_engine.assert_not_called()
    assert "m1" not in repo.get_models()


def test_unload_raises_if_model_missing():
    repo = ModelRepository()

    with pytest.raises(KeyError):
        repo.unload("unknown")
