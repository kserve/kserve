#!/usr/bin/env python3
"""Fix malformed Certificate dnsNames in generated Helm charts."""

import re
import os


def fix_certificate_file(filepath, service_name):
    """Fix malformed dnsNames in a Certificate YAML file."""
    if not os.path.exists(filepath):
        return

    with open(filepath, 'r') as f:
        content = f.read()

    # Pattern 1: kserve-webhook-server-service (has namespace before -webhook)
    pattern1 = r"- '{{ include \"{{ .Release.Namespace }}-chart.fullname\" . }}-{{ .Release.Namespace\s+}}-webhook-server-service.{{ .Release.Namespace }}.svc'"

    # Pattern 2: llmisvc-webhook-server-service (no namespace before -webhook)
    pattern2 = r"- '{{ include \"{{ .Release.Namespace }}-chart.fullname\" . }}-llmisvc-webhook-server-service.{{\s+.Release.Namespace }}.svc'"

    # Use fullname template for service name to match actual deployed service
    replacement = f'- {{{{ include "kserve-resources.fullname" . }}}}-{service_name}.{{{{ .Release.Namespace }}}}.svc'

    content = re.sub(pattern1, replacement, content)
    content = re.sub(pattern2, replacement, content)

    # Also fix commonName to match the fullname pattern
    # Pattern matches hardcoded service names like "kserve-webhook-server-service.kserve.svc"
    commonname_pattern = rf"commonName: {service_name}\.\w+\.svc"
    commonname_replacement = f"commonName: {{{{ include \"kserve-resources.fullname\" . }}}}-{service_name}.{{{{ .Release.Namespace }}}}.svc"
    content = re.sub(commonname_pattern, commonname_replacement, content)

    with open(filepath, 'w') as f:
        f.write(content)

    print(f"Fixed {filepath}")


if __name__ == '__main__':
    # Fix KServe controller certificate
    fix_certificate_file(
        'charts/kserve-resources/templates/serving-cert.yaml',
        'kserve-webhook-server-service'
    )

    # Fix LLMISvc controller certificate
    fix_certificate_file(
        'charts/kserve-resources/templates/llmisvc-serving-cert.yaml',
        'llmisvc-webhook-server-service'
    )
