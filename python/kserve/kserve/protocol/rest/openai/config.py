# Copyright 2023 The KServe Authors.
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


def get_open_ai_models(repository: ModelRepository) -> dict[str, Model]:
    """Retrieve all models in the repository that implement the OpenAI interface"""
    from .openai_model import OpenAIModel

    return {
        name: model
        for name, model in repository.get_models().items()
        if isinstance(model, OpenAIModel)
    }


def maybe_register_openai_endpoints(app: FastAPI, model_registry: ModelRepository):
    open_ai_models = get_open_ai_models(model_registry)
    # If no OpenAI models then no need to add the endpoints
    if len(open_ai_models) == 0:
        return
    from .dataplane import OpenAIDataPlane
    from .endpoints import register_openai_endpoints

    # Create a model repository with just the OpenAI models
    openai_model_registry = ModelRepository()
    for name, model in open_ai_models.items():
        openai_model_registry.update(model, name)

    # Add the OpenAI completion and chat completion endpoints.
    register_openai_endpoints(app, OpenAIDataPlane(openai_model_registry))
