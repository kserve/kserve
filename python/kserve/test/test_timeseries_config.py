import pytest
from unittest.mock import MagicMock

from fastapi import FastAPI
from kserve.model_repository import ModelRepository
from kserve.protocol.rest.timeseries.config import maybe_register_time_series_endpoints


# Mock classes
class HuggingFaceTimeSeriesModel:
    pass


class OtherModel:
    pass


def test_maybe_register_time_series_endpoints_registers_only_time_series_models():
    # Setup
    app = FastAPI()

    # Create mock models
    ts_model = HuggingFaceTimeSeriesModel()
    other_model = OtherModel()

    # Mock repository
    repository = MagicMock(spec=ModelRepository)
    repository.get_models.return_value = {
        "ts_model": ts_model,
        "other_model": other_model,
    }

    # Patch register_time_series_endpoints to track calls
    from kserve.protocol.rest.timeseries.endpoints import register_time_series_endpoints

    register_mock = MagicMock()
    import kserve.protocol.rest.timeseries.config

    kserve.protocol.rest.timeseries.config.register_time_series_endpoints = (
        register_mock
    )

    # Call the function
    result = maybe_register_time_series_endpoints(app, repository)

    # Assertions
    assert result is True
    register_mock.assert_called_once()

    # Extract TimeSeriesDataPlane (2nd arg)
    ts_data_plane = register_mock.call_args[0][1]

    # ✔ FIXED: use model_registry instead of model_repository
    models_in_repo = ts_data_plane.model_registry.get_models()

    assert "ts_model" in models_in_repo
    assert "other_model" not in models_in_repo
