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

"""Restore pre-generation copyright years without spawning one process per file."""

import argparse
import re
from pathlib import Path


COPYRIGHT = re.compile(rb"Copyright [0-9]{4} The KServe Authors")


def restore_copyright_years(cache_path: Path, root: Path) -> None:
    """Restore the years recorded in a tab-separated cache.

    The cache contains paths relative to ``root`` and the year found before
    generation. Missing files are ignored because code generation can remove
    or rename a previously generated file.
    """
    with cache_path.open("r", encoding="utf-8") as cache:
        for line in cache:
            filename, separator, year = line.rstrip("\r\n").partition("\t")
            if not separator or not year.isdigit():
                continue

            path = Path(filename)
            if not path.is_absolute():
                path = root / path
            if not path.is_file():
                continue

            content = path.read_bytes()
            replacement = f"Copyright {year} The KServe Authors".encode()
            updated = COPYRIGHT.sub(replacement, content)
            if updated != content:
                path.write_bytes(updated)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("cache", type=Path)
    parser.add_argument("--root", type=Path, default=Path.cwd())
    args = parser.parse_args()
    restore_copyright_years(args.cache, args.root)


if __name__ == "__main__":
    main()
