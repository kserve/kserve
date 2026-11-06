#!/usr/bin/env python3

# Copyright 2022 The KServe Contributors
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

"""Check link regressions with Lychee, or report existing repository link debt."""

import argparse
import json
import os
import signal
import subprocess
import tempfile
from contextlib import ExitStack, contextmanager
from datetime import datetime, timedelta, timezone
from pathlib import Path
from urllib.parse import quote, unquote, urlsplit

from linkcheck.api import GitHubAPI
from linkcheck.checkout import checkout_target, inspect_checkout
from linkcheck.github import (
    GitHubVerifier,
    github_target,
    notebook_cells,
    service_status,
)
from linkcheck.history import by_target
from linkcheck.validation import CONTROL, DETAIL_LIMIT


ROOT = Path(__file__).resolve().parents[1]
DOCUMENTS = ("*.md", "*.mdx", "*.html", "*.htm", "*.ipynb")


def failures(report):
    """Lychee records timeouts separately from other failures."""
    for name in ("error_map", "timeout_map"):
        for source, entries in report.get(name, {}).items():
            for entry in entries:
                yield source, entry


def local_failures(report, root):
    result = {}
    for source, entry in failures(report):
        url = urlsplit(entry["url"])
        if url.scheme != "file":
            continue
        target = os.path.relpath(unquote(url.path), root)
        if url.fragment:
            target += "#" + url.fragment
        # Line numbers can shift without introducing a new broken link.
        source_path = Path(source)
        if source_path.is_absolute():
            source_path = source_path.relative_to(root)
        result[(str(source_path), target)] = entry["status"]["text"]
    return result


def classify_external(first, confirmation):
    original = {entry["url"]: entry for _, entry in failures(first)}
    confirmed, uncertain = {}, {}
    for _, entry in failures(confirmation):
        url = entry["url"]
        if url not in original:
            continue
        if all(
            item["status"].get("code") in (404, 410) for item in (entry, original[url])
        ):
            confirmed[url] = entry["status"]["text"]
        else:
            uncertain[url] = entry["status"]["text"]
    return confirmed, uncertain


def inert(detail):
    """Fit free text to the detail rules of validate_observation."""
    detail = CONTROL.sub(" ", detail)
    return detail if len(detail) <= DETAIL_LIMIT else detail[: DETAIL_LIMIT - 1] + "…"


def document_paths(root):
    output = subprocess.run(
        ["git", "ls-files", "-z", "--cached", "--", *DOCUMENTS],
        cwd=root,
        check=True,
        capture_output=True,
    ).stdout
    return ["./" + os.fsdecode(p) for p in output.split(b"\0") if p]


def canonical_url(url):
    # General website anchors are not verified, so their HTTP target and
    # observation history are shared. GitHub document anchors are independent.
    return url if github_target(url) else urlsplit(url)._replace(fragment="").geturl()


@contextmanager
def documentation_inputs(root, enabled=True):
    """Give notebook prose a Markdown suffix and preserve relative link bases."""
    aliases = {}
    paths = []
    with ExitStack() as stack:
        for path in document_paths(root) if enabled else []:
            if not path.endswith(".ipynb"):
                paths.append(path)
                continue
            original = root / path
            # Each cell is a separate Lychee input, as in notebook rendering.
            # Temporary siblings preserve relative bases and are always removed.
            for cell in notebook_cells(original.read_text()):
                temporary = stack.enter_context(
                    tempfile.NamedTemporaryFile(
                        mode="w", suffix=".md", prefix=".lychee-", dir=original.parent
                    )
                )
                temporary.write(cell)
                temporary.flush()
                generated = Path(temporary.name)
                relative = "./" + str(generated.relative_to(root))
                paths.append(relative)
                aliases[relative] = path
                aliases[str(generated)] = str(original)
                aliases[generated.as_uri()] = original.as_uri()
        yield paths, aliases


