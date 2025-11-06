#!/usr/bin/env python3
"""
Fix worker.initContainers in Helm chart templates after helmify runs.

Helmify removes worker.initContainers when converting from Kustomize to Helm.
This script restores it from the Kustomize source using string manipulation
to preserve Helm template syntax.
"""

import sys
import yaml
import re
from pathlib import Path


def restore_worker_initcontainers(chart_file: Path, kustomize_file: Path) -> bool:
    """Restore worker.initContainers from Kustomize source to Helm chart."""
    if not chart_file.exists():
        print(f"Error: Chart file not found: {chart_file}")
        return False

    if not kustomize_file.exists():
        print(f"Error: Kustomize file not found: {kustomize_file}")
        return False

    # Read Helm chart as text (preserves template syntax)
    with open(chart_file, 'r', encoding='utf-8') as f:
        chart_content = f.read()

    # Check if worker.initContainers already exists - if so, we'll replace it
    worker_initcontainers_exists = False
    if 'worker:' in chart_content and 'initContainers:' in chart_content:
        # Check if it's in the worker section
        worker_match = re.search(r'worker:\s*\n\s*initContainers:', chart_content)
        if worker_match:
            worker_initcontainers_exists = True

    # Load Kustomize source (this is valid YAML, no templates)
    with open(kustomize_file, 'r', encoding='utf-8') as f:
        kustomize_docs = list(yaml.safe_load_all(f))

    # Find the config in Kustomize output
    kustomize_config = None
    for doc in kustomize_docs:
        if (doc.get('kind') == 'LLMInferenceServiceConfig' and
                doc.get('metadata', {}).get('name') == 'kserve-config-llm-decode-worker-data-parallel'):
            kustomize_config = doc
            break

    if not kustomize_config:
        print(f"Warning: Could not find config in Kustomize file: {kustomize_file}")
        return False

    # Extract worker.initContainers from Kustomize source
    worker_initcontainers = (kustomize_config
                             .get('spec', {})
                             .get('worker', {})
                             .get('initContainers'))

    if not worker_initcontainers:
        print("Warning: worker.initContainers not found in Kustomize source")
        return False

    # Convert initContainers to YAML string (indented for worker section)
    initcontainers_yaml = yaml.dump({'initContainers': worker_initcontainers},
                                    default_flow_style=False,
                                    sort_keys=False,
                                    allow_unicode=True,
                                    indent=2)
    # Remove any control characters
    initcontainers_yaml = ''.join(c for c in initcontainers_yaml if ord(c) >= 32 or c in '\n\r\t')

    # Add proper indentation (4 spaces for worker section)
    # The YAML dump creates: initContainers:\n- args:\n  - --port=8000\n  env:\n  - name: ...\n    valueFrom:\n      fieldRef:\n        fieldPath: ...
    # worker: is at 2 spaces, so initContainers: should be at 4 spaces
    # We need to add 4 spaces to every non-empty line to preserve relative indentation
    lines = initcontainers_yaml.split('\n')
    indented_lines = []
    for line in lines:
        if line.strip():  # Non-empty line - add 4 spaces
            indented_lines.append('    ' + line)
        else:  # Empty line - keep as is
            indented_lines.append(line)
    initcontainers_content = '\n'.join(indented_lines)

    # Find the worker: section and insert/replace initContainers before containers:
    if worker_initcontainers_exists:
        # Replace existing worker.initContainers section
        # Find the worker: section first, then find initContainers within it
        # Pattern: worker:\n    initContainers: ... (until we hit containers: or end of worker section)
        # We need to match from worker: initContainers: to just before containers: (or end of worker section)
        # First, find where worker: starts
        worker_start = chart_content.find('worker:')
        if worker_start == -1:
            print(f"Warning: Could not find 'worker:' section in {chart_file.name}")
            return False

        # Find the next top-level key after worker: (containers: or another key at same indentation)
        # Look for containers: at 4 spaces indentation (same as initContainers)
        worker_section_end = chart_content.find('\n    containers:', worker_start)
        if worker_section_end == -1:
            # Try to find end of worker section (next key at 2 spaces or end of file)
            worker_section_end = len(chart_content)

        # Extract worker section
        worker_section = chart_content[worker_start:worker_section_end]

        # Find initContainers: in worker section
        initcontainers_start = worker_section.find('    initContainers:')
        if initcontainers_start == -1:
            print(f"Warning: Could not find 'initContainers:' in worker section in {chart_file.name}")
            return False

        # Find where initContainers section ends (containers: or end of worker section)
        initcontainers_end = worker_section.find('\n    containers:', initcontainers_start)
        if initcontainers_end == -1:
            initcontainers_end = len(worker_section)

        # Replace the initContainers section
        new_worker_section = (worker_section[:initcontainers_start] +
                              initcontainers_content.rstrip() + '\n' +
                              worker_section[initcontainers_end:])

        # Replace the entire worker section in the chart content
        chart_content = (chart_content[:worker_start] +
                         new_worker_section +
                         chart_content[worker_start + len(worker_section):])
    else:
        # Insert initContainers before containers
        # Pattern: worker:\n    containers:
        pattern = r'(worker:\s*\n)(\s+containers:)'
        if not re.search(pattern, chart_content):
            print(f"Warning: Could not find 'worker:' section in {chart_file.name}")
            return False
        # Insert initContainers before containers
        replacement = r'\1' + initcontainers_content.rstrip() + '\n\2'
        chart_content = re.sub(pattern, replacement, chart_content)

    # Remove any control characters from the entire file
    chart_content = ''.join(c for c in chart_content if ord(c) >= 32 or c in '\n\r\t')

    # Write back to file
    with open(chart_file, 'w', encoding='utf-8') as f:
        f.write(chart_content)

    print(f"✅ Restored worker.initContainers in {chart_file.name}")
    return True


def main():
    if len(sys.argv) < 3:
        print("Usage: fix_worker_initcontainers.py <chart-file> <kustomize-file>")
        sys.exit(1)

    chart_file = Path(sys.argv[1])
    kustomize_file = Path(sys.argv[2])

    if restore_worker_initcontainers(chart_file, kustomize_file):
        sys.exit(0)
    else:
        sys.exit(1)


if __name__ == '__main__':
    main()
