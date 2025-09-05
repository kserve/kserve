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
import tempfile
import pytest
from catboostserver import CatBoostModelRepository

# Use example model following KServe pattern
example_model_dir = os.path.join(os.path.dirname(__file__), "..", "example_model")


@pytest.mark.asyncio
async def test_load():
    """Test loading model from repository using example model"""
    # Copy example model to temporary directory for testing
    with tempfile.TemporaryDirectory() as temp_dir:
        model_dir = os.path.join(temp_dir, "model")
        os.makedirs(model_dir)

        # Copy example model
        import shutil

        example_model_path = os.path.join(example_model_dir, "model", "model.cbm")
        shutil.copy(example_model_path, os.path.join(model_dir, "model.cbm"))

        # Test model repository
        repository = CatBoostModelRepository(temp_dir)
        await repository.load("model")

        loaded_model = repository.get_model("model")
        assert loaded_model is not None
        assert loaded_model.ready


@pytest.mark.asyncio
async def test_load_fail():
    # Test loading non-existent model
    with tempfile.TemporaryDirectory() as temp_dir:
        # Create empty model directory to avoid storage path issues
        model_dir = os.path.join(temp_dir, "non-existent-model")
        os.makedirs(model_dir)

        repository = CatBoostModelRepository(temp_dir)

        # This should not raise an exception but return False
        result = await repository.load("non-existent-model")
        assert result is False
