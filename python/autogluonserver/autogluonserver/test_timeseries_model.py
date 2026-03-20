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

import pandas as pd
import pytest
from kserve.errors import InferenceError
from kserve.protocol.infer_type import InferRequest, InferInput

from autogluonserver.timeseries_model import AutoGluonTimeSeriesModel


class FakeTimeSeriesPredictor:
    def __init__(self):
        self.last_data = None
        self.last_known_covariates = None

    def predict(self, data, known_covariates=None):
        self.last_data = data
        self.last_known_covariates = known_covariates
        idx = pd.MultiIndex.from_tuples(
            [("i1", pd.Timestamp("2024-01-05"))],
            names=["item_id", "timestamp"],
        )
        return pd.DataFrame({"mean": [3.14], "0.1": [2.0]}, index=idx)


def _write_ts_artifact(tmp_path, metadata, with_predictor_dir=True):
    (tmp_path / "predictor_metadata.json").write_text(
        json.dumps(metadata), encoding="utf-8"
    )
    if with_predictor_dir:
        (tmp_path / "predictor").mkdir()


def test_timeseries_load_and_predict_v1(monkeypatch, tmp_path):
    _write_ts_artifact(
        tmp_path,
        {
            "target": "y",
            "id_column": "item_id",
            "timestamp_column": "ts",
            "prediction_length": 1,
            "known_covariates_names": [],
        },
    )
    fake = FakeTimeSeriesPredictor()
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: fake,
    )

    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    assert model.load()
    resp = model.predict(
        {
            "instances": [
                {"item_id": "i1", "ts": "2024-01-01", "y": 1.0},
                {"item_id": "i1", "ts": "2024-01-02", "y": 2.0},
            ]
        }
    )
    assert fake.last_known_covariates is None
    assert "predictions" in resp
    assert len(resp["predictions"]) == 1
    row = resp["predictions"][0]
    assert row["item_id"] == "i1"
    assert row["mean"] == pytest.approx(3.14)
    assert row["0.1"] == pytest.approx(2.0)


def test_timeseries_known_covariates_passed(monkeypatch, tmp_path):
    _write_ts_artifact(
        tmp_path,
        {
            "target": "y",
            "id_column": "item_id",
            "timestamp_column": "ts",
            "prediction_length": 1,
            "known_covariates_names": ["promo"],
        },
    )
    fake = FakeTimeSeriesPredictor()
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: fake,
    )
    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    model.load()
    model.predict(
        {
            "instances": [{"item_id": "i1", "ts": "2024-01-01", "y": 1.0}],
            "known_covariates": [
                {"item_id": "i1", "ts": "2024-01-05", "promo": 1},
            ],
        }
    )
    assert fake.last_known_covariates is not None


def test_timeseries_missing_known_covariates_raises(monkeypatch, tmp_path):
    _write_ts_artifact(
        tmp_path,
        {
            "target": "y",
            "id_column": "item_id",
            "timestamp_column": "ts",
            "prediction_length": 1,
            "known_covariates_names": ["promo"],
        },
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: FakeTimeSeriesPredictor(),
    )
    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    model.load()
    with pytest.raises(InferenceError, match="known_covariates"):
        model.predict({"instances": [{"item_id": "i1", "ts": "2024-01-01", "y": 1.0}]})


def test_timeseries_v2_request_raises(monkeypatch, tmp_path):
    _write_ts_artifact(
        tmp_path,
        {
            "target": "y",
            "id_column": "item_id",
            "timestamp_column": "ts",
            "prediction_length": 1,
            "known_covariates_names": [],
        },
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: FakeTimeSeriesPredictor(),
    )
    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    model.load()
    req = InferRequest(
        model_name="forecast",
        infer_inputs=[
            InferInput(name="x", shape=[1], datatype="FP64", data=[1.0]),
        ],
    )
    with pytest.raises(InferenceError, match="REST v1 JSON"):
        model.predict(req)


def test_artifact_layout_model_full(tmp_path):
    from autogluonserver.artifact_layout import resolve_timeseries_artifact_paths

    full = tmp_path / "MODEL_FULL"
    full.mkdir()
    (full / "predictor").mkdir()
    (full / "predictor_metadata.json").write_text("{}", encoding="utf-8")
    pred, meta = resolve_timeseries_artifact_paths(str(full))
    assert pred.endswith("predictor")
    assert meta is not None and meta.endswith("predictor_metadata.json")
