#!/usr/bin/env python3
"""
Extract kserve.servingruntime.* values from kustomization.yaml and source files.

This script:
1. Parses config/runtimes/kustomization.yaml to extract image names/tags
2. Parses source config/runtimes/*.yaml files to understand structure
3. Generates proper kserve.servingruntime.* values
4. Adds them to values.yaml if missing
"""

import sys
import yaml
from pathlib import Path

# Try to use ruamel.yaml for better YAML preservation, fallback to standard yaml
try:
    from ruamel.yaml import YAML
    RUAMEL_AVAILABLE = True
except ImportError:
    RUAMEL_AVAILABLE = False


def parse_kustomization(kustomization_path):
    """Parse kustomization.yaml to extract image transformations."""
    with open(kustomization_path, 'r', encoding='utf-8') as f:
        kustomization = yaml.safe_load(f)

    images = {}
    for img in kustomization.get('images', []):
        name = img.get('name', '')
        new_name = img.get('newName', '')
        new_tag = img.get('newTag', 'latest')
        images[name] = {
            'image': new_name,
            'tag': new_tag
        }

    return images


def parse_runtime_file(runtime_path):
    """Parse a ClusterServingRuntime YAML file to extract metadata."""
    with open(runtime_path, 'r', encoding='utf-8') as f:
        content = f.read()
        runtime = yaml.safe_load(content)

    metadata = {
        'name': runtime['metadata']['name'],
        'has_modelClass': '{{.Labels.modelClass}}' in content,
        'has_serviceEnvelope': '{{.Labels.serviceEnvelope}}' in content,
        'has_modelName': '{{.Name}}' in content,
    }

    # Check for special fields
    if 'huggingfaceserver' in metadata['name']:
        metadata['has_lmcache'] = 'lmcacheUseExperimental' in content.lower()
        metadata['has_devShm'] = 'devshm' in content.lower() or 'devShm' in content
        metadata['has_hostIPC'] = 'hostIPC' in content

    return metadata


def map_image_name_to_runtime(image_name, runtime_name):
    """Map kustomization image name to runtime name."""
    runtime_key = runtime_name.replace('kserve-', '')

    # Direct matches
    if image_name == runtime_key:
        return True

    # Special cases
    mappings = {
        'mlserver': 'kserve-mlserver',
        'tensorflow-serving': 'kserve-tensorflow-serving',
        'huggingfaceserver': 'kserve-huggingfaceserver',
        'huggingfaceserver-gpu': 'kserve-huggingfaceserver-multinode',
    }

    if image_name in mappings:
        return mappings[image_name] == runtime_name

    # Try with kserve- prefix
    if f'kserve-{image_name}' == runtime_name:
        return True

    return False


