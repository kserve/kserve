"""Discover CRD references in config/, charts/, and hack/ directories."""

from __future__ import annotations

import re
from pathlib import Path

import yaml

_CRD_REF_RE = re.compile(r"(\w+)\.serving\.kserve\.io")

_CRD_KIND_RE = re.compile(r"^kind:\s+(\w+)", re.MULTILINE)

_CRD_SCHEMA_FILE_RE = re.compile(r"serving\.kserve\.io_(\w+)\.yaml$")

_plural_to_kind: dict[str, str] = {}
_singular_to_kind: dict[str, str] = {}
_known_crd_kinds: set[str] = set()
_name_keywords: dict[str, list[str]] = {}
_initialized = False


def _load_yaml(path: Path) -> dict | None:
    """Load a YAML file, returning None only on file/parse errors.

    A missing PyYAML dependency is a hard failure (raised at import time),
    never silently swallowed - otherwise the CRD tables build empty and every
    config/chart file conservatively escalates to running all tests.
    """
    try:
        with open(path) as f:
            return yaml.safe_load(f)
    except (OSError, yaml.YAMLError):
        return None


def _init_crd_tables(repo_root: Path) -> None:
    """Build lookup tables from actual CRD YAML definitions (spec.names)."""
    global _initialized
    if _initialized:
        return
    _initialized = True

    crd_files = list((repo_root / "config" / "crd").rglob("serving.kserve.io_*.yaml"))
    seen_kinds: set[str] = set()

    for crd_file in crd_files:
        doc = _load_yaml(crd_file)
        if not doc or not isinstance(doc, dict):
            continue

        names = doc.get("spec", {}).get("names", {})
        kind = names.get("kind")
        plural = names.get("plural")
        singular = names.get("singular")
        short_names = names.get("shortNames", [])

        if not kind or not plural or kind in seen_kinds:
            continue
        seen_kinds.add(kind)

        _plural_to_kind[plural] = kind
        if singular:
            _singular_to_kind[singular] = kind
        _known_crd_kinds.add(kind)

        if singular:
            _name_keywords.setdefault(singular, []).append(kind)
        for short in short_names:
            _name_keywords.setdefault(short, []).append(kind)

    _add_keyword_aliases()


def _add_keyword_aliases() -> None:
    """Merge KEYWORD_ALIASES into the dynamic keyword table."""
    from ..selector.rules import KEYWORD_ALIASES

    for keyword, kinds in KEYWORD_ALIASES.items():
        existing = _name_keywords.get(keyword, [])
        merged = sorted(set(existing + [k for k in kinds if k in _known_crd_kinds]))
        if merged:
            _name_keywords[keyword] = merged


def discover_config_to_crds(repo_root: Path) -> dict[str, list[str]]:
    """Scan config/, charts/, and hack/ for CRD references.

    Returns: {directory_or_file_path: [CRD kind names]}
    Paths are relative to repo_root (e.g., "config/llmisvc", "charts/kserve-llmisvc-crd").
    """
    _init_crd_tables(repo_root)

    result: dict[str, list[str]] = {}

    for top_dir in ["config", "charts", "hack"]:
        top_path = repo_root / top_dir
        if not top_path.is_dir():
            continue
        _scan_tree(top_path, repo_root, result)

    return result


def _scan_tree(directory: Path, repo_root: Path, result: dict[str, list[str]]) -> None:
    """Recursively scan a directory tree, mapping each subdirectory to CRD kinds."""
    for sub in sorted(directory.iterdir()):
        if not sub.is_dir():
            continue
        if sub.name.startswith("."):
            continue

        rel = str(sub.relative_to(repo_root))
        kinds = _scan_directory(sub, repo_root, result)

        if not kinds:
            kinds = _match_name(sub.name)

        if kinds:
            result[rel] = sorted(set(kinds))

        _scan_tree(sub, repo_root, result)


def _scan_directory(
    directory: Path, repo_root: Path, result: dict[str, list[str]]
) -> list[str]:
    """Scan all files in a directory (non-recursive) for CRD references.

    Also indexes individual files when their name contains a CRD keyword,
    so file-level lookups work for mixed directories like hack/setup/quick-install/.
    """
    dir_kinds: set[str] = set()

    for f in directory.iterdir():
        if f.is_dir():
            continue

        file_kinds: set[str] = set()

        schema_match = _CRD_SCHEMA_FILE_RE.search(f.name)
        if schema_match:
            plural = schema_match.group(1)
            kind = _plural_to_kind.get(plural)
            if kind:
                file_kinds.add(kind)

        if f.suffix in (".yaml", ".yml", ".sh"):
            try:
                content = f.read_text(errors="replace")
            except OSError:
                continue

            for m in _CRD_REF_RE.finditer(content):
                ref = m.group(1)
                kind = _plural_to_kind.get(ref) or _singular_to_kind.get(ref)
                if kind:
                    file_kinds.add(kind)

            if f.suffix in (".yaml", ".yml"):
                for m in _CRD_KIND_RE.finditer(content):
                    kind_val = m.group(1)
                    if kind_val in _known_crd_kinds:
                        file_kinds.add(kind_val)

        if not file_kinds:
            file_kinds = set(_match_name(f.stem))

        if file_kinds:
            rel_file = str(f.relative_to(repo_root))
            result[rel_file] = sorted(file_kinds)

        dir_kinds.update(file_kinds)

    return sorted(dir_kinds)


def _match_name(name: str) -> list[str]:
    """Try to match a directory/file name against CRD keywords (case-insensitive).

    Keywords are checked longest-first so that 'llminferenceservice' matches
    before 'inferenceservice' (which is a substring).
    """
    name_lower = name.lower()
    for keyword in sorted(_name_keywords, key=len, reverse=True):
        if keyword in name_lower:
            return _name_keywords[keyword]
    return []
