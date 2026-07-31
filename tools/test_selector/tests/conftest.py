from __future__ import annotations

from pathlib import Path

import pytest

from test_selector.mapping.builder import build_mapping
from test_selector.mapping.schema import Mapping


@pytest.fixture(scope="session")
def repo_root() -> Path:
    return Path(__file__).resolve().parent.parent.parent.parent


@pytest.fixture(scope="session")
def mapping(repo_root: Path) -> Mapping:
    return build_mapping(repo_root)