class Checker:
    def __init__(self, binary, output, default_branch=None):
        self.binary = str(Path(binary).resolve()) if "/" in binary else binary
        self.output = output
        self.default_branch = default_branch or os.environ.get(
            "CHECKOUT_BRANCH", os.environ.get("DEFAULT_BRANCH", "master")
        )
        self.default_branch = self.default_branch.removeprefix("refs/heads/")
        self.repository_default_branch = os.environ.get(
            "DEFAULT_BRANCH", self.default_branch
        )

    def checkout(self, url, root):
        return checkout_target(
            url,
            root,
            self.default_branch,
            default_branch=self.repository_default_branch,
        )

    def local(self, root, *, name, inventory=None):
        raw = self.run(
            root,
            ["--offline", "--include-fragments", "--format", "json"],
            name=name + "-lychee",
        )
        report = {"error_map": {}, "timeout_map": {}, "inconclusive_map": {}}
        for bucket in ("error_map", "timeout_map", "success_map"):
            for source, entries in raw.get(bucket, {}).items():
                for entry in entries:
                    parsed = urlsplit(entry["url"])
                    if (
                        parsed.scheme != "file"
                        or bucket == "success_map"
                        and not parsed.fragment
                    ):
                        continue
                    target = Path(unquote(parsed.path))
                    destination = (
                        "inconclusive_map" if bucket == "timeout_map" else "error_map"
                    )
                    if parsed.fragment and target.exists():
                        result = inspect_checkout(
                            target, unquote(parsed.fragment), self.fragment
                        )
                        if result["status"] == "ok":
                            continue
                        destination = (
                            "inconclusive_map"
                            if result["status"] == "unknown"
                            else "error_map"
                        )
                        entry = {
                            **entry,
                            "status": {"text": result["detail"]},
                        }
                    report[destination].setdefault(source, []).append(entry)
        inventory = self.inventory(root) if inventory is None else inventory
        for url, sources in inventory.items():
            local = self.checkout(url, root)
            if local is None:
                continue
            target, fragment = local
            result = inspect_checkout(target, fragment, self.fragment)
            if result["status"] == "ok":
                continue
            entry = {
                "url": target.as_uri()
                + ("#" + quote(fragment, safe="") if fragment else ""),
                "status": {"text": result["detail"]},
                "original_url": url,
            }
            destination = (
                "error_map" if result["status"] == "missing" else "inconclusive_map"
            )
            for source in sources:
                finding = dict(entry)
                if source.get("line"):
                    finding["span"] = {"line": source["line"]}
                report[destination].setdefault(source["path"], []).append(finding)
        (self.output / f"{name}.json").write_text(json.dumps(report, indent=2) + "\n")
        return report

    def run(self, root, options, *, inputs=None, name=None):
        with documentation_inputs(root, inputs is None) as (paths, aliases):
            result = self._run(root, options, inputs, name, paths, aliases)
        if name:
            (self.output / f"{name}.json").write_text(
                json.dumps(result, indent=2) + "\n"
            )
        return result

    def inventory(self, root):
        """Keep source locations even for URLs whose HTTP requests succeed."""
        result = {}
        print(f"Extracting documentation links in {root.name}", flush=True)
        with documentation_inputs(root) as (paths, aliases):
            for path in paths:
                source = aliases.get(path, path).removeprefix("./")
                text = (root / path).read_text()
                for url in self._run(root, ["--dump"], None, None, [path], aliases):
                    # Remapped or percent-encoded URLs may not appear literally
                    # in the source. A file link remains useful without a line.
                    matches = [
                        i for i, line in enumerate(text.splitlines(), 1) if url in line
                    ]
                    location = {"path": source}
                    if matches and not source.endswith(".ipynb"):
                        location["line"] = matches[0]
                    locations = result.setdefault(canonical_url(url), [])
                    if location not in locations:
                        locations.append(location)
        return result

    def fragment(self, content, suffix, fragment):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / ("target" + suffix)
            target.write_text(content)
            probe = root / "probe.md"
            probe.write_text(
                "[heading](<"
                + target.as_uri()
                + "#"
                + quote(fragment, safe="")
                + ">)\n"
            )
            result = self._run(
                root,
                ["--offline", "--include-fragments", "--format", "json"],
                None,
                "fragment",
                [str(probe)],
                {},
            )
            return "missing" if any(failures(result)) else "ok"

    def _run(self, root, options, inputs, name, paths, aliases):
        command = [
            self.binary,
            "--config",
            str(ROOT / "lychee.toml"),
            "--root-dir",
            str(root),
            "--no-progress",
            # No cross-run failure cache: every failure is checked afresh.
            "--cache=false",
            *(["--verbose"] if name else []),
            # Other legacy URLs (issues, pull requests, etc.) use the canonical
            # repository name to avoid false API 404s after the repository move.
            "--remap",
            r"^https://github\.com/kubeflow/kfserving/(.*)$ "
            r"https://github.com/kserve/kserve/$1",
            *options,
        ]
        if inputs is None:
            # Files are passed as arguments, so spaces and shell metacharacters
            # in PR filenames never become shell code or separate inputs.
            if not paths:
                return {} if name else set()
            command += ["--", *paths]
        else:
            if not inputs:
                return {} if name else set()
            command += ["--default-extension", "md", "--", "-"]
        if name and name != "fragment":
            print(f"Checking {name} in {root.name}", flush=True)
        completed = subprocess.run(
            command,
            cwd=root,
            input="\n".join("[target](<" + url + ">)" for url in inputs)
            if inputs is not None
            else None,
            text=True,
            capture_output=True,
            env={
                key: value
                for key, value in os.environ.items()
                if key not in ("GITHUB_TOKEN", "GH_TOKEN")
            },
        )
        if completed.returncode not in (0, 2):
            raise RuntimeError(completed.stderr or completed.stdout)
        if not name:
            return {
                line
                for line in completed.stdout.splitlines()
                if line.startswith(("http://", "https://"))
            }
        report = json.loads(completed.stdout)
        for map_name in ("error_map", "timeout_map", "success_map"):
            normalized = {}
            for source, entries in report.get(map_name, {}).items():
                normalized.setdefault(aliases.get(source, source), []).extend(entries)
            report[map_name] = normalized
            for entries in report[map_name].values():
                for entry in entries:
                    url = urlsplit(entry["url"])
                    target = url._replace(fragment="").geturl()
                    if target in aliases:
                        entry["url"] = aliases[target]
                        if url.fragment:
                            entry["url"] += "#" + url.fragment
        return report


