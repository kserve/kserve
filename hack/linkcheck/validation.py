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

import re
from datetime import date, datetime, timezone
from pathlib import PurePosixPath


# Text renders into one Markdown table row; line breaks would start new rows.
CONTROL = re.compile(r"[\x00-\x1f\x7f]")
DIGEST = re.compile(r"[0-9a-f]{64}")
# Keeps a row well inside the issue budget, so one finding cannot push out the
# rest of its group. Lychee network errors embed the target URL.
DETAIL_LIMIT = 2000

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
            or CONTROL.search(path)
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


def is_status(value):
    return isinstance(value, str) and value in STATUSES


def is_digest(value):
    return isinstance(value, str) and DIGEST.fullmatch(value) is not None


def is_date(value):
    """Accept canonical dates only; history compares them as strings."""
    try:
        return date.fromisoformat(value).isoformat() == value
    except (TypeError, ValueError):
        return False


def is_timestamp(value):
    try:
        timestamp(value)
    except (TypeError, ValueError):
        return False
    return True


def validate_observation(item, *, restored=False):
    """Reject scanner data that the issue cannot render as inert text.

    restored admits the history:<identity> targets that a registry keeps for
    published findings whose URL the issue snapshot omitted.
    """
    if (
        not isinstance(item, dict)
        or item.get("kind") not in ("local", "github", "external")
        or not is_status(item.get("status"))
    ):
        raise ValueError("Invalid link observation")
    url = item.get("url")
    if not isinstance(url, str) or not (
        url.startswith(("http://", "https://", "repo:/"))
        and len(url) <= 16384
        and not CONTROL.search(url)
        or restored
        and url.startswith("history:")
        and is_digest(url.removeprefix("history:"))
    ):
        raise ValueError("Invalid observed target")
    detail = item.get("detail")
    if (
        not isinstance(detail, str)
        or len(detail) > DETAIL_LIMIT
        or CONTROL.search(detail)
    ):
        raise ValueError(
            f"Observation detail must be at most {DETAIL_LIMIT} characters "
            "without control characters"
        )
    canonical_sources(item.get("sources"))


def is_sample(sample):
    return (
        isinstance(sample, dict)
        and is_date(sample.get("date"))
        and is_status(sample.get("status"))
        and ("checked_at" not in sample or is_timestamp(sample["checked_at"]))
    )


# Optional fields; absent means "not observed yet". first_failed is the only
# field history clears to null.
RECORD_FIELDS = {
    "first_failed": lambda value: value is None or is_date(value),
    "samples": lambda value: isinstance(value, list) and all(map(is_sample, value)),
    "publication_id": is_digest,
    **dict.fromkeys(
        (
            "last_seen",
            "last_failure",
            "last_referenced",
            "last_success",
            "observed_day",
        ),
        is_date,
    ),
    **dict.fromkeys(
        ("checked_at", "observed_at", "recovered_at", "outage_started_at"),
        is_timestamp,
    ),
}
STATE_FIELDS = {
    "last_run": lambda value: value is None or is_date(value),
    "checked_at": is_timestamp,
    "issue_number": lambda value: type(value) is int and value > 0,
    "closed_checkpoint": is_digest,
}


def validate_state(state):
    """Reject a whole registry if any part of it is malformed.

    Registries arrive as artifacts of earlier runs, so they get the same
    checks as scanner observations; skipping bad records would silently
    rewrite the alert history built from them.
    """
    if not isinstance(state, dict) or state.get("version") != 1:
        raise ValueError("Unsupported link history schema")
    if not isinstance(state.get("records"), dict):
        raise ValueError("Invalid link history records")
    for field, valid in STATE_FIELDS.items():
        if field in state and not valid(state[field]):
            raise ValueError(f"Invalid link history field: {field}")
    for key, record in state["records"].items():
        if not is_digest(key):
            raise ValueError("Invalid link history identity")
        try:
            validate_observation(record, restored=True)
        except ValueError as error:
            raise ValueError(f"Invalid link history record {key}: {error}") from None
        if type(record.get("actionable")) is not bool:
            raise ValueError(f"Invalid link history record {key}: actionable")
        for field, valid in RECORD_FIELDS.items():
            if field in record and not valid(record[field]):
                raise ValueError(f"Invalid link history record {key}: {field}")


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
