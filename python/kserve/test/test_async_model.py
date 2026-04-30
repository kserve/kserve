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

import threading
from concurrent.futures import ThreadPoolExecutor

import pytest

from kserve import AsyncModel


class SyncPredictModel(AsyncModel):
    def __init__(self, name="sync-model"):
        super().__init__(name)
        self.ready = True

    def predict(self, payload, headers=None):
        return threading.get_ident()


@pytest.fixture(autouse=True)
def reset_async_model_state():
    SyncPredictModel._cleanup_executor()
    SyncPredictModel._shared_executor = None
    SyncPredictModel._max_workers = None
    SyncPredictModel._cleanup_registered = False
    yield
    SyncPredictModel._cleanup_executor()
    SyncPredictModel._shared_executor = None
    SyncPredictModel._max_workers = None
    SyncPredictModel._cleanup_registered = False


@pytest.mark.asyncio
async def test_async_model_offloads_sync_predict():
    model = SyncPredictModel()
    loop_thread_id = threading.get_ident()
    result = await model.predict({"instances": [1]})
    assert result != loop_thread_id


@pytest.mark.asyncio
async def test_async_model_env_workers(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    executor = SyncPredictModel._get_executor()
    assert isinstance(executor, ThreadPoolExecutor)
    assert executor._max_workers == 1


def test_async_model_invalid_env_workers(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "invalid")
    executor = SyncPredictModel._get_executor()
    assert executor is None
    assert SyncPredictModel._max_workers == -1


def test_async_model_cleanup_shared_executor(monkeypatch):
    monkeypatch.setenv("ASYNC_MODEL_WORKERS", "1")
    executor = SyncPredictModel._get_executor()
    assert isinstance(executor, ThreadPoolExecutor)
    SyncPredictModel._cleanup_executor()
    assert SyncPredictModel._shared_executor is None
