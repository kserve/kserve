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

"""Reconcile one bot-owned issue and preserve text outside its managed section."""

import base64
import copy
import hashlib
import html
import json
import re
import zlib
from datetime import datetime, timezone
from urllib.parse import quote

from .history import restore_reported
from .validation import canonical_sources


MARKER = "<!-- kserve-link-check:v1 -->"
START = "<!-- link-check:start -->"
END = "<!-- link-check:end -->"
AUTO_CLOSED = "<!-- link-check:auto-closed -->"


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True).encode()).hexdigest()


def reported_snapshot(body):
    match = re.search(r"<!-- link-check:keys:([A-Za-z0-9+/=]+) -->", body)
    if not match:
        if AUTO_CLOSED in body:
            return {"version": 1, "records": {}}
        raise ValueError("Open link issue has no valid recovery snapshot")
    # A compressed snapshot preserves the already reported targets if the
    # cache is evicted, including links removed since the previous audit.
    packed = base64.b64decode(match[1], validate=True)
    decompressor = zlib.decompressobj()
    data = decompressor.decompress(packed, 1_000_000)
    if not decompressor.eof:
        raise ValueError("Invalid or oversized issue snapshot")
    snapshot = json.loads(data)
    if (
        not isinstance(snapshot, dict)
        or snapshot.get("version") != 1
        or not isinstance(snapshot.get("records"), dict)
    ):
        raise ValueError("Invalid issue snapshot schema")
    records = snapshot["records"]
    if not isinstance(snapshot.get("defaults", {}), dict) or any(
        not isinstance(record, dict) for record in records.values()
    ):
        raise ValueError("Invalid issue snapshot records")
    if snapshot.get("key_encoding") == "base64":
        records = {
            base64.b64decode(key, validate=True).hex(): record
            for key, record in records.items()
        }
    records = {
        key: {**snapshot.get("defaults", {}), **record}
        for key, record in records.items()
    }
    snapshot["records"] = records
    if any(not re.fullmatch(r"[0-9a-f]{64}", key) for key in records):
        raise ValueError("Invalid issue target identities")
    return snapshot


def reported_records(body):
    snapshot = reported_snapshot(body)
    return {
        key: {
            "url": f"history:{key}",
            "kind": "external",
            "status": "unknown",
            "detail": "Published target restored without full metadata; see the earlier audit",
            "sources": [],
            "first_failed": None,
            **(
                {"observed_at": snapshot["observed_at"]}
                if snapshot.get("observed_at")
                else {}
            ),
            **(
                {"observed_day": snapshot["observed_day"]}
                if snapshot.get("observed_day")
                else {}
            ),
            **record,
        }
        for key, record in snapshot["records"].items()
    }


def encode_snapshot(snapshot):
    fields = {
        "url",
        "kind",
        "status",
        "first_failed",
        "observed_at",
        "observed_day",
        "checked_at",
        "last_seen",
        "recovered_at",
        "outage_started_at",
        "samples",
    }
    snapshot = {
        **snapshot,
        "records": {
            key: {field: value for field, value in record.items() if field in fields}
            for key, record in snapshot["records"].items()
        },
    }
    for record in snapshot["records"].values():
        if "samples" in record:
            record["samples"] = record["samples"][-8:]
    records = snapshot["records"]
    if records:
        first = next(iter(records.values()))
        defaults = {
            field: value
            for field, value in first.items()
            if all(record.get(field) == value for record in records.values())
        }
        snapshot["defaults"] = defaults
        snapshot["records"] = {
            base64.b64encode(bytes.fromhex(key)).decode(): {
                field: value for field, value in record.items() if field not in defaults
            }
            for key, record in records.items()
        }
        snapshot["key_encoding"] = "base64"
    data = json.dumps(snapshot).encode()
    encoded = base64.b64encode(zlib.compress(data)).decode()
    if len(data) > 1_000_000 or len(encoded) > 44000:
        # Preserve every identity and its recovery evidence even when long URLs
        # exceed the issue budget. Current observations refill missing metadata.
        for record in snapshot["records"].values():
            record.pop("url", None)
        data = json.dumps(snapshot).encode()
        encoded = base64.b64encode(zlib.compress(data)).decode()
    if len(data) > 1_000_000:
        raise ValueError("Issue recovery snapshot exceeds its size limit")
    if len(encoded) > 54000:
        raise ValueError(
            "Too many published identities for one issue; audit artifacts were retained"
        )
    return encoded


def reported_keys(body):
    return sorted(reported_records(body))


def seed_from_issue(issue, report):
    return reconcile_from_issue(None, issue)


def observation_time(value):
    parsed = datetime.fromisoformat(value)
    return parsed.replace(tzinfo=timezone.utc) if parsed.tzinfo is None else parsed


