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
from fastapi import FastAPI
from fastapi.testclient import TestClient

from kserve import Model, ModelRepository
from kserve.protocol.rest.timeseries.config import maybe_register_time_series_endpoints


class HuggingFaceTimeSeriesModel(Model):
    """Lightweight model accepted by the time series endpoint registration."""


@pytest.mark.parametrize("names", [["timesfm"], ["timesfm-small", "timesfm-large"]])
def test_list_registered_time_series_models(names):
    repository = ModelRepository()
    repository.update(Model("other-model"))
    for name in names:
        repository.update(HuggingFaceTimeSeriesModel(name))
    app = FastAPI()
    assert maybe_register_time_series_endpoints(app, repository)

    with TestClient(app) as client:
        response = client.get("/v1/timeseries/models")

    assert response.status_code == 200
    assert response.json() == names
