#!/usr/bin/env python3
"""
Script to escape embedded Go template syntax in Helm chart templates.

This script is specifically designed for KServe Helm charts that contain ConfigMaps
with embedded Go template syntax meant for the KServe runtime controller, not Helm.

Problem:
--------
KServe ConfigMaps contain templates like:
    {{ ChildName .ObjectMeta.Name `-suffix` }}
    {{ .Spec.Model.Name }}
    {{- if .Spec.Parallelism.Expert -}}...{{- end }}

These are meant to be evaluated at runtime by the KServe controller, NOT by Helm.
When Helmify converts these to Helm charts, it preserves them as-is, causing Helm
template errors like "function ChildName not defined" or "nil pointer evaluating
interface {}.Model".

Solution:
---------
This script escapes these expressions by converting them to:
    {{ "{{" }} ChildName .ObjectMeta.Name `-suffix` {{ "}}" }}
    {{ "{{" }} .Spec.Model.Name {{ "}}" }}
    {{ "{{" }} - if .Spec.Parallelism.Expert - {{ "}}" }}...{{ "{{" }} - end {{ "}}" }}

This tells Helm to output the literal {{ and }} characters without evaluating
what's inside, allowing the KServe controller to process them later.

Usage:
------
# Dry run to see what would be changed
./escape_helm_templates.py --dry-run /path/to/chart/templates/

# Apply changes to all ConfigMap files
./escape_helm_templates.py /path/to/chart/templates/config-*.yaml

# Process entire directory
./escape_helm_templates.py /path/to/chart/templates/

Examples:
---------
# After running Helmify
cat /tmp/llmisvc.yaml | helmify /tmp/llmisvc-helmify-chart

# Fix the embedded templates
./escape_helm_templates.py /tmp/llmisvc-helmify-chart/templates/kserve-config-*.yaml

# Verify it works
helm template test /tmp/llmisvc-helmify-chart --dry-run

Author: Generated for KServe Helm chart conversion
"""

import re
import sys
from pathlib import Path


def escape_embedded_templates(content: str) -> str:
    """
    Escape embedded Go template syntax in YAML content.

    Args:
        content: The YAML content with embedded templates

    Returns:
        Content with escaped templates
    """
    # Strategy: Look for patterns that contain KServe-specific references
    # These are template expressions that reference KServe CRD fields, not Helm values

    # Patterns that indicate KServe runtime templates (not Helm templates):
    # - .Spec.* (KServe CRD spec fields)
    # - .ObjectMeta.* (Kubernetes object metadata)
    # - .Status.* (KServe CRD status fields)
    # - .GlobalConfig.* (KServe global configuration)
    # - ChildName function (KServe-specific helper)

    kserve_patterns = [
        # Match template expressions with KServe CRD references
        r'\{\{-?\s*\.Spec\.[^}]+\}\}',                    # {{ .Spec.Model.Name }}
        r'\{\{-?\s*\.ObjectMeta\.[^}]+\}\}',              # {{ .ObjectMeta.Name }}
        r'\{\{-?\s*\.Status\.[^}]+\}\}',                  # {{ .Status.* }}
        r'\{\{-?\s*\.GlobalConfig\.[^}]+\}\}',            # {{ .GlobalConfig.* }}
        r'\{\{-?\s*if\s+\.Spec\.[^}]+\}\}',               # {{- if .Spec.* -}}
        r'\{\{-?\s*if\s+\.GlobalConfig\.[^}]+\}\}',       # {{- if .GlobalConfig.* -}}
        r'\{\{-?\s*end\s*-?\}\}',                          # {{- end }} or {{ end }}
        r'\{\{-?\s*else\s*-?\}\}',                         # {{- else }} or {{ else }}
        r'\{\{-?\s*or\s+\.Spec\.[^}]+\}\}',               # {{ or .Spec.* default }}
        r'\{\{\s*ChildName\s+[^}]+\}\}',                  # {{ ChildName ... }}
    ]

    def replace_match(match):
        # Get the matched text
        matched = match.group(0)
        # Check if already escaped (contains {{ "{{" }})
        if '"{{" }}' in matched or '{{ "}}" }}' in matched:
            return matched
        # Get the inner content without the outer {{ }}
        inner = matched[2:-2].strip()
        # Escape any backticks in the inner content
        inner = inner.replace('`', '{{ "`" }}')
        # Return escaped version
        return '{{ "{{" }} ' + inner + ' {{ "}}" }}'

    for pattern in kserve_patterns:
        content = re.sub(pattern, replace_match, content, flags=re.DOTALL)

    return content


def process_file(file_path: Path, dry_run: bool = False) -> bool:
    """
    Process a single file and escape embedded templates.

    Args:
        file_path: Path to the file to process
        dry_run: If True, only show what would be changed

    Returns:
        True if file was modified (or would be modified in dry_run)
    """
    try:
        with open(file_path, 'r') as f:
            original_content = f.read()

        escaped_content = escape_embedded_templates(original_content)

        if original_content != escaped_content:
            if dry_run:
                print(f"Would modify: {file_path}")
                # Show diff preview
                original_lines = original_content.splitlines()
                escaped_lines = escaped_content.splitlines()
                for i, (orig, esc) in enumerate(zip(original_lines, escaped_lines), 1):
                    if orig != esc:
                        print(f"  Line {i}:")
                        print(f"    - {orig}")
                        print(f"    + {esc}")
            else:
                with open(file_path, 'w') as f:
                    f.write(escaped_content)
                print(f"✓ Modified: {file_path}")
            return True
        else:
            if not dry_run:
                print(f"  No changes: {file_path}")
            return False

    except Exception as e:
        print(f"✗ Error processing {file_path}: {e}", file=sys.stderr)
        return False


def main():
    import argparse

    parser = argparse.ArgumentParser(
        description='Escape embedded Go template syntax in Helm charts',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Dry run to see what would be changed
  %(prog)s --dry-run /tmp/llmisvc-helmify-chart/templates/

  # Apply changes to all ConfigMap files
  %(prog)s /tmp/llmisvc-helmify-chart/templates/kserve-config-*.yaml

  # Process entire directory
  %(prog)s /tmp/llmisvc-helmify-chart/templates/
        """
    )

    parser.add_argument(
        'paths',
        nargs='+',
        help='Files or directories to process'
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Show what would be changed without modifying files'
    )
    parser.add_argument(
        '--pattern',
        default='*.yaml',
        help='File pattern to match when processing directories (default: *.yaml)'
    )

    args = parser.parse_args()

    files_to_process = []

    # Collect all files to process
    for path_str in args.paths:
        path = Path(path_str)
        if path.is_file():
            files_to_process.append(path)
        elif path.is_dir():
            files_to_process.extend(path.glob(args.pattern))
        else:
            print(f"Warning: {path} is not a file or directory", file=sys.stderr)

    if not files_to_process:
        print("No files to process", file=sys.stderr)
        return 1

    print(f"Processing {len(files_to_process)} file(s)...")
    if args.dry_run:
        print("DRY RUN - No files will be modified\n")

    modified_count = 0
    for file_path in sorted(files_to_process):
        if process_file(file_path, args.dry_run):
            modified_count += 1

    print(f"\n{'Would modify' if args.dry_run else 'Modified'} {modified_count} file(s)")

    return 0


if __name__ == '__main__':
    sys.exit(main())
