"""
ConfigMap generator for Helm charts
Handles generation of ConfigMap data fields with various types
"""


class ConfigMapGenerator:
    """Generator for ConfigMap templates"""

    def generate_configmap_field(self, field_name: str, field_config: dict) -> str:
        """Generate a single ConfigMap data field template

        Handles different types:
        1. JSON fields with valuePath+defaultValue (credentials, ingress, etc.)
        2. Explainers with image/defaultImageVersion (special case)
        3. Structured fields with image/tag separation (agent, logger, etc.)
        4. LocalModel with defaultJobImage/defaultJobImageTag separation
        5. Simple structured fields (deploy, security, etc.)

        Args:
            field_name: Name of the ConfigMap data field
            field_config: Field configuration from mapper

        Returns:
            Formatted ConfigMap field template
        """
        # Validate field_config is a dict
        if not isinstance(field_config, dict):
            raise ValueError(f"ConfigMap field '{field_name}' config must be a dict, got {type(field_config)}")

        # Check if this field has a valuePath (JSON format - simple passthrough)
        if 'valuePath' in field_config and 'defaultValue' in field_config:
            # JSON format - use toJson
            value_path = field_config['valuePath']
            return f'  {field_name}: |-\n    {{{{- toJson .Values.{value_path} | nindent 4 }}}}\n'

        # Check if this is localModel pattern (has defaultJobImage field)
        # Structure-based detection instead of field name hardcoding
        if 'defaultJobImage' in field_config:
            return self._generate_localmodel_field(field_name, field_config)

        # Check if this is explainers pattern (nested dict where each child has image config)
        # Structure-based detection: if all dict values have 'image' and 'tag' fields
        if self._is_explainers_pattern(field_config):
            return self._generate_explainers_field(field_config)

        # Check if this field has image/tag structure
        has_image = 'image' in field_config and 'tag' in field_config
        has_individual_fields = any(
            isinstance(v, dict) and 'valuePath' in v
            for k, v in field_config.items()
            if k not in ['image', 'tag']
        )

        if has_image:
            # Image-based fields (agent, logger, storageInitializer, batcher, router)
            return self._generate_image_based_field(field_name, field_config)

        if has_individual_fields:
            # Generate JSON with individual fields (metricsAggregator, autoscaler, security, service, etc.)
            return self._generate_individual_fields(field_name, field_config)

        # Simple structured fields (deploy, credentials, ingress, etc.) - use toJson
        return self._generate_simple_structured_field(field_name, field_config)

    def _build_json_field_line(self, parent_path: str, field_key: str, field_type: str) -> str:
        """Build a single JSON field line with proper type handling

        Args:
            parent_path: Parent values path (e.g., 'inferenceServiceConfig.agent')
            field_key: Field key name (e.g., 'memoryRequest')
            field_type: Field type ('boolean', 'number', 'array', 'string')

        Returns:
            JSON field line with proper quotes/toJson based on type
        """
        if field_type in ['boolean', 'number']:
            # Boolean/Number - no quotes
            return f'        "{field_key}": {{{{ .Values.{parent_path}.{field_key} }}}}'
        elif field_type == 'array':
            # Array - use toJson
            return f'        "{field_key}": {{{{- toJson .Values.{parent_path}.{field_key} }}}}'
        else:
            # String (default) - with quotes
            return f'        "{field_key}": "{{{{ .Values.{parent_path}.{field_key} }}}}"'

    def _generate_explainers_field(self, field_config: dict) -> str:
        """Generate ConfigMap field for explainers with defaultImageVersion

        Explainers uses defaultImageVersion instead of tag in ConfigMap.
        Structure: {"art": {"image": "...", "defaultImageVersion": "..."}}
        Uses simplified Helm dict/range with inline dict creation.

        Args:
            field_config: Field configuration from mapper

        Returns:
            Formatted explainers field template
        """
        # Validate field_config is a dict (for consistency)
        if not isinstance(field_config, dict):
            raise ValueError(f"Explainers field config must be a dict, got {type(field_config)}")

        lines = ['  explainers: |-']
        lines.append('    {{- $explainers := dict }}')
        lines.append('    {{- range $name, $config := .Values.inferenceServiceConfig.explainers }}')
        lines.append('      {{- $_ := set $explainers $name (dict "image" $config.image "defaultImageVersion" $config.tag) }}')
        lines.append('    {{- end }}')
        lines.append('    {{- toJson $explainers | nindent 4 }}')
        return '\n'.join(lines) + '\n'

    def _generate_json_field_with_type_support(
            self, field_name: str, field_config: dict,
            image_field_name: str = None) -> str:
        """Generate ConfigMap JSON field with type-aware field rendering

        Unified function for generating JSON fields with proper type handling.
        Supports optional image field combination.

        Args:
            field_name: ConfigMap data field name (e.g., 'agent', 'metricsAggregator')
            field_config: Field configuration from mapper
            image_field_name: If provided, combines image:tag into this field name
                            (e.g., 'image' for agent/logger, 'defaultJobImage' for localModel)

        Returns:
            Formatted ConfigMap field with JSON structure
        """
        # Validate field_config is a dict
        if not isinstance(field_config, dict):
            raise ValueError(
                f"ConfigMap field '{field_name}': field_config must be a dict, got {type(field_config)}"
            )

        parent_path = f'inferenceServiceConfig.{field_name}'
        lines = [f'  {field_name}: |-', '    {']
        json_lines = []

        # Step 1: Add image field if specified (combines image:tag into single field)
        # Most components use 'image'/'tag', but LocalModel uses 'defaultJobImage'/'defaultJobImageTag'
        if image_field_name:
            img_template = f'{{{{ .Values.{parent_path}.image }}}}:{{{{ .Values.{parent_path}.tag | default .Values.kserve.version }}}}'
            if image_field_name == 'defaultJobImage':
                img_template = f'{{{{ .Values.{parent_path}.defaultJobImage }}}}:{{{{ .Values.{parent_path}.defaultJobImageTag | default .Values.kserve.version }}}}'
            json_lines.append(f'        "{image_field_name}" : "{img_template}"')

        # Step 2: Add other fields from config (skip image-related keys already handled above)
        for key, value in field_config.items():
            if key in ['image', 'tag', 'defaultJobImage', 'defaultJobImageTag']:
                continue

            if not isinstance(value, dict) or 'valuePath' not in value:
                continue

            try:
                value_path = value['valuePath']
            except KeyError:
                raise ValueError(
                    f"ConfigMap field '{field_name}.{key}': missing required 'valuePath' in config"
                )

            # Extract output field key from valuePath (last component)
            field_key = value_path.split('.')[-1]
            field_type = value.get('type', 'string')

            # Build JSON line with proper type handling (string vs non-string)
            json_lines.append(self._build_json_field_line(parent_path, field_key, field_type))

        # Join with commas
        for i, line in enumerate(json_lines):
            lines.append(line + (',' if i < len(json_lines) - 1 else ''))

        lines.extend(['    }', ''])
        return '\n'.join(lines)

    def _generate_image_based_field(self, field_name: str, field_config: dict) -> str:
        """Generate ConfigMap field for image-based components (agent, logger, etc.)

        Args:
            field_name: ConfigMap data field name
            field_config: Field configuration from mapper

        Returns:
            Formatted ConfigMap field
        """
        return self._generate_json_field_with_type_support(field_name, field_config, image_field_name='image')

    def _generate_individual_fields(self, field_name: str, field_config: dict) -> str:
        """Generate ConfigMap field for components with individual fields (metricsAggregator, autoscaler, etc.)

        Args:
            field_name: ConfigMap data field name
            field_config: Field configuration from mapper

        Returns:
            Formatted ConfigMap field
        """
        return self._generate_json_field_with_type_support(field_name, field_config)

    def _generate_simple_structured_field(self, field_name: str, field_config: dict) -> str:
        """Generate ConfigMap field for simple structured fields (deploy, security, etc.)

        Just uses toJson on the entire object.

        Args:
            field_name: ConfigMap data field name
            field_config: Field configuration from mapper

        Returns:
            Formatted ConfigMap field
        """
        return f'  {field_name}: |-\n    {{{{- toJson .Values.inferenceServiceConfig.{field_name} | nindent 4 }}}}\n'

    def _is_explainers_pattern(self, field_config: dict) -> bool:
        """Check if field config matches explainers pattern

        Explainers pattern:
        - Nested dict structure where each child is a dict
        - Each child has 'image' and 'tag' fields with valuePath

        Args:
            field_config: Field configuration from mapper

        Returns:
            True if this is explainers pattern, False otherwise

        Example:
            {
                'art': {'image': {'valuePath': '...'}, 'tag': {'valuePath': '...'}},
                'alibi': {'image': {'valuePath': '...'}, 'tag': {'valuePath': '...'}}
            }
        """
        if not isinstance(field_config, dict):
            return False

        # Check if all values are dicts
        dict_values = [v for v in field_config.values() if isinstance(v, dict)]
        if not dict_values or len(dict_values) != len(field_config):
            return False

        # Check if all dict values have 'image' and 'tag' with valuePath
        for child_config in dict_values:
            if not isinstance(child_config, dict):
                return False
            if 'image' not in child_config or 'tag' not in child_config:
                return False
            if not isinstance(child_config['image'], dict) or 'valuePath' not in child_config['image']:
                return False
            if not isinstance(child_config['tag'], dict) or 'valuePath' not in child_config['tag']:
                return False

        return True

    def _generate_localmodel_field(self, field_name: str, field_config: dict) -> str:
        """Generate ConfigMap field for localModel with defaultJobImage/defaultJobImageTag

        Args:
            field_name: ConfigMap data field name
            field_config: Field configuration from mapper

        Returns:
            Formatted ConfigMap field
        """
        return self._generate_json_field_with_type_support(field_name, field_config, image_field_name='defaultJobImage')