def reconcile_closed(previous, issue):
    """Restore confirmed recovery before evaluating any subsequent outage."""
    state = copy.deepcopy(previous) if previous is not None else restore_reported({})
    snapshot = reported_snapshot(issue.get("body") or "")
    checkpoint = digest(snapshot)
    if state.get("closed_checkpoint") == checkpoint:
        return state
    stamp = snapshot.get("closed_at") or issue.get("updated_at")
    if not stamp:
        raise ValueError("Closed link issue has no recovery timestamp")
    cutoff = observation_time(stamp)
    records = state["records"]
    recovered = snapshot["records"]
    for key in records.keys() | recovered.keys():
        record = records.get(key)
        if (
            record
            and observation_time(
                record.get("checked_at") or record.get("last_seen") or "0001-01-01"
            )
            >= cutoff
        ):
            continue
        if key in recovered:
            records[key] = {
                "url": f"history:{key}",
                "kind": "external",
                "status": "removed",
                "detail": "Confirmed recovery restored from issue",
                "sources": [],
                "first_failed": None,
                "samples": [],
                "actionable": False,
                **copy.deepcopy(recovered[key]),
            }
        elif record:
            # Legacy closures and pruned targets have no detailed checkpoint.
            # Discard obsolete outage age without inventing newer evidence.
            record.update(
                actionable=False,
                first_failed=None,
                samples=[],
                recovered_at=stamp,
                checked_at=stamp,
            )
    state["closed_checkpoint"] = checkpoint
    if not state.get("checked_at") or observation_time(state["checked_at"]) < cutoff:
        state["checked_at"] = stamp
    return state


def reconcile_from_issue(previous, issue):
    """Merge published facts even when a restored registry predates the issue."""
    if not issue:
        return previous
    if AUTO_CLOSED in (issue.get("body") or ""):
        return reconcile_closed(previous, issue)
    # Manual closure suppresses publication, not the observed failure facts.
    # Restore those too so an older healthy registry cannot invent a recurrence.
    state = copy.deepcopy(previous) if previous is not None else restore_reported({})
    records = state["records"]
    for key, old in reported_records(issue.get("body") or "").items():
        publication_id = digest(old)
        record = records.setdefault(key, {**old, "samples": [], "actionable": False})
        if record.get("publication_id") == publication_id:
            continue
        stamp = old.get("observed_at") or issue.get("updated_at")
        samples = record.get("samples", [])
        recovered = False
        if stamp:
            cutoff = observation_time(stamp)
            samples = [
                sample
                for sample in samples
                if observation_time(sample.get("checked_at", sample["date"])) >= cutoff
            ]
            observed_day = old.get("observed_day") or cutoff.date().isoformat()
            if not any(sample["date"] == observed_day for sample in samples):
                samples.append(
                    {
                        "date": observed_day,
                        "status": old["status"],
                        "checked_at": stamp,
                    }
                )
            samples.sort(key=lambda sample: sample["date"])
            recovered = bool(
                record.get("recovered_at")
                and observation_time(record["recovered_at"]) >= cutoff
            )
            recovered = recovered or any(
                first["status"] in ("ok", "removed", "suppressed")
                and second["status"] in ("ok", "removed", "suppressed")
                for first, second in zip(samples, samples[1:], strict=False)
            )
            if (
                not record.get("checked_at")
                or observation_time(record["checked_at"]) < cutoff
            ):
                record.update(
                    {
                        field: old[field]
                        for field in (
                            "url",
                            "kind",
                            "status",
                            "detail",
                            "sources",
                            "first_failed",
                        )
                        if field in old
                    }
                )
                record["checked_at"] = stamp
                record["last_seen"] = observed_day
        elif not record["actionable"]:
            # Older snapshots have no observation timestamp. Latch once and
            # require fresh recovery evidence instead of trusting an old cache.
            samples = []
        record["actionable"] = record["actionable"] or not recovered
        record["samples"] = samples
        record["publication_id"] = publication_id
        if old.get("observed_at") and (
            not state.get("checked_at")
            or observation_time(state["checked_at"])
            < observation_time(old["observed_at"])
        ):
            state["checked_at"] = old["observed_at"]
    return state


