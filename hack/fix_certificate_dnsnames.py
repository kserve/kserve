#!/usr/bin/env python3
"""Fix malformed Certificate dnsNames in generated Helm charts."""

import re
import os


def fix_certificate_file(filepath, service_name, chart_name="kserve-chart"):
    """Fix malformed dnsNames in a Certificate YAML file."""
    if not os.path.exists(filepath):
        return

    with open(filepath, 'r') as f:
        content = f.read()

    # Fix malformed include pattern first (if it exists)
    # Pattern: {{ include "{{ .Release.Namespace }}-chart.fullname" . }}
    # Should be: {{ include "chart.fullname" . }}
    malformed_include_pattern = r'\{\{\s*include\s+"\{\{\s*\.Release\.Namespace\s*\}\}-chart\.fullname"\s+\.\s*\}\}'
    fixed_include = f'{{{{ include "{chart_name}.fullname" . }}}}'
    content = re.sub(malformed_include_pattern, fixed_include, content)

    # Fix dnsNames: Remove {{ .Release.Namespace }}- prefix that helmify incorrectly adds to service name
    # Pattern: {{ include "chart.fullname" . }}-{{ .Release.Namespace }}-service-name.{{ .Release.Namespace }}.svc
    # Should be: {{ include "chart.fullname" . }}-service-name.{{ .Release.Namespace }}.svc
    # Handle both single-line and multi-line dnsNames entries
    chart_name_escaped = chart_name.replace('.', r'\.')
    # Pattern: {{ include "chart.fullname" . }}-{{ .Release.Namespace }}-service-name
    dnsnames_pattern = (
        r'(\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*)'
        r'\{\{\s*\.Release\.Namespace\s*\}\}\s*-\s*'
        r'(' + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc)'
    )
    dnsnames_replacement = r'\1\2'
    content = re.sub(dnsnames_pattern, dnsnames_replacement, content, flags=re.MULTILINE | re.DOTALL)

    # Also handle the case where the pattern spans multiple lines
    # Pattern with line breaks: {{ include ... }}-{{ .Release.Namespace\n    }}-service-name
    dnsnames_multiline_pattern = (
        r'(\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*)'
        r'\{\{\s*\.Release\.Namespace\s*\n\s*\}\}\s*-\s*'
        r'(' + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc)'
    )
    content = re.sub(dnsnames_multiline_pattern, dnsnames_replacement, content, flags=re.MULTILINE | re.DOTALL)

    # Fix issuerRef name: should be just "selfsigned-issuer", not templated
    chart_name_escaped = chart_name.replace('.', r'\.')
    issuer_pattern = r"name:\s*'?\{\{\s*include\s+\"" + chart_name_escaped + \
        r"\.fullname\"\s+\.\s*\}\}\s*-\s*selfsigned-issuer'?"
    issuer_replacement = "name: selfsigned-issuer"
    content = re.sub(issuer_pattern, issuer_replacement, content)

    # Also fix commonName to match the fullname pattern (if hardcoded)
    # Pattern matches hardcoded service names like "kserve-webhook-server-service.kserve.svc"
    commonname_pattern = rf"commonName:\s*{service_name}\.\w+\.svc"
    commonname_replacement = f"commonName: {{{{ include \"{chart_name}.fullname\" . }}}}-{service_name}.{{{{ .Release.Namespace }}}}.svc"
    content = re.sub(commonname_pattern, commonname_replacement, content)

    with open(filepath, 'w') as f:
        f.write(content)

    print(f"Fixed {filepath}")


if __name__ == '__main__':
    # Fix KServe controller certificate in kserve-resources
    fix_certificate_file(
        'charts/kserve-resources/templates/serving-cert.yaml',
        'webhook-server-service',
        'kserve-resources'
    )

    # Fix LLMISvc controller certificate in kserve-resources
    fix_certificate_file(
        'charts/kserve-resources/templates/llmisvc-serving-cert.yaml',
        'llmisvc-webhook-server-service',
        'kserve-resources'
    )
