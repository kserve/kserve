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

from kserve import Model
from kserve_storage import Storage

from autogluonserver.artifact_layout import has_timeseries_metadata_marker
from autogluonserver.tabular_model import AutoGluonTabularModel
from autogluonserver.timeseries_model import AutoGluonTimeSeriesModel

ENV_PREDICTOR_TYPE = "AUTOGLUON_PREDICTOR_TYPE"


def create_autogluon_model(name: str, model_dir: str) -> Model:
    """
    Instantiate the correct AutoGluon model implementation.

    ``AUTOGLUON_PREDICTOR_TYPE``:
      - ``tabular`` — TabularPredictor only.
      - ``timeseries`` — TimeSeriesPredictor only.
      - ``auto`` (default) — if ``predictor_metadata.json`` is present next to the
        artifact (see :mod:`autogluonserver.artifact_layout`), use time series;
        otherwise tabular.
    """
    mode = os.environ.get(ENV_PREDICTOR_TYPE, "auto").lower().strip()
    if mode == "tabular":
        return AutoGluonTabularModel(name, model_dir)
    if mode == "timeseries":
        return AutoGluonTimeSeriesModel(name, model_dir)

    local = Storage.download(model_dir)
    if has_timeseries_metadata_marker(local):
        return AutoGluonTimeSeriesModel(name, model_dir)
    return AutoGluonTabularModel(name, model_dir)
