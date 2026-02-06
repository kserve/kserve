"""
Chart Generator Module

Generates Helm chart templates from Kubernetes manifests and mapping configuration.
"""

import yaml
from pathlib import Path
from typing import Dict, Any

# Import generators
from .generators import (
    WorkloadGenerator,
    MetadataGenerator,
    GenericPlaceholderGenerator,
    LLMIsvcConfigGenerator,
    CommonTemplateGenerator,
    CustomDumper,
    quote_numeric_strings_in_labels,
    escape_go_templates_in_resource,
    replace_cert_manager_namespace
)
from .constants import MAIN_COMPONENTS, KSERVE_CORE_CRDS, COMPONENT_SPECIFIC_CRDS


class ChartGenerator:
    """Generates Helm chart files from manifests and mapping"""

    def __init__(self, mapping: Dict[str, Any], manifests: Dict[str, Any], output_dir: Path, repo_root: Path):
        self.mapping = mapping
        self.manifests = manifests
        self.output_dir = output_dir
        self.repo_root = repo_root
        self.templates_dir = output_dir / 'templates'

        # Initialize generators
        self.workload_gen = WorkloadGenerator(mapping)
        self.metadata_gen = MetadataGenerator(mapping, self.templates_dir)
        self.generic_gen = GenericPlaceholderGenerator(mapping)
        self.llmisvc_config_gen = LLMIsvcConfigGenerator(mapping)
        self.common_gen = CommonTemplateGenerator(mapping)

    def generate(self):
        """Generate all Helm chart files"""
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.templates_dir.mkdir(parents=True, exist_ok=True)

        self.metadata_gen.generate_chart_yaml(self.output_dir)
        self.common_gen.generate_common_templates(self.templates_dir, self.manifests)

        # ClusterStorageContainer generation
        if 'common' in self.manifests and 'storageContainer-default' in self.manifests['common']:
            storage_mapping = self.mapping.get('storageContainer', {})
            storage_config = storage_mapping.get('clusterStorageContainer', {}).copy()
            enabled_config = storage_mapping.get('enabled', {})

            if 'valuePath' in enabled_config:
                storage_config['enabledPath'] = enabled_config['valuePath']
            if 'fallback' in enabled_config:
                storage_config['enabledFallback'] = enabled_config['fallback']

            storage_resource_list = [{
                'config': storage_config,
                'manifest': self.manifests['common']['storageContainer-default'],
                'copyAsIs': False
            }]
            self.generic_gen.generate_templates(self.templates_dir, storage_resource_list, 'common')

        self._generate_component_templates()
        self.generic_gen.generate_templates(self.templates_dir, self.manifests.get('runtimes', []), 'runtimes')
        self.llmisvc_config_gen.generate_llmisvc_configs_templates(self.templates_dir, self.manifests)

        self.metadata_gen.generate_helpers()
        self.metadata_gen.generate_notes()

    def show_plan(self):
        """Show what would be generated (dry run)"""
        print("  Would generate Chart.yaml")
        print("  Would generate templates/_helpers.tpl")
        print("  Would generate templates/NOTES.txt")

        if 'common' in self.manifests and self.manifests['common']:
            print("  Would generate templates/common/")

        for component_name in self.manifests.get('components', {}).keys():
            print(f"  Would generate templates/{component_name}/")

        if self.manifests.get('runtimes'):
            print(f"  Would generate templates/runtimes/ ({len(self.manifests['runtimes'])} runtimes)")

        if self.manifests.get('llmisvcConfigs'):
            print(f"  Would generate templates/llmisvcconfigs/ ({len(self.manifests['llmisvcConfigs'])} configs)")

    def _generate_component_templates(self):
        """Generate templates for components (kserve, llmisvc, localmodel, localmodelnode)"""
        for component_name, component_data in self.manifests.get('components', {}).items():
            # LocalModel chart uses flat structure (no subdirectory)
            chart_name = self.mapping['metadata']['name']
            if chart_name == 'kserve-localmodel-resources':
                component_dir = self.templates_dir
            else:
                component_dir = self.templates_dir / component_name
                component_dir.mkdir(exist_ok=True)

            if 'manifests' in component_data:
                for manifest_key, manifest in component_data['manifests'].items():
                    # Skip 'resources' - handled separately
                    if manifest_key == 'resources':
                        continue

                    # Check if this is a workload manifest
                    if not isinstance(manifest, dict) or 'kind' not in manifest:
                        continue

                    workload_kind = manifest['kind']

                    # Select generator based on kind
                    if workload_kind == 'Deployment':
                        self.workload_gen.generate_deployment(
                            component_dir,
                            component_name,
                            component_data,
                            manifest_key
                        )
                    elif workload_kind == 'DaemonSet':
                        self.workload_gen.generate_daemonset(
                            component_dir,
                            component_name,
                            component_data,
                            manifest_key
                        )

            # Generate other resources from kustomize build (static with namespace replacement)
            if 'manifests' in component_data and 'resources' in component_data['manifests']:
                copy_as_is = component_data.get('copyAsIs', False)
                self._generate_kustomize_resources(
                    component_dir,
                    component_name,
                    component_data['manifests']['resources'],
                    copy_as_is
                )

    def _generate_kustomize_resources(self, output_dir: Path, component_name: str, resources: Dict[str, Any], copy_as_is: bool = False):
        """
        Generate templates for resources from kustomize build

        Skip Deployment (we generate it separately with templating)
        For other resources, replace namespace with .Release.Namespace

        Args:
            copy_as_is: If True, copy resources as-is without escaping Go templates (for resources that already use Go templates)
        """
        chart_name = self.mapping['metadata']['name']
        is_main_component = component_name in [chart_name] + MAIN_COMPONENTS

        # Collect component-specific CRDs for crds/ directory
        crds_for_crds_dir = []

        for resource_key, resource in resources.items():
            kind = resource.get('kind')
            name = resource.get('metadata', {}).get('name', 'unnamed')

            # Skip Deployment - we generate it separately
            if kind == 'Deployment' and 'manager' in name:
                continue

            # Skip DaemonSet - we generate it separately (e.g., nodeAgent)
            if kind == 'DaemonSet':
                continue

            # Skip Namespace - Helm manages namespaces via --namespace flag and {{ .Release.Namespace }}
            # Users should create namespace with --create-namespace or beforehand
            if kind == 'Namespace':
                continue

            # Handle CustomResourceDefinitions
            if kind == 'CustomResourceDefinition':
                if name in KSERVE_CORE_CRDS:
                    continue

                if component_name in COMPONENT_SPECIFIC_CRDS and name in COMPONENT_SPECIFIC_CRDS[component_name]:
                    crds_for_crds_dir.append((name, resource))
                    continue

                continue

            filename = f"{kind.lower()}_{name}.yaml"

            # For copyAsIs resources (e.g., LLMInferenceServiceConfig with Go templates),
            # don't escape Go templates - just copy as-is with namespace replacement
            if not copy_as_is:
                resource = escape_go_templates_in_resource(resource)

            # Replace namespace with placeholder (will be replaced after yaml.dump)
            if 'metadata' in resource and 'namespace' in resource['metadata']:
                resource['metadata']['namespace'] = '__NAMESPACE_PLACEHOLDER__'

            # Replace namespace in cert-manager annotations with Helm template
            if 'metadata' in resource and 'annotations' in resource['metadata']:
                resource['metadata']['annotations'] = replace_cert_manager_namespace(
                    resource['metadata']['annotations']
                )

            # For webhook configurations, also replace namespace in webhooks
            if kind in ['MutatingWebhookConfiguration', 'ValidatingWebhookConfiguration']:
                if 'webhooks' in resource:
                    for webhook in resource['webhooks']:
                        if 'clientConfig' in webhook and 'service' in webhook['clientConfig']:
                            webhook['clientConfig']['service']['namespace'] = '__NAMESPACE_PLACEHOLDER__'

            # Main component resources are always installed, localmodel needs enabled check
            if is_main_component:
                template = yaml.dump(resource, Dumper=CustomDumper, default_flow_style=False, sort_keys=False, width=float('inf'))
                template = quote_numeric_strings_in_labels(template)
                template = template.replace('namespace: __NAMESPACE_PLACEHOLDER__', 'namespace: {{ .Release.Namespace }}')
            else:
                enabled_path = f"{component_name}.enabled"
                template = f'{{{{- if .Values.{enabled_path} }}}}\n'
                resource_yaml = yaml.dump(resource, Dumper=CustomDumper, default_flow_style=False, sort_keys=False, width=float('inf'))
                resource_yaml = quote_numeric_strings_in_labels(resource_yaml)
                resource_yaml = resource_yaml.replace('namespace: __NAMESPACE_PLACEHOLDER__', 'namespace: {{ .Release.Namespace }}')
                template += resource_yaml
                template += '{{- end }}\n'

            output_file = output_dir / filename
            with open(output_file, 'w') as f:
                f.write(template)

        # Generate component-specific CRDs in crds/ directory
        if crds_for_crds_dir:
            crds_dir = self.output_dir / 'crds'
            crds_dir.mkdir(exist_ok=True)

            for crd_name, crd_resource in crds_for_crds_dir:
                # CRDs in crds/ directory should not have namespace
                if 'metadata' in crd_resource and 'namespace' in crd_resource['metadata']:
                    del crd_resource['metadata']['namespace']

                # Replace namespace in cert-manager annotations with Helm template
                if 'metadata' in crd_resource and 'annotations' in crd_resource['metadata']:
                    crd_resource['metadata']['annotations'] = replace_cert_manager_namespace(
                        crd_resource['metadata']['annotations']
                    )

                filename = f"{crd_name}.yaml"
                crd_yaml = yaml.dump(crd_resource, Dumper=CustomDumper, default_flow_style=False, sort_keys=False, width=float('inf'))

                output_file = crds_dir / filename
                with open(output_file, 'w') as f:
                    f.write(crd_yaml)
