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
import logging

import pandas as pd
import pytest
from kserve.errors import InferenceError
from kserve.protocol.infer_type import InferRequest, InferInput

from autogluonserver.timeseries_model import (
    AutoGluonTimeSeriesModel,
    TimeSeriesInferenceMetadata,
    _forecast_columns_rename_map,
    _forecast_to_records,
    _load_ts_metadata,
)

pytestmark = pytest.mark.autogluon


def _write_predictor_metadata(
    tmp_path,
    *,
    target: str = "y",
    id_column: str = "item_id",
    timestamp_column: str = "ts",
) -> None:
    payload = {
        "target": target,
        "id_column": id_column,
        "timestamp_column": timestamp_column,
    }
    (tmp_path / "predictor_metadata.json").write_text(
        json.dumps(payload),
        encoding="utf-8",
    )


class FakeTimeSeriesPredictor:
    def __init__(self, known_covariates_names=None):
        self.last_data = None
        self.last_known_covariates = None
        self.target = "y"
        self.prediction_length = 1
        self.known_covariates_names = known_covariates_names or []

    def predict(self, data, known_covariates=None, **kwargs):
        self.last_data = data
        self.last_known_covariates = known_covariates
        idx = pd.MultiIndex.from_tuples(
            [("i1", pd.Timestamp("2024-01-05"))],
            names=["item_id", "timestamp"],
        )
        return pd.DataFrame({"mean": [3.14], "0.1": [2.0]}, index=idx)


def test_timeseries_load_and_predict_v1(monkeypatch, tmp_path):
    _write_predictor_metadata(tmp_path)
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
    assert row["ts"] == "2024-01-05T00:00:00"
    assert "timestamp" not in row
    assert row["mean"] == pytest.approx(3.14)
    assert row["0.1"] == pytest.approx(2.0)


def test_timeseries_response_uses_metadata_column_names(monkeypatch, tmp_path):
    _write_predictor_metadata(tmp_path, id_column="idddd", timestamp_column="czas")
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
                {"idddd": "D1737", "czas": "2024-01-01", "y": 1.0},
                {"idddd": "D1737", "czas": "2024-01-02", "y": 2.0},
            ]
        }
    )
    row = resp["predictions"][0]
    assert row["idddd"] == "i1"
    assert row["czas"] == "2024-01-05T00:00:00"
    assert "item_id" not in row
    assert "timestamp" not in row
    assert row["mean"] == pytest.approx(3.14)


def test_timeseries_known_covariates_passed(monkeypatch, tmp_path):
    _write_predictor_metadata(tmp_path)
    fake = FakeTimeSeriesPredictor(known_covariates_names=["promo"])
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
    _write_predictor_metadata(tmp_path)
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: FakeTimeSeriesPredictor(known_covariates_names=["promo"]),
    )
    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    model.load()
    with pytest.raises(InferenceError, match="known_covariates"):
        model.predict({"instances": [{"item_id": "i1", "ts": "2024-01-01", "y": 1.0}]})


def test_timeseries_without_metadata_json_uses_default_columns(
    caplog, monkeypatch, tmp_path
):
    fake = FakeTimeSeriesPredictor()
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.timeseries_model.TimeSeriesPredictor.load",
        lambda path: fake,
    )
    model = AutoGluonTimeSeriesModel("forecast", "s3://bucket/artifact")
    with caplog.at_level(logging.WARNING, logger="kserve"):
        assert model.load()
    assert any(
        "predictor_metadata.json" in r.message
        and "default inference column names" in r.message
        for r in caplog.records
    )
    resp = model.predict(
        {
            "instances": [
                {"item_id": "i1", "timestamp": "2024-01-01", "y": 1.0},
                {"item_id": "i1", "timestamp": "2024-01-02", "y": 2.0},
            ]
        }
    )
    assert fake.last_data is not None
    assert "predictions" in resp


