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

import os
import re
from typing import Dict, List, Optional, Tuple, Union

import numpy as np
import pandas as pd
from autogluon.tabular import TabularPredictor

from kserve import Model
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferOutput, InferRequest, InferResponse
from kserve.utils.utils import generate_uuid, get_predict_input, get_predict_response
from kserve_storage import Storage

ENV_PREDICT_PROBA = "PREDICT_PROBA"
PROBLEM_TYPE_REGRESSION = "regression"
PROBLEM_TYPE_QUANTILE = "quantile"
PROBLEM_TYPE_BINARY = "binary"
PROBLEM_TYPE_MULTICLASS = "multiclass"


def _get_features(predictor) -> list:
    """Return list of feature names. AutoGluon exposes this as a method features(), not an attribute."""
    if predictor is None:
        return []
    f = getattr(predictor, "features", None)
    if f is None:
        return []
    return f() if callable(f) else list(f)


def _tensor_to_dataframe(instances, predictor) -> pd.DataFrame:
    """Build a DataFrame with model feature names from v2 tensor input (ndarray or DataFrame with integer columns)."""
    features = _get_features(predictor)
    if isinstance(instances, np.ndarray):
        if not features:
            return pd.DataFrame(instances)
        arr = instances
        if arr.ndim == 1:
            arr = arr.reshape(1, -1)
        n_features = len(features)
        if arr.shape[1] != n_features:
            raise InferenceError(
                f"v2 tensor has {arr.shape[1]} columns but model expects {n_features} features "
                f"(order: {features}). Send data in row-major order matching GET /v2/models/{{name}} inputs."
            )
        return pd.DataFrame(arr, columns=features)

    if isinstance(instances, pd.DataFrame):
        cols = instances.columns.tolist()
        # Integer column names 0,1,...,n-1 from v2 path without column semantics
        if (
            features
            and len(cols) == len(features)
            and all(isinstance(c, (int, np.integer)) for c in cols)
            and cols == list(range(len(cols)))
        ):
            df = instances.copy()
            df.columns = features
            return df
        return instances

    return pd.DataFrame(instances)


def _is_predict_proba_enabled() -> bool:
    return os.environ.get(ENV_PREDICT_PROBA, "false").lower() == "true"


def _get_problem_type(predictor) -> Optional[str]:
    return getattr(predictor, "problem_type", None)


def _get_type_map_raw(predictor) -> Dict[str, str]:
    """Best-effort extraction of raw feature type metadata from AutoGluon predictor."""
    feature_metadata = getattr(predictor, "feature_metadata", None)
    if feature_metadata is None:
        return {}
    type_map_raw = getattr(feature_metadata, "type_map_raw", None)
    if isinstance(type_map_raw, dict):
        return type_map_raw
    get_type_map_raw = getattr(feature_metadata, "get_type_map_raw", None)
    if callable(get_type_map_raw):
        val = get_type_map_raw()
        if isinstance(val, dict):
            return val
    return {}


def _feature_to_v2_datatype(raw_type: Optional[str]) -> str:
    """Map AutoGluon raw feature types to v2 tensor datatypes."""
    t = (raw_type or "").lower()
    if t in {
        "int",
        "int8",
        "int16",
        "int32",
        "int64",
        "uint8",
        "uint16",
        "uint32",
        "uint64",
    }:
        return "INT64"
    if t in {"float", "float16", "float32", "float64"}:
        return "FP64"
    if t in {"bool", "boolean"}:
        return "BOOL"
    # category/object/text/datetime and any unknown types are safest as BYTES
    return "BYTES"


def _decode_bytes_like(value):
    if isinstance(value, (bytes, bytearray)):
        return value.decode("utf-8")
    return value


def _v2_tabular_contract_hint(features: List[str]) -> str:
    hint = (
        "Expected one tensor per feature with input.name == feature name and shape [batch] "
        "(or [batch,1], which is flattened internally)."
    )
    if features:
        hint += f" Required feature names: {features}."
    return hint


