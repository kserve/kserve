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

"""Resolve Kubeflow / AutoGluon artifact directory layouts on local disk."""

from __future__ import annotations

import os
from typing import Optional, Tuple

METADATA_FILENAME = "predictor_metadata.json"


def resolve_timeseries_artifact_paths(
    local_root: str,
) -> Tuple[str, Optional[str]]:
    """
    Given a downloaded storage root, return:
      (path passed to TimeSeriesPredictor.load / TabularPredictor.load,
       path to predictor_metadata.json or None).

    Supports:
      - ``<MODEL>_FULL/predictor/`` + ``<MODEL>_FULL/predictor_metadata.json``
      - Direct AutoGluon save directory with optional sibling
        ``../predictor_metadata.json`` or embedded ``predictor_metadata.json``.
    """
    root = os.path.realpath(local_root)
    predictor_sub = os.path.join(root, "predictor")
    if os.path.isdir(predictor_sub):
        meta = os.path.join(root, METADATA_FILENAME)
        return predictor_sub, meta if os.path.isfile(meta) else None

    meta_here = os.path.join(root, METADATA_FILENAME)
    if os.path.isfile(meta_here):
        return root, meta_here

    parent_meta = os.path.join(os.path.dirname(root), METADATA_FILENAME)
    if os.path.isfile(parent_meta):
        return root, parent_meta

    return root, None


def has_timeseries_metadata_marker(local_root: str) -> bool:
    """True if predictor_metadata.json is present for this artifact (auto mode)."""
    _, meta = resolve_timeseries_artifact_paths(local_root)
    return meta is not None and os.path.isfile(meta)
