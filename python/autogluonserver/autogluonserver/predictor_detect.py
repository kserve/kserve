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

from typing import List, Literal, Tuple, Union

from autogluon.tabular import TabularPredictor
from autogluon.timeseries import TimeSeriesPredictor
from kserve.errors import InferenceError


def detect_and_load_predictor(
    predictor_dir: str,
) -> Tuple[
    Literal["timeseries", "tabular"], Union[TimeSeriesPredictor, TabularPredictor]
]:
    """
    Discover which AutoGluon predictor type the save directory contains and load it.

    ``predictor_dir`` is the AutoGluon save path (same as for ``*.save()`` / ``*.load()``).
    """
    errors: List[str] = []
    try:
        ts = TimeSeriesPredictor.load(predictor_dir)
        if isinstance(ts, TimeSeriesPredictor):
            return "timeseries", ts
        errors.append(
            f"timeseries: loaded object is not TimeSeriesPredictor: {type(ts)!r}"
        )
    except Exception as e:
        errors.append(f"timeseries: {e}")
    try:
        tb = TabularPredictor.load(predictor_dir)
        if isinstance(tb, TabularPredictor):
            return "tabular", tb
        errors.append(f"tabular: loaded object is not TabularPredictor: {type(tb)!r}")
    except Exception as e:
        errors.append(f"tabular: {e}")
    detail = "; ".join(errors)
    raise InferenceError(
        f"Could not load AutoGluon predictor from {predictor_dir!r}: {detail}"
    )
