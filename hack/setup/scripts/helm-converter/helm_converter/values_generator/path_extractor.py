"""
Path Extractor Module

Utilities for extracting values from manifests using path specifications.
Supports split operations for image strings (e.g., repository:tag).
"""

from typing import Any, Dict, Tuple, Optional
import json


def parse_split_spec(path_spec: str) -> Tuple[str, Optional[Tuple[str, int]]]:
    """Parse path specification into base path and split specification.

    Args:
        path_spec: Path with optional split operation
                  e.g., "spec.image+(:,0)" means get spec.image, split by ':', take index 0

    Returns:
        Tuple of (base_path, split_spec)
        split_spec is None or (delimiter, index)

    Raises:
        ValueError: Invalid split specification format

    Examples:
        >>> parse_split_spec("spec.image")
        ("spec.image", None)

        >>> parse_split_spec("spec.image+(:,0)")
        ("spec.image", (":", 0))
    """
    if '+' not in path_spec:
        return path_spec.strip(), None

    base_path, split_spec_str = path_spec.split('+', 1)
    base_path = base_path.strip()
    split_spec_str = split_spec_str.strip()

    # Parse split_spec: "(delimiter,index)"
    if not (split_spec_str.startswith('(') and split_spec_str.endswith(')')):
        raise ValueError(
            f"Invalid split specification: {split_spec_str}. "
            f"Must be in format (delimiter,index)"
        )

    split_params = split_spec_str[1:-1].split(',', 1)
    if len(split_params) != 2:
        raise ValueError(
            f"Split spec must have format (delimiter,index): {split_spec_str}"
        )

    delimiter = split_params[0].strip()
    try:
        split_index = int(split_params[1].strip())
    except ValueError:
        raise ValueError(f"Split index must be integer: {split_params[1]}")

    return base_path, (delimiter, split_index)


def apply_split(value: str, delimiter: str, index: int) -> str:
    """Apply split operation to a string value.

    Uses rsplit for ':' delimiter to handle registry:port correctly.
    e.g., "registry:5000/image:tag" -> rsplit(':', 1) -> ["registry:5000/image", "tag"]

    Args:
        value: String to split
        delimiter: Split delimiter
        index: Index to extract after split

    Returns:
        Split value at the specified index

    Raises:
        ValueError: Cannot split non-string value
        IndexError: Split index out of range

    Examples:
        >>> apply_split("kserve/controller:v1.0", ":", 0)
        "kserve/controller"

        >>> apply_split("kserve/controller:v1.0", ":", 1)
        "v1.0"

        >>> apply_split("registry:5000/image:tag", ":", 0)
        "registry:5000/image"  # rsplit keeps registry:port together
    """
    if not isinstance(value, str):
        raise ValueError(f"Cannot split non-string value: {type(value)}")

    # For image fields, always use rsplit to handle registry:port correctly
    if delimiter == ':':
        parts = value.rsplit(delimiter, 1)
    else:
        parts = value.split(delimiter)

    try:
        return parts[index]
    except IndexError:
        raise IndexError(
            f"Split index {index} out of range. "
            f"String '{value}' split by '{delimiter}' has {len(parts)} parts"
        )


def extract_from_manifest(manifest: Dict[str, Any], path_spec: str) -> Any:
    """Extract value from manifest using path specification.

    Supports array indexing and split operations for image strings.

    Path spec format:
        - Basic path: "spec.template.spec.containers[0].image"
        - With split: "spec.template.spec.containers[0].image+(:,0)"
          where + indicates split operation
          (:,0) means split by ':' and take index 0 (repository)
          (:,1) means split by ':' and take index 1 (tag)

    Args:
        manifest: Kubernetes manifest dictionary
        path_spec: Path with optional split operation

    Returns:
        Extracted value

    Raises:
        KeyError: Path not found in manifest
        IndexError: Invalid split index or array index
        ValueError: Invalid path specification format

    Examples:
        >>> manifest = {"spec": {"containers": [{"image": "kserve/controller:v1.0"}]}}
        >>> extract_from_manifest(manifest, "spec.containers[0].image")
        "kserve/controller:v1.0"

        >>> extract_from_manifest(manifest, "spec.containers[0].image+(:,0)")
        "kserve/controller"

        >>> extract_from_manifest(manifest, "spec.containers[0].image+(:,1)")
        "v1.0"
    """
    # Parse split specification
    base_path, split_spec = parse_split_spec(path_spec)

    # Navigate base path
    value = manifest
    path_parts = []
    current_part = ""
    in_bracket = False

    for char in base_path:
        if char == '[':
            if current_part:
                path_parts.append(('key', current_part))
                current_part = ""
            in_bracket = True
        elif char == ']':
            if in_bracket and current_part:
                try:
                    path_parts.append(('index', int(current_part)))
                except ValueError:
                    raise ValueError(f"Array index must be integer: {current_part}")
                current_part = ""
            in_bracket = False
        elif char == '.' and not in_bracket:
            if current_part:
                path_parts.append(('key', current_part))
                current_part = ""
        else:
            current_part += char

    if current_part:
        path_parts.append(('key', current_part))

    # Navigate through path_parts
    for part_type, part_value in path_parts:
        if part_type == 'key':
            if not isinstance(value, dict):
                raise KeyError(
                    f"Cannot access key '{part_value}' on non-dict type {type(value)}"
                )
            value = value[part_value]
        elif part_type == 'index':
            if not isinstance(value, list):
                raise IndexError(
                    f"Cannot access index {part_value} on non-list type {type(value)}"
                )
            value = value[part_value]

    # Apply split operation if specified
    if split_spec:
        delimiter, split_index = split_spec
        value = apply_split(value, delimiter, split_index)

    return value