def find_issue(api, repository, number=None):
    if number is not None:
        if not isinstance(number, int) or number <= 0:
            raise ValueError("Invalid tracked issue number")
        status, issue = api.get(f"/repos/{repository}/issues/{number}")
        if (
            status != 200
            or issue.get("number") != number
            or issue.get("repository_url")
            != f"https://api.github.com/repos/{repository}"
            or issue.get("user", {}).get("login") != "github-actions[bot]"
            or MARKER not in (issue.get("body") or "")
        ):
            raise ValueError(
                "Tracked bot issue is unavailable or its marker was removed; refusing to create a replacement"
            )
        return issue
    for page in range(1, 101):
        status, issues = api.get(
            f"/repos/{repository}/issues?state=all&creator=github-actions%5Bbot%5D&per_page=100&page={page}"
        )
        if status != 200:
            raise RuntimeError(
                f"Could not discover the existing link issue: HTTP {status}"
            )
        for issue in issues:
            if (
                "pull_request" not in issue
                and issue.get("user", {}).get("login") == "github-actions[bot]"
                and issue.get("title") == "Documentation link failures"
                and MARKER not in (issue.get("body") or "")
            ):
                raise ValueError(
                    "Tracked bot issue marker was removed; refusing to create a replacement"
                )
            if (
                "pull_request" not in issue
                and issue.get("user", {}).get("type") == "Bot"
                and MARKER in (issue.get("body") or "")
            ):
                return issue
        if len(issues) < 100:
            return None
    raise RuntimeError("Too many bot issues to safely find the link issue")


def safe_text(value):
    return (
        html.escape(str(value))
        .replace("`", "&#96;")
        .replace("|", "&#124;")
        .replace("\n", " ")
        .replace("@", "@\u200b")
        .replace("#", "#\u200b")
    )


def link_label(value):
    return safe_text(value).replace("[", "\\[").replace("]", "\\]")


def finding_row(record, repository, sha):
    url = record["url"]
    destination = (
        f"https://github.com/{repository}/blob/{sha}/" + quote(url[6:], safe="/#")
        if url.startswith("repo:/")
        else quote(url, safe=":/?#%[]@!$&*,;=+-_.~")
    )
    target = (
        f"[{link_label(url.removeprefix('repo:/'))}]({destination})"
        if url.startswith(("repo:/", "http://", "https://"))
        else link_label(
            "Previously reported target " + url.removeprefix("history:")[:12]
        )
    )
    sources = []
    for source in canonical_sources(record["sources"])[:3]:
        location = f"https://github.com/{repository}/blob/{sha}/" + quote(
            source["path"], safe="/"
        )
        label = source["path"]
        if source.get("line"):
            label += ":" + str(source["line"])
            location += "#L" + str(source["line"])
        sources.append(f"[{link_label(label)}]({location})")
    if len(record["sources"]) > 3:
        sources.append(f"+{len(record['sources']) - 3} more in the audit")
    outcome = (
        "recovering (1/2)"
        if record["status"] in ("ok", "removed", "suppressed")
        else record["status"]
    )
    since = " since " + record["first_failed"] if record.get("first_failed") else ""
    return f"| {target} | {safe_text(outcome + since)}: {safe_text(record['detail'])} | {'<br>'.join(sources)} |"


