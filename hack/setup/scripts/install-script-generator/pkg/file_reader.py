#!/usr/bin/env python3

# Copyright 2025 The KServe Authors.
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

"""File reading and section extraction utilities."""

import yaml
from pathlib import Path
from typing import Any


def find_git_root(start_dir: Path) -> Path:
    """Find git repository root by walking up directory tree.

    Args:
        start_dir: Starting directory path

    Returns:
        Path to git repository root

    Raises:
        RuntimeError: If .git directory not found
    """
    current = start_dir.resolve()
    while current != current.parent:
        if (current / ".git").exists():
            return current
        current = current.parent
    raise RuntimeError("Could not find git repository root")


def read_env_file(file_path: Path, require_assignment: bool = False) -> list[str]:
    """Read environment file and return valid lines.

    Skips empty lines and comments (lines starting with #).

    Args:
        file_path: Path to environment file
        require_assignment: If True, only return lines containing '='

    Returns:
        List of valid lines from the file
    """
    if not file_path.exists():
        return []
    lines = []
    with open(file_path) as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if require_assignment and "=" not in line:
                continue
            lines.append(line)
    return lines


def read_yaml_file(file_path: Path) -> dict[str, Any]:
    """Read and parse YAML file.

    Args:
        file_path: Path to YAML file

    Returns:
        Dictionary parsed from YAML file, or empty dict if parsing fails

    Raises:
        FileNotFoundError: If file does not exist
    """
    with open(file_path) as f:
        content = f.read()

    try:
        config = yaml.safe_load(content)
        return config if config else {}
    except yaml.YAMLError:
        return {}


def extract_marked_section(file_path: Path,
                           start_marker: str,
                           end_marker: str,
                           preserve_indent: bool = False,
                           skip_empty: bool = False) -> list[str]:
    """Extract section between start and end markers.

    Args:
        file_path: Path to file
        start_marker: Starting marker (e.g., "# VARIABLES")
        end_marker: Ending marker (e.g., "# VARIABLES END")
        preserve_indent: If True, preserve indentation (rstrip only),
                        if False, strip all whitespace
        skip_empty: If True, skip empty lines in the section

    Returns:
        List of lines between markers (excluding marker lines)
    """
    lines = []
    in_section = False

    with open(file_path) as f:
        for line in f:
            stripped = line.strip()

            if stripped == start_marker:
                in_section = True
                continue

            if stripped == end_marker:
                break

            if in_section:
                if skip_empty and not stripped:
                    continue

                if preserve_indent:
                    lines.append(line.rstrip())
                else:
                    lines.append(stripped)

    return lines
