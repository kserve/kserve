"""
Generic Placeholder Generator Module

Generates Helm templates for simple resource types (ClusterServingRuntime, LLMInferenceServiceConfig, etc.)
using a generic placeholder-based approach.
"""

from pathlib import Path
from typing import Dict, Any
import yaml
import copy

from .base_generator import BaseGenerator
from .utils import (
    CustomDumper,
    quote_numeric_strings_in_labels,
    escape_go_templates_in_resource,
    build_template_with_fallback
)


class GenericPlaceholderGenerator(BaseGenerator):
    """Generates templates for simple resource types using placeholder substitution"""

    def generate_templates(self, templates_dir: Path, resource_list: list, subdir_name: str):
        """Generate templates for a list of resources

        Args:
            templates_dir: Path to templates directory
            resource_list: List of resource data dicts [{config, manifest, ...}, ...]
            subdir_name: Subdirectory name (e.g., 'runtimes', 'llmisvcconfigs')
        """
        if not resource_list:
            return

        output_dir = templates_dir / subdir_name
        self._ensure_directory(output_dir)

        for resource_data in resource_list:
            self._generate_single_template(output_dir, resource_data, subdir_name)

    def _generate_single_template(self, output_dir: Path, resource_data: Dict[str, Any], subdir_name: str):
        """Generate a single resource template

        Uses placeholder substitution to replace specific fields with Helm values references,
        while preserving all other fields as-is.

        Args:
            output_dir: Output directory for the template
            resource_data: Resource data dict with 'config', 'manifest', etc.
            subdir_name: Subdirectory name for context
        """
        # Validate resource_data structure (required fields)
        try:
            config = resource_data['config']
            manifest = copy.deepcopy(resource_data['manifest'])
        except KeyError as e:
            raise ValueError(
                f"Resource data missing required field - {e}\n"
                f"Resource data must have: 'config', 'manifest'"
            )

        copy_as_is = resource_data.get('copyAsIs', False)

        # Validate config has name
        try:
            resource_name = config['name']
        except KeyError:
            raise ValueError("Resource config missing required field 'name'")

        # Step 1: Escape Go template expressions
        # All resources may contain Go templates that should be preserved for runtime evaluation
        # We always escape them so Helm doesn't try to process them
        manifest = escape_go_templates_in_resource(manifest)

        # Step 2: Replace configured fields with placeholders
        placeholders = {}

        # Handle image field (repository + tag)
        if 'image' in config:
            img_repo_path = config['image']['repository']['valuePath']
            img_tag_config = config['image'].get('tag', {})
            img_tag_path = img_tag_config.get('valuePath', '')
            fallback = img_tag_config.get('fallback', '')
            placeholder_key = f'__IMAGE_PLACEHOLDER_{img_repo_path}_{img_tag_path}__'

            # Set placeholder in manifest
            # Note: Assuming spec.containers[0].image for most resources
            if 'spec' in manifest and 'containers' in manifest['spec']:
                manifest['spec']['containers'][0]['image'] = placeholder_key
                # Use build_template_with_fallback to handle optional fallback from mapper
                tag_template = build_template_with_fallback(img_tag_path, fallback)
                placeholders[placeholder_key] = f'{{{{ .Values.{img_repo_path} }}}}:{tag_template}'

        # Handle resources field
        if 'resources' in config:
            resources_path = config['resources']['valuePath']
            placeholder_key = f'__RESOURCES_PLACEHOLDER_{resources_path}__'

            # Set placeholder in manifest
            if 'spec' in manifest and 'containers' in manifest['spec']:
                manifest['spec']['containers'][0]['resources'] = placeholder_key
                placeholders[placeholder_key] = f'{{{{- toYaml .Values.{resources_path} | nindent 6 }}}}'

        # Handle container-level fields for ClusterStorageContainer
        if 'container' in config:
            container_config = config['container']

            # Handle container name
            if 'name' in container_config:
                name_config = container_config['name']
                if 'valuePath' in name_config:
                    name_path = name_config['valuePath']
                    placeholder_key = f'__CONTAINER_NAME_PLACEHOLDER_{name_path}__'
                    if 'spec' in manifest and 'container' in manifest['spec']:
                        manifest['spec']['container']['name'] = placeholder_key
                        # manifest value is the default (no hardcoded default needed)
                        placeholders[placeholder_key] = f'{{{{ .Values.{name_path} }}}}'

            # Handle imagePullPolicy
            if 'imagePullPolicy' in container_config:
                policy_config = container_config['imagePullPolicy']
                if 'valuePath' in policy_config:
                    policy_path = policy_config['valuePath']
                    placeholder_key = f'__IMAGE_PULL_POLICY_PLACEHOLDER_{policy_path}__'
                    if 'spec' in manifest and 'container' in manifest['spec']:
                        manifest['spec']['container']['imagePullPolicy'] = placeholder_key
                        placeholders[placeholder_key] = f'{{{{ .Values.{policy_path} }}}}'

            # Handle image for container (ClusterStorageContainer uses spec.container.image)
            if 'image' in container_config:
                img_config = container_config['image']
                if 'repository' in img_config and 'tag' in img_config:
                    img_repo_path = img_config['repository']['valuePath']
                    img_tag_path = img_config['tag']['valuePath']
                    # Get fallback from tag config (same as workload_generator.py)
                    img_tag_config = img_config['tag']
                    fallback = img_tag_config.get('fallback', '')
                    placeholder_key = f'__CONTAINER_IMAGE_PLACEHOLDER_{img_repo_path}_{img_tag_path}__'
                    if 'spec' in manifest and 'container' in manifest['spec']:
                        manifest['spec']['container']['image'] = placeholder_key
                        # Use build_template_with_fallback for tag (same pattern as other image tags)
                        tag_template = build_template_with_fallback(img_tag_path, fallback)
                        placeholders[placeholder_key] = f'{{{{ .Values.{img_repo_path} }}}}:{tag_template}'

            # Handle resources for container
            if 'resources' in container_config:
                resources_config = container_config['resources']
                if 'valuePath' in resources_config:
                    resources_path = resources_config['valuePath']
                    placeholder_key = f'__CONTAINER_RESOURCES_PLACEHOLDER_{resources_path}__'
                    if 'spec' in manifest and 'container' in manifest['spec']:
                        manifest['spec']['container']['resources'] = placeholder_key
                        placeholders[placeholder_key] = f'{{{{- toYaml .Values.{resources_path} | nindent 6 }}}}'

        # Handle supportedUriFormats
        if 'supportedUriFormats' in config:
            formats_config = config['supportedUriFormats']
            if 'valuePath' in formats_config:
                formats_path = formats_config['valuePath']
                placeholder_key = f'__SUPPORTED_URI_FORMATS_PLACEHOLDER_{formats_path}__'
                if 'spec' in manifest and 'supportedUriFormats' in manifest['spec']:
                    manifest['spec']['supportedUriFormats'] = placeholder_key
                    placeholders[placeholder_key] = f'{{{{- toYaml .Values.{formats_path} | nindent 4 }}}}'

        # Handle workloadType
        if 'workloadType' in config:
            workload_config = config['workloadType']
            if 'valuePath' in workload_config:
                workload_path = workload_config['valuePath']
                placeholder_key = f'__WORKLOAD_TYPE_PLACEHOLDER_{workload_path}__'
                if 'spec' in manifest and 'workloadType' in manifest['spec']:
                    manifest['spec']['workloadType'] = placeholder_key
                    # manifest value is the default (no hardcoded default needed)
                    placeholders[placeholder_key] = f'{{{{ .Values.{workload_path} }}}}'

        # Step 3: Add Helm labels to metadata
        if 'metadata' not in manifest:
            manifest['metadata'] = {}
        manifest['metadata']['labels'] = '__HELM_LABELS_PLACEHOLDER__'

        # Add namespace for copyAsIs resources (LLMInferenceServiceConfig)
        if copy_as_is:
            manifest['metadata']['namespace'] = '__NAMESPACE_PLACEHOLDER__'

        # Step 4: Dump manifest to YAML
        manifest_yaml = yaml.dump(manifest, Dumper=CustomDumper, default_flow_style=False, sort_keys=False, width=float('inf'))

        # Quote numeric strings in labels
        manifest_yaml = quote_numeric_strings_in_labels(manifest_yaml)

        # Step 5: Replace placeholders with Helm templates
        chart_name = self._get_chart_name()

        manifest_yaml = manifest_yaml.replace(
            'labels: __HELM_LABELS_PLACEHOLDER__',
            f'labels:\n    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}'
        )

        if copy_as_is:
            manifest_yaml = manifest_yaml.replace(
                'namespace: __NAMESPACE_PLACEHOLDER__',
                'namespace: {{ .Release.Namespace }}'
            )

        # Replace placeholders with Helm templates
        # Two placeholder types: standard resources (ClusterServingRuntime) use IMAGE/RESOURCES,
        # ClusterStorageContainer uses CONTAINER_IMAGE/CONTAINER_RESOURCES for spec.container fields
        for placeholder_key, helm_template in placeholders.items():
            if placeholder_key.startswith('__IMAGE_PLACEHOLDER_'):
                manifest_yaml = manifest_yaml.replace(f'image: {placeholder_key}', f'image: {helm_template}')
            elif placeholder_key.startswith('__CONTAINER_IMAGE_'):
                manifest_yaml = manifest_yaml.replace(f'image: {placeholder_key}', f'image: {helm_template}')
            elif placeholder_key.startswith('__RESOURCES_PLACEHOLDER_'):
                manifest_yaml = manifest_yaml.replace(f'resources: {placeholder_key}', f'resources: {helm_template}')
            elif placeholder_key.startswith('__CONTAINER_RESOURCES_'):
                manifest_yaml = manifest_yaml.replace(f'resources: {placeholder_key}', f'resources: {helm_template}')
            elif placeholder_key.startswith('__CONTAINER_NAME_'):
                manifest_yaml = manifest_yaml.replace(f'name: {placeholder_key}', f'name: {helm_template}')
            elif placeholder_key.startswith('__IMAGE_PULL_POLICY_'):
                manifest_yaml = manifest_yaml.replace(f'imagePullPolicy: {placeholder_key}', f'imagePullPolicy: {helm_template}')
            elif placeholder_key.startswith('__SUPPORTED_URI_FORMATS_'):
                manifest_yaml = manifest_yaml.replace(f'supportedUriFormats: {placeholder_key}', f'supportedUriFormats: {helm_template}')
            elif placeholder_key.startswith('__WORKLOAD_TYPE_'):
                manifest_yaml = manifest_yaml.replace(f'workloadType: {placeholder_key}', f'workloadType: {helm_template}')

        # Step 6: Wrap with conditional blocks
        template = self._wrap_with_conditionals(manifest_yaml, config, subdir_name)

        # Step 7: Write template file
        filename = self._get_output_filename(resource_name, resource_data)
        output_file = output_dir / filename
        self._write_file(output_file, template)

    def _wrap_with_conditionals(self, manifest_yaml: str, config: Dict[str, Any], subdir_name: str) -> str:
        """Wrap manifest YAML with Helm conditional blocks

        Args:
            manifest_yaml: YAML string of the manifest
            config: Resource configuration
            subdir_name: Subdirectory name (runtimes or llmisvcconfigs)

        Returns:
            Template string with conditional wrapping
        """
        # For runtimes: dual conditional (global enabled + individual enabled)
        if subdir_name == 'runtimes':
            enabled_path = config.get('enabledPath')
            if enabled_path:
                return f'''{{{{- if .Values.runtimes.enabled }}}}
{{{{- if .Values.{enabled_path} }}}}
{manifest_yaml}{{{{- end }}}}
{{{{- end }}}}
'''
            else:
                return f'''{{{{- if .Values.runtimes.enabled }}}}
{manifest_yaml}{{{{- end }}}}
'''

        # For llmisvcConfigs: single conditional
        elif subdir_name == 'llmisvcconfigs':
            return f'''{{{{- if .Values.llmisvcConfigs.enabled }}}}
{manifest_yaml}{{{{- end }}}}
'''

        # For common resources (ClusterStorageContainer): conditional with optional fallback from mapper
        elif subdir_name == 'common':
            enabled_path = config.get('enabledPath', 'storageContainer.enabled')
            fallback = config.get('enabledFallback', '')
            if fallback:
                return f'''{{{{- if .Values.{enabled_path} | default .Values.{fallback} }}}}
{manifest_yaml}{{{{- end }}}}
'''
            else:
                return f'''{{{{- if .Values.{enabled_path} }}}}
{manifest_yaml}{{{{- end }}}}
'''

        # Default: no conditional
        return manifest_yaml

    def _get_output_filename(self, resource_name: str, resource_data: Dict[str, Any]) -> str:
        """Get output filename for resource

        Args:
            resource_name: Resource name
            resource_data: Resource data dict

        Returns:
            Output filename
        """
        # Use original filename if specified
        original_filename = resource_data.get('original_filename')
        if original_filename:
            return original_filename

        # Use consistent {kind}_{name}.yaml pattern (same as chart_generator.py:203)
        manifest = resource_data.get('manifest', {})
        kind = manifest.get('kind', 'unknown')
        name = manifest.get('metadata', {}).get('name', resource_name)

        filename = f"{kind.lower()}_{name}.yaml"
        return filename
