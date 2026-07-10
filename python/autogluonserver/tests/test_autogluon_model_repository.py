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

import os
import pytest

from autogluonserver import AutoGluonModelRepository

pytestmark = pytest.mark.autogluon


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


@pytest.mark.asyncio
async def test_path_traversal_blocked_parent_directory(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model path traversal detected"):
        await repo.load("../etc/passwd")


@pytest.mark.asyncio
async def test_path_traversal_blocked_double_dot(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model path traversal detected"):
        await repo.load("../../tmp/evil")


@pytest.mark.asyncio
async def test_path_traversal_blocked_absolute_path(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model name cannot be an absolute path"):
        await repo.load("/etc/passwd")


@pytest.mark.asyncio
@pytest.mark.skipif(os.name != "nt", reason="Windows-specific path test")
async def test_path_traversal_blocked_windows_absolute(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model name cannot be an absolute path"):
        await repo.load("C:\\Windows\\System32")


@pytest.mark.asyncio
async def test_path_traversal_blocked_symlink_escape(tmp_path):
    models_dir = tmp_path / "models"
    models_dir.mkdir()

    repo = AutoGluonModelRepository(str(models_dir))

    with pytest.raises(ValueError, match="Model path traversal detected"):
        await repo.load("model/../../../etc/passwd")


@pytest.mark.asyncio
async def test_path_traversal_blocked_mixed_separators(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    # On Unix, backslashes are literal characters not separators, so this won't traverse
    # On Windows, this should be caught. Either way, it won't succeed.
    with pytest.raises((ValueError, Exception)):
        await repo.load(r"..\..\tmp\evil")


@pytest.mark.asyncio
async def test_path_traversal_blocked_url_encoded(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    # URL-encoded paths should still fail validation since realpath resolves them
    with pytest.raises((ValueError, Exception)):
        await repo.load("..%2F..%2Ftmp%2Fevil")


@pytest.mark.asyncio
async def test_empty_model_name_rejected(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model name cannot be empty"):
        await repo.load("")


@pytest.mark.asyncio
async def test_whitespace_model_name_rejected(tmp_path):
    repo = AutoGluonModelRepository(str(tmp_path))

    with pytest.raises(ValueError, match="Model name cannot be empty"):
        await repo.load("   ")


@pytest.mark.asyncio
async def test_valid_model_name_accepted(monkeypatch, tmp_path):
    model_name = "valid-model-123"
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


@pytest.mark.asyncio
async def test_validate_model_path_returns_real_path(tmp_path):
    model_name = "test-model"
    (tmp_path / model_name).mkdir()

    repo = AutoGluonModelRepository.__new__(AutoGluonModelRepository)
    repo.models_dir = str(tmp_path)
    repo.models = {}

    validated_path = repo._validate_model_path(model_name)

    assert os.path.isabs(validated_path)
    assert validated_path == os.path.realpath(os.path.join(str(tmp_path), model_name))
    assert validated_path.startswith(os.path.realpath(str(tmp_path)))
