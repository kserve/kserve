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

"""Template discovery and embedding utilities for component scripts."""

from pathlib import Path
from typing import Optional


def find_component_template_dir(component_name: str, infra_dir: Path) -> Optional[Path]:
    """Find component's templates directory following naming conventions.

    Uses progressive folder search similar to find_component_script():
    For component "knative-operator-helm", searches:
      1. knative-operator-helm/templates/
      2. knative-operator/templates/
      3. knative/templates/
      4. (root)/templates/

    Args:
        component_name: Component name (e.g., "knative-operator-helm")
        infra_dir: Infrastructure directory path

    Returns:
        Path to templates directory or None if not found
    """
    # Generate folder candidates: knative-operator-helm → knative-operator → knative → ""
    parts = component_name.split('-')
    folder_candidates = []
    for i in range(len(parts), 0, -1):
        folder_candidates.append('-'.join(parts[:i]))
    folder_candidates.append("")  # root directory

    # Search for templates/ subdirectory in each candidate
    for folder in folder_candidates:
        base_dir = infra_dir / folder if folder else infra_dir
        templates_dir = base_dir / "templates"
        if templates_dir.exists() and templates_dir.is_dir():
            return templates_dir

    return None


def discover_component_templates(component_name: str, infra_dir: Path) -> dict[str, str]:
    """Discover and read template files from component's templates/ directory.

    Searches for .yaml, .yml, and .tmpl files in the templates/ subdirectory.

    Args:
        component_name: Component name
        infra_dir: Infrastructure directory path

    Returns:
        Dictionary mapping template names to their contents
        Example: {"knative-serving-istio": "apiVersion: v1...",
                  "knative-serving-kourier": "apiVersion: v1..."}
        Returns empty dict if no templates directory found
    """
    templates_dir = find_component_template_dir(component_name, infra_dir)
    if not templates_dir:
        return {}

    templates = {}
    # Search for template files (.yaml, .yml, .tmpl)
    for pattern in ['*.yaml', '*.yml', '*.tmpl']:
        for template_file in templates_dir.glob(pattern):
            if template_file.is_file():
                # Remove extensions to get template name
                template_name = template_file.name
                template_name = template_name.replace('.yaml', '').replace('.yml', '').replace('.tmpl', '')

                # Read file content
                with open(template_file, 'r', encoding='utf-8') as f:
                    templates[template_name] = f.read()

    return templates


def template_name_to_function_name(template_name: str) -> str:
    """Convert template name to bash function name.

    Replaces hyphens with underscores for bash compatibility.

    Args:
        template_name: Template name (e.g., "knative-serving-istio")

    Returns:
        Bash function name (e.g., "get_knative_serving_istio")

    Examples:
        "knative-serving-istio" → "get_knative_serving_istio"
        "metallb-config" → "get_metallb_config"
    """
    # Replace hyphens with underscores (bash function naming requirement)
    safe_name = template_name.replace('-', '_')
    return f"get_{safe_name}"


def generate_template_functions(component_name: str, templates: dict[str, str]) -> str:
    """Generate bash getter functions for template files.

    Creates bash functions that output template content using heredoc.
    Uses quoted heredoc to preserve variables and special characters.

    Args:
        component_name: Component name (for documentation)
        templates: Dictionary of template names to contents

    Returns:
        Bash function definitions as string

    Example output:
        get_knative_serving_istio() {
            cat <<'KNATIVE_SERVING_ISTIO_EOF'
        apiVersion: v1
        kind: Namespace
        ...
        KNATIVE_SERVING_ISTIO_EOF
        }
    """
    if not templates:
        return ""

    functions = []
    for template_name, content in sorted(templates.items()):
        func_name = template_name_to_function_name(template_name)
        # Create heredoc delimiter: convert template name to uppercase with underscores
        eof_marker = template_name.replace('-', '_').upper() + '_EOF'

        # Generate function using quoted heredoc to preserve content as-is
        function_code = f"""{func_name}() {{
    cat <<'{eof_marker}'
{content}{eof_marker}
}}"""
        functions.append(function_code)

    return "\n\n".join(functions)
