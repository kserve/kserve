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

"""Component discovery and processing utilities."""

from pathlib import Path
from typing import Any, Optional

from . import bash_parser
from . import file_reader


# Section markers for extraction
VARIABLES_SECTION_START = '# VARIABLES'
VARIABLES_SECTION_END = '# VARIABLES END'
INCLUDE_SECTION_START = '# INCLUDE_IN_GENERATED_SCRIPT_START'
INCLUDE_SECTION_END = '# INCLUDE_IN_GENERATED_SCRIPT_END'


def find_component_script(component: str, infra_dir: Path, method: Optional[str] = None) -> Optional[Path]:
    """Find component script following naming conventions with progressive folder search.

    For a component like "gateway-api-crd" with method "helm", searches in order:
    1. gateway-api-crd/manage.gateway-api-crd-helm.sh (method-specific in deepest folder)
    2. gateway-api-crd/manage.gateway-api-crd.sh (base in deepest folder)
    3. gateway-api/manage.gateway-api-crd-helm.sh (method-specific in parent folder)
    4. gateway-api/manage.gateway-api-crd.sh (base in parent folder)
    5. gateway/manage.gateway-api-crd-helm.sh (method-specific in grandparent folder)
    6. gateway/manage.gateway-api-crd.sh (base in grandparent folder)
    7. manage.gateway-api-crd-helm.sh (method-specific in root)
    8. manage.gateway-api-crd.sh (base in root)

    Args:
        component: Component name (e.g., "gateway-api-crd")
        infra_dir: Infrastructure directory path
        method: Installation method (e.g., "helm", "kustomize"), optional

    Returns:
        Path to component script or None if not found
    """
    # Generate folder candidates: gateway-api-crd → gateway-api → gateway → "" (root)
    folder_candidates = []
    parts = component.split('-')
    for i in range(len(parts), 0, -1):
        folder_candidates.append('-'.join(parts[:i]))
    folder_candidates.append("")  # root directory

    # For each folder, try to find the script (method-specific first, then base)
    for folder in folder_candidates:
        base_dir = infra_dir / folder if folder else infra_dir

        # Try method-specific script first (higher priority)
        if method:
            path = base_dir / f"manage.{component}-{method}.sh"
            if path.exists():
                return path

        # Try base script as fallback
        path = base_dir / f"manage.{component}.sh"
        if path.exists():
            return path

    return None


def process_component(comp_config: dict[str, Any], infra_dir: Path, method: Optional[str] = None) -> dict[str, Any]:
    """Process single component: find script, extract and rename functions.

    Args:
        comp_config: Component configuration dict with keys:
                     - name: Component name
                     - env: Environment variables for this component
        infra_dir: Infrastructure directory path
        method: Installation method (e.g., "helm", "kustomize"), optional

    Returns:
        Processed component data dict with keys:
        - name: Component name
        - install_func: Renamed install function name
        - uninstall_func: Renamed uninstall function name
        - install_code: Install function code
        - uninstall_code: Uninstall function code
        - variables: List of variable declarations
        - include_section: Code to include in generated script
        - env: Environment variables

    Raises:
        RuntimeError: If script or required functions not found
    """
    name = comp_config["name"]

    script_file = find_component_script(name, infra_dir, method)
    if not script_file:
        raise RuntimeError(f"Script not found for: {name} (method: {method})")

    # Extract functions
    install_raw = bash_parser.extract_bash_function(script_file, "install")
    uninstall_raw = bash_parser.extract_bash_function(script_file, "uninstall")

    if not install_raw:
        raise RuntimeError(f"Function 'install()' not found in: {script_file}")
    if not uninstall_raw:
        raise RuntimeError(f"Function 'uninstall()' not found in: {script_file}")

    # Extract variables and include section
    variables = file_reader.extract_marked_section(
        script_file,
        VARIABLES_SECTION_START,
        VARIABLES_SECTION_END,
        preserve_indent=False,
        skip_empty=True
    )

    include_section = file_reader.extract_marked_section(
        script_file,
        INCLUDE_SECTION_START,
        INCLUDE_SECTION_END,
        preserve_indent=True,
        skip_empty=False
    )

    # Rename functions
    suffix = name.replace("-", "_")
    install_func = f"install_{suffix}"
    uninstall_func = f"uninstall_{suffix}"

    # Rename install function and its calls
    install_code = bash_parser.rename_bash_function(install_raw, "install", install_func)
    # Also rename any calls to uninstall within install function
    install_code = bash_parser.rename_bash_function(install_code, "uninstall", uninstall_func)

    # Rename uninstall function and its calls
    uninstall_code = bash_parser.rename_bash_function(uninstall_raw, "uninstall", uninstall_func)
    # Also rename any calls to install within uninstall function (less common but possible)
    uninstall_code = bash_parser.rename_bash_function(uninstall_code, "install", install_func)

    return {
        "name": name,
        "install_func": install_func,
        "uninstall_func": uninstall_func,
        "install_code": install_code,
        "uninstall_code": uninstall_code,
        "variables": variables,
        "include_section": include_section,
        "env": comp_config["env"]
    }
