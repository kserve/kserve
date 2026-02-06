"""
Values Generator Module

Generates values.yaml file from mapping configuration and default values.
"""

import yaml
from pathlib import Path
from typing import Dict, Any

from .values_generator.utils import OrderedDumper, generate_header, print_keys
from .values_generator.configmap_builder import ConfigMapBuilder
from .values_generator.component_builder import ComponentBuilder
from .values_generator.runtime_builder import RuntimeBuilder
from .values_generator.path_extractor import process_field_with_priority, extract_from_manifest
from .generators.utils import load_kserve_deps_env, get_field_value, set_nested_value


class ValuesGenerator:
    """Generates values.yaml file for Helm chart"""

    def __init__(self, mapping: Dict[str, Any], manifests: Dict[str, Any], output_dir: Path):
        self.mapping = mapping
        self.manifests = manifests
        self.output_dir = output_dir

        # Initialize builders
        self.configmap_builder = ConfigMapBuilder()
        self.component_builder = ComponentBuilder(mapping)
        self.runtime_builder = RuntimeBuilder(mapping)

    def process_globals(self):
        """Process globals section to update mapping metadata

        This must be called before ChartGenerator to ensure metadata fields
        (e.g., metadata.version, metadata.appVersion) are updated from kserve-deps.env
        """
        if 'globals' not in self.mapping:
            return

        env_vars = load_kserve_deps_env()
        for key, config in self.mapping['globals'].items():
            value_path = config['valuePath']
            value = get_field_value(config, None, env_vars)
            if value is not None and value_path.startswith('metadata.'):
                # Update mapping metadata for Chart.yaml generation
                set_nested_value(self.mapping, value_path, value)

    def generate(self):
        """Generate values.yaml file"""
        values = self._build_values()
        values_file = self.output_dir / 'values.yaml'

        with open(values_file, 'w') as f:
            # Add header comment
            chart_name = self.mapping['metadata']['name']
            description = self.mapping['metadata']['description']
            f.write(generate_header(chart_name, description))

            # Generate YAML content
            yaml_content = yaml.dump(values, Dumper=OrderedDumper,
                                     default_flow_style=False,
                                     sort_keys=False,
                                     width=120,
                                     allow_unicode=True)

            # Write final content
            f.write(yaml_content)

    def show_plan(self):
        """Show what values would be generated (dry run)"""
        values = self._build_values()
        print("\n  Sample values structure:")
        print_keys(values, indent=4)

    def _build_values(self) -> Dict[str, Any]:
        """Build the complete values dictionary"""
        values = {}

        # Load environment variables from kserve-deps.env
        env_vars = load_kserve_deps_env()

        # Process globals first (from kserve-deps.env)
        # This ensures global values like kserve.version are available before component processing
        # Also supports updating metadata fields for Chart.yaml
        if 'globals' in self.mapping:
            for key, config in self.mapping['globals'].items():
                value_path = config['valuePath']
                # For globals, manifest is None (values come from env_vars)
                value = get_field_value(config, None, env_vars)
                if value is not None:
                    if value_path.startswith('metadata.'):
                        # Chart.yaml metadata fields: update mapping itself
                        set_nested_value(self.mapping, value_path, value)
                    else:
                        # values.yaml fields: update values dict
                        set_nested_value(values, value_path, value)

        # Add component-specific values (merge with globals, don't overwrite)
        chart_name = self.mapping['metadata']['name']
        if chart_name in self.mapping:
            component_values = self.component_builder.build_component_values(
                chart_name,
                self.mapping[chart_name],
                self.manifests
            )
            # Merge component values with existing values (from globals)
            if chart_name not in values:
                values[chart_name] = {}
            values[chart_name].update(component_values)

        # Add inferenceServiceConfig values (may reference kserve.version anchor)
        if 'inferenceServiceConfig' in self.mapping:
            values['inferenceServiceConfig'] = self.configmap_builder.build_inference_service_config_values(
                self.mapping,
                self.manifests
            )

        # Add certManager values
        if 'certManager' in self.mapping:
            if 'enabled' in self.mapping['certManager']:
                # Extract enabled value using priority logic
                has_value, enabled_value = process_field_with_priority(
                    self.mapping['certManager']['enabled'],
                    None,
                    None
                )

                if has_value:
                    values['certManager'] = {'enabled': enabled_value}
                else:
                    # Fallback: cert-manager is typically enabled by default
                    values['certManager'] = {'enabled': True}

        # Add storageContainer values
        if 'storageContainer' in self.mapping and 'storageContainer-default' in self.manifests.get('common', {}):
            storage_manifest = self.manifests['common']['storageContainer-default']
            storage_config = self.mapping['storageContainer']

            # Build storageContainer values from manifest
            storage_values = {}

            # Extract enabled value using priority logic (same pattern as certManager)
            if 'enabled' in storage_config:
                has_value, enabled_value = process_field_with_priority(
                    storage_config['enabled'],
                    None,
                    None
                )
                if has_value:
                    storage_values['enabled'] = enabled_value

            # Extract container configuration using mapper settings
            container_config = storage_config.get('clusterStorageContainer', {}).get('container', {})
            if container_config:
                storage_values['container'] = {}

                # Extract name using mapper
                if 'name' in container_config:
                    has_value, name_value = process_field_with_priority(
                        container_config['name'],
                        storage_manifest,
                        extract_from_manifest
                    )
                    if has_value:
                        storage_values['container']['name'] = name_value

                # Extract image repository and tag using mapper (respects value="" for fallback)
                if 'image' in container_config:
                    # Repository
                    if 'repository' in container_config['image']:
                        has_value, repo_value = process_field_with_priority(
                            container_config['image']['repository'],
                            storage_manifest,
                            extract_from_manifest
                        )
                        if has_value:
                            storage_values['container']['image'] = repo_value

                    # Tag (mapper has value="", which enables fallback to kserve.version)
                    if 'tag' in container_config['image']:
                        has_value, tag_value = process_field_with_priority(
                            container_config['image']['tag'],
                            storage_manifest,
                            extract_from_manifest
                        )
                        if has_value:
                            storage_values['container']['tag'] = tag_value

                # Extract imagePullPolicy using mapper
                if 'imagePullPolicy' in container_config:
                    has_value, policy_value = process_field_with_priority(
                        container_config['imagePullPolicy'],
                        storage_manifest,
                        extract_from_manifest
                    )
                    if has_value:
                        storage_values['container']['imagePullPolicy'] = policy_value

                # Extract resources using mapper
                if 'resources' in container_config:
                    has_value, resources_value = process_field_with_priority(
                        container_config['resources'],
                        storage_manifest,
                        extract_from_manifest
                    )
                    if has_value:
                        storage_values['container']['resources'] = resources_value

            # Extract supportedUriFormats
            if 'spec' in storage_manifest and 'supportedUriFormats' in storage_manifest['spec']:
                storage_values['supportedUriFormats'] = storage_manifest['spec']['supportedUriFormats']

            # Extract workloadType
            if 'spec' in storage_manifest and 'workloadType' in storage_manifest['spec']:
                storage_values['workloadType'] = storage_manifest['spec']['workloadType']

            values['storageContainer'] = storage_values

        # Add llmisvcConfigs values if present (even if not main chart)
        if 'llmisvcConfigs' in self.mapping and chart_name != 'llmisvc':
            # For llmisvc configs component, just add enabled flag
            if 'enabled' in self.mapping['llmisvcConfigs']:
                has_value, enabled_value = process_field_with_priority(
                    self.mapping['llmisvcConfigs']['enabled'],
                    None,
                    None
                )

                if has_value:
                    values['llmisvcConfigs'] = {'enabled': enabled_value}
                else:
                    # Fallback: LLM configs are optional, default to False
                    values['llmisvcConfigs'] = {'enabled': False}
        # Also support old 'llmisvc' key for backward compatibility
        elif 'llmisvc' in self.mapping and chart_name != 'llmisvc':
            if 'enabled' in self.mapping['llmisvc']:
                has_value, enabled_value = process_field_with_priority(
                    self.mapping['llmisvc']['enabled'],
                    None,
                    None
                )

                if has_value:
                    values['llmisvcConfigs'] = {'enabled': enabled_value}
                else:
                    # Fallback: LLM configs are optional, default to False
                    values['llmisvcConfigs'] = {'enabled': False}

        # Add localmodel values if present
        if 'localmodel' in self.mapping:
            values['localmodel'] = self.component_builder.build_component_values(
                'localmodel',
                self.mapping['localmodel'],
                self.manifests
            )

        # Add localmodelnode values to localmodel section (they belong together)
        if 'localmodelnode' in self.mapping:
            if 'localmodel' not in values:
                values['localmodel'] = {}
            localmodelnode_values = self.component_builder.build_component_values(
                'localmodelnode',
                self.mapping['localmodelnode'],
                self.manifests
            )
            # Merge localmodelnode values into localmodel section
            values['localmodel'].update(localmodelnode_values)

        # Add runtime values (support both clusterServingRuntimes and runtimes keys)
        if 'clusterServingRuntimes' in self.mapping:
            values['runtimes'] = self.runtime_builder.build_runtime_values(
                'clusterServingRuntimes',
                self.manifests
            )
        elif 'runtimes' in self.mapping:
            values['runtimes'] = self.runtime_builder.build_runtime_values(
                'runtimes',
                self.manifests
            )

        return values
