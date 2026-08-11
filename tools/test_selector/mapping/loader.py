"""Load mapping.json and merge with overrides at query time."""

from __future__ import annotations

import json
import sys
from pathlib import Path

from ..mapping.schema import Mapping


def load_mapping(repo_root: Path) -> Mapping:
    """Load the pre-computed mapping from mapping.json."""
    mapping_path = repo_root / "tools" / "test_selector" / "mapping.json"

    if not mapping_path.exists():
        print(
            "Error: mapping.json not found. Run 'learn' first.",
            file=sys.stderr,
        )
        raise SystemExit(1)

    data = json.loads(mapping_path.read_text())
    return Mapping.from_dict(data)
