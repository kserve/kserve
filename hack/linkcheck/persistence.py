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


def artifacts(api, repository, run_id, name):
    status, response = api.get(
        f"/repos/{repository}/actions/runs/{run_id}/artifacts?name={name}&per_page=100"
    )
    if status != 200:
        raise RuntimeError(
            f"Could not inspect trusted history artifacts: HTTP {status}"
        )
    return [artifact for artifact in response["artifacts"] if artifact["name"] == name]


def on_branch(api, repository, sha, branch):
    """Tag runs report the tag as head_branch, so check the commit itself.

    The ref is qualified because git resolves a short name to a tag first.
    """
    status, response = api.get(
        f"/repos/{repository}/compare/{quote(sha, safe='')}..."
        f"{quote('refs/heads/' + branch, safe='/')}?per_page=1"
    )
    if status == 404:
        return False  # Unknown commit, for example one rewritten out of the branch.
    if status != 200:
        raise RuntimeError(f"Could not verify the audited commit: HTTP {status}")
    return response["status"] in ("ahead", "identical")


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
                if (
                    all(
                        artifact["expired"]
                        for artifact in artifacts(
                            api, repository, run_id, "link-triage-report"
                        )
                    )
                    # A dry run uploads its registry as a preview, so a report
                    # beside one was uploaded by another job of that run.
                    or artifacts(api, repository, run_id, "link-triage-preview")
                    or not on_branch(api, repository, run["head_sha"], branch)
                ):
                    continue
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