def _sanitize_label(label: object) -> str:
    normalized = str(label).strip().replace(" ", "_")
    normalized = re.sub(r"[^0-9A-Za-z_]", "", normalized)
    if not normalized:
        normalized = "class"
    if normalized[0].isdigit():
        normalized = f"_{normalized}"
    return normalized


def _get_proba_output_names(labels: List[object]) -> List[str]:
    names: List[str] = []
    used = set()
    for label in labels:
        base = f"proba_{_sanitize_label(label)}"
        candidate = base
        index = 2
        while candidate in used:
            candidate = f"{base}_{index}"
            index += 1
        used.add(candidate)
        names.append(candidate)
    return names


def _determine_prediction_datatype(predictor) -> str:
    """Determine v2 output datatype for classification labels from predictor.class_labels."""
    labels = list(getattr(predictor, "class_labels", None) or [])
    if not labels:
        return "BYTES"

    if all(
        isinstance(label, (int, np.integer)) and not isinstance(label, bool)
        for label in labels
    ):
        return "INT64"

    numeric_labels = pd.to_numeric(pd.Series(labels), errors="coerce")
    if numeric_labels.isna().any():
        return "BYTES"

    return "FP64"


def _series_to_prediction_numpy(result: pd.Series, datatype: str) -> np.ndarray:
    """Convert prediction series to numpy array matching the selected v2 datatype."""
    if datatype == "INT64":
        numeric = pd.to_numeric(result, errors="coerce")
        if numeric.isna().any():
            raise InferenceError(
                "prediction labels are not fully numeric but output datatype is INT64"
            )
        values = numeric.to_numpy(dtype=np.float64)
        if not np.all(np.equal(values, np.floor(values))):
            raise InferenceError(
                "prediction labels contain fractional values but output datatype is INT64"
            )
        return values.astype(np.int64)
    if datatype == "FP64":
        numeric = pd.to_numeric(result, errors="coerce")
        if numeric.isna().any():
            raise InferenceError(
                "prediction labels are not fully numeric but output datatype is FP64"
            )
        return numeric.to_numpy(dtype=np.float64)
    return result.astype(str).to_numpy(dtype=np.object_)


def _infer_request_to_dataframe(payload: InferRequest, predictor) -> pd.DataFrame:
    """Parse v2 InferRequest into DataFrame in a protocol-compliant way.

    Supports tabular payloads:
    - one tensor per feature
    - tensor name == feature name
    - shape [batch] or [batch,1]
    """
    features = _get_features(predictor)
    inputs = payload.inputs or []
    if len(inputs) == 0:
        raise InferenceError(
            f"v2 infer request has no inputs. {_v2_tabular_contract_hint(features)}"
        )

    columns: Dict[str, List] = {}
    n_rows: Optional[int] = None
    for input_tensor in inputs:
        name = input_tensor.name
        arr = input_tensor.as_numpy()
        if arr.ndim == 1:
            pass
        elif arr.ndim == 2 and arr.shape[1] == 1:
            arr = arr.reshape(-1)
        else:
            raise InferenceError(
                f"input '{name}' has invalid shape {list(arr.shape)}. "
                "Each feature tensor must be shape [batch] or [batch,1]. "
                f"{_v2_tabular_contract_hint(features)}"
            )
        values = [_decode_bytes_like(v) for v in arr.tolist()]
        if n_rows is None:
            n_rows = len(values)
        elif n_rows != len(values):
            raise InferenceError(
                f"inconsistent batch length in request: expected {n_rows}, got {len(values)} "
                f"for input '{name}'. {_v2_tabular_contract_hint(features)}"
            )
        columns[name] = values

    if features:
        missing = [f for f in features if f not in columns]
        if missing:
            raise InferenceError(
                f"missing required feature columns for v2 infer: {missing}. "
                f"{_v2_tabular_contract_hint(features)}"
            )
        # Ignore additional columns to keep compatibility with clients that may send extras
        columns = {f: columns[f] for f in features}
    return pd.DataFrame(columns)


