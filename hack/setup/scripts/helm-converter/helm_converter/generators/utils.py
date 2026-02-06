"""
Utility functions for Helm chart generation
"""
from typing import Dict, Any
import yaml
import re


def quote_label_value_if_needed(value: str) -> str:
    """Quote label value if it contains special characters or is a boolean-like string

    Args:
        value: Label value to potentially quote

    Returns:
        Quoted or unquoted value as appropriate
    """
    value_str = str(value)

    # Values that look like booleans should be quoted
    if value_str.lower() in ['true', 'false', 'yes', 'no']:
        return f'"{value_str}"'

    # Values that look like numbers (int or float) should be quoted
    # This includes: "1.0", "123", "3.14", etc.
    try:
        float(value_str)
        return f'"{value_str}"'
    except ValueError:
        pass

    # If it contains special characters, quote it
    if any(char in value_str for char in [':', '{', '}', '[', ']', ',', '&', '*', '#', '?', '|', '-', '<', '>', '=', '!', '%', '@', '`']):
        return f'"{value_str}"'

    return value_str


def add_kustomize_labels(manifest_labels: Dict[str, Any]) -> str:
    """Generate label lines for Kustomize labels (skipping Helm-managed labels)

    Args:
        manifest_labels: Original labels from Kustomize manifest

    Returns:
        YAML string with label lines
    """
    helm_managed_labels = {
        'helm.sh/chart',
        'app.kubernetes.io/managed-by',
        'app.kubernetes.io/instance',
        'app.kubernetes.io/name',
        'app.kubernetes.io/version'
    }

    label_lines = []
    for key, value in manifest_labels.items():
        if key not in helm_managed_labels:
            label_lines.append(f'    {key}: {quote_label_value_if_needed(value)}')

    return '\n'.join(label_lines) if label_lines else ''


def yaml_to_string(obj: Any, indent: int = 0) -> str:
    """Convert a Python object to indented YAML string

    Args:
        obj: Python object to convert
        indent: Number of spaces to indent

    Returns:
        Indented YAML string
    """
    # Convert object to YAML string with error handling
    try:
        yaml_str = yaml.dump(obj, default_flow_style=False, sort_keys=False)
    except Exception as e:
        raise ValueError(f"Failed to convert object to YAML: {e}")

    # Add indentation
    lines = yaml_str.split('\n')
    indented = '\n'.join(' ' * indent + line if line else '' for line in lines)
    return indented


# ============================================================================
# YAML Custom Dumper for Helm Templates
# ============================================================================

class LiteralString(str):
    """String subclass to represent literal block scalars in YAML"""
    pass


class CustomDumper(yaml.SafeDumper):
    """Custom YAML dumper that represents multiline strings as literal block scalars"""
    pass


def str_representer(dumper, data):
    """Represent strings, using literal block scalar for multiline strings"""
    if '\n' in data:
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|-')
    # Use double quotes for strings that look like numbers or booleans
    # to match kustomize output style
    if data in ('True', 'False', 'true', 'false', 'yes', 'no', 'on', 'off'):
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    try:
        float(data)  # Try to parse as number
        return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='"')
    except ValueError:
        pass
    return dumper.represent_scalar('tag:yaml.org,2002:str', data)


def literal_str_representer(dumper, data):
    """Represent LiteralString as YAML literal block scalar (|-)"""
    return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|-')


# Add custom representers
CustomDumper.add_representer(str, str_representer)
CustomDumper.add_representer(LiteralString, literal_str_representer)


