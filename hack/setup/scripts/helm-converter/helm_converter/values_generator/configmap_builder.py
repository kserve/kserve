"""
ConfigMap Builder Module

Builds inferenceServiceConfig values section from ConfigMap manifests.
"""

from typing import Dict, Any

from .path_extractor import extract_from_configmap, process_field_with_priority

# Metadata fields that should be skipped during recursive processing
METADATA_FIELDS = {'valuePath', 'description', 'path', 'value', 'type'}


class ConfigMapBuilder:
    """Builds ConfigMap-based values (inferenceServiceConfig)"""

    def __init__(self):
        pass

    def build_inference_service_config_values(
        self,
        mapping: Dict[str, Any],
        manifests: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Build inferenceServiceConfig section of values.

        Reads from kustomize ConfigMap manifest using path specifications from mapper.
        This ensures that helm template output matches kustomize build output.

        Args:
            mapping: Mapping configuration
            manifests: Kubernetes manifests

        Returns:
            InferenceServiceConfig values dictionary
        """
        isvc_config = mapping['inferenceServiceConfig']
        values = {}

        # Enabled flag - extract using generic priority logic
        if 'enabled' in isvc_config:
            has_value, enabled_value = process_field_with_priority(
                isvc_config['enabled'],
                None,
                extract_from_configmap
            )
            if has_value:
                values['enabled'] = enabled_value
            else:
                # Fallback: inferenceServiceConfig is required, default to True
                values['enabled'] = True

        # Find the ConfigMap manifest
        configmap_manifest = None
        for resource_key, resource in manifests.get('common', {}).items():
            if 'inferenceservice-config' in resource_key.lower():
                configmap_manifest = resource
                break

        if not configmap_manifest or 'data' not in configmap_manifest:
            return values

        # Get dataFields config - new dict format with path-based extraction
        data_fields_config = isvc_config.get('configMap', {}).get('dataFields', {})

        # Process each top-level data field (agent, logger, etc.)
        for field_name, field_config in data_fields_config.items():
            # Recursively process this field
            values[field_name] = self._process_configmap_field(
                field_config,
                configmap_manifest,
                f"inferenceServiceConfig.{field_name}"
            )

        return values

    def _process_configmap_field(
        self,
        field_config: Any,
        configmap_manifest: Dict[str, Any],
        value_path_prefix: str
    ) -> Any:
        """Process a ConfigMap field configuration recursively.

        Args:
            field_config: Field configuration from mapper (can be dict or simple value)
            configmap_manifest: ConfigMap manifest to extract values from
            value_path_prefix: Prefix for valuePath (e.g., "inferenceServiceConfig.agent")

        Returns:
            Extracted value(s) according to field configuration
        """
        # If field_config is not a dict, return as-is
        if not isinstance(field_config, dict):
            return field_config

        # Check for 'value' or 'path' field with priority handling
        has_value, extracted_value = process_field_with_priority(
            field_config,
            configmap_manifest,
            extract_from_configmap,
            path_spec_key='path'
        )

        if has_value:
            return extracted_value

        # No 'path' field - this is a nested structure, recurse into it
        result = {}
        for key, value in field_config.items():
            # Skip metadata fields
            if key in METADATA_FIELDS:
                continue

            # Recursively process nested fields
            result[key] = self._process_configmap_field(
                value,
                configmap_manifest,
                f"{value_path_prefix}.{key}"
            )

        return result
