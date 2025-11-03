#!/usr/bin/env python3
"""
Fix inferenceservice-config.yaml template to use proper YAML block scalar format (|-) for JSON strings.
This ensures ConfigMap data values are properly formatted as strings, not objects.
"""
import sys
import re


def fix_template(file_path):
    """Fix the ConfigMap template to use |- format for JSON string fields."""
    with open(file_path, 'r') as f:
        content = f.read()

    # List of JSON string fields (exclude _example which should use toYaml)
    json_fields = [
        'agent', 'autoscaler', 'batcher', 'credentials', 'deploy',
        'explainers', 'inferenceService', 'ingress', 'localModel',
        'logger', 'metricsAggregator', 'opentelemetryCollector',
        'router', 'security', 'storageInitializer'
    ]

    for field in json_fields:
        # Pattern: field: {{ .Values.inferenceserviceConfig.field | indent 2 }}
        # Replace with:
        # field: |-
        # {{ .Values.inferenceserviceConfig.field | indent 4 }}
        # Escape braces properly for regex
        old_pattern = (r'^  ' + re.escape(f'{field}:') +
                       r' \{\{ \.Values\.inferenceserviceConfig\.' + re.escape(field) +
                       r' \| indent 2 \}\}$')
        new_pattern = f'  {field}: |-\\n{{{{ .Values.inferenceserviceConfig.{field} | indent 4 }}}}'

        # Use multiline mode
        content = re.sub(old_pattern, new_pattern, content, flags=re.MULTILINE)

        # Also handle case where toYaml might still be present
        old_pattern_with_toYaml = (r'^  ' + re.escape(f'{field}:') +
                                   r' \{\{ \.Values\.inferenceserviceConfig\.' + re.escape(field) +
                                   r' \| toYaml \| indent \d+ \}\}$')
        content = re.sub(old_pattern_with_toYaml, new_pattern, content, flags=re.MULTILINE)

    with open(file_path, 'w') as f:
        f.write(content)


if __name__ == '__main__':
    if len(sys.argv) != 2:
        print(f'Usage: {sys.argv[0]} <template_file>', file=sys.stderr)
        sys.exit(1)

    fix_template(sys.argv[1])
