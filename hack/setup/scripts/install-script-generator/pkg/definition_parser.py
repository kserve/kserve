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

"""Definition file parsing and normalization utilities."""

from pathlib import Path
from typing import Any

from . import file_reader


# ============================================================================
# Helper Functions
# ============================================================================

def resolve_definition_path(include_path: str, base_definition_file: Path) -> Path:
    """Resolve include path relative to base definition file.

    Args:
        include_path: Path from INCLUDE_DEFINITIONS (can be relative or absolute)
        base_definition_file: Current definition file being parsed

    Returns:
        Resolved absolute path to included definition file
    """
    path = Path(include_path)

    # If absolute path, use as-is
    if path.is_absolute():
        return path

    # Otherwise, resolve relative to base definition file's directory
    return (base_definition_file.parent / path).resolve()


def merge_tools(base_tools: list[str], new_tools: list[str]) -> list[str]:
    """Merge tool lists with last-wins strategy.

    Args:
        base_tools: Existing tools list
        new_tools: Tools to add

    Returns:
        Merged tools list (last occurrence wins for duplicates)
    """
    # Process new tools - if duplicate, remove old position and add at end
    result = []
    for tool in base_tools:
        if tool.lower() not in [t.lower() for t in new_tools]:
            result.append(tool)

    # Add all new tools (including duplicates which override)
    for tool in new_tools:
        result.append(tool)

    return result


def merge_components(
    base_components: list[dict[str, Any]],
    new_components: list[dict[str, Any]]
) -> list[dict[str, Any]]:
    """Merge component lists with last-wins strategy.

    Args:
        base_components: Existing components list
        new_components: Components to add

    Returns:
        Merged components list (last occurrence wins for duplicates, including env)
    """
    # Create dict to track component names
    new_names = {comp["name"] for comp in new_components}

    # Keep base components that don't have duplicates in new_components
    result = []
    for comp in base_components:
        if comp["name"] not in new_names:
            result.append(comp)

    # Add all new components (including ones that override)
    result.extend(new_components)

    return result


# ============================================================================
# Parsing Helper Functions
# ============================================================================

def parse_tools(config: dict[str, Any]) -> list[str]:
    """Parse TOOLS field from config.

    Args:
        config: Raw YAML config dict

    Returns:
        List of tool names
    """
    tools_data = config.get("TOOLS", [])
    if isinstance(tools_data, str):
        return [t.strip() for t in tools_data.split(",") if t.strip()]
    elif isinstance(tools_data, list):
        return [str(t).strip() for t in tools_data]
    else:
        return []


def parse_components(config: dict[str, Any], definition_file: Path) -> list[dict[str, Any]]:
    """Parse COMPONENTS field from config.

    Args:
        config: Raw YAML config dict
        definition_file: Definition file path (for error messages)

    Returns:
        List of component dicts with 'name' and 'env' keys

    Raises:
        ValueError: If COMPONENTS missing or invalid
    """
    components_data = config.get("COMPONENTS")
    if not components_data:
        raise ValueError(f"Error in {definition_file}: COMPONENTS not found")
    if not isinstance(components_data, list):
        raise ValueError(f"Error in {definition_file}: COMPONENTS must be a list")

    components = []
    for item in components_data:
        if isinstance(item, str):
            components.append({"name": item.strip(), "env": {}})
        elif isinstance(item, dict):
            components.append({
                "name": item.get("name", "").strip(),
                "env": item.get("env", {})
            })

    return components


def parse_global_env(config: dict[str, Any]) -> dict[str, str]:
    """Parse GLOBAL_ENV field from config.

    Args:
        config: Raw YAML config dict

    Returns:
        Dict of global environment variables
    """
    global_env_data = config.get("GLOBAL_ENV", {})
    if isinstance(global_env_data, str):
        global_env = {}
        for pair in global_env_data.split():
            if "=" in pair:
                k, v = pair.split("=", 1)
                global_env[k] = v
        return global_env
    elif isinstance(global_env_data, dict):
        return {k: str(v) for k, v in global_env_data.items()}
    else:
        return {}


