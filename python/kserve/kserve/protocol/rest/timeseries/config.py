# Copyright 2025 The KServe Authors.
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

from fastapi import FastAPI

from ....model import Model
from ....model_repository import ModelRepository

from kserve.logging import logger

from .dataplane import TimeSeriesDataPlane
from .endpoints import register_time_series_endpoints
from .time_series_model import HuggingFaceTimeSeriesModel


def get_time_series_models(repository: ModelRepository) -> dict[str, Model]:
    """Retrieve all models in the repository that implement the Times Series interface"""

    return {
        name: model
        for name, model in repository.get_models().items()
        if isinstance(model, HuggingFaceTimeSeriesModel)
    }


def maybe_register_time_series_endpoints(app: FastAPI, model_registry: ModelRepository):
    ts_models = get_time_series_models(model_registry)

    if len(ts_models) == 0:
        return False

    # Create a model repository with just the Time Series models
    ts_model_registry = ModelRepository()
    for name, model in ts_models.items():
        ts_model_registry.update(model, name)

    # Add the Time Series endpoints.
    register_time_series_endpoints(app, TimeSeriesDataPlane(ts_model_registry))
    return True
