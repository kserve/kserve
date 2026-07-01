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

import numpy as np
import pandas as pd
import pytest
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferInput, InferRequest

from autogluonserver.tabular_model import (
    AutoGluonTabularModel,
    _determine_prediction_datatype,
)


class DummyFeatureMetadata:
    def __init__(self, type_map_raw=None):
        self.type_map_raw = type_map_raw or {}


class DummyPredictor:
    def __init__(
        self,
        *,
        features=None,
        class_labels=None,
        problem_type=None,
        type_map_raw=None,
        predict_result=None,
        predict_proba_result=None,
    ):
        self._features = list(features or [])
        self.class_labels = class_labels
        self.problem_type = problem_type
        self.feature_metadata = DummyFeatureMetadata(type_map_raw=type_map_raw)
        self._predict_result = (
            predict_result if predict_result is not None else pd.Series(["ok"])
        )
        self._predict_proba_result = (
            predict_proba_result
            if predict_proba_result is not None
            else pd.DataFrame({"yes": [0.7], "no": [0.3]})
        )
        self.last_df = None

    def features(self):
        return self._features

    def predict(self, df: pd.DataFrame):
        self.last_df = df.copy()
        return self._predict_result

    def predict_proba(self, df: pd.DataFrame):
        self.last_df = df.copy()
        return self._predict_proba_result


def _make_v2_request(columns):
    infer_inputs = []
    for name, values in columns.items():
        first = values[0] if len(values) > 0 else None
        datatype = "BYTES" if isinstance(first, (bytes, bytearray, str)) else "FP64"
        infer_inputs.append(
            InferInput(name=name, shape=[len(values)], datatype=datatype, data=values)
        )
    return InferRequest(model_name="model", infer_inputs=infer_inputs)


def test_load_success_sets_ready_and_output_datatype(monkeypatch, tmp_path):
    predictor = DummyPredictor(
        features=["f1"], class_labels=[0, 1], problem_type="binary"
    )
    monkeypatch.setattr(
        "autogluonserver.tabular_model.Storage.download", lambda _: str(tmp_path)
    )
    monkeypatch.setattr(
        "autogluonserver.tabular_model.TabularPredictor.load", lambda _: predictor
    )

    model = AutoGluonTabularModel("model", "s3://bucket/path")
    assert model.load()
    assert model.ready
    assert model._prediction_datatype == "INT64"


def test_load_raises_model_missing_error(monkeypatch, tmp_path):
    missing_dir = tmp_path / "missing"
    monkeypatch.setattr(
        "autogluonserver.tabular_model.Storage.download", lambda _: str(missing_dir)
    )

    model = AutoGluonTabularModel("model", "s3://bucket/path")
    with pytest.raises(ModelMissingError):
        model.load()


