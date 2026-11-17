"""Shared inputs for exercising the real scanner and issue policy."""

from contextlib import contextmanager, redirect_stdout
from functools import partial
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from linkcheck.notifications import plan_issue

ROOT = Path(__file__).resolve().parents[2]
LYCHEE = Path(os.environ.get("LYCHEE_TEST_BINARY", ROOT / "bin/lychee"))
plan = partial(plan_issue, repository="kserve/kserve", sha="abc", run_url=None)


def load_script(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / "hack" / f"{name}.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


links = load_script("verify-doc-links")


def write_files(root, files):
    for name, content in files.items():
        path = root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)


def track(root):
    subprocess.run(["git", "add", "."], cwd=root, check=True)


@contextmanager
def checkout(files=None):
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        write_files(
            root,
            {
                "lychee.toml": (ROOT / "lychee.toml").read_text(),
                ".github/link-check-exceptions.json": '{"version":1,"exceptions":[]}',
                **(files or {}),
            },
        )
        subprocess.run(["git", "init", "--quiet", str(root)], check=True)
        track(root)
        yield root


def notebook(*cells):
    return json.dumps(
        {
            "cells": [
                {"cell_type": "markdown", "source": [cell]}
                if isinstance(cell, str)
                else cell
                for cell in cells
            ]
        }
    )


def run_check(head, base=None, api=None, *, http=None, offline=False, advisory=False):
    args = [
        "verify-doc-links.py",
        "--lychee",
        str(LYCHEE),
        "--output",
        str(head / "report"),
    ]
    if base is not None:
        args += ["--base-dir", str(base)]
    if offline:
        args.append("--offline")
    if advisory:
        args.append("--advisory")
    original, calls = links.Checker.run, []

    def run(checker, root, options, *, inputs=None, name=None):
        if name in ("external", "external-recheck") and http:
            calls.append((name, inputs))
            return http(name, inputs)
        return original(checker, root, options, inputs=inputs, name=name)

    with (
        patch.object(links, "ROOT", head),
        patch.object(links, "GitHubAPI", return_value=api),
        patch.object(links.Checker, "run", run),
        patch.object(sys, "argv", args),
        redirect_stdout(io.StringIO()),
    ):
        code = links.main()
    report = json.loads((head / "report/observations.json").read_text())
    return code, report, calls


class API:
    def __init__(self, *, repository=200, refs=None, content=None, rename=None):
        self.repository = repository
        self.refs = refs if refs is not None else ["main"]
        self.content = content if content is not None else (404, {})
        self.calls = []
        self.rename = rename

    def get(self, path):
        self.calls.append(path)
        if "/branches/" in path:
            return (
                (200, {"name": self.rename})
                if self.rename and path.endswith("/master")
                else (404, {})
            )
        if "/git/matching-refs/" in path:
            namespace = "heads" if "/heads/" in path else "tags"
            return 200, [{"ref": f"refs/{namespace}/{ref}"} for ref in self.refs]
        if "/contents/" in path or path.endswith("/contents"):
            if isinstance(self.content, list):
                return self.content.pop(0)
            return self.content
        return self.repository, {"default_branch": "main", "private": False}


def observation(
    status="missing", kind="external", url="https://site.invalid/doc", sources=None
):
    return {
        "url": url,
        "kind": kind,
        "status": status,
        "detail": status,
        "sources": sources or [],
    }


def report(*items, stamp=None):
    return {
        "version": 1,
        "scope": "audit",
        "complete": True,
        "observations": list(items),
        **({"generated_at": stamp} if stamp else {}),
    }


def audit(status="missing", stamp=None, kind="external"):
    return report(observation(status, kind), stamp=stamp)


def timed_audit(status="missing", stamp="2026-01-01T12:00:00+00:00", kind="external"):
    return audit(status, stamp, kind)


def item(status="missing"):
    return observation(
        status,
        kind="github",
        url="https://github.com/other/repo/blob/main/doc.md",
        sources=[{"path": "README.md", "line": 7}],
    )


def issue(body, status="open"):
    return {"number": 42, "body": body, "state": status}
