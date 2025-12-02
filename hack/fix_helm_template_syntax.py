#!/usr/bin/env python3
"""
Fix Helm template syntax issues in generated Helm charts.

This script fixes common issues with helmify output:
1. Converts {{ if to {{- if (proper whitespace trimming)
2. Converts {{ else to {{- else
3. Converts {{ end to {{- end
4. Converts {{ range to {{- range
"""
import sys
import re
from pathlib import Path


def fix_template_syntax(content):
    """Fix Helm template syntax issues."""
    # Fix {{ if to {{- if (with proper spacing)
    content = re.sub(r'\{\{\s+if\s+', '{{- if ', content)

    # Fix {{ else to {{- else
    content = re.sub(r'\{\{\s+else\s*\}\}', '{{- else }}', content)

    # Fix {{ end to {{- end
    content = re.sub(r'\{\{\s+end\s*\}\}', '{{- end }}', content)

    # Fix {{ range to {{- range
    content = re.sub(r'\{\{\s+range\s+', '{{- range ', content)

    return content


def fix_file(file_path):
    """Fix template syntax in a single file."""
    with open(file_path, 'r') as f:
        content = f.read()

    fixed_content = fix_template_syntax(content)

    if content != fixed_content:
        with open(file_path, 'w') as f:
            f.write(fixed_content)
        return True
    return False


def main():
    if len(sys.argv) < 2:
        print(f'Usage: {sys.argv[0]} <template_file_or_directory> [<template_file_or_directory> ...]', file=sys.stderr)
        sys.exit(1)

    files_modified = 0

    for arg in sys.argv[1:]:
        path = Path(arg)

        if path.is_file():
            if path.suffix in ['.yaml', '.yml', '.tpl']:
                if fix_file(path):
                    files_modified += 1
                    print(f'Fixed: {path}')
        elif path.is_dir():
            # Process all YAML files in directory
            for file_path in path.rglob('*.yaml'):
                if fix_file(file_path):
                    files_modified += 1
                    print(f'Fixed: {file_path}')
            for file_path in path.rglob('*.yml'):
                if fix_file(file_path):
                    files_modified += 1
                    print(f'Fixed: {file_path}')
            for file_path in path.rglob('*.tpl'):
                if fix_file(file_path):
                    files_modified += 1
                    print(f'Fixed: {file_path}')
        else:
            print(f'Warning: Path not found: {path}', file=sys.stderr)

    if files_modified > 0:
        print(f'✅ Fixed template syntax in {files_modified} file(s)')
    else:
        print('No files needed fixing')


if __name__ == '__main__':
    main()