def test_predict_v1_instances(monkeypatch):
    predictor = DummyPredictor(
        features=["a", "b"], predict_result=pd.Series(["yes", "no"])
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor
    model.ready = True

    response = model.predict({"instances": [[1, "x"], [2, "y"]]})
    assert response["predictions"] == ["yes", "no"]
    assert predictor.last_df.columns.tolist() == ["a", "b"]


def test_predict_v2_success_decodes_bytes_and_returns_int64():
    predictor = DummyPredictor(
        features=["f1", "f2"],
        class_labels=[0, 1],
        problem_type="binary",
        predict_result=pd.Series([1, 0]),
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor
    model._prediction_datatype = "INT64"
    model.ready = True

    request = _make_v2_request({"f1": [1.0, 2.0], "f2": [b"x", b"y"]})
    infer_response = model.predict(request)
    infer_dict, _ = infer_response.to_rest()

    assert infer_dict["outputs"][0]["name"] == "predictions"
    assert infer_dict["outputs"][0]["datatype"] == "INT64"
    assert infer_dict["outputs"][0]["data"] == [1, 0]
    assert predictor.last_df["f2"].tolist() == ["x", "y"]


def test_predict_v2_uses_predict_proba_when_enabled(monkeypatch):
    predictor = DummyPredictor(
        features=["f1", "f2"],
        class_labels=["yes", "no"],
        problem_type="binary",
        predict_result=pd.Series(["yes", "no"]),
        predict_proba_result=pd.DataFrame({"yes": [0.7, 0.2], "no": [0.3, 0.8]}),
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor
    model._prediction_datatype = "BYTES"
    model.ready = True

    monkeypatch.setenv("PREDICT_PROBA", "true")
    request = _make_v2_request({"f1": [1.0, 2.0], "f2": [b"x", b"y"]})
    infer_response = model.predict(request)
    infer_dict, _ = infer_response.to_rest()

    assert [output["name"] for output in infer_dict["outputs"]] == [
        "proba_yes",
        "proba_no",
    ]
    assert all(output["datatype"] == "FP64" for output in infer_dict["outputs"])
    assert infer_dict["outputs"][0]["data"] == pytest.approx([0.7, 0.2])
    assert infer_dict["outputs"][1]["data"] == pytest.approx([0.3, 0.8])
    assert predictor.last_df["f2"].tolist() == ["x", "y"]


def test_predict_v1_uses_predict_proba_when_enabled(monkeypatch):
    predictor = DummyPredictor(
        features=["f1", "f2"],
        class_labels=["yes", "no"],
        problem_type="binary",
        predict_result=pd.Series(["yes", "no"]),
        predict_proba_result=pd.DataFrame({"yes": [0.61, 0.42], "no": [0.39, 0.58]}),
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor
    model.ready = True

    monkeypatch.setenv("PREDICT_PROBA", "true")
    response = model.predict({"instances": [[1.0, "x"], [2.0, "y"]]})

    assert response["predictions"] == [
        {"yes": 0.61, "no": 0.39},
        {"yes": 0.42, "no": 0.58},
    ]
    assert predictor.last_df.columns.tolist() == ["f1", "f2"]


def test_predict_v2_missing_feature_raises_inference_error():
    predictor = DummyPredictor(
        features=["f1", "f2"], class_labels=[0, 1], predict_result=pd.Series([1])
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor
    model._prediction_datatype = "INT64"

    request = _make_v2_request({"f1": [1.0]})
    with pytest.raises(InferenceError, match="missing required feature columns"):
        model.predict(request)


def test_predict_v2_invalid_shape_raises_inference_error():
    predictor = DummyPredictor(features=["f1", "f2"], predict_result=pd.Series([1, 0]))
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor

    request = InferRequest(
        model_name="model",
        infer_inputs=[
            InferInput(
                name="f1",
                shape=[2, 2],
                datatype="FP64",
                data=[[1.0, 2.0], [3.0, 4.0]],
            ),
            InferInput(name="f2", shape=[2], datatype="FP64", data=[5.0, 6.0]),
        ],
    )
    with pytest.raises(InferenceError, match="invalid shape"):
        model.predict(request)


def test_predict_v2_inconsistent_batch_raises_inference_error():
    predictor = DummyPredictor(features=["f1", "f2"], predict_result=pd.Series([1, 0]))
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor

    request = _make_v2_request({"f1": [1.0, 2.0], "f2": [5.0]})
    with pytest.raises(InferenceError, match="inconsistent batch length"):
        model.predict(request)


def test_get_input_and_output_types_for_regression():
    predictor = DummyPredictor(
        features=["age", "city"],
        problem_type="regression",
        type_map_raw={"age": "int64", "city": "object"},
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor

    assert model.get_input_types() == [
        {"name": "age", "datatype": "INT64", "shape": [-1]},
        {"name": "city", "datatype": "BYTES", "shape": [-1]},
    ]
    assert model.get_output_types() == [
        {"name": "predictions", "datatype": "FP64", "shape": [-1]}
    ]


def test_get_output_types_for_predict_proba(monkeypatch):
    predictor = DummyPredictor(
        features=["f1"],
        class_labels=["A", "A", "1 B"],
        problem_type="multiclass",
    )
    model = AutoGluonTabularModel("model", "/tmp/model")
    model._predictor = predictor

    monkeypatch.setenv("PREDICT_PROBA", "true")
    output_types = model.get_output_types()
    assert [item["name"] for item in output_types] == [
        "proba_A",
        "proba_A_2",
        "proba__1_B",
    ]
    assert all(item["datatype"] == "FP64" for item in output_types)


def test_determine_prediction_datatype_variants():
    assert (
        _determine_prediction_datatype(DummyPredictor(class_labels=[0, 1])) == "INT64"
    )
    assert (
        _determine_prediction_datatype(
            DummyPredictor(class_labels=[0.1, np.float64(2)])
        )
        == "FP64"
    )
    assert (
        _determine_prediction_datatype(DummyPredictor(class_labels=["no", "yes"]))
        == "BYTES"
    )