def _build_v2_outputs(
    result, predictor, prediction_datatype: str = "BYTES"
) -> Tuple[List[InferOutput], List[Dict]]:
    """Build InferResponse outputs and corresponding metadata descriptors."""
    problem_type = (_get_problem_type(predictor) or "").lower()
    if isinstance(result, pd.Series):
        if problem_type in {PROBLEM_TYPE_REGRESSION, PROBLEM_TYPE_QUANTILE}:
            values = pd.to_numeric(result, errors="coerce").to_numpy(dtype=np.float64)
            output = InferOutput(
                name="predictions", shape=[len(values)], datatype="FP64"
            )
            output.set_data_from_numpy(values, binary_data=False)
            return [output], [
                {"name": "predictions", "datatype": "FP64", "shape": [-1]}
            ]
        values = _series_to_prediction_numpy(result, prediction_datatype)
        output = InferOutput(
            name="predictions", shape=[len(values)], datatype=prediction_datatype
        )
        output.set_data_from_numpy(values, binary_data=False)
        return [output], [
            {"name": "predictions", "datatype": prediction_datatype, "shape": [-1]}
        ]

    if isinstance(result, pd.DataFrame):
        if problem_type == PROBLEM_TYPE_QUANTILE:
            values = result.apply(pd.to_numeric, errors="coerce").to_numpy(
                dtype=np.float64
            )
            output = InferOutput(
                name="predictions", shape=list(values.shape), datatype="FP64"
            )
            output.set_data_from_numpy(values, binary_data=False)
            width = values.shape[1] if values.ndim == 2 else -1
            return [output], [
                {"name": "predictions", "datatype": "FP64", "shape": [-1, width]}
            ]

        if _is_predict_proba_enabled():
            labels = list(getattr(predictor, "class_labels", None) or [])
            if len(labels) != len(result.columns):
                labels = list(result.columns)
            output_names = _get_proba_output_names(labels)
            outputs: List[InferOutput] = []
            metadata: List[Dict] = []
            for col, output_name in zip(result.columns, output_names, strict=False):
                values = pd.to_numeric(result[col], errors="coerce").to_numpy(
                    dtype=np.float64
                )
                output = InferOutput(
                    name=output_name, shape=[len(values)], datatype="FP64"
                )
                output.set_data_from_numpy(values, binary_data=False)
                outputs.append(output)
                metadata.append(
                    {"name": output_name, "datatype": "FP64", "shape": [-1]}
                )
            return outputs, metadata

        outputs: List[InferOutput] = []
        metadata: List[Dict] = []
        for col in result.columns:
            col_name = str(col)
            values = pd.to_numeric(result[col], errors="coerce").to_numpy(
                dtype=np.float64
            )
            output = InferOutput(name=col_name, shape=[len(values)], datatype="FP64")
            output.set_data_from_numpy(values, binary_data=False)
            outputs.append(output)
            metadata.append({"name": col_name, "datatype": "FP64", "shape": [-1]})
        return outputs, metadata

    if isinstance(result, np.ndarray):
        arr = result
    else:
        arr = np.array(result)

    if arr.ndim == 0:
        arr = arr.reshape(1)

    if np.issubdtype(arr.dtype, np.number):
        if problem_type in {PROBLEM_TYPE_REGRESSION, PROBLEM_TYPE_QUANTILE}:
            values = arr.astype(np.float64)
            output = InferOutput(
                name="predictions", shape=list(values.shape), datatype="FP64"
            )
            output.set_data_from_numpy(values, binary_data=False)
            return [output], [
                {"name": "predictions", "datatype": "FP64", "shape": [-1]}
            ]
        if prediction_datatype == "INT64":
            values = arr.astype(np.float64)
            if not np.all(np.equal(values, np.floor(values))):
                raise InferenceError(
                    "prediction labels contain fractional values but output datatype is INT64"
                )
            values = values.astype(np.int64)
            output = InferOutput(
                name="predictions", shape=list(values.shape), datatype="INT64"
            )
            output.set_data_from_numpy(values, binary_data=False)
            return [output], [
                {"name": "predictions", "datatype": "INT64", "shape": [-1]}
            ]
        if prediction_datatype == "FP64":
            values = arr.astype(np.float64)
            output = InferOutput(
                name="predictions", shape=list(values.shape), datatype="FP64"
            )
            output.set_data_from_numpy(values, binary_data=False)
            return [output], [
                {"name": "predictions", "datatype": "FP64", "shape": [-1]}
            ]

    values = arr.astype(str).astype(np.object_)
    output = InferOutput(name="predictions", shape=list(values.shape), datatype="BYTES")
    output.set_data_from_numpy(values, binary_data=False)
    return [output], [{"name": "predictions", "datatype": "BYTES", "shape": [-1]}]


