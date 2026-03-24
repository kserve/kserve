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

from unittest.mock import MagicMock

from autogluonserver.predictor_factory import (
    AutoGluonDetectedModel,
    create_autogluon_model,
)
from autogluonserver.tabular_model import AutoGluonTabularModel
from autogluonserver.timeseries_model import AutoGluonTimeSeriesModel


def test_factory_returns_detected_model(tmp_path):
    m = create_autogluon_model("n", str(tmp_path))
    assert isinstance(m, AutoGluonDetectedModel)


def test_load_delegates_to_tabular(monkeypatch, tmp_path):
    fake = MagicMock()
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.detect_and_load_predictor",
        lambda _: ("tabular", fake),
    )
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.Storage.download", lambda _: str(tmp_path)
    )
    m = create_autogluon_model("n", str(tmp_path))
    assert m.load()
    assert isinstance(m._impl, AutoGluonTabularModel)
    assert m._impl._predictor is fake


def test_load_delegates_to_timeseries(monkeypatch, tmp_path):
    fake = MagicMock()
    fake.target = "y"
    fake.prediction_length = 1
    fake.known_covariates_names = []
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.detect_and_load_predictor",
        lambda _: ("timeseries", fake),
    )
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.Storage.download", lambda _: str(tmp_path)
    )
    m = create_autogluon_model("n", str(tmp_path))
    assert m.load()
    assert isinstance(m._impl, AutoGluonTimeSeriesModel)
    assert m._impl._predictor is fake
    assert m._impl._metadata.target == "y"