def quote_numeric_strings_in_labels(yaml_str: str) -> str:
    """
    Quote numeric-looking strings in YAML label values to preserve them as strings.

    This fixes the issue where "1.0" becomes 1.0 during yaml.dump().
    Specifically targets patterns like:
    - controller-tools.k8s.io: 1.0 → controller-tools.k8s.io: "1.0"
    """
    lines = yaml_str.split('\n')
    result_lines = []
    in_labels_section = False
    labels_indent = 0

    for line in lines:
        # Track if we're in a labels section
        if re.match(r'^(\s*)labels:\s*$', line):
            in_labels_section = True
            labels_indent = len(re.match(r'^(\s*)', line).group(1))
            result_lines.append(line)
            continue

        # Check if we've exited the labels section
        if in_labels_section and line.strip():
            current_indent = len(re.match(r'^(\s*)', line).group(1))
            # If indent is less than or equal to labels indent, we've exited
            if current_indent <= labels_indent:
                in_labels_section = False

        # Apply pattern matching only in labels section
        if in_labels_section and line.strip():
            # Match label lines with numeric values
            # Pattern: "    controller-tools.k8s.io: 1.0" → '    controller-tools.k8s.io: "1.0"'
            match = re.match(r'^(\s+)([\w\-\.]+):\s*(\d+\.?\d*)\s*$', line)
            if match:
                indent_str = match.group(1)
                key = match.group(2)
                value = match.group(3)
                line = f'{indent_str}{key}: "{value}"'

        result_lines.append(line)

    return '\n'.join(result_lines)


def escape_go_templates_in_resource(obj: Any) -> Any:
    """
    Recursively escape Go template expressions in a resource object

    This is needed for resources that contain Go templates as part of their
    configuration (e.g., LLMInferenceServiceConfig args).

    Uses {{ "{{" }} syntax instead of backticks to avoid conflicts with
    backticks used within Go template expressions.
    """
    if isinstance(obj, str):
        # Escape Go template expressions using {{ "{{" }} syntax
        # Use placeholders to avoid double-replacement
        result = obj.replace('{{', '__HELM_OPEN__').replace('}}', '__HELM_CLOSE__')
        result = result.replace('__HELM_OPEN__', '{{ "{{" }}').replace('__HELM_CLOSE__', '{{ "}}" }}')
        # Use LiteralString for multiline strings to ensure YAML uses literal block scalar (|-)
        if '\n' in result:
            return LiteralString(result)
        return result
    elif isinstance(obj, dict):
        return {k: escape_go_templates_in_resource(v) for k, v in obj.items()}
    elif isinstance(obj, list):
        return [escape_go_templates_in_resource(item) for item in obj]
    else:
        return obj


def replace_cert_manager_namespace(annotations: Dict[str, str]) -> Dict[str, str]:
    """
    Replace namespace in cert-manager annotations with Helm template

    Converts annotations like:
        cert-manager.io/inject-ca-from: kserve/serving-cert
    To:
        cert-manager.io/inject-ca-from: {{ .Release.Namespace }}/serving-cert

    Args:
        annotations: Dictionary of Kubernetes annotations

    Returns:
        New dictionary with cert-manager namespace references replaced.
        Returns None if input is None.
        Returns empty dict if input is empty dict.
        Original dictionary is not modified (uses copy).

    Examples:
        >>> replace_cert_manager_namespace({'cert-manager.io/inject-ca-from': 'kserve/serving-cert'})
        {'cert-manager.io/inject-ca-from': '{{ .Release.Namespace }}/serving-cert'}

        >>> replace_cert_manager_namespace({'other-annotation': 'value'})
        {'other-annotation': 'value'}
    """
    if not annotations:
        return annotations

    result = annotations.copy()
    cert_manager_keys = [
        'cert-manager.io/inject-ca-from',
        'cert-manager.io/issuer',
        'cert-manager.io/cluster-issuer'
    ]

    for key in cert_manager_keys:
        if key in result:
            value = result[key]
            # Format: <namespace>/<resource-name>
            # Only replace if there's a slash (indicating namespace/name format)
            if '/' in value:
                parts = value.split('/', 1)  # Split only on first slash
                if len(parts) == 2:
                    namespace, resource_name = parts
                    # Replace namespace with Helm template
                    result[key] = f'{{{{ .Release.Namespace }}}}/{resource_name}'

    return result