@pytest.mark.parametrize(
    "env_id,env_ts",
    [
        ("", ""),
        ("   ", "  "),
    ],
    ids=["empty", "whitespace"],
)
def test_load_ts_metadata_empty_env_uses_fallbacks(
    monkeypatch, tmp_path, env_id, env_ts
):
    """Empty or whitespace-only env vars do not override defaults or JSON column names."""
    monkeypatch.setenv("AUTOGLUON_TS_ID_COLUMN", env_id)
    monkeypatch.setenv("AUTOGLUON_TS_TIMESTAMP_COLUMN", env_ts)

    without_file = _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))
    assert without_file.id_column == "item_id"
    assert without_file.timestamp_column == "timestamp"

    _write_predictor_metadata(
        tmp_path, id_column="custom_id", timestamp_column="custom_ts"
    )
    with_file = _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))
    assert with_file.id_column == "custom_id"
    assert with_file.timestamp_column == "custom_ts"


def test_load_ts_metadata_without_file_env_strips_whitespace(monkeypatch, tmp_path):
    monkeypatch.setenv("AUTOGLUON_TS_ID_COLUMN", "  series_id  ")
    monkeypatch.setenv("AUTOGLUON_TS_TIMESTAMP_COLUMN", "  time  ")
    meta = _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))
    assert meta.id_column == "series_id"
    assert meta.timestamp_column == "time"


def test_timeseries_env_overrides_column_names(monkeypatch, tmp_path):
    _write_predictor_metadata(tmp_path)
    monkeypatch.setenv("AUTOGLUON_TS_ID_COLUMN", "series_id")
    monkeypatch.setenv("AUTOGLUON_TS_TIMESTAMP_COLUMN", "time")
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
                {"series_id": "i1", "time": "2024-01-01", "y": 1.0},
                {"series_id": "i1", "time": "2024-01-02", "y": 2.0},
            ]
        }
    )
    assert fake.last_data is not None
    row = resp["predictions"][0]
    assert row["series_id"] == "i1"
    assert row["time"] == "2024-01-05T00:00:00"
    assert "item_id" not in row
    assert "timestamp" not in row


def test_forecast_columns_rename_map_non_multiindex_uses_ag_defaults():
    meta = TimeSeriesInferenceMetadata(
        target="y",
        id_column="series_id",
        timestamp_column="time",
        prediction_length=1,
        known_covariates_names=[],
    )
    forecasts = pd.DataFrame({"mean": [3.14]})
    assert _forecast_columns_rename_map(forecasts, meta) == {
        "item_id": "series_id",
        "timestamp": "time",
    }


def test_forecast_to_records_renames_flat_dataframe_columns():
    meta = TimeSeriesInferenceMetadata(
        target="y",
        id_column="series_id",
        timestamp_column="time",
        prediction_length=1,
        known_covariates_names=[],
    )
    forecasts = pd.DataFrame(
        {
            "item_id": ["i1"],
            "timestamp": [pd.Timestamp("2024-01-05")],
            "mean": [3.14],
        }
    )
    row = _forecast_to_records(forecasts, meta)[0]
    assert row["series_id"] == "i1"
    assert row["time"] == "2024-01-05T00:00:00"
    assert "item_id" not in row
    assert "timestamp" not in row


def test_forecast_to_records_converts_nan_to_none():
    meta = TimeSeriesInferenceMetadata(
        target="y",
        id_column="item_id",
        timestamp_column="timestamp",
        prediction_length=1,
        known_covariates_names=[],
    )
    forecasts = pd.DataFrame(
        {
            "item_id": ["i1"],
            "timestamp": [pd.Timestamp("2024-01-05")],
            "mean": [float("nan")],
            "0.1": [2.0],
        }
    )

    row = _forecast_to_records(forecasts, meta)[0]

    assert row["item_id"] == "i1"
    assert row["timestamp"] == "2024-01-05T00:00:00"
    assert row["mean"] is None
    assert row["0.1"] == pytest.approx(2.0)


