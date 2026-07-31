"""Map config/ and chart/ changes to test suites using pattern matching."""

from __future__ import annotations

import json
from fnmatch import fnmatch
from pathlib import Path


def load_overrides(repo_root: Path) -> list[dict]:
    """Load overrides from config.json."""
    config_path = repo_root / "tools" / "test_selector" / "config.json"
    if config_path.exists():
        data = json.loads(config_path.read_text())
        return data.get("overrides", [])
    return []


def match_override(file_path: str, overrides: list[dict]) -> list[dict]:
    """Find all overrides matching a file path."""
    matches = []
    for override in overrides:
        pattern = override.get("pattern", "")
        if _matches_pattern(file_path, pattern):
            matches.append(override)
    return matches


def _matches_pattern(file_path: str, pattern: str) -> bool:
    """Check if file_path matches a glob-like pattern."""
    if pattern.endswith("/**"):
        prefix = pattern[:-3]
        return file_path.startswith(prefix + "/") or file_path == prefix
    if pattern.endswith("/*"):
        prefix = pattern[:-2]
        return (
            file_path.startswith(prefix + "/")
            and "/" not in file_path[len(prefix) + 1 :]
        )
    if "*" in pattern:
        return fnmatch(file_path, pattern)
    return file_path == pattern
