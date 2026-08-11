"""Format test selection output as JSON or YAML."""

from __future__ import annotations

import json

from ..mapping.schema import TestSelection


def format_selection(selection: TestSelection, fmt: str = "json") -> str:
    """Format a TestSelection as JSON or YAML string."""
    data = selection.to_dict()

    if fmt == "yaml":
        return _to_yaml(data)

    return json.dumps(data, indent=2)


def _to_yaml(data: dict, indent: int = 0) -> str:
    """Minimal YAML serializer (no PyYAML dependency)."""
    lines: list[str] = []
    prefix = "  " * indent

    for key, value in data.items():
        if isinstance(value, dict):
            lines.append(f"{prefix}{key}:")
            lines.append(_to_yaml(value, indent + 1))
        elif isinstance(value, list):
            if not value:
                lines.append(f"{prefix}{key}: []")
            else:
                lines.append(f"{prefix}{key}:")
                for item in value:
                    if isinstance(item, dict):
                        lines.append(f"{prefix}  -")
                        lines.append(_to_yaml(item, indent + 2))
                    else:
                        lines.append(f"{prefix}  - {_yaml_scalar(item)}")
        elif isinstance(value, bool):
            lines.append(f"{prefix}{key}: {str(value).lower()}")
        elif isinstance(value, (int, float)):
            lines.append(f"{prefix}{key}: {value}")
        elif value is None:
            lines.append(f"{prefix}{key}: null")
        else:
            lines.append(f"{prefix}{key}: {_yaml_scalar(value)}")

    return "\n".join(lines)


def _yaml_scalar(value: object) -> str:
    """Format a scalar value for YAML output."""
    s = str(value)
    if any(c in s for c in ":{},[]&*#?|-<>=!%@\\"):
        return f'"{s}"'
    return s
