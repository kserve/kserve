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

"""Backward-compatible re-exports for tabular AutoGluon model helpers."""

from .tabular_model import (
    AutoGluonTabularModel,
    _build_v2_outputs,
    _determine_prediction_datatype,
    _get_features,
    _get_problem_type,
    _infer_request_to_dataframe,
    _tensor_to_dataframe,
)

# Historical name used by tests and callers
AutoGluonModel = AutoGluonTabularModel

__all__ = [
    "AutoGluonModel",
    "AutoGluonTabularModel",
    "_build_v2_outputs",
    "_determine_prediction_datatype",
    "_get_features",
    "_get_problem_type",
    "_infer_request_to_dataframe",
    "_tensor_to_dataframe",
]