def exceptions(path, today):
    data = json.loads(path.read_text())
    if (
        not isinstance(data, dict)
        or data.get("version") != 1
        or not isinstance(data.get("exceptions"), list)
    ):
        raise ValueError("Unsupported link exception schema")
    active, expired = {}, []
    seen = set()
    for entry in data["exceptions"]:
        if (
            not isinstance(entry, dict)
            or not isinstance(entry.get("url"), str)
            or not entry["url"]
            or not isinstance(entry.get("reason"), str)
            or not entry["reason"].strip()
            or not isinstance(entry.get("expires"), str)
        ):
            raise ValueError("Link exceptions require an exact URL and a reason")
        try:
            expiry = datetime.strptime(entry["expires"], "%Y-%m-%d").date()
        except ValueError as error:
            raise ValueError(
                "Link exceptions require an expiry in YYYY-MM-DD format"
            ) from error
        if expiry > today + timedelta(days=366):
            raise ValueError("Temporary link exceptions cannot exceed one year")
        url = entry["url"]
        key = canonical_url(url) if url.startswith(("http://", "https://")) else url
        if key in seen:
            raise ValueError("Duplicate link exception: " + key)
        seen.add(key)
        if expiry >= today:
            active[key] = entry["reason"]
        else:
            expired.append(entry["url"])
    return active, expired


