"""
Common Template Generator Module

Generates Helm templates for common/base resources (inferenceServiceConfig and certManager).
"""

from pathlib import Path
from typing import Dict, Any

from .base_generator import BaseGenerator
from .utils import yaml_to_string
from .configmap_generator import ConfigMapGenerator


class CommonTemplateGenerator(BaseGenerator):
    """Generates templates for common/base resources"""

    def __init__(self, mapping: Dict[str, Any]):
        super().__init__(mapping)
        self.configmap_gen = ConfigMapGenerator()

    def generate_common_templates(self, templates_dir: Path, manifests: Dict[str, Any]):
        """Generate templates for common/base resources (inferenceServiceConfig and certManager)

        Args:
            templates_dir: Path to templates directory
            manifests: Dictionary containing all manifests
        """
        if 'common' not in manifests or not manifests['common']:
            return

        common_dir = templates_dir / 'common'
        self._ensure_directory(common_dir)

        # Generate ConfigMap template if inferenceServiceConfig is enabled
        if 'inferenceservice-config' in manifests['common'] and 'inferenceServiceConfig' in self.mapping:
            self._generate_configmap_template(common_dir)

        # Generate cert-manager Issuer template if certManager is enabled
        if 'certManager-issuer' in manifests['common'] and 'certManager' in self.mapping:
            self._generate_issuer_template(common_dir, manifests['common']['certManager-issuer'])

    def _generate_configmap_template(self, output_dir: Path):
        """Generate inferenceservice-config ConfigMap template

        Handles both old list format and new dict format for dataFields.
        New format supports individual fields with image/tag separation.
        """
        try:
            config = self.mapping['inferenceServiceConfig']['configMap']
        except KeyError as e:
            raise ValueError(
                f"ConfigMap template generation failed: missing required config - {e}\n"
                f"Required path: mapping['inferenceServiceConfig']['configMap']"
            )

        name = config.get('name', 'inferenceservice-config')

        try:
            data_fields = config['dataFields']
        except KeyError:
            raise ValueError("ConfigMap config missing required field 'dataFields'")

        chart_name = self._get_chart_name()

        # Get enabled configuration from mapper for conditional wrapping
        enabled_config = self.mapping.get('inferenceServiceConfig', {}).get('enabled', {})
        enabled_path = enabled_config.get('valuePath', 'inferenceServiceConfig.enabled')
        fallback = enabled_config.get('fallback', '')

        # Build conditional with optional fallback from mapper
        if fallback:
            template = f'''{{{{- if .Values.{enabled_path} | default .Values.{fallback} }}}}
apiVersion: v1'''
        else:
            template = f'''{{{{- if .Values.{enabled_path} }}}}
apiVersion: v1'''

        template += f'''
kind: ConfigMap
metadata:
  name: {name}
  namespace: {{{{ .Release.Namespace }}}}
  labels:
    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}
data:
'''

        # Support both old list format and new dict format
        if isinstance(data_fields, list):
            # Old format: list of {key, valuePath, defaultValue}
            for field in data_fields:
                key = field.get('key')
                value_path = field.get('valuePath')
                if not key or not value_path:
                    raise ValueError(f"ConfigMap field missing required 'key' or 'valuePath': {field}")
                template += f'  {key}: |-\n    {{{{- toJson .Values.{value_path} | nindent 4 }}}}\n'
        else:
            # New format: nested dictionary with individual fields
            for field_name, field_config in data_fields.items():
                template += self.configmap_gen.generate_configmap_field(field_name, field_config)

        template += '{{- end }}\n'

        # Use consistent {kind}_{name}.yaml pattern (same as chart_generator.py:203)
        output_file = output_dir / f'configmap_{name}.yaml'
        self._write_file(output_file, template)

    def _generate_issuer_template(self, output_dir: Path, issuer_manifest: Dict[str, Any]):
        """Generate cert-manager Issuer template

        Args:
            output_dir: Output directory for the template
            issuer_manifest: Issuer manifest from kustomize build
        """
        try:
            api_version = issuer_manifest['apiVersion']
            kind = issuer_manifest['kind']
            name = issuer_manifest['metadata']['name']
            spec = issuer_manifest['spec']
        except KeyError as e:
            raise ValueError(
                f"Issuer template generation failed: missing required field - {e}\n"
                f"Issuer manifest must have: apiVersion, kind, metadata.name, spec"
            )

        chart_name = self._get_chart_name()

        # Get enabled configuration from mapper for conditional wrapping
        enabled_config = self.mapping.get('certManager', {}).get('enabled', {})
        enabled_path = enabled_config.get('valuePath', 'certManager.enabled')
        fallback = enabled_config.get('fallback', '')

        # Build conditional with optional fallback from mapper
        if fallback:
            template = f'''{{{{- if .Values.{enabled_path} | default .Values.{fallback} }}}}
apiVersion: {api_version}
kind: {kind}'''
        else:
            template = f'''{{{{- if .Values.{enabled_path} }}}}
apiVersion: {api_version}
kind: {kind}'''

        template += f'''
metadata:
  name: {name}
  namespace: {{{{ .Release.Namespace }}}}
  labels:
    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}
spec:
'''
        # Add spec fields
        template += yaml_to_string(spec, indent=2)

        template += '{{- end }}\n'

        # Use consistent {kind}_{name}.yaml pattern (same as chart_generator.py:203)
        output_file = output_dir / f'{kind.lower()}_{name}.yaml'
        self._write_file(output_file, template)
