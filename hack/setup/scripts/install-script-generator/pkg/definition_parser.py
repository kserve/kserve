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


def parse_definition(definition_file: Path) -> dict[str, Any]:
    """Parse and normalize definition file.

    Args:
        definition_file: Path to definition file

    Returns:
        Normalized config dictionary with keys:
        - file_name: Output file name
        - description: Script description
        - method: Installation method (helm/kustomize)
        - embed_manifests: Whether to embed KServe manifests
        - tools: List of required tools
        - global_env: Global environment variables
        - components: List of component configs
    """
    config = file_reader.read_yaml_file(definition_file)

    # Parse components
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

    # Parse tools
    tools_data = config.get("TOOLS", [])
    if isinstance(tools_data, str):
        tools = [t.strip() for t in tools_data.split(",") if t.strip()]
    elif isinstance(tools_data, list):
        tools = [str(t).strip() for t in tools_data]
    else:
        tools = []

    # Parse global_env
    global_env_data = config.get("GLOBAL_ENV", {})
    if isinstance(global_env_data, str):
        global_env = {}
        for pair in global_env_data.split():
            if "=" in pair:
                k, v = pair.split("=", 1)
                global_env[k] = v
    elif isinstance(global_env_data, dict):
        global_env = {k: str(v) for k, v in global_env_data.items()}
    else:
        global_env = {}

    # Parse embed_manifests - controls whether to embed KServe manifests in script
    embed_val = config.get("EMBED_MANIFESTS", False)
    embed_manifests = str(embed_val).lower() == "true" if isinstance(embed_val, str) else embed_val

    # Parse release - controls whether to add method suffix to output filename
    release_val = config.get("RELEASE", False)
    release = str(release_val).lower() == "true" if isinstance(release_val, str) else release_val

    return {
        "file_name": config.get("FILE_NAME") or definition_file.stem,
        "description": config.get("DESCRIPTION", "Install infrastructure components"),
        "method": config.get("METHOD", "helm"),
        "embed_manifests": embed_manifests,
        "release": release,
        "tools": tools,
        "global_env": global_env,
        "components": components
    }
