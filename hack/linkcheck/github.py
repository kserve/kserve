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

"""Verify public repository files, revisions and anchors across organizations."""

import base64
import json
import re
from pathlib import PurePosixPath
from urllib.parse import quote, unquote, urlsplit


def github_target(url):
    parsed = urlsplit(url)
    if parsed.scheme not in ("http", "https") or parsed.username or parsed.password:
        return None
    if parsed.port not in (None, 443 if parsed.scheme == "https" else 80):
        return None
    parts = unquote(parsed.path).strip("/").split("/")
    if len(parts) < 2:
        return None
    owner, repo = parts[:2]
    if not re.fullmatch(r"[\w.-]+", owner) or not re.fullmatch(r"[\w.-]+", repo):
        return None
    if parsed.hostname == "raw.githubusercontent.com" and len(parts) >= 4:
        return owner, repo, parts[2:], unquote(parsed.fragment)
    if parsed.hostname == "github.com":
        if len(parts) == 2:
            return owner, repo.removesuffix(".git"), [], unquote(parsed.fragment)
        if len(parts) >= 4 and parts[2] in ("blob", "tree", "raw"):
            remaining = parts[3:]
            return owner, repo, remaining, unquote(parsed.fragment)
    return None


def service_status(code):
    if code == -1:
        return "unknown"
    return "restricted" if code in (401, 403, 404, 429) else "unreachable"


README_NAMES = (
    "readme.md",
    "readme.mdx",
    "readme.markdown",
    "readme.html",
    "readme.ipynb",
)


def split_ref(parts):
    if parts[:2] in (["refs", "heads"], ["refs", "tags"]):
        return parts[1], parts[2:]
    return None, parts