def plan_issue(issue, state, repository, sha, run_url, *, audit_url=None):
    active = {
        key: record for key, record in state["records"].items() if record["actionable"]
    }
    old_body = (issue.get("body") or "") if issue else ""
    keys = sorted(active)
    fingerprint = digest(keys)
    if (
        issue
        and issue["state"] == "closed"
        and AUTO_CLOSED not in old_body
        and set(keys).issubset(
            {
                key
                for key in reported_keys(old_body)
                if not (
                    state["records"].get(key, {}).get("recovered_at")
                    and state["records"][key].get("outage_started_at")
                    and (issue.get("closed_at") or issue.get("updated_at"))
                    and observation_time(state["records"][key]["outage_started_at"])
                    > observation_time(state["records"][key]["recovered_at"])
                    and observation_time(state["records"][key]["outage_started_at"])
                    > observation_time(issue.get("closed_at") or issue["updated_at"])
                )
            }
        )
    ):
        return {"action": "none", "reason": "Maintainer already closed these findings"}
    if not active:
        if not issue or issue["state"] == "closed":
            return {"action": "none", "reason": "No actionable findings"}
        recovered_keys = set(reported_records(old_body)) | {
            key
            for key, record in state["records"].items()
            if record.get("publication_id")
        }
        snapshot = encode_snapshot(
            {
                "version": 1,
                "closed_at": state.get("checked_at") or state["last_run"],
                "closed_day": state["last_run"],
                "records": {
                    key: state["records"][key]
                    for key in recovered_keys
                    if key in state["records"]
                },
            }
        )
        section = (
            START
            + "\n"
            + MARKER
            + "\n"
            + AUTO_CLOSED
            + f"\n<!-- link-check:keys:{snapshot} -->"
            + "\nAll reported targets recovered or were removed in two completed audits.\n"
            + END
        )
        return {"action": "close", "body": replace_section(old_body, section)}
    # Changes in run URL, commit SHA, or observation count must not produce a
    # nightly issue edit. Only changes in the actionable findings do that.
    view = {
        key: {
            field: record.get(field)
            for field in ("url", "kind", "status", "detail", "sources", "first_failed")
        }
        for key, record in active.items()
    }
    content_hash = digest({"format": 3, "findings": view})
    if (
        issue
        and issue["state"] == "open"
        and f"<!-- link-check:content:{content_hash} -->" in old_body
    ):
        return {"action": "none", "reason": "Actionable findings are unchanged"}
    published = {
        key: {
            **item,
            "observed_at": active[key].get("checked_at")
            or active[key].get("last_seen")
            or state.get("last_run"),
            "observed_day": active[key].get("last_seen") or state.get("last_run"),
        }
        for key, item in view.items()
    }
    snapshot = encode_snapshot({"version": 1, "records": published})
    links = []
    if audit_url:
        links.append(f"[Download full audit (ZIP)]({audit_url})")
    if run_url:
        links.append(f"[Workflow run]({run_url})")
    if links:
        audit_links = " · ".join(links)
        if not audit_url:
            audit_links += " — download `link-check-report` from its Artifacts section"
        audit_links += (
            ". Artifact downloads require GitHub sign-in and are retained for 30 days. "
            f"If expired, use the [latest audits](https://github.com/{repository}/actions/workflows/check-links.yml)."
        )
    else:
        audit_links = "Workflow run and artifact links are populated when this check runs in GitHub Actions."
    lines = [
        START,
        MARKER,
        f"<!-- link-check:targets:{fingerprint} -->",
        f"<!-- link-check:content:{content_hash} -->",
        f"<!-- link-check:keys:{snapshot} -->",
        "",
        "### Documentation links need attention",
        "",
        f"**{len(active)} targets · {sum(len(record['sources']) for record in active.values())} references · {len({source['path'] for record in active.values() for source in record['sources']})} source files**",
        "",
        audit_links,
        "",
        f"Findings below refer to revision `{safe_text(sha[:12])}`.",
        "",
        "Local and verified GitHub targets are reported immediately. Other missing or unreachable websites require at least seven days and five observations. Access restrictions are inconclusive.",
        "",
        "**To resolve:** update the link or restore its target. For generated documentation, fix the generator and regenerate. For unreachable websites, investigate availability before replacing the URL.",
    ]
    budget = 58_000 - len("\n".join(lines))
    for kind, title in (
        ("local", "Local files and headings"),
        ("github", "Verified GitHub targets"),
        ("external", "Persistent website failures"),
    ):
        group = sorted(
            (record for record in active.values() if record["kind"] == kind),
            key=lambda record: record["url"],
        )
        if not group:
            continue
        lines += [
            "",
            "<details>",
            f"<summary>{title} ({len(group)})</summary>",
            "",
            "| Target | Evidence | References to update |",
            "|---|---|---|",
        ]
        for record in group:
            row = finding_row(record, repository, sha)
            if len(row) > budget:
                lines += [
                    "",
                    "Additional targets are included in the full audit artifact.",
                ]
                break
            lines.append(row)
            budget -= len(row) + 1
        lines += ["", "</details>"]
    lines += [
        "",
        "**Recovery:** this issue closes after every reported target is healthy, removed, or covered by an active exception in two completed audits on different UTC days. Inconclusive checks cannot confirm recovery.",
        "",
        "Exceptions require a reason and expiry in `.github/link-check-exceptions.json`.",
        END,
    ]
    section = "\n".join(lines)
    body = replace_section(old_body, section) if issue else section
    if len(body) > 65_000:
        raise ValueError(
            "Issue body exceeds GitHub's limit; audit artifacts were retained"
        )
    return {
        "action": "create"
        if not issue
        else "reopen"
        if issue["state"] == "closed"
        else "update",
        "body": body,
    }


def replace_section(body, section):
    if START not in body or END not in body:
        raise ValueError(
            "Existing link issue has no managed section; refusing to overwrite it"
        )
    before, rest = body.split(START, 1)
    _, after = rest.split(END, 1)
    result = before + section + after
    if len(result) > 65_000:
        raise ValueError(
            "Issue body exceeds GitHub's limit; audit artifacts were retained"
        )
    return result


def publish_plan(api, repository, issue, plan):
    action = plan["action"]
    if action == "none":
        return issue
    if action == "create":
        return api.write(
            "POST",
            f"/repos/{repository}/issues",
            {"title": "Documentation link failures", "body": plan["body"]},
        )
    else:
        payload = {"body": plan["body"]}
        if action in ("close", "reopen"):
            payload["state"] = "closed" if action == "close" else "open"
        return api.write(
            "PATCH", f"/repos/{repository}/issues/{issue['number']}", payload
        )
