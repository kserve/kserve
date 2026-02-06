"""
Workload generator for Helm charts
Handles generation of Deployment and DaemonSet templates
"""
from typing import Dict, Any, Optional
from pathlib import Path
from .base_generator import BaseGenerator
from .utils import (
    add_kustomize_labels,
    quote_label_value_if_needed,
    yaml_to_string,
    replace_cert_manager_namespace,
    build_template_with_fallback
)
from ..constants import MAIN_COMPONENTS


class WorkloadGenerator(BaseGenerator):
    """Generator for Deployment and DaemonSet templates"""

    def generate_deployment(
            self, output_dir: Path, component_name: str,
            component_data: Dict[str, Any], manifest_key: str) -> None:
        """Generate Deployment template for a component

        Args:
            output_dir: Output directory for template
            component_name: Name of the component
            component_data: Component configuration and manifests
            manifest_key: Key in component_data['manifests'] for the deployment
        """
        self._generate_workload(
            output_dir=output_dir,
            component_name=component_name,
            component_data=component_data,
            workload_type='deployment',
            config_key=manifest_key
        )

    def generate_daemonset(
            self, output_dir: Path, component_name: str,
            component_data: Dict[str, Any], manifest_key: str) -> None:
        """Generate DaemonSet template for a component's nodeAgent

        Args:
            output_dir: Output directory for template
            component_name: Name of the component
            component_data: Component configuration and manifests
            manifest_key: Key in component_data['manifests'] for the daemonset
        """
        self._generate_workload(
            output_dir=output_dir,
            component_name=component_name,
            component_data=component_data,
            workload_type='daemonset',
            config_key=manifest_key
        )

    def _extract_workload_manifest(
            self,
            component_data: Dict[str, Any],
            config_key: str
    ) -> Optional[Dict[str, Any]]:
        """Extract workload manifest from component data

        Args:
            component_data: Component configuration and manifests
            config_key: Key to lookup in manifests (e.g., 'controllerManager', 'nodeAgent')

        Returns:
            Workload manifest dict, or None if not found
        """
        # Validate component_data has manifests
        if 'manifests' not in component_data:
            raise ValueError(
                f"Component data missing required 'manifests' section for '{config_key}'"
            )

        manifest = component_data['manifests'].get(config_key)
        if not manifest:
            return None

        # Handle both dict and list formats
        if isinstance(manifest, dict):
            return manifest
        elif isinstance(manifest, list) and len(manifest) > 0:
            return manifest[0]
        else:
            return None

    def _determine_component_status(
            self,
            component_name: str,
            component_data: Dict[str, Any]
    ) -> tuple[bool, Optional[str]]:
        """Determine if component is main and get enabled path

        Args:
            component_name: Name of the component
            component_data: Component configuration and manifests

        Returns:
            Tuple of (is_main_component, enabled_path)
        """
        chart_name = self._get_chart_name()
        is_main = component_name in [chart_name] + MAIN_COMPONENTS
        enabled_path = None if is_main else component_data['config'].get('enabled', {}).get('valuePath')
        return is_main, enabled_path

    def _generate_selector_labels(self, match_labels: Dict[str, str]) -> str:
        """Generate selector labels template

        Args:
            match_labels: Label dict from spec.selector.matchLabels

        Returns:
            YAML template string for selector labels
        """
        lines = []
        for key, value in match_labels.items():
            lines.append(f'      {key}: {quote_label_value_if_needed(value)}')
        return '\n'.join(lines)

    def _get_output_filename(self, workload_type: str, workload_name: str) -> str:
        """Get output filename for workload

        Args:
            workload_type: 'deployment' or 'daemonset'
            workload_name: Name of the workload resource

        Returns:
            Output filename (e.g., 'deployment_foo.yaml', 'daemonset_foo.yaml')
        """
        if workload_type == 'deployment':
            return f'deployment_{workload_name}.yaml'
        else:  # daemonset
            return f'daemonset_{workload_name}.yaml'

    def _generate_pod_metadata(
            self,
            pod_metadata: Dict[str, Any],
            workload_type: str
    ) -> str:
        """Generate pod metadata with correct field ordering

        Args:
            pod_metadata: Pod metadata from spec.template.metadata
            workload_type: 'deployment' or 'daemonset'

        Returns:
            YAML template string for pod metadata
        """
        lines = ['    metadata:']

        # Consistent field order for all workload types: labels → annotations
        field_order = ['labels', 'annotations']

        for field_name in field_order:
            if field_name == 'labels':
                if 'labels' not in pod_metadata:
                    continue
                lines.append('      labels:')
                for key, value in pod_metadata['labels'].items():
                    lines.append(f'        {key}: {quote_label_value_if_needed(value)}')
            elif field_name == 'annotations' and 'annotations' in pod_metadata:
                processed_annotations = replace_cert_manager_namespace(pod_metadata['annotations'])
                lines.append('      annotations:')
                for key, value in processed_annotations.items():
                    lines.append(f'        {key}: {value}')

        return '\n'.join(lines)

    def _generate_pod_spec_fields(
            self,
            pod_spec: Dict[str, Any],
            component_config: Dict[str, Any],
            workload_type: str
    ) -> str:
        """Generate pod spec fields (containers and others)

        Args:
            pod_spec: Pod spec from spec.template.spec
            component_config: Component configuration (controllerManager or nodeAgent)
            workload_type: 'deployment' or 'daemonset'

        Returns:
            YAML template string for pod spec fields
        """
        lines = []

        for field_name, field_value in pod_spec.items():
            if field_name == 'containers':
                # Special handling for containers (image/resources substitution)
                lines.append('      containers:')

                # Check if new containers section exists in mapper
                containers_config = component_config.get('containers', {})

                for container in pod_spec['containers']:
                    container_name = container['name']

                    if container_name in containers_config:
                        container_specific_config = containers_config[container_name]
                        is_configured = True
                    else:
                        container_specific_config = {}
                        is_configured = False

                    container_spec = self._generate_container_spec(
                        container, is_configured, container_specific_config, workload_type,
                        container_name=container_name
                    )
                    lines.append(container_spec)
            else:
                if field_name in component_config:
                    mapper_config = component_config[field_name]
                    template = self._render_field_generic(
                        field_name, field_value, mapper_config, base_indent=6
                    )
                    lines.append(template)
                else:
                    # Not in mapper → static (keep manifest value as-is)
                    if isinstance(field_value, (dict, list)):
                        lines.append(f'      {field_name}:')
                        lines.append(yaml_to_string(field_value, indent=8))
                    else:
                        # Scalar value (string, int, bool, etc.)
                        yaml_value = str(field_value).lower() if isinstance(field_value, bool) else field_value
                        lines.append(f'      {field_name}: {yaml_value}')

        return '\n'.join(lines)

    def _render_field_generic(
            self,
            field_name: str,
            field_value: Any,
            mapper_config: Dict[str, Any],
            base_indent: int
    ) -> str:
        """Render a field with three modes: configurable (valuePath), nested, or static."""
        lines = []
        content_indent = base_indent + 2
        indent_str = ' ' * base_indent

        if isinstance(mapper_config, dict) and 'valuePath' in mapper_config:
            # Case 1: Entire field configurable
            value_path = mapper_config['valuePath']

            # Special handling for affinity/tolerations: always render with default empty values
            # This ensures empty affinity: {} and tolerations: [] from Kustomize manifests are preserved
            if field_name in ['affinity', 'tolerations']:
                default_val = 'dict' if field_name == 'affinity' else 'list'
                lines.append(f"{indent_str}{field_name}: {{{{- toYaml (.Values.{value_path} | default {default_val}) | nindent {content_indent} }}}}")
            else:
                # Use {{- with }} for optional fields (only renders if value exists)
                lines.append(f"{indent_str}{{{{- with .Values.{value_path} }}}}")
                lines.append(f"{indent_str}{field_name}:")
                lines.append(f"{indent_str}  {{{{- toYaml . | nindent {content_indent} }}}}")
                lines.append(f"{indent_str}{{{{- end }}}}")

        elif isinstance(mapper_config, dict) and isinstance(field_value, dict):
            # Case 2: Nested mapper (partial configurability)
            # Check if it's a nested config (has sub-fields with path/valuePath)
            has_nested_config = any(
                isinstance(v, dict) and ('path' in v or 'valuePath' in v)
                for v in mapper_config.values()
            )

            if has_nested_config:
                # Render as mixed nested (configurable + static)
                nested_template = self._render_nested_field(
                    field_name, field_value, mapper_config, indent=base_indent
                )
                lines.append(nested_template)
            else:
                # Not nested config, treat as static
                lines.append(f"{indent_str}{field_name}:")
                lines.append(yaml_to_string(field_value, indent=content_indent))

        else:
            # Case 3: Static
            if isinstance(field_value, (dict, list)):
                lines.append(f"{indent_str}{field_name}:")
                lines.append(yaml_to_string(field_value, indent=content_indent))
            else:
                yaml_value = str(field_value).lower() if isinstance(field_value, bool) else field_value
                lines.append(f"{indent_str}{field_name}: {yaml_value}")

        return '\n'.join(lines)

    def _generate_workload(
            self,
            output_dir: Path,
            component_name: str,
            component_data: Dict[str, Any],
            workload_type: str,
            config_key: str
    ) -> None:
        """Generate workload (Deployment or DaemonSet) template

        Args:
            output_dir: Output directory for template
            component_name: Name of the component
            component_data: Component configuration and manifests
            workload_type: 'deployment' or 'daemonset'
            config_key: Config key to lookup ('controllerManager' or 'nodeAgent')
        """
        # Extract manifest
        workload = self._extract_workload_manifest(component_data, config_key)
        if not workload:
            return

        component_config = component_data['config'].get(config_key, {})
        is_main, enabled_path = self._determine_component_status(component_name, component_data)

        # Validate workload manifest structure (required for template generation)
        try:
            match_labels = workload['spec']['selector']['matchLabels']
            pod_metadata = workload['spec']['template']['metadata']
            pod_spec = workload['spec']['template']['spec']
            workload_name = workload['metadata']['name']
        except KeyError as e:
            raise ValueError(
                f"Workload manifest missing required field - {e}\n"
                f"Required structure: metadata.name, spec.selector.matchLabels, "
                f"spec.template.metadata, spec.template.spec"
            )

        # Generate template
        template = self._generate_workload_header(workload, is_main, enabled_path)
        template += '\nspec:\n  selector:\n    matchLabels:\n'
        template += self._generate_selector_labels(match_labels)
        template += '\n  template:\n'
        template += self._generate_pod_metadata(pod_metadata, workload_type)
        template += '\n    spec:\n'
        template += self._generate_pod_spec_fields(pod_spec, component_config, workload_type)

        if not is_main:
            template += '{{- end }}\n'

        # Write file
        filename = self._get_output_filename(workload_type, workload_name)
        output_file = output_dir / filename
        self._ensure_directory(output_file.parent)
        self._write_file(output_file, template)

    def _generate_workload_header(
            self, workload: Dict[str, Any], is_main_component: bool,
            enabled_path: str = None) -> str:
        """Generate workload (Deployment/DaemonSet) header with metadata and labels

        Args:
            workload: Workload manifest (Deployment or DaemonSet)
            is_main_component: Whether this is a main component (always enabled)
            enabled_path: Path to enabled flag in values (only for non-main components)

        Returns:
            YAML template string with header, metadata, and labels
        """
        chart_name = self._get_chart_name()

        # Validate workload manifest structure
        try:
            api_version = workload['apiVersion']
            kind = workload['kind']
            name = workload['metadata']['name']
        except KeyError as e:
            raise ValueError(
                f"Workload header generation failed: missing required field - {e}\n"
                f"Workload manifest must have: apiVersion, kind, metadata.name"
            )

        lines = []

        # Add conditional wrapper for non-main components
        if not is_main_component and enabled_path:
            lines.append(f'{{{{- if .Values.{enabled_path} }}}}')

        # Add apiVersion, kind, metadata
        lines.extend([
            f'apiVersion: {api_version}',
            f'kind: {kind}',
            'metadata:',
            f'  name: {name}',
            '  namespace: {{ .Release.Namespace }}',
            '  labels:',
            f'    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}'
        ])

        # Add Kustomize labels
        if 'labels' in workload['metadata']:
            kustomize_labels = add_kustomize_labels(workload['metadata']['labels'])
            if kustomize_labels:
                lines.append(kustomize_labels)

        return '\n'.join(lines)

    def _generate_container_spec(
            self, container: Dict[str, Any], is_configured: bool,
            component_config: Dict[str, Any],
            workload_type: str = 'deployment',
            container_name: str = '') -> str:
        """Generate complete container specification

        Args:
            container: Container spec from manifest
            is_configured: Whether this container is configured in mapper (configurable vs static)
            component_config: Container-specific configuration from mapper
            workload_type: 'deployment' or 'daemonset' (affects which fields to include)
            container_name: Name of the container

        Returns:
            Complete container YAML template string
        """
        lines = [f'      - name: {container["name"]}']

        # Define special fields that need custom processing
        CONTAINER_SPECIAL_FIELDS = {'image', 'imagePullPolicy', 'resources'}

        # Track processed fields to avoid duplicates
        processed_fields = {'name'}

        # --- SPECIAL FIELD HANDLING ---

        # 1. Image configuration (split repository:tag)
        if 'image' in container:
            if is_configured and 'image' in component_config:
                img_repo_path = component_config['image']['repository']['valuePath']
                img_tag_config = component_config['image'].get('tag', {})
                img_tag_path = img_tag_config.get('valuePath', '')
                fallback = img_tag_config.get('fallback', '')

                # Use build_template_with_fallback (same as GenericPlaceholderGenerator)
                tag_template = build_template_with_fallback(img_tag_path, fallback)
                lines.append(f'        image: "{{{{ .Values.{img_repo_path} }}}}:{tag_template}"')
            else:
                lines.append(f'        image: "{container["image"]}"')
            processed_fields.add('image')

        # 2. Image pull policy (tied to image config)
        if 'imagePullPolicy' in container:
            if is_configured and 'image' in component_config and 'pullPolicy' in component_config['image']:
                policy_path = component_config['image']['pullPolicy']['valuePath']
                lines.append(f'        imagePullPolicy: {{{{ .Values.{policy_path} }}}}')
            else:
                lines.append(f'        imagePullPolicy: {container["imagePullPolicy"]}')
            processed_fields.add('imagePullPolicy')

        # 3. Resources (special extraction logic)
        if 'resources' in container:
            if is_configured and 'resources' in component_config:
                resources_path = component_config['resources']['valuePath']
                lines.append(f'        resources: {{{{- toYaml .Values.{resources_path} | nindent 10 }}}}')
            else:
                lines.append('        resources:')
                lines.append(yaml_to_string(container['resources'], indent=10))
            processed_fields.add('resources')

        # --- GENERIC FIELD HANDLING ---
        # Process all remaining fields using generic mapper-based logic
        for field_name, field_value in container.items():
            # Skip already processed fields
            if field_name in processed_fields or field_name in CONTAINER_SPECIAL_FIELDS:
                continue

            # Skip deployment-only fields in daemonset
            if workload_type == 'daemonset' and field_name in ['args', 'ports']:
                continue

            # Skip deployment-only probes in daemonset
            if workload_type == 'daemonset' and field_name in ['livenessProbe', 'readinessProbe']:
                continue

            # Check mapper for configurability
            if field_name in component_config:
                mapper_config = component_config[field_name]

                # Use generic rendering for configured containers
                # For unconfigured containers (sidecar), render as static
                if is_configured:
                    template = self._render_field_generic(
                        field_name, field_value, mapper_config, base_indent=8
                    )
                    lines.append(template)
                else:
                    # Unconfigured container - always static
                    if isinstance(field_value, (dict, list)):
                        lines.append(f'        {field_name}:')
                        lines.append(yaml_to_string(field_value, indent=10))
                    else:
                        yaml_value = str(field_value).lower() if isinstance(field_value, bool) else field_value
                        lines.append(f'        {field_name}: {yaml_value}')
            else:
                # Not in mapper → static
                if isinstance(field_value, (dict, list)):
                    lines.append(f'        {field_name}:')
                    lines.append(yaml_to_string(field_value, indent=10))
                else:
                    yaml_value = str(field_value).lower() if isinstance(field_value, bool) else field_value
                    lines.append(f'        {field_name}: {yaml_value}')

            processed_fields.add(field_name)

        return '\n'.join(lines)

    def _render_nested_field(
            self,
            field_name: str,
            manifest_value: Dict[str, Any],
            mapper_config: Dict[str, Any],
            indent: int = 8
    ) -> str:
        """
        Render nested field with mixed configurable/static parts

        This enables partial configurability of complex fields like securityContext,
        where some sub-fields are configurable while others remain static.

        Args:
            field_name: Name of the field (e.g., 'securityContext')
            manifest_value: Actual value from manifest (e.g., {'runAsNonRoot': True, ...})
            mapper_config: Nested mapper configuration with sub-field mappings
            indent: Base indentation level (default: 8)

        Returns:
            Rendered Helm template string with mixed configurable and static parts

        Example:
            manifest_value = {
                'runAsNonRoot': True,
                'seccompProfile': {'type': 'RuntimeDefault'}
            }

            mapper_config = {
                'runAsNonRoot': {
                    'path': 'spec.template.spec.securityContext.runAsNonRoot',
                    'valuePath': 'llmisvc.controller.securityContext.runAsNonRoot'
                }
                # seccompProfile not in mapper -> stays static
            }

            Returns:
                securityContext:
                  runAsNonRoot: {{ .Values.llmisvc.controller.securityContext.runAsNonRoot }}
                  seccompProfile:
                    type: RuntimeDefault
        """
        lines = []
        lines.append(f"{' ' * (indent - 2)}{field_name}:")

        for key, value in manifest_value.items():
            if key in mapper_config:
                sub_config = mapper_config[key]

                if isinstance(sub_config, dict) and 'valuePath' in sub_config:
                    # Leaf configurable node
                    value_path = sub_config['valuePath']

                    # Check if value is dict or list - use toYaml for complex types
                    if isinstance(value, (dict, list)):
                        lines.append(f"{' ' * indent}{key}: {{{{- toYaml .Values.{value_path} | nindent {indent + 2} }}}}")
                    else:
                        # Scalar value - direct substitution
                        lines.append(f"{' ' * indent}{key}: {{{{ .Values.{value_path} }}}}")

                elif isinstance(value, dict) and isinstance(sub_config, dict):
                    # Recursive nested - check if sub_config has nested mappings
                    has_nested_mappings = any(
                        isinstance(v, dict) and ('path' in v or 'valuePath' in v)
                        for v in sub_config.values()
                    )

                    if has_nested_mappings:
                        # Recurse deeper
                        nested = self._render_nested_field(key, value, sub_config, indent + 2)
                        lines.append(nested)
                    else:
                        # No nested mappings, treat as static
                        lines.append(f"{' ' * indent}{key}:")
                        lines.append(yaml_to_string(value, indent + 2))

                else:
                    # Static leaf with unexpected type
                    lines.append(f"{' ' * indent}{key}:")
                    lines.append(yaml_to_string(value, indent + 2))
            else:
                # Key not in mapper -> static
                if isinstance(value, dict):
                    lines.append(f"{' ' * indent}{key}:")
                    lines.append(yaml_to_string(value, indent + 2))
                elif isinstance(value, list):
                    lines.append(f"{' ' * indent}{key}:")
                    lines.append(yaml_to_string(value, indent + 2))
                else:
                    # Scalar value
                    yaml_value = str(value).lower() if isinstance(value, bool) else value
                    lines.append(f"{' ' * indent}{key}: {yaml_value}")

        return '\n'.join(lines)