def extract_from_configmap(
    configmap_manifest: Dict[str, Any],
    path_spec: str
) -> Any:
    """Extract value from ConfigMap using path specification with JSON parsing.

    ConfigMap data fields often contain JSON strings. This function automatically
    parses JSON when navigating into ConfigMap data fields.

    Path spec format:
        - data.agent.image+(:,0) - Extract from JSON in data.agent, get image field,
                                   split by :, take index 0
        - data.credentials - Extract entire JSON from data.credentials

    Args:
        configmap_manifest: ConfigMap manifest
        path_spec: Path with optional split operation

    Returns:
        Extracted value

    Raises:
        KeyError: Path not found
        IndexError: Invalid split index
        ValueError: Invalid path specification
        json.JSONDecodeError: Invalid JSON in ConfigMap data field

    Examples:
        >>> cm = {"data": {"agent": '{"image": "kserve/agent:v1.0"}'}}
        >>> extract_from_configmap(cm, "data.agent.image")
        "kserve/agent:v1.0"

        >>> extract_from_configmap(cm, "data.agent.image+(:,0)")
        "kserve/agent"
    """
    # Parse split specification
    base_path, split_spec = parse_split_spec(path_spec)

    # Navigate path
    parts = base_path.split('.')
    value = configmap_manifest

    for i, part in enumerate(parts):
        value = value[part]

        # If this is the second level (data.FIELD) and value is a string, parse as JSON
        if i == 1 and parts[0] == 'data' and isinstance(value, str):
            try:
                value = json.loads(value)
            except json.JSONDecodeError as e:
                raise json.JSONDecodeError(
                    f"Failed to parse JSON from ConfigMap data.{part}: {e.msg}",
                    e.doc,
                    e.pos
                )

    # Apply split operation if specified
    if split_spec:
        delimiter, split_index = split_spec
        value = apply_split(value, delimiter, split_index)

    return value


def process_field_with_priority(
    field_config: Any,
    manifest: Optional[Dict[str, Any]],
    extractor_func: callable,
    path_spec_key: str = 'path'
) -> Tuple[bool, Any]:
    """Process a field configuration with priority: value > path > None.

    Priority order:
    1. 'value' field (highest priority) - use this value directly
    2. 'path' field - extract from manifest using extractor function
    3. None - field should be processed recursively or skipped

    Args:
        field_config: Field configuration (can be dict, string, or any type)
        manifest: Manifest to extract from (can be None if not needed)
        extractor_func: Function to extract value from manifest
                       Should have signature: (manifest, path_spec) -> Any
                       Can be None if manifest is None
        path_spec_key: Key name for path specification (default: 'path')

    Returns:
        Tuple of (has_value, value):
        - (True, value) if 'value' or 'path' field found
        - (False, None) if neither found (caller should handle recursively)

    Examples:
        >>> # Case 1: 'value' field present
        >>> config = {'value': True, 'path': 'data.enabled'}
        >>> has_value, result = process_field_with_priority(config, manifest, extract_from_manifest)
        >>> assert has_value is True and result is True  # Uses 'value', ignores 'path'

        >>> # Case 2: Only 'path' field present
        >>> config = {'path': 'data.enabled'}
        >>> has_value, result = process_field_with_priority(config, manifest, extract_from_manifest)
        >>> assert has_value is True and result == <extracted_value>

        >>> # Case 3: Neither field present (recursive processing)
        >>> config = {'nested': {'field': 'value'}}
        >>> has_value, result = process_field_with_priority(config, manifest, extract_from_manifest)
        >>> assert has_value is False  # Caller should recurse
    """
    # If not a dict, no special processing needed
    if not isinstance(field_config, dict):
        return False, None

    # Priority 1: Check for 'value' field (highest priority)
    if 'value' in field_config:
        return True, field_config['value']

    # Priority 2: Check for 'path' field
    if path_spec_key in field_config:
        path_spec = field_config[path_spec_key]
        try:
            value = extractor_func(manifest, path_spec)
            return True, value
        except (KeyError, IndexError, ValueError, json.JSONDecodeError) as e:
            # Log warning and return None if path extraction fails
            print(f"Warning: Failed to extract value from path '{path_spec}': {e}")
            return True, None

    # No 'value' or 'path' - caller should handle recursively
    return False, None
