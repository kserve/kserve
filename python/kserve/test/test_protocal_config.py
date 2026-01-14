import pytest
from unittest.mock import MagicMock, patch

from fastapi import FastAPI

from kserve.protocol.rest.openai.config import maybe_register_openai_endpoints
from kserve.model_repository import ModelRepository


class DummyOpenAIModel:
    """Minimal stand-in for OpenAIModel"""
    pass


class DummyNonOpenAIModel:
    """Model that should be ignored"""
    pass


def test_no_openai_models_does_not_register_endpoints():
    app = FastAPI()
    repository = ModelRepository()

    # Add a non-OpenAI model
    repository.update(DummyNonOpenAIModel(), "non_openai")

    with patch(
        "kserve.protocol.rest.openai.endpoints.register_openai_endpoints"
    ) as register_mock:
        maybe_register_openai_endpoints(app, repository)

        # Should not register endpoints
        register_mock.assert_not_called()


def test_openai_models_register_endpoints():
    app = FastAPI()
    repository = ModelRepository()

    openai_model = DummyOpenAIModel()

    repository.update(openai_model, "openai_model")

    with patch(
        "kserve.protocol.rest.openai.openai_model.OpenAIModel",
        DummyOpenAIModel,
    ), patch(
        "kserve.protocol.rest.openai.endpoints.register_openai_endpoints"
    ) as register_mock, patch(
        "kserve.protocol.rest.openai.dataplane.OpenAIDataPlane"
    ) as dataplane_mock:

        maybe_register_openai_endpoints(app, repository)

        # Endpoints should be registered exactly once
        register_mock.assert_called_once()

        # Validate arguments
        args, _ = register_mock.call_args
        assert args[0] is app
        dataplane_mock.assert_called_once()