# ============================================================================
# Mapper-based Helper Functions (Phase 1)
# ============================================================================

def get_field_value(field_config: dict, manifest: Dict[str, Any], env_vars: dict) -> Any:
    """Extract field value with priority: value > path > kserve-deps.

    Returns extracted value or None if no valid source found.
    """
    # Priority 1: value (explicit override, even if empty string)
    if 'value' in field_config:
        return field_config['value']

    # Priority 2: path (extract from manifest)
    if 'path' in field_config:
        # Import here to avoid circular dependency
        from helm_converter.values_generator.path_extractor import extract_from_manifest
        return extract_from_manifest(manifest, field_config['path'])

    # Priority 3: kserve-deps (environment variable from kserve-deps.env)
    if 'kserve-deps' in field_config:
        env_key = field_config['kserve-deps']
        return env_vars.get(env_key, '')

    return None


def build_template_with_fallback(value_path: str, fallback: str = '') -> str:
    """Build Helm template string with optional fallback

    This function generates Helm template syntax for accessing values.yaml fields,
    with support for fallback values using the Helm 'default' function.

    Args:
        value_path: Dot-notation path to the value in values.yaml
            Example: 'kserve.controller.tag'
        fallback: Optional dot-notation path to fallback value
            Example: 'kserve.version'

    Returns:
        Helm template string ready for insertion into template files

    Examples:
        >>> # Simple value reference without fallback
        >>> build_template_with_fallback('kserve.controller.image')
        '{{ .Values.kserve.controller.image }}'

        >>> # Value reference with fallback
        >>> build_template_with_fallback('kserve.controller.tag', 'kserve.version')
        '{{ .Values.kserve.controller.tag | default .Values.kserve.version }}'

    Note:
        - This is Template-only (not for Values generation)
        - Fallback creates a Helm template that evaluates at deployment time
        - The fallback value must also be a path in values.yaml, not a literal string
    """
    if fallback:
        return f'{{{{ .Values.{value_path} | default .Values.{fallback} }}}}'
    else:
        return f'{{{{ .Values.{value_path} }}}}'


def load_kserve_deps_env(env_file: str = 'kserve-deps.env') -> dict:
    """Load environment variables from kserve-deps.env file.

    Searches provided path, current directory, and up to 5 parent directories.
    Returns empty dict if file not found (optional dependency).
    """
    import os
    from pathlib import Path

    env_vars = {}

    # Try multiple locations to find the file
    search_paths = [env_file]  # Start with the provided path

    # If it's a relative path (default case), also search parent directories
    if not os.path.isabs(env_file):
        cwd = Path.cwd()
        # Search current directory and up to 5 parent directories
        for i in range(6):
            search_paths.append(cwd / env_file)
            cwd = cwd.parent

    # Try each path until we find the file
    for path in search_paths:
        try:
            with open(path) as f:
                for line in f:
                    line = line.strip()
                    # Skip empty lines and comments
                    if line and not line.startswith('#') and '=' in line:
                        key, value = line.split('=', 1)
                        env_vars[key.strip()] = value.strip()
                # Successfully loaded - return
                return env_vars
        except FileNotFoundError:
            # Try next path
            continue
        except Exception:
            # For any other error, continue trying other paths
            continue

    # If we get here, file was not found in any location
    # Return empty dict (file is optional)
    return env_vars


def set_nested_value(target_dict: Dict[str, Any], path: str, value: Any) -> None:
    """Set value in nested dictionary using dot-notation path (e.g., 'kserve.controller.tag').

    Creates intermediate dictionaries as needed. Modifies target_dict in-place.
    """
    keys = path.split('.')
    current = target_dict

    # Navigate/create nested structure
    for key in keys[:-1]:
        if key not in current:
            current[key] = {}
        current = current[key]

    # Set the final value
    current[keys[-1]] = value
