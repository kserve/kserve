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

"""Template processing and placeholder replacement utilities."""

from pathlib import Path


def read_template(template_path: Path) -> str:
    """Read template file content.

    Args:
        template_path: Path to template file

    Returns:
        Template content as string

    Raises:
        FileNotFoundError: If template file not found
    """
    if not template_path.exists():
        raise FileNotFoundError(f"Template file not found: {template_path}")

    with open(template_path) as f:
        return f.read()


def replace_placeholders(template: str, replacements: dict[str, str]) -> str:
    """Replace {{PLACEHOLDER}} markers in template with actual values.

    Args:
        template: Template string containing {{PLACEHOLDER}} markers
        replacements: Dictionary mapping placeholder names to replacement values

    Returns:
        Template with all placeholders replaced

    Example:
        >>> template = "Hello {{NAME}}, version {{VERSION}}"
        >>> replace_placeholders(template, {"NAME": "World", "VERSION": "1.0"})
        'Hello World, version 1.0'
    """
    result = template
    for placeholder, value in replacements.items():
        marker = f"{{{{{placeholder}}}}}"
        result = result.replace(marker, value)
    return result


def generate_from_template(template_path: Path,
                           replacements: dict[str, str]) -> str:
    """Read template file and replace all placeholders.

    Args:
        template_path: Path to template file
        replacements: Dictionary mapping placeholder names to values

    Returns:
        Final content with all replaceholders replaced
    """
    template = read_template(template_path)
    return replace_placeholders(template, replacements)
