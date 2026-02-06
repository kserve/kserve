"""
Metadata generator for Helm charts
Handles generation of Chart.yaml, _helpers.tpl, and NOTES.txt
"""
from typing import Dict, Any
from pathlib import Path
from ..constants import MAIN_COMPONENTS
from .base_generator import BaseGenerator


class MetadataGenerator(BaseGenerator):
    """Generator for chart metadata files"""

    def __init__(self, mapping: Dict[str, Any], templates_dir: Path):
        """Initialize MetadataGenerator

        Args:
            mapping: Chart mapping configuration
            templates_dir: Templates directory path
        """
        super().__init__(mapping)
        self.templates_dir = templates_dir

    def generate_chart_yaml(self, chart_dir: Path) -> None:
        """Generate Chart.yaml file

        Args:
            chart_dir: Chart root directory
        """
        # Validate mapping has metadata
        try:
            metadata = self.mapping['metadata']
        except KeyError:
            raise ValueError("Mapping missing required 'metadata' section")

        # Validate metadata has name
        try:
            name = metadata['name']
        except KeyError:
            raise ValueError("Mapping metadata missing required 'name' field")

        chart_yaml = f'''apiVersion: v2
name: {name}
description: {metadata.get('description', 'A Helm chart for Kubernetes')}
type: application
version: {metadata.get('version', '0.1.0')}
appVersion: {metadata.get('appVersion', '1.0.0')}
'''
        chart_file = chart_dir / 'Chart.yaml'
        self._write_file(chart_file, chart_yaml)

    def generate_helpers(self) -> None:
        """Generate _helpers.tpl file"""
        chart_name = self._get_chart_name()

        # Check if we need to generate deploymentName helper
        # This is needed when the deployment name differs from chart name
        # Find the first Deployment workload
        deployment_name = None
        for component_name, component_config in self.mapping.items():
            if isinstance(component_config, dict):
                for config_key, config_value in component_config.items():
                    if (isinstance(config_value, dict) and
                            config_value.get('kind') == 'Deployment' and
                            'name' in config_value):
                        deployment_name = config_value['name']
                        break
                if deployment_name:
                    break

        # Determine which name helper to use in selectorLabels
        name_helper = f'{chart_name}.deploymentName' if deployment_name else f'{chart_name}.name'

        helpers = f'''{{{{/*
Expand the name of the chart.
*/}}}}
{{{{- define "{chart_name}.name" -}}}}
{{{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}}}
{{{{- end }}}}

{{{{/*
Create a default fully qualified app name.
*/}}}}
{{{{- define "{chart_name}.fullname" -}}}}
{{{{- if .Values.fullnameOverride }}}}
{{{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}}}
{{{{- else }}}}
{{{{- $name := default .Chart.Name .Values.nameOverride }}}}
{{{{- if contains $name .Release.Name }}}}
{{{{- .Release.Name | trunc 63 | trimSuffix "-" }}}}
{{{{- else }}}}
{{{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}}}
{{{{- end }}}}
{{{{- end }}}}
{{{{- end }}}}

{{{{/*
Create chart name and version as used by the chart label.
*/}}}}
{{{{- define "{chart_name}.chart" -}}}}
{{{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}}}
{{{{- end }}}}

{{{{/*
Common labels
*/}}}}
{{{{- define "{chart_name}.labels" -}}}}
helm.sh/chart: {{{{ include "{chart_name}.chart" . }}}}
{{{{ include "{chart_name}.selectorLabels" . }}}}
{{{{- if .Chart.AppVersion }}}}
app.kubernetes.io/version: {{{{ .Chart.AppVersion | quote }}}}
{{{{- end }}}}
app.kubernetes.io/managed-by: {{{{ .Release.Service }}}}
{{{{- end }}}}

{{{{/*
Selector labels
*/}}}}
{{{{- define "{chart_name}.selectorLabels" -}}}}
app.kubernetes.io/name: {{{{ include "{name_helper}" . }}}}
app.kubernetes.io/instance: {{{{ .Release.Name }}}}
{{{{- end }}}}
'''

        # Add deploymentName helper if needed
        if deployment_name:
            helpers += f'''
{{{{/*
Create the deployment name
*/}}}}
{{{{- define "{chart_name}.deploymentName" -}}}}
{deployment_name}
{{{{- end }}}}
'''

        helpers_file = self.templates_dir / '_helpers.tpl'
        self._write_file(helpers_file, helpers)

    def generate_notes(self) -> None:
        """Generate NOTES.txt file"""
        chart_name = self._get_chart_name()

        notes = '''Thank you for installing {{ .Chart.Name }}.

Your release is named {{ .Release.Name }}.

To learn more about the release, try:

  $ helm status {{ .Release.Name }}
  $ helm get all {{ .Release.Name }}

'''

        # Add component status based on what's in the mapping
        if 'inferenceServiceConfig' in self.mapping or 'certManager' in self.mapping:
            notes += '''Component Status:
{{- if .Values.inferenceServiceConfig.enabled | default .Values.kserve.createSharedResources }}
  ✓ InferenceService Config: Enabled
{{- else }}
  ✗ InferenceService Config: Disabled
{{- end }}
{{- if .Values.certManager.enabled | default .Values.kserve.createSharedResources }}
  ✓ Cert-Manager Issuer: Enabled
{{- else }}
  ✗ Cert-Manager Issuer: Disabled
{{- end }}

'''

        # Add main component status (always enabled components)
        if chart_name in MAIN_COMPONENTS:
            notes += f'''  ✓ {chart_name.upper()} controller: Always Enabled

'''

        # Add localmodel status
        # kserve-localmodel-resources chart: always enabled (no conditional)
        # other charts with localmodel: conditional
        if chart_name == 'kserve-localmodel-resources':
            notes += '''  ✓ LocalModel controller: Always Enabled

'''
        elif 'localmodel' in self.mapping:
            notes += '''{{- if .Values.localmodel.enabled }}
  ✓ LocalModel controller: Enabled
{{- else }}
  ✗ LocalModel controller: Disabled
{{- end }}

'''

        # Add runtimes status
        if 'clusterServingRuntimes' in self.mapping or 'runtimes' in self.mapping:
            notes += '''{{- if .Values.runtimes.enabled }}
  ✓ ClusterServingRuntimes: Enabled
{{- else }}
  ✗ ClusterServingRuntimes: Disabled
{{- end }}

'''

        # Add llmisvc configs status
        if 'llmisvcConfigs' in self.mapping:
            notes += '''{{- if .Values.llmisvcConfigs.enabled }}
  ✓ LLMInferenceServiceConfigs: Enabled
{{- else }}
  ✗ LLMInferenceServiceConfigs: Disabled
{{- end }}

'''

        notes_file = self.templates_dir / 'NOTES.txt'
        self._write_file(notes_file, notes)
