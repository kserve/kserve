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

"""Filesystem paths for AutoGluon in containers (OpenShift / non-root)."""

from __future__ import annotations

import os


def ensure_autogluon_runtime_paths() -> None:
    """Ensure matplotlib cache and process cwd allow AutoGluon to mkdir ./AutogluonModels.

    SeasonalNaive fallback (short TS history) calls setup_outputdir under the current working
    directory. Default cwd is often ``/``; arbitrary UIDs may also lack a usable HOME
    (see ``os.access`` false positives vs ``makedirs``). Prefer ``/tmp`` first.
    """
    if not os.environ.get("MPLCONFIGDIR"):
        os.environ["MPLCONFIGDIR"] = "/tmp/matplotlib"

    root = os.path.realpath("/")
    candidates: list[str] = []
    tmp = os.environ.get("TMPDIR") or "/tmp"
    candidates.append(tmp)
    home = os.environ.get("HOME")
    if home:
        expanded = os.path.expanduser(home)
        if expanded not in ("", "/") and os.path.realpath(expanded) != root:
            candidates.append(expanded)
    candidates.append("/home/kserve")

    seen: set[str] = set()
    for d in candidates:
        if not d or d in seen:
            continue
        seen.add(d)
        try:
            rp = os.path.realpath(d)
        except OSError:
            continue
        if rp == root:
            continue
        if os.path.isdir(rp) and os.access(rp, os.W_OK):
            try:
                os.chdir(rp)
                return
            except OSError:
                continue
