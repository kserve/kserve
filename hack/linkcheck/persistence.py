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

"""Locate the latest history from successful default-branch audit runs."""

import os
from urllib.parse import quote

from .api import GitHubAPI


def previous_run(api, repository, branch, current_run):
    latest = []
    for event in ("schedule", "workflow_dispatch"):
        found = False
        for page in range(1, 11):
            status, response = api.get(
                f"/repos/{repository}/actions/workflows/check-links.yml/runs?branch={quote(branch, safe='')}&event={event}&status=success&per_page=100&page={page}"
            )
            if status == 404:
                return None  # A newly introduced workflow has no history yet.
            if status != 200:
                raise RuntimeError(
                    f"Could not locate previous audit runs: HTTP {status}"
                )
            runs = response["workflow_runs"]
            for run in sorted(runs, key=lambda item: item["id"], reverse=True):
                run_id = run["id"]
                if (
                    str(run_id) == str(current_run)
                    or run["event"] != event
                    or run["status"] != "completed"
                    or run["conclusion"] != "success"
                    or run["head_branch"] != branch
                    or run.get("head_repository", {}).get("full_name") != repository
                    or run["path"].split("@", 1)[0]
                    != ".github/workflows/check-links.yml"
                ):
                    continue
                status, response = api.get(
                    f"/repos/{repository}/actions/runs/{run_id}/artifacts?name=link-triage-report&per_page=100"
                )
                if status != 200:
                    raise RuntimeError(
                        f"Could not inspect trusted history artifacts: HTTP {status}"
                    )
                if any(
                    artifact["name"] == "link-triage-report" and not artifact["expired"]
                    for artifact in response["artifacts"]
                ):
                    latest.append(run_id)
                    found = True
                    break
            if found or len(runs) < 100:
                break
        else:
            raise RuntimeError("Too many trusted audits to locate previous history")
    return max(latest) if latest else None


def main():
    run_id = previous_run(
        GitHubAPI(),
        os.environ["GITHUB_REPOSITORY"],
        os.environ["DEFAULT_BRANCH"],
        os.environ["GITHUB_RUN_ID"],
    )
    if run_id:
        with open(os.environ["GITHUB_OUTPUT"], "a") as output:
            output.write(f"run_id={int(run_id)}\n")
        print(f"Recovering observation history from trusted run {int(run_id)}")
    else:
        print(
            "No previous history artifact; published issue targets will still be recovered"
        )


if __name__ == "__main__":
    main()
