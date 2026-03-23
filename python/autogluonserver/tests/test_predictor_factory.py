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

import json

from autogluonserver.predictor_factory import create_autogluon_model
from autogluonserver.tabular_model import AutoGluonTabularModel
from autogluonserver.timeseries_model import AutoGluonTimeSeriesModel


def test_factory_tabular_env(monkeypatch, tmp_path):
    monkeypatch.setenv("AUTOGLUON_PREDICTOR_TYPE", "tabular")
    m = create_autogluon_model("n", str(tmp_path))
    assert isinstance(m, AutoGluonTabularModel)


def test_factory_timeseries_env(monkeypatch, tmp_path):
    monkeypatch.setenv("AUTOGLUON_PREDICTOR_TYPE", "timeseries")
    m = create_autogluon_model("n", str(tmp_path))
    assert isinstance(m, AutoGluonTimeSeriesModel)


def test_factory_auto_uses_metadata(monkeypatch, tmp_path):
    monkeypatch.delenv("AUTOGLUON_PREDICTOR_TYPE", raising=False)
    (tmp_path / "predictor_metadata.json").write_text(
        json.dumps({"target": "y", "id_column": "i", "timestamp_column": "t"}),
        encoding="utf-8",
    )
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.Storage.download", lambda _: str(tmp_path)
    )
    m = create_autogluon_model("n", str(tmp_path))
    assert isinstance(m, AutoGluonTimeSeriesModel)


def test_factory_auto_default_tabular(monkeypatch, tmp_path):
    monkeypatch.delenv("AUTOGLUON_PREDICTOR_TYPE", raising=False)
    monkeypatch.setattr(
        "autogluonserver.predictor_factory.Storage.download", lambda _: str(tmp_path)
    )
    m = create_autogluon_model("n", str(tmp_path))
    assert isinstance(m, AutoGluonTabularModel)
