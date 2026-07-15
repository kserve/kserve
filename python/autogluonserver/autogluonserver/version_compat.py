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

import importlib.metadata
import os
from typing import Any, Optional, Protocol, TypeVar

from kserve.errors import InferenceError
from packaging.version import InvalidVersion, Version

_T = TypeVar("_T", covariant=True)


class _PredictorClass(Protocol[_T]):
    __module__: str

    @classmethod
    def load(
        cls,
        path: str,
        require_version_match: bool = ...,
    ) -> _T: ...


# AutoGluon saves the version here; legacy name used in autogluon.tabular <= 1.1.0.
_VERSION_FILENAMES = ("version.txt", "__version__")


def load_predictor_tolerating_patch_mismatch(
    predictor_cls: _PredictorClass[_T], path: str
) -> _T:
    """
    Load an AutoGluon predictor, allowing patch-level version mismatches.

    AutoGluon writes the training-time version to ``version.txt`` inside the
    predictor directory.  This function reads that file and compares it against
    the installed package version *before* calling ``load()``:

    * Same major and minor, different patch or local label (e.g. ``+rhaiv.1``):
      load proceeds with ``require_version_match=False``.
    * Major or minor differs: raises ``InferenceError`` immediately.
    * Version file absent or unreadable: delegates to ``load()`` unchanged so
      AutoGluon's own version-check logic runs as normal.
    """
    saved_v = _read_saved_version(path)
    current_v = _get_installed_version(predictor_cls) if saved_v is not None else None

    if saved_v is not None and current_v is not None:
        if saved_v.major != current_v.major or saved_v.minor != current_v.minor:
            raise InferenceError(
                f"AutoGluon major or minor version mismatch prevents loading: "
                f"predictor was saved with {saved_v}, installed version is {current_v}. "
                "Re-train the predictor with the current AutoGluon version."
            )
        if saved_v != current_v:
            return predictor_cls.load(path, require_version_match=False)

    return predictor_cls.load(path)


def _read_saved_version(predictor_path: str) -> Optional[Version]:
    """Read the version AutoGluon wrote to the predictor directory."""
    for filename in _VERSION_FILENAMES:
        filepath = os.path.join(predictor_path, filename)
        try:
            with open(filepath, "r") as f:
                return Version(f.read().strip())
        except (OSError, InvalidVersion):
            pass
    return None


def _get_installed_version(predictor_cls: _PredictorClass[Any]) -> Optional[Version]:
    """Return the installed version of the AutoGluon package that owns predictor_cls."""
    package = ".".join(predictor_cls.__module__.split(".")[:2])
    try:
        return Version(importlib.metadata.version(package))
    except (importlib.metadata.PackageNotFoundError, InvalidVersion):
        return None
