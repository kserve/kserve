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
from typing import Dict, List, Optional, Union

from kserve import Model
from kserve.errors import InferenceError, ModelMissingError
from kserve.protocol.infer_type import InferRequest, InferResponse
from kserve_storage import Storage

from autogluonserver.predictor_detect import detect_and_load_predictor
from autogluonserver.tabular_model import (
    AutoGluonTabularModel,
    _determine_prediction_datatype,
)
from autogluonserver.timeseries_model import AutoGluonTimeSeriesModel, _load_ts_metadata


class AutoGluonDetectedModel(Model):
    """
    Loads the model directory with AutoGluon, detects tabular vs time series by try-load,
    and delegates to :class:`AutoGluonTabularModel` or :class:`AutoGluonTimeSeriesModel`.
    """

    def __init__(self, name: str, model_dir: str):
        super().__init__(name)
        self.name = name
        self.model_dir = model_dir
        self.platform = "autogluon"
        self.versions = ["1"]
        self.ready = False
        self._impl: Optional[Model] = None

    def load(self) -> bool:
        local = Storage.download(self.model_dir)
        if not os.path.isdir(local):
            raise ModelMissingError(local)
        kind, _predictor = detect_and_load_predictor(local)
        if kind == "timeseries":
            impl = AutoGluonTimeSeriesModel(self.name, self.model_dir)
            impl._predictor = _predictor
            impl._metadata = _load_ts_metadata(_predictor)
            impl.ready = True
        else:
            impl = AutoGluonTabularModel(self.name, self.model_dir)
            impl._predictor = _predictor
            impl._prediction_datatype = _determine_prediction_datatype(_predictor)
            impl.ready = True
        self._impl = impl
        self.platform = impl.platform
        self.ready = True
        return True

    def predict(
        self,
        payload: Union[Dict, InferRequest],
        headers: Optional[Dict[str, str]] = None,
    ) -> Union[Dict, InferResponse]:
        if self._impl is None:
            raise InferenceError("model is not loaded")
        return self._impl.predict(payload, headers)

    def get_input_types(self) -> List[Dict]:
        if self._impl is None:
            return []
        return self._impl.get_input_types()

    def get_output_types(self) -> List[Dict]:
        if self._impl is None:
            return []
        return self._impl.get_output_types()


def create_autogluon_model(name: str, model_dir: str) -> Model:
    """Return a KServe model that auto-detects Tabular vs TimeSeries on load."""
    return AutoGluonDetectedModel(name, model_dir)
