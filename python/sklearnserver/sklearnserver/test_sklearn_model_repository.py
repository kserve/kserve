# Copyright 2021 The KServe Authors.
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
from sklearnserver import SKLearnModelRepository

_MODEL_DIR = os.path.join(os.path.dirname(__file__), "example_models")
JOBLIB_FILE_DIR = os.path.join(_MODEL_DIR, "joblib")
PICKLE_FILE_DIRS = [os.path.join(_MODEL_DIR, "pkl"), os.path.join(_MODEL_DIR, "pickle")]
INVALID_MODEL_DIR = os.path.join(os.path.dirname(__file__), "models_not_exist")


@pytest.mark.asyncio
async def test_load_pickle():
    for model_dir in PICKLE_FILE_DIRS:
        repo = SKLearnModelRepository(model_dir)
        model_name = "model"
        await repo.load(model_name)
        assert repo.get_model(model_name) is not None
        assert repo.is_model_ready(model_name)


@pytest.mark.asyncio
async def test_load_joblib():
    repo = SKLearnModelRepository(JOBLIB_FILE_DIR)
    model_name = "model"
    await repo.load(model_name)
    assert repo.get_model(model_name) is not None
    assert repo.is_model_ready(model_name)


@pytest.mark.asyncio
async def test_load_multiple():
    repo = SKLearnModelRepository(_MODEL_DIR + "/multi/model_repository")
    for model in ["model1", "model2"]:
        assert repo.get_model(model) is not None
        assert repo.is_model_ready(model)


@pytest.mark.asyncio
async def test_load_fail():
    with pytest.raises(FileNotFoundError):
        repo = SKLearnModelRepository(INVALID_MODEL_DIR)
        model_name = "model"
        assert repo.get_model(model_name) is None
        assert not repo.is_model_ready(model_name)