class AutoGluonModel(Model):
    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.platform = "autogluon-tabular"
        self.versions = ["1"]
        self.ready = False
        self._prediction_datatype = "BYTES"

    def load(self) -> bool:
        model_path = Storage.download(self.model_dir)
        if not os.path.isdir(model_path):
            raise ModelMissingError(model_path)
        self._predictor = TabularPredictor.load(model_path)
        self._prediction_datatype = _determine_prediction_datatype(self._predictor)
        self.ready = True
        return self.ready

    def get_input_types(self) -> List[Dict]:
        """Return v2 model metadata inputs: one tensor per feature, in predictor.features() order."""
        predictor = getattr(self, "_predictor", None)
        features = _get_features(predictor)
        if not features:
            return []
        type_map_raw = _get_type_map_raw(predictor)
        # One entry per feature so clients know column order and expected dtype for v2 payloads
        return [
            {
                "name": name,
                "datatype": _feature_to_v2_datatype(type_map_raw.get(name)),
                "shape": [-1],
            }
            for name in features
        ]

    def get_output_types(self) -> List[Dict]:
        """Return v2 model metadata outputs matching current prediction mode."""
        predictor = getattr(self, "_predictor", None)
        if predictor is None:
            return []
        if _is_predict_proba_enabled() and hasattr(predictor, "class_labels"):
            labels = getattr(predictor, "class_labels", None) or []
            if labels:
                output_names = _get_proba_output_names(list(labels))
                return [
                    {"name": output_name, "datatype": "FP64", "shape": [-1]}
                    for output_name in output_names
                ]
        problem_type = (_get_problem_type(predictor) or "").lower()
        if problem_type == PROBLEM_TYPE_QUANTILE:
            levels = list(getattr(predictor, "quantile_levels", None) or [])
            width = len(levels) if levels else -1
            return [{"name": "predictions", "datatype": "FP64", "shape": [-1, width]}]
        if problem_type == PROBLEM_TYPE_REGRESSION:
            return [{"name": "predictions", "datatype": "FP64", "shape": [-1]}]
        return [
            {
                "name": "predictions",
                "datatype": getattr(self, "_prediction_datatype", "BYTES"),
                "shape": [-1],
            }
        ]

    def predict(
        self, payload: Union[Dict, InferRequest], headers: Dict[str, str] = None
    ) -> Union[Dict, InferResponse]:
        try:
            if isinstance(payload, InferRequest):
                df = _infer_request_to_dataframe(payload, self._predictor)
            else:
                instances = get_predict_input(payload)
                # v1 / generic dict payload path
                if isinstance(instances, (np.ndarray, pd.DataFrame)) and getattr(
                    self, "_predictor", None
                ):
                    df = _tensor_to_dataframe(instances, self._predictor)
                elif isinstance(instances, pd.DataFrame):
                    df = instances
                else:
                    df = pd.DataFrame(instances)

            if _is_predict_proba_enabled() and hasattr(
                self._predictor, "predict_proba"
            ):
                result = self._predictor.predict_proba(df)
            else:
                result = self._predictor.predict(df)

            if isinstance(payload, InferRequest):
                outputs, _metadata = _build_v2_outputs(
                    result,
                    self._predictor,
                    getattr(self, "_prediction_datatype", "BYTES"),
                )
                return InferResponse(
                    response_id=payload.id if payload.id else generate_uuid(),
                    model_name=self.name,
                    infer_outputs=outputs,
                    use_binary_outputs=payload.use_binary_outputs,
                    requested_outputs=payload.request_outputs,
                )

            if isinstance(result, pd.Series):
                result = result.tolist()
            return get_predict_response(payload, result, self.name)
        except Exception as e:
            raise InferenceError(str(e))
