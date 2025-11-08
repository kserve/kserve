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

"""Script content building and assembly utilities."""

import re
from pathlib import Path
from typing import Any

from . import bash_parser
from . import file_reader
from . import manifest_builder
from . import template_engine


def build_component_variables(components: list[dict[str, Any]],
                              global_env: dict[str, str]) -> str:
    """Build component variables section with deduplication.

    Args:
        components: List of processed components
        global_env: Global environment variables

    Returns:
        Deduplicated variable declarations as string
    """
    # Collect all variables
    all_variables = []
    for comp in components:
        all_variables.extend(comp["variables"])

    # Deduplicate variables
    deduplicated_vars = bash_parser.deduplicate_variables(all_variables)

    # Remove variables that are defined in GLOBAL_ENV only
    # Component env variables are kept in COMPONENT_VARIABLES for default values
    env_vars = set(global_env.keys()) if global_env else set()

    # Remove only GLOBAL_ENV variables from COMPONENT_VARIABLES
    final_vars = []
    for var_line in deduplicated_vars:
        var_name = None
        match = re.match(r"^([A-Z_][A-Z0-9_]*)=", var_line)
        if match:
            var_name = match.group(1)

        # Skip this variable only if it's defined in GLOBAL_ENV
        if var_name and var_name in env_vars:
            continue

        final_vars.append(var_line)

    return "\n".join(final_vars)


def build_component_functions(components: list[dict[str, Any]]) -> str:
    """Build component functions section.

    Args:
        components: List of processed components

    Returns:
        All component functions as string
    """
    component_functions = ""
    for comp in components:
        component_functions += f'''# ----------------------------------------
# CLI/Component: {comp["name"]}
# ----------------------------------------

{comp["uninstall_code"]}

{comp["install_code"]}

'''
    return component_functions


def build_definition_global_env(global_env: dict[str, str]) -> str:
    """Build DEFINITION_GLOBAL_ENV section.

    Uses ${VAR:-default} pattern to respect runtime environment variables.

    Args:
        global_env: Global environment variables

    Returns:
        Environment variable export statements
    """
    if not global_env:
        return ""

    env_lines = [f'    export {k}="${{{k}:-{v}}}"' for k, v in global_env.items()]
    return "\n".join(env_lines)


def build_install_calls(components: list[dict[str, Any]],
                        global_env: dict[str, str],
                        embed_manifests: bool = False) -> str:
    """Build install function calls with env handling.

    Uses set_env_with_priority helper function for proper env variable precedence.
    When embed_manifests is True, uses install_kserve_manifest instead of install_kserve_* for KServe components.

    Args:
        components: List of processed components
        global_env: Global environment variables
        embed_manifests: Whether EMBED_MANIFESTS mode is enabled

    Returns:
        Install function calls as string
    """
    install_calls = []
    for comp in components:
        # Determine actual install function to call
        install_func = comp["install_func"]
        if embed_manifests and comp["name"] in ("kserve", "kserve-helm", "kserve-kustomize"):
            install_func = "install_kserve_manifest"

        if comp["env"]:
            env_calls = []
            for k, v in comp["env"].items():
                # Get global_env value if exists
                global_val = global_env.get(k, "") if global_env else ""

                # Extract default value from component variables
                default_val = ""
                for var_line in comp["variables"]:
                    # Match pattern: VAR="${VAR:-default}"
                    match = re.match(rf'^{k}="\${{{k}:-([^}}]*)}}"', var_line)
                    if match:
                        default_val = match.group(1)
                        break

                env_calls.append(f'        set_env_with_priority "{k}" "{v}" "{global_val}" "{default_val}"')
            env_code = "\n".join(env_calls)

            # Add include_section if present
            include_code = ""
            if comp["include_section"]:
                include_lines = [f'        {line}' for line in comp["include_section"]]
                include_code = "\n" + "\n".join(include_lines) + "\n"

            install_calls.append(f'    (\n{env_code}{include_code}\n        {install_func}\n    )')
        else:
            # No env, but check if there's include_section
            if comp["include_section"]:
                include_lines = [f'        {line}' for line in comp["include_section"]]
                include_code = "\n".join(include_lines)
                install_calls.append(f'    (\n{include_code}\n        {install_func}\n    )')
            else:
                install_calls.append(f'    {install_func}')
    return "\n".join(install_calls)


def build_uninstall_calls(components: list[dict[str, Any]], embed_manifests: bool = False) -> str:
    """Build uninstall function calls (in reverse order).

    When embed_manifests is True, uses uninstall_kserve_manifest instead of uninstall_kserve_* for KServe components.

    Args:
        components: List of processed components
        embed_manifests: Whether EMBED_MANIFESTS mode is enabled

    Returns:
        Uninstall function calls as string
    """
    uninstall_calls = []
    for comp in reversed(components):
        uninstall_func = comp["uninstall_func"]
        if embed_manifests and comp["name"] in ("kserve-helm", "kserve-kustomize"):
            uninstall_func = "uninstall_kserve_manifest"
        uninstall_calls.append(f'        {uninstall_func}')
    return "\n".join(uninstall_calls)


def generate_script_content(definition_file: Path,
                            config: dict[str, Any],
                            components: list[dict[str, Any]],
                            repo_root: Path) -> str:
    """Generate complete script content by filling template.

    Args:
        definition_file: Definition file path
        config: Parsed definition config
        components: Processed components
        repo_root: Repository root path

    Returns:
        Complete script content
    """
    # Generate KServe manifest functions if EMBED_MANIFESTS mode
    kserve_manifest_functions = ""
    if config["embed_manifests"]:
        crd_manifest, core_manifest = manifest_builder.build_kserve_manifests(
            repo_root, config, components
        )
        kserve_manifest_functions = manifest_builder.generate_manifest_functions(
            crd_manifest, core_manifest
        )

    # Read common functions
    common_sh = repo_root / "hack/setup/common.sh"
    with open(common_sh) as f:
        common_functions = bash_parser.extract_common_functions(f.read())

    # Build all sections
    component_variables = build_component_variables(components, config["global_env"])
    component_functions = build_component_functions(components)
    definition_global_env = build_definition_global_env(config["global_env"])
    install_calls_str = build_install_calls(components, config["global_env"], config["embed_manifests"])
    uninstall_calls = build_uninstall_calls(components, config["embed_manifests"])

    # Read env files
    kserve_deps_content = "\n".join(file_reader.read_env_file(repo_root / "kserve-deps.env"))
    global_vars_content = "\n".join(file_reader.read_env_file(
        repo_root / "hack/setup/global-vars.env",
        require_assignment=True
    ))

    # Build replacements dictionary
    replacements = {
        "TEMPLATE_NAME": definition_file.name,
        "DESCRIPTION": config["description"],
        "FILE_NAME": config["file_name"],
        "RELEASE": "true" if config.get("release", False) else "false",
        "KSERVE_DEPS_CONTENT": kserve_deps_content,
        "GLOBAL_VARS_CONTENT": global_vars_content,
        "COMMON_FUNCTIONS": common_functions,
        "COMPONENT_VARIABLES": component_variables,
        "KSERVE_MANIFEST_FUNCTIONS": kserve_manifest_functions,
        "COMPONENT_FUNCTIONS": component_functions,
        "DEFINITION_GLOBAL_ENV": definition_global_env,
        "UNINSTALL_CALLS": uninstall_calls,
        "INSTALL_CALLS": install_calls_str,
    }

    # Load template and replace placeholders
    script_dir = Path(__file__).parent
    template_path = script_dir.parent / "templates" / "generated-script.template"
    return template_engine.generate_from_template(template_path, replacements)
