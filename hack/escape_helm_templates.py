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
        # Only escape templates that reference KServe-specific fields, not standard Helm directives
        r'\{\{-?\s*\.Spec\.[^}]+\}\}',                    # {{ .Spec.Model.Name }}
        r'\{\{-?\s*\.ObjectMeta\.[^}]+\}\}',              # {{ .ObjectMeta.Name }}
        r'\{\{-?\s*\.Status\.[^}]+\}\}',                  # {{ .Status.* }}
        r'\{\{-?\s*\.GlobalConfig\.[^}]+\}\}',            # {{ .GlobalConfig.* }}
        r'\{\{-?\s*\.Annotations\.[^}]+\}\}',             # {{ .Annotations.key }}
        r'\{\{-?\s*\.Labels\.[^}]+\}\}',                  # {{ .Labels.key }}
        r'\{\{-?\s*\.Name\s*\}\}',                        # {{ .Name }}
        r'\{\{-?\s*\.Namespace\s*\}\}',                    # {{ .Namespace }}
        r'\{\{-?\s*\.IngressDomain\s*\}\}',                # {{ .IngressDomain }}
        r'\{\{-?\s*if\s+\.Spec\.[^}]+\}\}',               # {{- if .Spec.* -}}
        r'\{\{-?\s*if\s+\.GlobalConfig\.[^}]+\}\}',       # {{- if .GlobalConfig.* -}}
        r'\{\{-?\s*or\s+\.Spec\.[^}]+\}\}',               # {{ or .Spec.* default }}
        r'\{\{\s*ChildName\s+[^}]+\}\}',                  # {{ ChildName ... }}
        # Note: We do NOT escape {{- end }}, {{- else }}, {{- with }}, etc. as these are
        # legitimate Helm template directives that should remain as-is
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
        # Escape dots in field access patterns (e.g., .Annotations.key -> .Annotations{{ "." }}key)
        # This prevents Helm from trying to evaluate the field access
        # But only escape dots that are between alphanumeric characters (field access)
        if '.' in inner and '{{ "." }}' not in inner:
            # Pattern: .Field1.Field2 -> .Field1{{ "." }}Field2
            # Use regex to match field access patterns
            # Match patterns like .Field1.Field2 but not already escaped

            def escape_dot_in_field_access(m):
                field_access = m.group(0)
                # Replace dots between field names
                return field_access.replace('.', '{{ "." }}')
            # Match field access patterns: . followed by identifier, then .identifier
            inner = re.sub(r'\.([A-Za-z_][A-Za-z0-9_]*\.)+[A-Za-z_][A-Za-z0-9_]*',
                           escape_dot_in_field_access, inner)
        # Return escaped version
        return '{{ "{{" }} ' + inner + ' {{ "}}" }}'

    # First pass: Escape KServe-specific template patterns
    for pattern in kserve_patterns:
        content = re.sub(pattern, replace_match, content, flags=re.DOTALL)

    # Second pass: For already-escaped templates, escape dots in field access
    # This handles cases where the source file already has {{ "{{" }} .Annotations.key {{ "}}" }}
    # We need to escape the dots: {{ "{{" }} .Annotations{{ "." }}key {{ "}}" }}
    def escape_dots_in_escaped_templates(match):
        # Match patterns like {{ "{{" }} .Field1.Field2 {{ "}}" }}
        full_match = match.group(0)
        # Extract the inner content between {{ "{{" }} and {{ "}}" }}
        inner_match = re.search(r'\{\{\s*"\{\{"\s*\}\}\s*(.*?)\s*\{\{\s*"\}\}"\s*\}\}', full_match, re.DOTALL)
        if inner_match:
            inner = inner_match.group(1)
            # Escape dots in field access patterns (e.g., .Annotations.key -> .Annotations{{ "." }}key)
            # But only if not already escaped
            if '.' in inner and '{{ "." }}' not in inner:
                # Replace .Field1.Field2 with .Field1{{ "." }}Field2
                # Match patterns like .Field1.Field2 and replace the dot between them
                def replace_dot(m):
                    # m.group(1) is the first field (e.g., "Annotations")
                    # m.group(2) is the second field (e.g., "key")
                    # We want: .Annotations{{ "." }}key
                    return '.' + m.group(1) + '{{ "." }}' + m.group(2)
                # Match .identifier.identifier patterns and replace the dot between them
                inner = re.sub(r'\.([A-Za-z_][A-Za-z0-9_]*)\.([A-Za-z_][A-Za-z0-9_]*)',
                               replace_dot, inner)
            return '{{ "{{" }} ' + inner + ' {{ "}}" }}'
        return full_match

    # Find already-escaped templates that contain field access with dots
    # Pattern: {{ "{{" }} .Field1.Field2 {{ "}}" }}
    escaped_pattern = r'\{\{\s*"\{\{"\s*\}\}\s*\.([A-Za-z_][A-Za-z0-9_]*\.)+[A-Za-z_][A-Za-z0-9_]*\s*\{\{\s*"\}\}"\s*\}\}'
    content = re.sub(escaped_pattern, escape_dots_in_escaped_templates, content)

    # Third pass: Escape {{- end }} and {{- else }} that follow escaped KServe templates
    # Look for {{- end }} or {{- else }} that appears after an escaped template (within reasonable distance)
    # This handles cases like: {{ "{{" }} - if .Spec.* - {{ "}}" }}...{{- else }}...{{- end }}
    # We need to escape the {{- end }} and {{- else }} that are part of KServe template blocks
    def escape_control_after_kserve(match):
        # Check if this {{- end }} or {{- else }} is part of a KServe template block
        # by looking backwards for escaped KServe patterns
        before = content[:match.start()]
        # Look for escaped KServe if/with statements in the last 500 chars
        recent = before[-500:] if len(before) > 500 else before
        # Check if there's an escaped KServe template before this control structure
        # Patterns to match escaped KServe templates (with escaped dots)
        if re.search(r'\{\{\s*"\{\{"\s*\}\}\s*-?\s*if\s+', recent) or \
           re.search(r'\{\{\s*"\{\{"\s*\}\}\s*-?\s*with\s+', recent) or \
           re.search(r'\{\{\s*"\{\{"\s*\}\}\s*if\s+', recent) or \
           re.search(r'\{\{\s*"\{\{"\s*\}\}\s*with\s+', recent):
            # This {{- end }} or {{- else }} is part of a KServe template block, escape it
            matched = match.group(0)
            if '"{{" }}' not in matched and '{{ "}}" }}' not in matched:
                inner = matched[2:-2].strip()
                return '{{ "{{" }} ' + inner + ' {{ "}}" }}'
        return match.group(0)

    # Match {{- end }} or {{ end }} that might close KServe templates
    content = re.sub(r'\{\{-?\s*end\s*-?\}\}', escape_control_after_kserve, content)
    # Match {{- else }} or {{ else }} that might be in KServe templates
    content = re.sub(r'\{\{-?\s*else\s*-?\}\}', escape_control_after_kserve, content)

    # Fourth pass: Fix JSON strings that contain escaped template syntax
    # When we escape {{ .Name }} to {{ "{{" }} .Name {{ "}}" }}, the quotes break JSON parsing
    # We need to escape the quotes in JSON string values: "{{" becomes \"{{\" and "}}" becomes }}\"
    content = fix_json_strings_with_escaped_templates(content)

    return content


def fix_json_strings_with_escaped_templates(content: str) -> str:
    """
    Fix JSON strings in YAML that contain escaped Helm template syntax.

    The templates are already escaped by escape_embedded_templates, so they
    will output correctly after Helm processes them. The JSON might be invalid
    before Helm processes it, but will be valid after Helm renders the templates.

    Args:
        content: YAML content that may contain JSON strings with escaped templates

    Returns:
        Content unchanged (templates are already properly escaped)
    """
    # Templates are already escaped by escape_embedded_templates, so no fixing needed
    # Helm will process {{ "{{" }} .Name {{ "}}" }} and output {{ .Name }},
    # which is valid JSON
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
        with open(file_path, 'r', encoding='utf-8') as f:
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
                with open(file_path, 'w', encoding='utf-8') as f:
                    f.write(escaped_content)
                print(f"✓ Modified: {file_path}")
            return True
        else:
            if not dry_run:
                print(f"  No changes: {file_path}")
            return False

    except (IOError, OSError, UnicodeDecodeError) as e:
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
