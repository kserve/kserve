#!/usr/bin/env python3
"""Fix malformed Certificate dnsNames in generated Helm charts."""

import re
import os


def fix_certificate_file(filepath, service_name, chart_name="kserve-chart"):
    """Fix malformed dnsNames in a Certificate YAML file."""
    if not os.path.exists(filepath):
        return

    with open(filepath, 'r') as f:
        lines = f.readlines()

    # Process line by line to handle split lines
    new_lines = []
    i = 0
    while i < len(lines):
        line = lines[i]
        # Check if this line contains the malformed include pattern
        if '{{ include "{{ .Release.Namespace }}-chart.fullname"' in line:
            # Collect the full dnsNames entry (may span multiple lines)
            dns_entry_lines = [line]
            j = i + 1
            # Continue collecting until we find the closing quote
            while j < len(lines):
                dns_entry_lines.append(lines[j])
                # Check if we have a complete entry
                full_entry = ''.join(dns_entry_lines)
                if full_entry.count("'") >= 2 and '.svc' in full_entry:
                    break
                j += 1
                if j >= len(lines):
                    break

            # Join and fix the entry
            dns_entry = ''.join(dns_entry_lines)
            # Fix the malformed include
            fixed_entry = dns_entry.replace(
                '{{ include "{{ .Release.Namespace }}-chart.fullname" . }}',
                f'{{{{ include "{chart_name}.fullname" . }}}}'
            )
            # Fix the service name format - remove extra {{ .Release.Namespace }} before service name
            fixed_entry = fixed_entry.replace(
                f'{{{{ include "{chart_name}.fullname" . }}}}-{{{{ .Release.Namespace }}}}-{service_name}',
                f'{{{{ include "{chart_name}.fullname" . }}}}-{service_name}'
            )

            # Split back into lines preserving structure
            fixed_lines = fixed_entry.split('\n')
            # If the original was on multiple lines, try to preserve that structure
            if len(dns_entry_lines) > 1:
                # Keep the first line with the dash, put the rest on continuation
                if len(fixed_lines) > 1:
                    new_lines.append(fixed_lines[0] + '\n')
                    for fl in fixed_lines[1:]:
                        if fl.strip():
                            new_lines.append('    ' + fl.strip() + '\n')
                else:
                    new_lines.append(fixed_entry)
            else:
                new_lines.append(fixed_entry)
            i = j + 1
        else:
            new_lines.append(line)
            i += 1

    content = ''.join(new_lines)

    # Also fix commonName to match the fullname pattern
    # Pattern matches hardcoded service names like "kserve-webhook-server-service.kserve.svc"
    commonname_pattern = rf"commonName: {service_name}\.\w+\.svc"
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
        'llmisvc-webhook-svc',
        'kserve-resources'
    )
