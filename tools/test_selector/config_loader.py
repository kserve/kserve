"""Resolve which selector config file to load.

A local, git-ignored ``config.override.json`` takes precedence over the
committed ``config.json`` when present, so contributors can tweak selection
behavior locally without editing the shared defaults.
"""

from __future__ import annotations

from pathlib import Path

CONFIG_NAME = "config.json"
OVERRIDE_NAME = "config.override.json"


def resolve_config_path(config_dir: Path) -> Path:
    """Return the override config if it exists, otherwise the default config."""
    override = config_dir / OVERRIDE_NAME
    if override.exists():
        return override
    return config_dir / CONFIG_NAME
