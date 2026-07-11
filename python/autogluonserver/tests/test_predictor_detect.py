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


def test_detect_returns_timeseries_when_ts_loads(monkeypatch, tmp_path):
    ts_pred = _timeseries_predictor()
    calls = []

    def fake_load(cls, path):
        calls.append(cls)
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

    def fake_load(cls, path):
        calls.append(cls)
        assert path == expected_path
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

    def fake_load(cls, path):
        calls.append(cls)
        assert path == expected_path
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

    def fake_load(cls, path):
        calls.append(cls)
        assert path == str(tmp_path)
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

    def fake_load(cls, path):
        calls.append(cls)
        assert path == str(tmp_path)
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
