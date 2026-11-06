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

"""Pure daily history: age failures, latch alerts and confirm recovery."""

import copy
import hashlib
from datetime import date, timedelta

from .validation import canonical_sources, timestamp, validate_observation


def identity(item):
    return hashlib.sha256(item["url"].encode()).hexdigest()


def by_target(observations):
    """One finding per target, with all referring documents retained."""
    result = {}
    for item in observations:
        validate_observation(item)
        key = identity(item)
        if key not in result:
            result[key] = copy.deepcopy(item)
            result[key]["sources"] = canonical_sources(item["sources"])
            continue
        record = result[key]
        if (record["kind"], record["status"]) != (item["kind"], item["status"]):
            raise ValueError("Conflicting observations for the same target")
        record["sources"] = canonical_sources(record["sources"] + item["sources"])
    return dict(sorted(result.items(), key=lambda pair: pair[1]["url"]))


def restore_reported(records):
    """Recover open alerts from the issue if the workflow cache was evicted."""
    return {"version": 1, "records": copy.deepcopy(records), "last_run": None}


def advance(previous, report, today):
    if (
        report.get("version") != 1
        or not report.get("complete")
        or report.get("scope") != "audit"
    ):
        raise ValueError("History requires a complete full audit")
    if previous is not None and previous.get("version") != 1:
        raise ValueError("Unsupported link history schema")
    state = copy.deepcopy(previous) if previous else restore_reported({})
    checked_at = report.get("generated_at") or today.isoformat()
    if state.get("checked_at") and timestamp(checked_at) < timestamp(
        state["checked_at"]
    ):
        raise ValueError("An older audit cannot replace newer history")
    if state.get("last_run") and state["last_run"] > today.isoformat():
        raise ValueError("An older audit cannot replace newer history")
    current = by_target(report["observations"])
    records = state["records"]
    for key in list(records.keys() - current.keys()):
        last_reference = records[key].get("last_referenced", today.isoformat())
        if (
            not records[key]["actionable"]
            and (today - date.fromisoformat(last_reference)).days > 30
        ):
            del records[key]
    # Disappearing references were removed, fixed, or explicitly suppressed in
    # a complete audit. They still need two distinct good days to close alerts.
    for key in records.keys() - current.keys():
        current[key] = {
            **records[key],
            "status": "removed",
            "detail": "Reference is no longer failing or monitored",
        }
    for key, item in current.items():
        record = records.setdefault(
            key, {"samples": [], "actionable": False, "first_failed": None}
        )
        record.setdefault("samples", [])
        record.setdefault("first_failed", None)
        if item["status"] != "removed":
            record["last_referenced"] = today.isoformat()
        last_day = record.get("last_seen") or max(
            (sample["date"] for sample in record["samples"]), default=None
        )
        last_day = date.fromisoformat(last_day) if last_day else None
        last_failure = record.get("last_failure") or max(
            (
                sample["date"]
                for sample in record["samples"]
                if sample["status"] in ("missing", "unreachable")
            ),
            default=None,
        )
        last_failure = date.fromisoformat(last_failure) if last_failure else None
        samples = [
            sample
            for sample in record["samples"]
            if date.fromisoformat(sample["date"]) >= today - timedelta(days=30)
            and sample["date"] != today.isoformat()
        ]
        status = item["status"]
        if (
            status in ("missing", "unreachable")
            and not record["actionable"]
            and record.get("recovered_at")
            and (
                not record.get("outage_started_at")
                or timestamp(record["outage_started_at"])
                < timestamp(record["recovered_at"])
            )
        ):
            record["outage_started_at"] = checked_at
        # Unknowns do not establish recovery, and do not count as evidence of
        # continuous downtime. Long gaps restart the age of unreported failures.
        if status in ("ok", "removed", "suppressed"):
            record["first_failed"] = None
        elif last_day and (today - last_day).days > 2:
            record["first_failed"] = (
                today.isoformat() if status in ("missing", "unreachable") else None
            )
        elif status in ("missing", "unreachable") and not record["first_failed"]:
            record["first_failed"] = today.isoformat()
        if status in ("missing", "unreachable"):
            if last_failure and (today - last_failure).days > 2:
                record["first_failed"] = today.isoformat()
            record["last_failure"] = today.isoformat()
        elif (
            status in ("unknown", "restricted")
            and last_failure
            and (today - last_failure).days > 2
        ):
            record["first_failed"] = None
        samples.append(
            {"date": today.isoformat(), "status": status, "checked_at": checked_at}
        )
        record.update(
            {
                field: item[field]
                for field in ("url", "kind", "status", "detail", "sources")
            }
        )
        record["samples"] = samples
        record["last_seen"] = today.isoformat()
        record["checked_at"] = checked_at
        if status in ("ok", "removed", "suppressed"):
            record["last_success"] = today.isoformat()
            if len(samples) >= 2 and all(
                sample["status"] in ("ok", "removed", "suppressed")
                for sample in samples[-2:]
            ):
                record["actionable"] = False
                record["recovered_at"] = checked_at
                record.pop("publication_id", None)
        elif status == "missing" and item["kind"] in ("local", "github"):
            record["actionable"] = True
        elif status in ("missing", "unreachable") and record["first_failed"]:
            first = date.fromisoformat(record["first_failed"])
            recent = [
                sample
                for sample in samples
                if date.fromisoformat(sample["date"]) >= today - timedelta(days=7)
                and sample["status"] in ("missing", "unreachable")
            ]
            if (today - first).days >= 7 and len(recent) >= 5:
                record["actionable"] = True
    state["last_run"] = today.isoformat()
    state["checked_at"] = checked_at
    return state
