from importlib.util import find_spec
from typing import List

from fastapi import FastAPI

from ....model import Model
from ....model_repository import ModelRepository


def openai_is_available() -> bool:
    """Check if the openai package is available"""
    try:
        find_spec("openai")
        return True
    except ValueError:
        return False


def get_open_ai_models(repository: ModelRepository) -> List[Model]:
    """Retrieve all models in the repository that implement the OpenAI interface"""
    from .openai_model import OpenAIModel

    return [model for _, model in repository.get_models().items() if isinstance(model, OpenAIModel)]


def maybe_register_openai_endpoints(app: FastAPI, model_registry: ModelRepository):
    # Check if the openai package is available before continuing so we don't run into any import errors
    if not openai_is_available():
        return
    open_ai_models = get_open_ai_models(model_registry)
    # If no OpenAI models then no need to add the endpoints
    if len(open_ai_models) == 0:
        return
    from .dataplane import OpenAIDataPlane
    from .endpoints import register_openai_endpoints

    # Create a model repository with just the OpenAI models
    openai_model_registry = ModelRepository()
    for model in open_ai_models:
        openai_model_registry.update(model)

    # Add the OpenAI completion and chat completion endpoints.
    register_openai_endpoints(app, OpenAIDataPlane(openai_model_registry))
