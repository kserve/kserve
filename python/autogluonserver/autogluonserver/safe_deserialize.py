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
import pickletools
from pathlib import Path
from typing import Iterable, List, Sequence, Set, Tuple

from kserve.errors import InferenceError

_BLOCKED_GLOBALS: Set[Tuple[str, str]] = {
    ("builtins", "eval"),
    ("builtins", "exec"),
    ("os", "popen"),
    ("os", "system"),
    ("posix", "popen"),
    ("posix", "system"),
    ("subprocess", "Popen"),
}

_DEFAULT_PICKLE_SCAN_ENV = "AUTOGLUON_SAFE_PICKLE_SCAN"


def _scan_enabled() -> bool:
    raw = os.environ.get(_DEFAULT_PICKLE_SCAN_ENV, "true")
    return str(raw).strip().lower() not in {"0", "false", "no", "off"}


def _iter_pickle_files(model_dir: str) -> Iterable[Path]:
    root = Path(model_dir)
    if not root.is_dir():
        return []
    return sorted(root.rglob("*.pkl"))


def _parse_global_arg(arg: str | None) -> Tuple[str, str] | None:
    if not arg:
        return None
    parts = arg.split(" ", 1)
    if len(parts) != 2:
        return None
    module = parts[0].strip()
    name = parts[1].strip()
    if not module or not name:
        return None
    return module, name


def scan_pickle_for_blocked_globals(
    file_path: str | Path,
    *,
    blocked_globals: Sequence[Tuple[str, str]] = tuple(_BLOCKED_GLOBALS),
) -> List[str]:
    """
    Inspect pickle opcodes and return blocked ``GLOBAL module.name`` references.
    """
    blocked = set(blocked_globals)
    findings: List[str] = []
    with open(file_path, "rb") as fh:
        for op, arg, _ in pickletools.genops(fh):
            if op.name != "GLOBAL":
                continue
            parsed = _parse_global_arg(arg)
            if parsed is None:
                continue
            if parsed in blocked:
                findings.append(f"{parsed[0]}.{parsed[1]}")
    return findings


def assert_safe_autogluon_artifact(model_dir: str) -> None:
    """
    Reject known-dangerous pickle globals before model deserialization.
    """
    if not _scan_enabled():
        return

    all_findings: List[str] = []
    for pickle_file in _iter_pickle_files(model_dir):
        matches = scan_pickle_for_blocked_globals(pickle_file)
        for match in matches:
            all_findings.append(f"{pickle_file}: {match}")

    if all_findings:
        detail = "; ".join(all_findings)
        raise InferenceError(
            "Blocked unsafe pickle GLOBAL reference while loading AutoGluon artifact. "
            f"Set {_DEFAULT_PICKLE_SCAN_ENV}=false to bypass this check. "
            f"Details: {detail}"
        )
