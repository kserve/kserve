"""
LLMIsvc Config Generator Module

Generates Helm templates for LLMInferenceServiceConfigs.
"""

from pathlib import Path
from typing import Dict, Any

from .base_generator import BaseGenerator
from .utils import yaml_to_string


class LLMIsvcConfigGenerator(BaseGenerator):
    """Generates templates for LLMInferenceServiceConfigs"""

    def generate_llmisvc_configs_templates(self, templates_dir: Path, manifests: Dict[str, Any]):
        """Generate templates for all LLMInferenceServiceConfigs

        Args:
            templates_dir: Path to templates directory
            manifests: Dictionary containing all manifests
        """
        if not manifests.get('llmisvcConfigs'):
            return

        configs_dir = templates_dir / 'llmisvcconfigs'
        self._ensure_directory(configs_dir)

        for config_data in manifests['llmisvcConfigs']:
            self._generate_llmisvc_config_template(configs_dir, config_data)

    def _generate_llmisvc_config_template(self, output_dir: Path, config_data: Dict[str, Any]):
        """Generate a single LLMInferenceServiceConfig template

        These resources contain Go templates that should NOT be escaped
        """
        # Validate config_data structure (required fields)
        try:
            config = config_data['config']
            manifest = config_data['manifest']
        except KeyError as e:
            raise ValueError(
                f"LLMIsvc config data missing required field - {e}\n"
                f"Config data must have: 'config', 'manifest'"
            )

        copy_as_is = config_data.get('copyAsIs', True)
        original_yaml = config_data.get('original_yaml')
        original_filename = config_data.get('original_filename')

        # Validate config has name
        try:
            config_name = config['name']
        except KeyError:
            raise ValueError("LLMIsvc config missing required field 'name'")

        # For copyAsIs resources with original YAML, use it directly with minimal processing
        if copy_as_is and original_yaml:
            # Validate manifest structure
            try:
                api_version = manifest['apiVersion']
                kind = manifest['kind']
                name = manifest['metadata']['name']
            except KeyError as e:
                raise ValueError(
                    f"LLMIsvc manifest missing required field - {e}\n"
                    f"Manifest must have: apiVersion, kind, metadata.name"
                )

            chart_name = self._get_chart_name()

            # Parse the original YAML to extract just the spec section
            # We'll recreate the template with our conditional and labels
            template = f'''{{{{- if .Values.llmisvcConfigs.enabled }}}}
apiVersion: {api_version}
kind: {kind}
metadata:
  name: {name}
  namespace: {{{{ .Release.Namespace }}}}
  labels:
    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}
'''
            # Extract spec section from original YAML
            # Find the top-level "spec:" line and include it and everything after it
            lines = original_yaml.split('\n')
            spec_started = False
            for i, line in enumerate(lines):
                # Look for top-level spec: (no leading spaces)
                if not spec_started and line == 'spec:':
                    spec_started = True
                    template += 'spec:\n'
                elif spec_started:
                    # Escape Go template expressions so Helm doesn't try to process them
                    # Use placeholder technique to avoid double-replacement
                    escaped_line = line.replace('{{', '__HELM_OPEN__').replace('}}', '__HELM_CLOSE__')
                    escaped_line = escaped_line.replace('__HELM_OPEN__', '{{ "{{" }}').replace('__HELM_CLOSE__', '{{ "}}" }}')
                    template += escaped_line + '\n'

            template += '{{- end }}\n'

        else:
            # Validate manifest structure
            try:
                api_version = manifest['apiVersion']
                kind = manifest['kind']
                name = manifest['metadata']['name']
            except KeyError as e:
                raise ValueError(
                    f"LLMIsvc manifest missing required field - {e}\n"
                    f"Manifest must have: apiVersion, kind, metadata.name"
                )

            chart_name = self._get_chart_name()

            # Fallback to normal processing
            template = f'''{{{{- if .Values.llmisvcConfigs.enabled }}}}
apiVersion: {api_version}
kind: {kind}
metadata:
  name: {name}
  labels:
    {{{{- include "{chart_name}.labels" . | nindent 4 }}}}
'''
            if 'spec' in manifest:
                template += 'spec:\n'
                template += yaml_to_string(manifest['spec'], indent=2)

            template += '{{- end }}\n'

        # Use original filename if copyAsIs is True, otherwise sanitize
        if copy_as_is and original_filename:
            filename = original_filename
        else:
            # Sanitize filename
            filename = config_name.replace('kserve-config-', '').replace('kserve-', '') + '.yaml'

        output_file = output_dir / filename
        self._write_file(output_file, template)