class GitHubVerifier:
    def __init__(self, api, fragment_check):
        self.api = api
        self.fragment_check = fragment_check

    def check(self, url):
        result = self._check(url)
        if result["status"] == "missing":
            # Confirm the target independently; do not promote a first 404
            # followed by an outage or denied request to a missing target.
            result = self._check(url)
        return {"url": url, "kind": "github", **result}

    def _check(self, url):
        owner, repo, parts, fragment = github_target(url)
        endpoint = f"/repos/{quote(owner, safe='')}/{quote(repo, safe='')}"
        status, metadata = self.api.get(endpoint)
        if metadata.get("private"):
            return {
                "status": "restricted",
                "detail": "Private repository requires manual verification",
            }
        if status != 200:
            return {
                "status": service_status(status),
                "detail": "Repository visibility or GitHub availability could not be verified",
            }
        if not parts and not fragment:
            return {"status": "ok", "detail": "Repository exists"}
        if parts:
            if parts[0] == "HEAD":
                parts = metadata["default_branch"].split("/") + parts[1:]
            parsed = urlsplit(url)
            raw_url = (
                parsed.hostname == "github.com"
                and parsed.path.strip("/").split("/")[2] == "raw"
            )
            ref_status, ref, path = self.reference(
                endpoint, parts, follow_renames=not raw_url
            )
            if ref_status != "ok":
                return {
                    "status": ref_status,
                    "detail": "GitHub revision is missing"
                    if ref_status == "missing"
                    else "GitHub revision could not be verified",
                }
        else:
            ref, path = metadata["default_branch"], ""
        content_url = (
            endpoint
            + "/contents/"
            + quote(path, safe="/")
            + "?ref="
            + quote(ref, safe="")
        )
        status, document = self.api.get(content_url)
        if status == 404:
            return {
                "status": "missing",
                "detail": "File or directory is missing in an accessible public repository and revision",
            }
        if status != 200:
            return {
                "status": service_status(status),
                "detail": f"GitHub contents unavailable (HTTP {status})",
            }
        if not fragment:
            return {"status": "ok", "detail": "Target exists"}
        if isinstance(document, list):
            candidates = [
                item for item in document if item["name"].lower() in README_NAMES
            ]
            if not candidates:
                return {
                    "status": "unknown",
                    "detail": "Directory heading has no supported README to verify",
                }
            if fragment == "readme":
                return {"status": "ok", "detail": "Repository README exists"}
            readme_path = candidates[0]["path"]
            status, document = self.api.get(
                endpoint
                + "/contents/"
                + quote(readme_path, safe="/")
                + "?ref="
                + quote(ref, safe="")
            )
            if status != 200:
                return {
                    "status": service_status(status),
                    "detail": "README contents could not be retrieved",
                }
        if document.get("encoding") != "base64":
            return {
                "status": "unknown",
                "detail": "Target exists, but its contents cannot be used to verify this anchor",
            }
        try:
            content = base64.b64decode(document["content"]).decode("utf-8")
        except (ValueError, UnicodeDecodeError):
            return {
                "status": "unknown",
                "detail": "Binary document anchor requires manual verification",
            }
        return inspect_fragment(
            content,
            PurePosixPath(document.get("name", path)).suffix.lower(),
            fragment,
            self.fragment_check,
        )

    def reference(self, endpoint, parts, follow_renames=True):
        namespace, parts = split_ref(parts)
        if not parts:
            return "missing", None, None
        refs = []
        for candidate_namespace in (namespace,) if namespace else ("heads", "tags"):
            status, matches = self.api.get(
                endpoint
                + "/git/matching-refs/"
                + candidate_namespace
                + "/"
                + quote(parts[0], safe="")
            )
            if status != 200:
                return service_status(status), None, None
            for item in matches:
                name = item["ref"].split("/", 2)[2]
                segments = name.split("/")
                if parts[: len(segments)] == segments:
                    refs.append((len(segments), name))
        if refs:
            count, name = max(refs, key=lambda item: item[0])
            qualified = f"refs/{namespace}/{name}" if namespace else name
            return "ok", qualified, "/".join(parts[count:])
        # Commit links can use either a full SHA or an unambiguous short SHA.
        if namespace is None and re.fullmatch(r"[0-9a-fA-F]{7,40}", parts[0]):
            status, _ = self.api.get(endpoint + "/commits/" + parts[0])
            if status == 200:
                return "ok", parts[0], "/".join(parts[1:])
            if status != 404:
                return service_status(status), None, None
        if follow_renames and namespace is None:
            # The branches endpoint follows GitHub's rename redirects, unlike
            # matching-refs. Try longest names first to preserve slash-containing
            # branch names. Explicit refs and github.com/raw do not follow renames.
            for count in range(len(parts), 0, -1):
                candidate = quote("/".join(parts[:count]), safe="")
                status, branch = self.api.get(endpoint + "/branches/" + candidate)
                if status == 200:
                    qualified = (
                        f"refs/heads/{branch['name']}" if namespace else branch["name"]
                    )
                    return "ok", qualified, "/".join(parts[count:])
                if status != 404:
                    return service_status(status), None, None
        return "missing", None, None


def notebook_cells(content):
    """Return independently rendered Markdown cells; skip code and outputs."""
    return [
        "".join(cell["source"])
        for cell in json.loads(content)["cells"]
        if cell["cell_type"] == "markdown"
    ]


def inspect_fragment(content, suffix, fragment, fragment_check):
    """Apply identical GitHub fragment rules to fetched and checkout contents."""
    if match := re.fullmatch(r"L(\d+)(?:C\d+)?(?:-L(\d+)(?:C\d+)?)?", fragment):
        end = int(match[2] or match[1])
        valid = 1 <= int(match[1]) <= end <= len(content.splitlines())
        return {
            "status": "ok" if valid else "missing",
            "detail": "Source line range verified"
            if valid
            else "Source line range is missing",
        }
    if suffix == ".ipynb":
        results = [
            inspect_fragment(cell, ".md", fragment, fragment_check)
            for cell in notebook_cells(content)
        ]
        for status in ("ok", "unknown"):
            for result in results:
                if result["status"] == status:
                    return result
        return {"status": "missing", "detail": "Notebook heading is missing"}
    if suffix not in (".md", ".mdx", ".markdown", ".html", ".htm"):
        return {
            "status": "unknown",
            "detail": "Target exists; this document format does not support heading verification",
        }
    result = fragment_check(content, suffix, fragment.removeprefix("user-content-"))
    if result == "missing" and suffix == ".mdx":
        return {
            "status": "unknown",
            "detail": "Heading is absent from static MDX; components may generate it dynamically",
        }
    return {
        "status": result,
        "detail": "Document heading verified"
        if result == "ok"
        else "Document heading is missing",
    }
