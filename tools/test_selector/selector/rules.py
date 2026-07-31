"""All configuration tables for the test selector.

Loaded from config.json. When the project evolves (new CRD, test suite,
framework, or server), update config.json instead of this file.
"""

from __future__ import annotations

import json
from pathlib import Path


def _load_config() -> dict:
    config_path = Path(__file__).resolve().parent.parent / "config.json"
    return json.loads(config_path.read_text())


_config = _load_config()

# ---------------------------------------------------------------------------
# All known CRD kinds (from keyword_aliases values)
# ---------------------------------------------------------------------------

KEYWORD_ALIASES: dict[str, list[str]] = _config["keyword_aliases"]

ALL_CRD_KINDS: set[str] = {k for kinds in KEYWORD_ALIASES.values() for k in kinds}

# ---------------------------------------------------------------------------
# Python packages
# ---------------------------------------------------------------------------

PYTHON_ALL_E2E_PACKAGES: list[str] = _config["python"]["all_e2e_packages"]

# ---------------------------------------------------------------------------
# File patterns that can be safely ignored (docs, CI, images, etc.)
# ---------------------------------------------------------------------------

IGNORABLE_PATTERNS: list[str] = _config["ignorable_patterns"]