def test_load_ts_metadata_invalid_json_raises(tmp_path):
    meta = tmp_path / "predictor_metadata.json"
    meta.write_text("{not json", encoding="utf-8")
    with pytest.raises(InferenceError, match="Invalid JSON"):
        _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))


def test_load_ts_metadata_top_level_array_raises(tmp_path):
    meta = tmp_path / "predictor_metadata.json"
    meta.write_text(json.dumps([]), encoding="utf-8")
    with pytest.raises(InferenceError, match="JSON object at the top level"):
        _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))


def test_load_ts_metadata_json_target_ignored_uses_predictor_target(tmp_path):
    """``target`` in predictor_metadata.json does not override TimeSeriesPredictor.target."""
    _write_predictor_metadata(tmp_path, target="wrong_target")
    fake = FakeTimeSeriesPredictor()
    fake.target = "y"
    meta = _load_ts_metadata(fake, str(tmp_path))
    assert meta.target == "y"
    assert meta.id_column == "item_id"
    assert meta.timestamp_column == "ts"


def test_load_ts_metadata_raises_when_predictor_target_missing(tmp_path):
    fake = FakeTimeSeriesPredictor()
    fake.target = None
    with pytest.raises(InferenceError, match="TimeSeriesPredictor.target is not set"):
        _load_ts_metadata(fake, str(tmp_path))


def test_load_ts_metadata_raises_when_predictor_target_missing_with_json(tmp_path):
    _write_predictor_metadata(tmp_path, target="y")
    fake = FakeTimeSeriesPredictor()
    fake.target = None
    with pytest.raises(InferenceError, match="TimeSeriesPredictor.target is not set"):
        _load_ts_metadata(fake, str(tmp_path))


@pytest.mark.parametrize("prediction_length", [0, -3])
def test_load_ts_metadata_prediction_length_invalid_raises(tmp_path, prediction_length):
    fake = FakeTimeSeriesPredictor()
    fake.prediction_length = prediction_length
    with pytest.raises(InferenceError, match="prediction_length must be >= 1"):
        _load_ts_metadata(fake, str(tmp_path))


def test_load_ts_metadata_prediction_length_from_predictor_ignores_json(tmp_path):
    """``prediction_length`` in predictor_metadata.json is ignored; predictor attribute wins."""
    meta = tmp_path / "predictor_metadata.json"
    meta.write_text(
        json.dumps(
            {
                "target": "y",
                "id_column": "item_id",
                "timestamp_column": "ts",
                "prediction_length": "nope",
            }
        ),
        encoding="utf-8",
    )
    fake = FakeTimeSeriesPredictor()
    fake.prediction_length = 7
    loaded = _load_ts_metadata(fake, str(tmp_path))
    assert loaded.prediction_length == 7


def test_load_ts_metadata_missing_id_column_error_uses_meta_path_only(tmp_path):
    meta = tmp_path / "predictor_metadata.json"
    meta.write_text(json.dumps({"timestamp_column": "ts"}), encoding="utf-8")
    with pytest.raises(
        InferenceError,
        match=rf"{meta} is missing required string field 'id_column'",
    ):
        _load_ts_metadata(FakeTimeSeriesPredictor(), str(tmp_path))


def test_load_ts_metadata_known_covariate_name_overlap_raises(tmp_path):
    _write_predictor_metadata(tmp_path, id_column="item_id", timestamp_column="ts")
    fake = FakeTimeSeriesPredictor(known_covariates_names=["item_id"])
    with pytest.raises(InferenceError, match="overlap id/timestamp/target"):
        _load_ts_metadata(fake, str(tmp_path))


def test_load_ts_metadata_without_file_overlap_raises(monkeypatch, tmp_path):
    fake = FakeTimeSeriesPredictor(known_covariates_names=["item_id"])
    with pytest.raises(InferenceError, match="overlap id/timestamp/target"):
        _load_ts_metadata(fake, str(tmp_path))


def test_timeseries_v2_request_raises(monkeypatch, tmp_path):
    _write_predictor_metadata(tmp_path)
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
