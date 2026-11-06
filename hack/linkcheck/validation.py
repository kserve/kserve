#!/usr/bin/env python3

# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Validate scanner data before it influences privileged issue publishing."""

from datetime import datetime, timezone
from pathlib import PurePosixPath


STATUSES = {
    "ok",
    "missing",
    "unreachable",
    "restricted",
    "unknown",
    "removed",
    "suppressed",
}


def timestamp(value):
    parsed = datetime.fromisoformat(value)
    return parsed.replace(tzinfo=timezone.utc) if parsed.tzinfo is None else parsed


def canonical_sources(sources):
    if not isinstance(sources, list):
        raise ValueError("Link sources must be a list")
    locations = {}
    for source in sources:
        if not isinstance(source, dict):
            raise ValueError("Invalid source location")
        path = source.get("path")
        if (
            not isinstance(path, str)
            or not path
            or PurePosixPath(path).is_absolute()
            or ".." in PurePosixPath(path).parts
            or "\\" in path
            or any(ord(char) < 32 for char in path)
        ):
            raise ValueError("Source paths must be relative repository paths")
        line = source.get("line")
        if line is not None and (type(line) is not int or line <= 0):
            raise ValueError("Source line must be a positive integer")
        previous = locations.get(path)
        if (
            previous is None
            or line is not None
            and (previous.get("line") is None or line < previous["line"])
        ):
            locations[path] = {
                "path": path,
                **({"line": line} if line is not None else {}),
            }
    return [locations[path] for path in sorted(locations)]


def validate_observation(item):
    if (
        not isinstance(item, dict)
        or item.get("kind") not in ("local", "github", "external")
        or item.get("status") not in STATUSES
    ):
        raise ValueError("Invalid link observation")
    url = item.get("url")
    if (
        not isinstance(url, str)
        or not url.startswith(("http://", "https://", "repo:/"))
        or len(url) > 16384
        or any(ord(char) < 32 for char in url)
    ):
        raise ValueError("Invalid observed target")
    if not isinstance(item.get("detail"), str):
        raise ValueError("Invalid observation detail")
    canonical_sources(item.get("sources"))


def validate_report(report):
    if (
        not isinstance(report, dict)
        or report.get("version") != 1
        or report.get("scope") != "audit"
        or report.get("complete") is not True
    ):
        raise ValueError("History requires a complete full audit")
    if not isinstance(report.get("observations"), list):
        raise ValueError("Audit observations must be a list")
    if (
        not isinstance(report.get("generated_at"), str)
        or datetime.fromisoformat(report["generated_at"]).tzinfo is None
    ):
        raise ValueError("Audit requires a timezone-aware observation timestamp")
    for item in report["observations"]:
        validate_observation(item)


def read_json(path):
    import json

    with path.open("rb") as stream:
        data = stream.read(20_000_001)
    if len(data) > 20_000_000:
        raise ValueError("Link data exceeds the supported size limit")
    return json.loads(data)
