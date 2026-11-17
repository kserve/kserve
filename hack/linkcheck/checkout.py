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

"""Resolve same-repository GitHub targets without losing their URL semantics."""

from .github import README_NAMES, github_target, inspect_fragment, split_ref


def checkout_target(url, root, checkout_branch, *, default_branch=None):
    parsed = github_target(url)
    if parsed is None:
        return None
    owner, repo, parts, fragment = parsed
    if f"{owner}/{repo}".lower() not in ("kserve/kserve", "kubeflow/kfserving"):
        return None
    namespace, parts = split_ref(parts)
    if namespace == "tags":
        return None
    if (
        default_branch
        and checkout_branch != default_branch
        and (not parts or parts[0] == "HEAD" and namespace is None)
    ):
        return None
    if not parts:
        relative = ""
    else:
        count = (
            1
            if parts[0] == "HEAD" and namespace is None
            else len(checkout_branch.split("/"))
        )
        if parts[0] != "HEAD" or namespace is not None:
            if parts[:count] != checkout_branch.split("/"):
                return None
        relative = "/".join(parts[count:])
    target = (root / relative).resolve()
    if not target.is_relative_to(root.resolve()):
        return None
    return target, fragment


def readme_target(directory):
    candidates = [
        path
        for path in directory.iterdir()
        if path.is_file() and path.name.lower() in README_NAMES
    ]
    return (
        min(
            candidates,
            key=lambda path: (README_NAMES.index(path.name.lower()), path.name),
        )
        if candidates
        else None
    )


def inspect_checkout(target, fragment, fragment_check):
    if not target.exists():
        return {
            "status": "missing",
            "detail": "File not found. Check if file exists and path is correct",
        }
    if not fragment:
        return {"status": "ok", "detail": "Checkout target exists"}
    if target.is_dir():
        readme = readme_target(target)
        if readme is None:
            return {
                "status": "unknown",
                "detail": "Directory heading has no supported README to verify",
            }
        if fragment == "readme":
            return {"status": "ok", "detail": "Repository README exists"}
        target = readme
    try:
        content = target.read_text()
    except (OSError, UnicodeDecodeError):
        return {
            "status": "unknown",
            "detail": "Checkout contents cannot be used to verify this anchor",
        }
    return inspect_fragment(content, target.suffix.lower(), fragment, fragment_check)
