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

from autogluon.tabular import TabularPredictor
from autogluon.timeseries import TimeSeriesPredictor
from kserve.errors import InferenceError

from autogluonserver.predictor_detect import detect_and_load_predictor

pytestmark = pytest.mark.autogluon


def _timeseries_predictor():
    return TimeSeriesPredictor.__new__(TimeSeriesPredictor)


def _tabular_predictor():
    return TabularPredictor.__new__(TabularPredictor)


def _write_global_pickle(path, module: str, name: str) -> None:
    path.write_bytes(f"c{module}\n{name}\n.".encode("utf-8"))


def test_detect_returns_timeseries_when_ts_loads(monkeypatch, tmp_path):
    ts_pred = _timeseries_predictor()
    calls = []

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        lambda *_args, **_kwargs: None,
    )

    def fake_load(cls, path, **kwargs):
        calls.append(cls)
        assert kwargs.get("run_safe_load_validation") is False
        if cls is TimeSeriesPredictor:
            return ts_pred
        raise AssertionError("tabular load should not be attempted")

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    kind, pred = detect_and_load_predictor(str(tmp_path))

    assert kind == "timeseries"
    assert pred is ts_pred
    assert calls == [TimeSeriesPredictor]


def test_detect_falls_back_to_tabular_when_ts_load_fails(monkeypatch, tmp_path):
    tb_pred = _tabular_predictor()
    calls = []
    expected_path = str(tmp_path)
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        lambda *_args, **_kwargs: None,
    )

    def fake_load(cls, path, **kwargs):
        calls.append(cls)
        assert path == expected_path
        assert kwargs.get("run_safe_load_validation") is False
        if cls is TimeSeriesPredictor:
            raise ValueError("not a time series predictor directory")
        if cls is TabularPredictor:
            return tb_pred
        raise AssertionError(f"unexpected predictor class: {cls}")

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    kind, pred = detect_and_load_predictor(str(tmp_path))

    assert kind == "tabular"
    assert pred is tb_pred
    assert calls == [TimeSeriesPredictor, TabularPredictor]


def test_detect_falls_back_to_tabular_when_ts_returns_wrong_type(monkeypatch, tmp_path):
    tb_pred = _tabular_predictor()
    calls = []
    expected_path = str(tmp_path)
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        lambda *_args, **_kwargs: None,
    )

    def fake_load(cls, path, **kwargs):
        calls.append(cls)
        assert path == expected_path
        assert kwargs.get("run_safe_load_validation") is False
        if cls is TimeSeriesPredictor:
            return object()
        if cls is TabularPredictor:
            return tb_pred
        raise AssertionError(f"unexpected predictor class: {cls}")

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    kind, pred = detect_and_load_predictor(str(tmp_path))

    assert kind == "tabular"
    assert pred is tb_pred
    assert calls == [TimeSeriesPredictor, TabularPredictor]


def test_detect_raises_when_both_return_wrong_type(monkeypatch, tmp_path):
    calls = []
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        lambda *_args, **_kwargs: None,
    )

    def fake_load(cls, path, **kwargs):
        calls.append(cls)
        assert path == str(tmp_path)
        assert kwargs.get("run_safe_load_validation") is False
        return object()

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    with pytest.raises(InferenceError) as exc_info:
        detect_and_load_predictor(str(tmp_path))

    msg = str(exc_info.value)
    assert calls == [TimeSeriesPredictor, TabularPredictor]
    assert "timeseries: loaded object is not TimeSeriesPredictor" in msg
    assert "tabular: loaded object is not TabularPredictor" in msg


def test_detect_raises_when_both_loads_fail(monkeypatch, tmp_path):
    calls = []
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        lambda *_args, **_kwargs: None,
    )

    def fake_load(cls, path, **kwargs):
        calls.append(cls)
        assert path == str(tmp_path)
        assert kwargs.get("run_safe_load_validation") is False
        if cls is TimeSeriesPredictor:
            raise ValueError("timeseries load failed")
        if cls is TabularPredictor:
            raise RuntimeError("tabular load failed")
        raise AssertionError(f"unexpected predictor class: {cls}")

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    with pytest.raises(InferenceError) as exc_info:
        detect_and_load_predictor(str(tmp_path))

    msg = str(exc_info.value)
    assert calls == [TimeSeriesPredictor, TabularPredictor]
    assert str(tmp_path) in msg
    assert "timeseries: timeseries load failed" in msg
    assert "tabular: tabular load failed" in msg


def test_detect_runs_safe_load_validation_once(monkeypatch, tmp_path):
    calls = {"validate": 0, "load": 0}

    def fake_validate(path, **_kwargs):
        assert path == str(tmp_path)
        calls["validate"] += 1

    def fake_load(cls, path, **kwargs):
        assert path == str(tmp_path)
        assert kwargs.get("run_safe_load_validation") is False
        calls["load"] += 1
        if cls is TimeSeriesPredictor:
            raise ValueError("not ts")
        return _tabular_predictor()

    monkeypatch.setattr(
        "autogluonserver.predictor_detect.validate_model_artifacts_for_safe_load",
        fake_validate,
    )
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        fake_load,
    )

    kind, pred = detect_and_load_predictor(str(tmp_path))
    assert kind == "tabular"
    assert isinstance(pred, TabularPredictor)
    assert calls["validate"] == 1
    assert calls["load"] == 2


def test_detect_enforce_rejects_forbidden_pickle(monkeypatch, tmp_path):
    _write_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    monkeypatch.setenv("AUTOGLUON_SAFE_LOAD_MODE", "enforce")
    monkeypatch.setattr(
        "autogluonserver.predictor_detect.load_predictor_tolerating_patch_mismatch",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(
            AssertionError("load should not be called")
        ),
    )
    with pytest.raises(InferenceError, match="Safe-load validation"):
        detect_and_load_predictor(str(tmp_path))
