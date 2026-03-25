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

from __future__ import annotations

import os
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Union

import numpy as np
import pandas as pd
from autogluon.timeseries import TimeSeriesDataFrame, TimeSeriesPredictor

from kserve import Model
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve.utils.utils import get_predict_response
from kserve_storage import Storage

from autogluonserver.runtime_paths import ensure_autogluon_runtime_paths


@dataclass
class TimeSeriesInferenceMetadata:
    target: str
    id_column: str
    timestamp_column: str
    prediction_length: int
    known_covariates_names: List[str]


def _load_ts_metadata(predictor: TimeSeriesPredictor) -> TimeSeriesInferenceMetadata:
    known_raw = getattr(predictor, "known_covariates_names", None) or []
    if isinstance(known_raw, (list, tuple)):
        known_list = [str(x) for x in known_raw]
    else:
        known_list = []

    target = getattr(predictor, "target", None) or os.environ.get(
        "AUTOGLUON_TS_TARGET", "target"
    )
    pl = int(getattr(predictor, "prediction_length", 1) or 1)
    return TimeSeriesInferenceMetadata(
        target=str(target),
        id_column=os.environ.get("AUTOGLUON_TS_ID_COLUMN", "item_id"),
        timestamp_column=os.environ.get("AUTOGLUON_TS_TIMESTAMP_COLUMN", "timestamp"),
        prediction_length=pl,
        known_covariates_names=known_list,
    )


def _dataframe_to_tsdf(
    df: pd.DataFrame, meta: TimeSeriesInferenceMetadata
) -> TimeSeriesDataFrame:
    missing = {meta.id_column, meta.timestamp_column, meta.target} - set(df.columns)
    if missing:
        raise InferenceError(
            f"instances DataFrame is missing required columns {sorted(missing)}. "
            f"Expected id_column={meta.id_column!r}, timestamp_column={meta.timestamp_column!r}, "
            f"target={meta.target!r}."
        )
    return TimeSeriesDataFrame.from_data_frame(
        df,
        id_column=meta.id_column,
        timestamp_column=meta.timestamp_column,
    )


def _known_covariates_to_tsdf(
    rows: List[Dict[str, Any]], meta: TimeSeriesInferenceMetadata
) -> TimeSeriesDataFrame:
    df = pd.DataFrame(rows)
    required = {meta.id_column, meta.timestamp_column, *meta.known_covariates_names}
    missing = required - set(df.columns)
    if missing:
        raise InferenceError(
            f"known_covariates is missing columns {sorted(missing)}. "
            f"Required: id/timestamp and {meta.known_covariates_names}."
        )
    return TimeSeriesDataFrame.from_data_frame(
        df,
        id_column=meta.id_column,
        timestamp_column=meta.timestamp_column,
    )


def _payload_instances_to_dataframe(payload: Dict) -> pd.DataFrame:
    """Build history DataFrame from v1 JSON (list of row dicts)."""
    raw = payload.get("instances")
    if raw is None:
        raw = payload.get("inputs")
    if raw is None:
        raise InferenceError(
            "JSON body must include 'instances' (time series history rows)."
        )
    if len(raw) == 0:
        raise InferenceError("'instances' must be a non-empty array.")
    if isinstance(raw, pd.DataFrame):
        return raw
    if isinstance(raw, list) and all(isinstance(r, dict) for r in raw):
        return pd.DataFrame(raw)
    return pd.DataFrame(raw)


def _forecast_to_records(forecasts: pd.DataFrame) -> List[Dict[str, Any]]:
    work = forecasts.reset_index().copy()
    for col in work.columns:
        if pd.api.types.is_datetime64_any_dtype(work[col]):
            work[col] = work[col].dt.strftime("%Y-%m-%dT%H:%M:%S")
    records: List[Dict[str, Any]] = []
    for row in work.to_dict(orient="records"):
        out: Dict[str, Any] = {}
        for k, v in row.items():
            if isinstance(v, (np.floating, float)):
                out[k] = float(v)
            elif isinstance(v, (np.integer, int)) and not isinstance(v, bool):
                out[k] = int(v)
            elif pd.isna(v):
                out[k] = None
            else:
                out[k] = v
        records.append(out)
    return records


class AutoGluonTimeSeriesModel(Model):
    """Serve AutoGluon ``TimeSeriesPredictor`` via KServe REST v1 JSON."""

    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.platform = "autogluon-timeseries"
        self.versions = ["1"]
        self.ready = False
        self._predictor: Optional[TimeSeriesPredictor] = None
        self._metadata: Optional[TimeSeriesInferenceMetadata] = None

    def load(self) -> bool:
        local = Storage.download(self.model_dir)
        if not os.path.isdir(local):
            raise ModelMissingError(local)
        self._predictor = TimeSeriesPredictor.load(local)
        self._metadata = _load_ts_metadata(self._predictor)
        self.ready = True
        return self.ready

    def get_input_types(self) -> List[Dict]:
        """Time series uses REST v1 JSON only in phase 1; no v2 tensor schema."""
        return []

    def get_output_types(self) -> List[Dict]:
        return []

    def predict(
        self,
        payload: Union[Dict, InferRequest],
        headers: Optional[Dict[str, str]] = None,
    ) -> Union[Dict, InferResponse]:
        if isinstance(payload, InferRequest):
            raise InferenceError(
                "AutoGluon Time Series supports REST v1 JSON only: POST "
                "/v1/models/{model_name}:predict with Content-Type application/json. "
                "Use 'instances' for history and optional 'known_covariates' for the horizon."
            )
        if self._predictor is None or self._metadata is None:
            raise InferenceError("model is not loaded")

        try:
            instances = _payload_instances_to_dataframe(payload)

            meta = self._metadata
            ts_data = _dataframe_to_tsdf(instances, meta)

            known_covariates = payload.get("known_covariates")
            kc_tsdf: Optional[TimeSeriesDataFrame] = None
            if meta.known_covariates_names:
                if not known_covariates:
                    raise InferenceError(
                        "This model was trained with known_covariates_names; "
                        "include a top-level 'known_covariates' array in the JSON body."
                    )
                kc_tsdf = _known_covariates_to_tsdf(known_covariates, meta)

            ensure_autogluon_runtime_paths()
            # Avoid writing prediction_cache under the read-only downloaded model path (e.g. /s3/...).
            forecasts = self._predictor.predict(
                ts_data, known_covariates=kc_tsdf, use_cache=False
            )
            records = _forecast_to_records(forecasts)
            return get_predict_response(payload, records, self.name)
        except InferenceError:
            raise
        except Exception as e:
            raise InferenceError(str(e)) from e