def external_observations(urls, first, second):
    initial = {entry["url"]: entry for _, entry in failures(first)}
    final = {entry["url"]: entry for _, entry in failures(second)}
    succeeded_first = {
        entry["url"]
        for entries in first.get("success_map", {}).values()
        for entry in entries
    }
    succeeded_second = {
        entry["url"]
        for entries in second.get("success_map", {}).values()
        for entry in entries
    }
    confirmed, _ = classify_external(first, second)
    skipped = {
        entry["url"]
        for report in (first, second)
        for entries in report.get("excluded_map", {}).values()
        for entry in entries
    }
    result = []
    for url in sorted(urls):
        status, detail = "ok", "HTTP target verified"
        if url in skipped:
            status, detail = "unknown", "Target was excluded from HTTP verification"
        elif url in confirmed:
            status, detail = "missing", confirmed[url]
        elif url in initial and url not in succeeded_second:
            entry = final.get(url, initial[url])
            code = entry["status"].get("code")
            status = (
                "restricted"
                if code in (401, 403, 429)
                else "unknown"
                if code in (404, 410)
                else "unreachable"
            )
            detail = entry["status"]["text"]
        elif url not in succeeded_first and url not in succeeded_second:
            status, detail = "unknown", "Target was not present in the HTTP results"
        result.append(
            {"url": url, "kind": "external", "status": status, "detail": detail}
        )
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--lychee", default="lychee")
    parser.add_argument(
        "--base-dir", type=Path, help="Separate checkout of the PR base"
    )
    parser.add_argument("--offline", action="store_true")
    parser.add_argument(
        "--advisory", action="store_true", help="Report debt without failing"
    )
    parser.add_argument("--output", type=Path, default=ROOT / "bin/link-check")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    # A failed invocation must not leave a previous complete audit available
    # to a local triage command as if it were this run's result.
    (output / "observations.json").unlink(missing_ok=True)
    now = datetime.now(timezone.utc)
    ignored, expired = exceptions(
        ROOT / ".github/link-check-exceptions.json", now.date()
    )
    checker = Checker(args.lychee, output)
    inventory = checker.inventory(ROOT)
    local_report = checker.local(ROOT, inventory=inventory, name="local")
    local = local_failures(local_report, ROOT)
    local_unknown = local_failures(
        {"error_map": local_report["inconclusive_map"]}, ROOT
    )
    positions = {}
    for bucket in ("error_map", "inconclusive_map"):
        for source, entries in local_report[bucket].items():
            for entry in entries:
                for key in local_failures({"error_map": {source: [entry]}}, ROOT):
                    if entry.get("span", {}).get("line"):
                        line = entry["span"]["line"]
                        positions[key] = min(line, positions.get(key, line))
    observations = []
    for status, entries in (("missing", local), ("unknown", local_unknown)):
        for (source, target), detail in entries.items():
            url = "repo:/" + target
            location = {"path": source}
            if (source, target) in positions and not source.endswith(".ipynb"):
                location["line"] = positions[(source, target)]
            observations.append(
                {
                    "url": url,
                    "kind": "local",
                    "status": "suppressed" if url in ignored else status,
                    "detail": ignored.get(url, detail),
                    "sources": [location],
                }
            )
    local = {
        key: value for key, value in local.items() if "repo:/" + key[1] not in ignored
    }
    all_local_count = len(local)
    if args.base_dir:
        base = args.base_dir.resolve()
        base_inventory = checker.inventory(base)
        baseline = local_failures(
            checker.local(base, inventory=base_inventory, name="base-local"), base
        )
        local = {key: value for key, value in local.items() if key not in baseline}

    confirmed, github_missing = {}, {}
    uncertain = {
        f"{source} → {target}": detail
        for (source, target), detail in local_unknown.items()
        if "repo:/" + target not in ignored
    }
    destinations = {}
    if not args.offline:
        destinations = {
            url: sources
            for url, sources in inventory.items()
            if checker.checkout(url, ROOT) is None
        }
        if args.base_dir:
            existing = set(base_inventory)
            destinations = {
                url: sources
                for url, sources in destinations.items()
                if url not in existing
            }
        generic, github_urls = set(), []
        for url, sources in sorted(destinations.items()):
            if url in ignored:
                observations.append(
                    {
                        "url": url,
                        "kind": "github" if github_target(url) else "external",
                        "status": "suppressed",
                        "detail": ignored[url],
                        "sources": sources,
                    }
                )
            elif github_target(url):
                github_urls.append(url)
            else:
                generic.add(url)
        first = checker.run(
            ROOT, ["--format", "json"], inputs=sorted(generic), name="external"
        )
        failed_urls = sorted(
            {
                entry["url"]
                for _, entry in failures(first)
                if entry["status"].get("code") in (404, 410)
            }
        )
        second = checker.run(
            ROOT, ["--format", "json"], inputs=failed_urls, name="external-recheck"
        )
        external = external_observations(generic, first, second)
        api = GitHubAPI()
        for item in external:
            parsed = urlsplit(item["url"])
            parts = parsed.path.strip("/").split("/")
            if (
                item["status"] == "missing"
                and parsed.hostname == "github.com"
                and len(parts) >= 2
            ):
                status, metadata = api.get(
                    "/repos/"
                    + quote(parts[0], safe="")
                    + "/"
                    + quote(parts[1], safe="")
                )
                if status != 200 or metadata.get("private"):
                    item.update(
                        status="restricted"
                        if metadata.get("private")
                        else service_status(status),
                        detail="Repository access or GitHub availability could not be verified",
                    )
        verifier = GitHubVerifier(api, checker.fragment)
        for index, url in enumerate(github_urls, 1):
            print(f"Verifying GitHub target {index}/{len(github_urls)}", flush=True)
            external.append(verifier.check(url))
        for item in external:
            item["sources"] = destinations[item["url"]]
            observations.append(item)
            if item["status"] == "missing":
                (github_missing if item["kind"] == "github" else confirmed)[
                    item["url"]
                ] = item["detail"]
            elif item["status"] in ("unknown", "restricted", "unreachable"):
                uncertain[item["url"]] = item["detail"]

    # Lychee errors embed the failing URL and exception reasons are unbounded;
    # one detail that breaks the rules would reject the whole report.
    for item in observations:
        item["detail"] = inert(item["detail"])
    audit = {
        "version": 1,
        "complete": not args.offline,
        "scope": "pr" if args.base_dir else "audit",
        "generated_at": now.isoformat(),
        "run_id": os.environ.get("GITHUB_RUN_ID"),
        "observations": list(by_target(observations).values()),
    }
    (output / "observations.json").write_text(json.dumps(audit, indent=2) + "\n")

    lines = [
        "# Link check",
        "",
        f"Checked {len(document_paths(ROOT))} documentation files and "
        f"{len(destinations)} external destinations.",
        f"Local failures: {len(local)} reported ({all_local_count} total). "
        f"Verified missing GitHub targets: {len(github_missing)}. "
        f"Repeated external 404/410: {len(confirmed)}. "
        f"Inconclusive targets: {len(uncertain)}.",
        "",
    ]
    if args.base_dir:
        lines += [
            "Existing local failures and external URLs already in the target branch "
            "are excluded from PR failures. All local links are scanned so deleting "
            "a target can expose broken links in unchanged files.",
            "",
        ]
    if args.advisory:
        lines += [
            "Advisory audit: findings do not fail the scanner. The nightly triage "
            "job escalates local and verified GitHub failures immediately, and "
            "other missing or unreachable websites after seven days and five observations.",
            "",
        ]
    for title, entries in (
        ("Local link failures", {f"{s} → {u}": v for (s, u), v in local.items()}),
        ("Verified missing GitHub files, revisions or headings", github_missing),
        ("Repeated external 404/410", confirmed),
        (
            "Inconclusive targets",
            uncertain,
        ),
    ):
        lines += [f"## {title}", ""]
        lines += [f"- {json.dumps(k)}: {v}" for k, v in sorted(entries.items())] or [
            "None."
        ]
        lines.append("")
    if expired:
        lines += ["## Expired exceptions", "", *[f"- {url}" for url in expired], ""]
    unmatched = set(ignored) - set(inventory) - {item["url"] for item in observations}
    if unmatched:
        lines += [
            "## Unused exceptions",
            "",
            *[f"- {url}" for url in sorted(unmatched)],
            "",
        ]
    suppressed = [item for item in observations if item["status"] == "suppressed"]
    if suppressed:
        lines += [
            "## Active exceptions",
            "",
            *[f"- {item['url']}: {item['detail']}" for item in suppressed],
            "",
        ]
    summary = "\n".join(lines)
    (output / "summary.md").write_text(summary)
    print(summary)
    if summary_path := os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(summary_path, "a") as stream:
            stream.write(summary + "\n")
    return 0 if args.advisory else int(bool(local or confirmed or github_missing))


def terminate(signum, frame):
    raise SystemExit(128 + signum)


if __name__ == "__main__":
    signal.signal(signal.SIGTERM, terminate)
    raise SystemExit(main())