def generate_runtime_values(kustomization_path, runtimes_dir, values_yaml_path):
    """Generate kserve.servingruntime.* values and add to values.yaml."""
    # Parse kustomization.yaml
    images = parse_kustomization(kustomization_path)

    # Parse all runtime files
    runtime_files = list(Path(runtimes_dir).glob('kserve-*.yaml'))
    runtimes = {}
    for runtime_file in runtime_files:
        metadata = parse_runtime_file(runtime_file)
        runtimes[metadata['name']] = metadata

    # Map images to runtimes
    runtime_values = {}
    runtime_values['modelNamePlaceholder'] = '{{.Name}}'

    # Map each runtime to its image
    for runtime_name, metadata in runtimes.items():
        runtime_key = runtime_name.replace('kserve-', '')

        # Special key mappings (for helmify's value structure)
        key_mappings = {
            'tensorflow-serving': 'tensorflow',
            'huggingfaceserver-multinode': 'huggingfaceserver_multinode',
        }
        helmify_key = key_mappings.get(runtime_key, runtime_key)

        # Find matching image using mapping function
        image_info = None
        for img_name, img_data in images.items():
            if map_image_name_to_runtime(img_name, runtime_name):
                image_info = img_data
                break

        # If no match found, try direct lookup
        if not image_info:
            # Try direct name match
            if runtime_key in images:
                image_info = images[runtime_key]
            elif f'kserve-{runtime_key}' in images:
                image_info = images[f'kserve-{runtime_key}']

        if not image_info:
            print(f"Warning: No image found for {runtime_name} (key: {runtime_key})", file=sys.stderr)
            continue

        runtime_config = {
            'image': image_info['image'],
            'tag': image_info['tag'],
            'imagePullPolicy': 'IfNotPresent'
        }

        # Add special fields based on metadata
        if metadata['has_modelClass']:
            runtime_config['modelClassPlaceholder'] = '{{.Labels.modelClass}}'

        if metadata['has_serviceEnvelope']:
            runtime_config['serviceEnvelopePlaceholder'] = 'kservev2'

        # Handle huggingfaceserver special fields
        if runtime_name == 'kserve-huggingfaceserver':
            # Check if lmcache is present in the file
            if metadata.get('has_lmcache'):
                runtime_config['lmcacheUseExperimental'] = 'false'
            if metadata.get('has_devShm'):
                runtime_config['devShm'] = {'enabled': False}
            if metadata.get('has_hostIPC'):
                runtime_config['hostIPC'] = {'enabled': False}

        # Handle huggingfaceserver_multinode
        if runtime_name == 'kserve-huggingfaceserver-multinode':
            runtime_config['shm'] = {'enabled': False}

        # Use helmify_key for the values structure
        runtime_values[helmify_key] = runtime_config

    # Read existing values.yaml preserving order if ruamel is available
    if RUAMEL_AVAILABLE:
        yaml_loader = YAML()
        yaml_loader.preserve_quotes = True
        yaml_loader.width = 4096
        with open(values_yaml_path, 'r', encoding='utf-8') as f:
            values = yaml_loader.load(f) or {}
    else:
        with open(values_yaml_path, 'r', encoding='utf-8') as f:
            values = yaml.safe_load(f) or {}

    # Ensure kserve.servingruntime structure exists
    if 'kserve' not in values:
        values['kserve'] = {}
    if 'servingruntime' not in values['kserve']:
        values['kserve']['servingruntime'] = {}

    # Remove old keys that were replaced by key mappings
    keys_to_remove = ['tensorflow-serving', 'huggingfaceserver-multinode']
    for key in keys_to_remove:
        if key in values['kserve']['servingruntime']:
            del values['kserve']['servingruntime'][key]

    # Merge runtime values into existing structure, preserving order
    existing_servingruntime = values['kserve']['servingruntime']

    # Update existing entries with new fields
    for key, value in runtime_values.items():
        if key in existing_servingruntime:
            # Update existing entry with any new fields
            if isinstance(value, dict) and isinstance(existing_servingruntime[key], dict):
                # Merge nested dicts
                for subkey, subvalue in value.items():
                    if subkey not in existing_servingruntime[key]:
                        existing_servingruntime[key][subkey] = subvalue

    # Add new keys, preserving existing order
    if RUAMEL_AVAILABLE:
        # With ruamel.yaml, order is automatically preserved when we add new keys
        for key, value in runtime_values.items():
            if key not in existing_servingruntime:
                existing_servingruntime[key] = value
    else:
        # For standard yaml, preserve order by rebuilding dict with existing keys first
        # Python 3.7+ dicts preserve insertion order
        ordered_dict = {}
        # First add all existing keys in their current order
        for key in existing_servingruntime.keys():
            ordered_dict[key] = existing_servingruntime[key]
        # Then add new keys
        for key, value in runtime_values.items():
            if key not in ordered_dict:
                ordered_dict[key] = value
        values['kserve']['servingruntime'] = ordered_dict

    # Write back to values.yaml preserving order and formatting
    if RUAMEL_AVAILABLE:
        yaml_writer = YAML()
        yaml_writer.preserve_quotes = True
        yaml_writer.width = 4096
        yaml_writer.default_flow_style = False
        with open(values_yaml_path, 'w', encoding='utf-8') as f:
            yaml_writer.dump(values, f)
    else:
        with open(values_yaml_path, 'w', encoding='utf-8') as f:
            yaml.dump(values, f, default_flow_style=False, sort_keys=False, allow_unicode=True)

    print(f"✅ Extracted and added kserve.servingruntime.* values to {values_yaml_path}")


def main():
    if len(sys.argv) != 4:
        print(f'Usage: {sys.argv[0]} <kustomization.yaml> <runtimes_dir> <values.yaml>', file=sys.stderr)
        sys.exit(1)

    kustomization_path = Path(sys.argv[1])
    runtimes_dir = Path(sys.argv[2])
    values_yaml_path = Path(sys.argv[3])

    if not kustomization_path.exists():
        print(f'Error: Kustomization file not found: {kustomization_path}', file=sys.stderr)
        sys.exit(1)

    if not runtimes_dir.exists():
        print(f'Error: Runtimes directory not found: {runtimes_dir}', file=sys.stderr)
        sys.exit(1)

    if not values_yaml_path.exists():
        print(f'Error: Values YAML not found: {values_yaml_path}', file=sys.stderr)
        sys.exit(1)

    generate_runtime_values(kustomization_path, runtimes_dir, values_yaml_path)


if __name__ == '__main__':
    main()
