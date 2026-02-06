"""
Utilities Module

Common utilities for values generation including YAML dumping,
header generation, and display functions.
"""

import yaml
from typing import Dict, Any, Optional
from ..constants import CONTAINERS_PATH, RUNTIME_CONTAINERS_PATH, FIRST_CONTAINER_INDEX


# Custom YAML representer to handle dict in order
class OrderedDumper(yaml.SafeDumper):
    """YAML dumper that preserves dictionary order"""
    pass


def dict_representer(dumper, data):
    """Represent dict as ordered mapping"""
    return dumper.represent_mapping(
        yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG,
        data.items()
    )


# Register the representer
OrderedDumper.add_representer(dict, dict_representer)


def generate_header(chart_name: str, description: str) -> str:
    """Generate header comment for values.yaml.

    Args:
        chart_name: Name of the Helm chart
        description: Chart description

    Returns:
        Header comment string
    """
    return f"""# Default values for {chart_name}
# This is a YAML-formatted file.
# Declare variables to be passed into your templates.

# {description}

# NOTE: This file was auto-generated from {chart_name} mapping configuration.
# Source of truth: Kustomize manifests in config/

"""


def print_keys(d: Dict[str, Any], indent: int = 0):
    """Print dictionary keys recursively for dry run output.

    Args:
        d: Dictionary to print
        indent: Current indentation level
    """
    for key, value in d.items():
        if isinstance(value, dict):
            print(' ' * indent + f'- {key}:')
            print_keys(value, indent + 2)
        else:
            print(' ' * indent + f'- {key}: ...')


def get_container(manifest: Dict[str, Any], index: int = FIRST_CONTAINER_INDEX) -> Optional[Dict[str, Any]]:
    """Get container from workload manifest (Deployment/DaemonSet/Job).

    Args:
        manifest: Workload manifest
        index: Container index (default: 0 for first container)

    Returns:
        Container dict or None if not found
    """
    try:
        # Navigate to containers array using CONTAINERS_PATH
        current = manifest
        for key in CONTAINERS_PATH:
            current = current[key]

        # Get container at specified index
        return current[index] if index < len(current) else None
    except (KeyError, IndexError, TypeError):
        return None


def get_container_field(
    manifest: Dict[str, Any],
    field_name: str,
    index: int = FIRST_CONTAINER_INDEX,
    default: Any = None
) -> Any:
    """Get field from container in workload manifest.

    Args:
        manifest: Workload manifest
        field_name: Field name (e.g., 'image', 'resources', 'imagePullPolicy')
        index: Container index (default: 0 for first container)
        default: Default value if field not found

    Returns:
        Field value or default
    """
    container = get_container(manifest, index)
    if container is None:
        return default
    return container.get(field_name, default)


def get_runtime_container_field(
    manifest: Dict[str, Any],
    field_name: str,
    index: int = FIRST_CONTAINER_INDEX,
    default: Any = None
) -> Any:
    """Get field from container in runtime manifest (ClusterServingRuntime/ServingRuntime).

    Args:
        manifest: Runtime manifest
        field_name: Field name (e.g., 'image', 'resources')
        index: Container index (default: 0 for first container)
        default: Default value if field not found

    Returns:
        Field value or default
    """
    try:
        # Navigate to containers array using RUNTIME_CONTAINERS_PATH
        current = manifest
        for key in RUNTIME_CONTAINERS_PATH:
            current = current[key]

        # Get container at specified index
        if index < len(current):
            return current[index].get(field_name, default)
        return default
    except (KeyError, IndexError, TypeError):
        return default