def parse_embed_manifests(config: dict[str, Any]) -> bool:
    """Parse EMBED_MANIFESTS field from config.

    Args:
        config: Raw YAML config dict

    Returns:
        Boolean indicating whether to embed manifests
    """
    embed_val = config.get("EMBED_MANIFESTS", False)
    return str(embed_val).lower() == "true" if isinstance(embed_val, str) else embed_val


def parse_release(config: dict[str, Any]) -> bool:
    """Parse RELEASE field from config.

    Args:
        config: Raw YAML config dict

    Returns:
        Boolean indicating whether this is a release build
    """
    release_val = config.get("RELEASE", False)
    return str(release_val).lower() == "true" if isinstance(release_val, str) else release_val


# ============================================================================
# Main Parsing Functions
# ============================================================================

def parse_definition_recursive(
    definition_file: Path,
    visited: set[Path] | None = None
) -> dict[str, Any]:
    """Parse definition with INCLUDE_DEFINITIONS support.

    Args:
        definition_file: Path to definition file
        visited: Set of already processed files (for cycle detection)

    Returns:
        Normalized config dictionary

    Raises:
        ValueError: If cycle detected, included file not found, or invalid config
    """
    # Initialize visited set for cycle detection
    if visited is None:
        visited = set()

    # Check for circular dependency
    abs_path = definition_file.resolve()
    if abs_path in visited:
        raise ValueError(f"Circular dependency detected: {abs_path}")
    visited.add(abs_path)

    # Read current file
    config = file_reader.read_yaml_file(definition_file)

    # Process INCLUDE_DEFINITIONS if present
    include_paths = config.get("INCLUDE_DEFINITIONS", [])

    # Validate type
    if include_paths and not isinstance(include_paths, list):
        raise ValueError(
            f"INCLUDE_DEFINITIONS must be a list in {definition_file}, "
            f"got {type(include_paths).__name__}"
        )

    # Initialize merged lists
    merged_tools = []
    merged_components = []

    # Process included files
    if include_paths:
        for include_path in include_paths:
            # Resolve path
            included_file = resolve_definition_path(include_path, definition_file)

            # Validate file exists
            if not included_file.exists():
                raise ValueError(
                    f"Included definition file not found: {include_path}\n"
                    f"Resolved to: {included_file}\n"
                    f"Referenced from: {definition_file}"
                )

            # Recursively parse included file
            included_config = parse_definition_recursive(included_file, visited)

            # Merge tools and components (included files first)
            merged_tools = merge_tools(merged_tools, included_config["tools"])
            merged_components = merge_components(merged_components, included_config["components"])

    # Parse current file's TOOLS and COMPONENTS
    current_tools = parse_tools(config)
    current_components = parse_components(config, definition_file)

    # Merge current file's items (current items can override included ones)
    final_tools = merge_tools(merged_tools, current_tools)
    final_components = merge_components(merged_components, current_components)

    # Return normalized config
    return {
        "file_name": config.get("FILE_NAME") or definition_file.stem,
        "description": config.get("DESCRIPTION", "Install infrastructure components"),
        "method": config.get("METHOD", "helm"),
        "embed_manifests": parse_embed_manifests(config),
        "embed_templates": True,  # Always embed component templates
        "release": parse_release(config),
        "tools": final_tools,
        "global_env": parse_global_env(config),
        "components": final_components
    }


def parse_definition(definition_file: Path) -> dict[str, Any]:
    """Parse and normalize definition file.

    Public API function that supports INCLUDE_DEFINITIONS for composing
    definition files from multiple sources.

    Args:
        definition_file: Path to definition file

    Returns:
        Normalized config dictionary with keys:
        - file_name: Output file name
        - description: Script description
        - method: Installation method (helm/kustomize)
        - embed_manifests: Whether to embed KServe manifests
        - embed_templates: Whether to embed component template files
        - release: Whether to add method suffix to filename
        - tools: List of required tools
        - global_env: Global environment variables
        - components: List of component configs

    Raises:
        ValueError: If invalid config, circular dependency, or file not found
    """
    return parse_definition_recursive(definition_file, visited=set())
