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

"""Add boilerplate headers to Git-visible Go and Python source files."""

import argparse
import os
import stat
import subprocess
import tempfile
from collections.abc import Iterable
from datetime import date
from pathlib import Path


CURRENT_YEAR = str(date.today().year)
SOURCE_PATHS = (
    "kernelcache/mcv/pkg/*.go",
    "kernelcache/mcv/pkg/**/*.go",
    "kernelcache/mcv/cmd/*.go",
    "kernelcache/mcv/cmd/**/*.go",
    "pkg/*.go",
    "pkg/**/*.go",
    "cmd/*.go",
    "cmd/**/*.go",
    "python/*.py",
    "python/**/*.py",
    "test/e2e/*.py",
    "test/e2e/**/*.py",
)


def collect_source_files(repo_root: Path) -> list[Path]:
    """Return tracked and nonignored source files in the boilerplate scope."""
    result = subprocess.run(
        [
            "git",
            "ls-files",
            "-z",
            "--cached",
            "--others",
            "--exclude-standard",
            "--",
            *SOURCE_PATHS,
        ],
        cwd=repo_root,
        check=True,
        stdout=subprocess.PIPE,
    )

    files = []
    for raw_path in result.stdout.split(b"\0"):
        if not raw_path:
            continue
        path = repo_root / os.fsdecode(raw_path)
        if (path.suffix == ".py" and "_pb2" in path.name) or not path.is_file():
            continue
        files.append(path)
    return files


def file_year(path: Path, repo_root: Path) -> str:
    """Return the file's first-commit year, or the current year if untracked."""
    result = subprocess.run(
        [
            "git",
            "log",
            "--follow",
            "--diff-filter=A",
            "--format=%ad",
            "--date=format:%Y",
            "--",
            str(path.relative_to(repo_root)),
        ],
        cwd=repo_root,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
        text=True,
    )
    years = result.stdout.splitlines()
    return years[-1] if years else CURRENT_YEAR


def load_year_cache(cache_path: Path | None, repo_root: Path) -> dict[Path, str]:
    """Load cached first-commit years keyed by absolute repository path."""
    if cache_path is None:
        return {}

    if not cache_path.is_absolute():
        cache_path = repo_root / cache_path
    if not cache_path.is_file():
        return {}

    years = {}
    for line in cache_path.read_text(encoding="utf-8").splitlines():
        filename, separator, year = line.partition("\t")
        if not separator or not year.isdigit():
            continue
        path = Path(filename)
        if not path.is_absolute():
            path = repo_root / path
        years[path.resolve()] = year
    return years


def add_header(
    path: Path,
    template: Path,
    repo_root: Path,
    cached_years: dict[Path, str] | None = None,
) -> None:
    """Prepend a year-specific template while preserving file metadata."""
    year = (cached_years or {}).get(path.resolve())
    if year is None:
        year = file_year(path, repo_root)
    header = template.read_bytes().replace(b" YEAR", f" {year}".encode())
    mode = stat.S_IMODE(path.stat().st_mode)

    temporary_path = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="wb", dir=path.parent, prefix=f".{path.name}.", delete=False
        ) as temporary:
            temporary_path = Path(temporary.name)
            temporary.write(header)
            temporary.write(path.read_bytes())
        os.chmod(temporary_path, mode)
        os.replace(temporary_path, path)
    finally:
        if temporary_path is not None and temporary_path.exists():
            temporary_path.unlink()


def add_missing_headers(
    repo_root: Path,
    files: Iterable[Path],
    cache_path: Path | None = None,
) -> None:
    """Add the matching boilerplate to files that lack a copyright marker."""
    templates = {
        ".go": repo_root / "hack/boilerplate.go.txt",
        ".py": repo_root / "hack/boilerplate.python.txt",
    }
    cached_years = load_year_cache(cache_path, repo_root)
    for path in files:
        if b"Copyright" in path.read_bytes():
            continue
        add_header(path, templates[path.suffix], repo_root, cached_years)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--year-cache", type=Path)
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    add_missing_headers(repo_root, collect_source_files(repo_root), args.year_cache)


if __name__ == "__main__":
    main()
