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

"""Maintain daily link history and preview or publish the aggregate issue."""

import argparse
import json
import os
from datetime import datetime, timezone
from pathlib import Path

from linkcheck.api import GitHubAPI
from linkcheck.history import advance
from linkcheck.validation import read_json, validate_report
from linkcheck.notifications import (
    find_issue,
    plan_issue,
    publish_plan,
    reconcile_from_issue,
)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--report", type=Path, default=Path("bin/link-check/observations.json")
    )
    parser.add_argument("--state", type=Path, default=Path("bin/link-check/state.json"))
    parser.add_argument(
        "--repository", default=os.environ.get("GITHUB_REPOSITORY", "kserve/kserve")
    )
    parser.add_argument("--sha", default=os.environ.get("GITHUB_SHA", "master"))
    parser.add_argument(
        "--run-url",
        help="Workflow run containing the audit artifacts (populated by CI)",
    )
    parser.add_argument("--audit-url", help="Direct download URL from upload-artifact")
    parser.add_argument(
        "--publish",
        action="store_true",
        help="Publish from the trusted default-branch workflow",
    )
    args = parser.parse_args()
    report = read_json(args.report)
    validate_report(report)
    previous = read_json(args.state) if args.state.exists() else None
    now = datetime.now(timezone.utc)
    api, issue = GitHubAPI(), None
    if args.publish:
        if (
            os.environ.get("GITHUB_REPOSITORY") != "kserve/kserve"
            or args.repository != "kserve/kserve"
            or os.environ.get("GITHUB_EVENT_NAME")
            not in ("schedule", "workflow_dispatch")
            or os.environ.get("GITHUB_REF")
            != "refs/heads/" + os.environ.get("DEFAULT_BRANCH", "")
            or report.get("run_id") != os.environ.get("GITHUB_RUN_ID")
        ):
            raise ValueError(
                "Issue publishing requires this run's trusted default-branch audit"
            )
        age = now - datetime.fromisoformat(report["generated_at"])
        if not 0 <= age.total_seconds() <= 86400:
            raise ValueError("Refusing to publish a stale or future-dated audit")
        issue = find_issue(
            api, args.repository, previous.get("issue_number") if previous else None
        )
        previous = reconcile_from_issue(previous, issue)
    state = advance(previous, report, now.date())
    plan = plan_issue(
        issue, state, args.repository, args.sha, args.run_url, audit_url=args.audit_url
    )
    args.state.parent.mkdir(parents=True, exist_ok=True)
    (args.state.parent / "issue-plan.json").write_text(
        json.dumps(plan, indent=2) + "\n"
    )
    if "body" in plan:
        (args.state.parent / "issue-preview.md").write_text(plan["body"] + "\n")
    if args.publish:
        updated_issue = publish_plan(api, args.repository, issue, plan)
        if updated_issue:
            state["issue_number"] = updated_issue["number"]
    # Save state after issue reconciliation. If a write has an ambiguous
    # outcome, the next run discovers the existing marked issue before retrying.
    temporary = args.state.with_suffix(".tmp")
    temporary.write_text(json.dumps(state, indent=2) + "\n")
    temporary.replace(args.state)
    print(("Published" if args.publish else "Dry run") + ": " + plan["action"])
    if summary_path := os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(summary_path, "a") as stream:
            stream.write(
                "\n## Link triage\n\n"
                + ("Published" if args.publish else "Dry run")
                + ": "
                + plan["action"]
                + ". See the triage artifact for the issue preview and history.\n"
            )


if __name__ == "__main__":
    main()
