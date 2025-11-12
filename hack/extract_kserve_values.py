#!/usr/bin/env python3
"""
Extract kserve.* values from source ConfigMap and add them to Helm values.yaml.

This script:
1. Parses the source ConfigMap (config/configmap/inferenceservice.yaml)
2. Extracts values from JSON strings (e.g., "image": "kserve/agent:latest")
3. Parses the generated Helm template to find all .Values.kserve.* references
4. Maps source values to Helm values structure
5. Appends missing values to values.yaml
"""
import sys
import re
import json
import yaml
from pathlib import Path


def extract_json_block(content, field_name):
    """Extract JSON block for a field from YAML content."""
    # Find the field (non-example, has comment "# field contains")
    pattern = rf'{field_name}: \|-\s*\{{(?:\s*# .*?)?\s*"'
    match = re.search(pattern, content, re.MULTILINE | re.DOTALL)
    if not match:
        return None

    # Find the start of the JSON block
    start = match.end() - 1  # Back to the opening brace
    brace_count = 0
    i = start

    # Find matching closing brace
    while i < len(content):
        if content[i] == '{':
            brace_count += 1
        elif content[i] == '}':
            brace_count -= 1
            if brace_count == 0:
                # Found matching closing brace
                json_str = content[start:i+1]
                try:
                    return json.loads(json_str)
                except json.JSONDecodeError:
                    return None
        i += 1
    return None


def parse_configmap_source(source_path):
    """Parse the source ConfigMap and extract JSON values."""
    with open(source_path, 'r') as f:
        content = f.read()

    values = {}

    # Extract agent configuration (use the non-example block)
    agent_data = extract_json_block(content, 'agent')
    if agent_data and 'image' in agent_data:
        image_parts = agent_data['image'].rsplit(':', 1)
        values['agent'] = {
            'image': image_parts[0],
            'tag': image_parts[1] if len(image_parts) > 1 else 'latest'
        }

    # Extract router configuration
    router_data = extract_json_block(content, 'router')
    if router_data and 'image' in router_data:
        image_parts = router_data['image'].rsplit(':', 1)
        values['router'] = {
            'image': image_parts[0],
            'tag': image_parts[1] if len(image_parts) > 1 else 'latest',
            'imagePullPolicy': router_data.get('imagePullPolicy', 'IfNotPresent'),
            'imagePullSecrets': router_data.get('imagePullSecrets', [])
        }

    # Extract localModel configuration
    localmodel_data = extract_json_block(content, 'localModel')
    if localmodel_data:
        values['localmodel'] = {
            'enabled': localmodel_data.get('enabled', False),
            'jobNamespace': localmodel_data.get('jobNamespace', 'kserve-localmodel-jobs'),
            'jobTTLSecondsAfterFinished': localmodel_data.get('jobTTLSecondsAfterFinished', 3600),
            'disableVolumeManagement': localmodel_data.get('disableVolumeManagement', False),
            'securityContext': {
                'fsGroup': localmodel_data.get('fsGroup', 1000)
            },
            'agent': {
                'reconcilationFrequencyInSecs': localmodel_data.get('reconcilationFrequencyInSecs', 60)
            }
        }

    return values


def find_kserve_value_references(template_path):
    """Find all .Values.kserve.* references in the template."""
    with open(template_path, 'r') as f:
        content = f.read()

    # Find all .Values.kserve.* references
    pattern = r'\.Values\.kserve\.([\w\.]+)'
    matches = re.findall(pattern, content)

    # Build a set of unique paths
    paths = set()
    for match in matches:
        # Split by dots to get the path components
        parts = match.split('.')
        # Build paths for nested structures
        for i in range(1, len(parts) + 1):
            paths.add('.'.join(parts[:i]))

    return sorted(paths)


def add_values_to_yaml(values_yaml_path, extracted_values):
    """Add extracted values to values.yaml if they don't already exist."""
    # Read existing values.yaml
    with open(values_yaml_path, 'r') as f:
        content = f.read()

    # Check if kserve section already exists
    if 'kserve:' in content:
        # Values already exist, don't overwrite
        return

    # Append new values
    with open(values_yaml_path, 'a') as f:
        f.write('\n# Local model configuration\n')
        f.write('kserve:\n')

        # Add agent values
        if 'agent' in extracted_values:
            f.write('  agent:\n')
            f.write(f"    image: {extracted_values['agent']['image']}\n")
            f.write(f"    tag: {extracted_values['agent']['tag']}\n")

        # Add router values
        if 'router' in extracted_values:
            f.write('  router:\n')
            f.write(f"    image: {extracted_values['router']['image']}\n")
            f.write(f"    tag: {extracted_values['router']['tag']}\n")
            f.write(f"    imagePullPolicy: {extracted_values['router']['imagePullPolicy']}\n")
            # Format imagePullSecrets as YAML list
            secrets = extracted_values['router']['imagePullSecrets']
            if isinstance(secrets, list):
                if secrets:
                    f.write(f"    imagePullSecrets: {yaml.dump(secrets, default_flow_style=True).strip()}\n")
                else:
                    f.write('    imagePullSecrets: []\n')
            else:
                f.write(f"    imagePullSecrets: {secrets}\n")

        # Add localmodel values
        if 'localmodel' in extracted_values:
            f.write('  localmodel:\n')
            lm = extracted_values['localmodel']
            f.write(f"    enabled: {str(lm['enabled']).lower()}\n")
            f.write(f"    jobNamespace: \"{lm['jobNamespace']}\"\n")
            f.write(f"    jobTTLSecondsAfterFinished: {lm['jobTTLSecondsAfterFinished']}\n")
            f.write(f"    disableVolumeManagement: {str(lm['disableVolumeManagement']).lower()}\n")
            f.write('    securityContext:\n')
            f.write(f"      fsGroup: {lm['securityContext']['fsGroup']}\n")
            f.write('    agent:\n')
            f.write(f"      reconcilationFrequencyInSecs: {lm['agent']['reconcilationFrequencyInSecs']}\n")


def main():
    if len(sys.argv) != 4:
        print(f'Usage: {sys.argv[0]} <source_configmap> <template_file> <values_yaml>', file=sys.stderr)
        sys.exit(1)

    source_configmap = Path(sys.argv[1])
    template_file = Path(sys.argv[2])
    values_yaml = Path(sys.argv[3])

    if not source_configmap.exists():
        print(f'Error: Source ConfigMap not found: {source_configmap}', file=sys.stderr)
        sys.exit(1)

    if not template_file.exists():
        print(f'Error: Template file not found: {template_file}', file=sys.stderr)
        sys.exit(1)

    if not values_yaml.exists():
        print(f'Error: Values YAML not found: {values_yaml}', file=sys.stderr)
        sys.exit(1)

    # Extract values from source ConfigMap
    extracted_values = parse_configmap_source(source_configmap)

    # Add values to values.yaml
    add_values_to_yaml(values_yaml, extracted_values)

    print(f'✅ Extracted and added kserve.* values to {values_yaml}')


if __name__ == '__main__':
    main()
