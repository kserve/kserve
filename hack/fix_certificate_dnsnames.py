#!/usr/bin/env python3
"""Fix malformed Certificate dnsNames in generated Helm charts."""

import re
import os


def fix_certificate_file(filepath, service_name, chart_name="kserve-chart", remove_fullname_prefix=False):
    """Fix malformed dnsNames in a Certificate YAML file.

    Args:
        filepath: Path to the certificate YAML file
        service_name: Name of the service (e.g., 'webhook-server-service' or 'llmisvc-webhook-server-service')
        chart_name: Name of the Helm chart (e.g., 'kserve-resources')
        remove_fullname_prefix: If True, remove fullname prefix from both commonName and dnsNames (for llmisvc)
    """
    if not os.path.exists(filepath):
        return

    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Fix malformed include pattern first (if it exists)
    # Pattern: {{ include "{{ .Release.Namespace }}-chart.fullname" . }}
    # Should be: {{ include "chart.fullname" . }}
    malformed_include_pattern = r'\{\{\s*include\s+"\{\{\s*\.Release\.Namespace\s*\}\}-chart\.fullname"\s+\.\s*\}\}'
    fixed_include = f'{{{{ include "{chart_name}.fullname" . }}}}'
    content = re.sub(malformed_include_pattern, fixed_include, content)

    chart_name_escaped = chart_name.replace('.', r'\.')

    if remove_fullname_prefix:
        # For llmisvc: Remove fullname prefix from dnsNames entirely
        # Pattern: {{ include "chart.fullname" . }}-service-name.{{ .Release.Namespace }}.svc
        # Should be: service-name.{{ .Release.Namespace }}.svc
        dnsnames_with_fullname_pattern = (
            r'\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*'
            + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc'
        )
        dnsnames_without_fullname = f"{service_name}.{{{{ .Release.Namespace }}}}.svc"
        content = re.sub(dnsnames_with_fullname_pattern, dnsnames_without_fullname,
                         content, flags=re.MULTILINE | re.DOTALL)
    else:
        # For kserve: The service name is "kserve-webhook-server-service" (no fullname prefix)
        # But helmify generates dnsNames with fullname prefix and namespace prefix:
        # Pattern: {{ include "chart.fullname" . }}-{{ .Release.Namespace }}-webhook-server-service.{{ .Release.Namespace }}.svc
        # Should be: kserve-webhook-server-service.{{ .Release.Namespace }}.svc
        if service_name == "webhook-server-service":
            # Remove both fullname prefix and namespace prefix from dnsNames to match actual service name "kserve-webhook-server-service"
            # Pattern 1: {{ include "chart.fullname" . }}-{{ .Release.Namespace }}-webhook-server-service.{{ .Release.Namespace }}.svc
            dnsnames_with_both_prefixes_pattern = (
                r'\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*'
                r'\{\{\s*\.Release\.Namespace\s*\}\}\s*-\s*'
                + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc'
            )
            dnsnames_without_prefixes = f"kserve-{service_name}.{{{{ .Release.Namespace }}}}.svc"
            content = re.sub(dnsnames_with_both_prefixes_pattern, dnsnames_without_prefixes,
                             content, flags=re.MULTILINE | re.DOTALL)

            # Pattern 2: {{ include "chart.fullname" . }}-webhook-server-service.{{ .Release.Namespace }}.svc (if namespace prefix was already removed)
            dnsnames_with_fullname_pattern = (
                r'\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*'
                + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc'
            )
            content = re.sub(dnsnames_with_fullname_pattern, dnsnames_without_prefixes,
                             content, flags=re.MULTILINE | re.DOTALL)
        else:
            # For other services: Remove {{ .Release.Namespace }}- prefix that helmify incorrectly adds to service name
            # Pattern: {{ include "chart.fullname" . }}-{{ .Release.Namespace }}-service-name.{{ .Release.Namespace }}.svc
            # Should be: {{ include "chart.fullname" . }}-service-name.{{ .Release.Namespace }}.svc
            dnsnames_pattern = (
                r'(\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*)'
                r'\{\{\s*\.Release\.Namespace\s*\}\}\s*-\s*'
                + r'(' + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc)'
            )
            dnsnames_replacement = r'\1\2'
            content = re.sub(dnsnames_pattern, dnsnames_replacement, content, flags=re.MULTILINE | re.DOTALL)

            # Also handle the case where the pattern spans multiple lines
            dnsnames_multiline_pattern = (
                r'(\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*)'
                r'\{\{\s*\.Release\.Namespace\s*\n\s*\}\}\s*-\s*'
                + r'(' + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc)'
            )
            content = re.sub(dnsnames_multiline_pattern, dnsnames_replacement, content, flags=re.MULTILINE | re.DOTALL)

    # Fix issuerRef name
    chart_name_escaped = chart_name.replace('.', r'\.')

    # For kserve-resources: All certificates should use fullname template to match the single Issuer
    # For kserve-llmisvc-resources: issuerRef should use fullname template
    if chart_name == "kserve-resources":
        # All certificates in kserve-resources should reference the same Issuer with fullname template
        # Fix issuerRef to use fullname template (if it's just "selfsigned-issuer")
        issuer_pattern = r"name:\s*selfsigned-issuer"
        issuer_replacement = f'name: {{{{ include "{chart_name}.fullname" . }}}}-selfsigned-issuer'
        content = re.sub(issuer_pattern, issuer_replacement, content)
    elif chart_name == "kserve-llmisvc-resources":
        # Fix issuerRef to use fullname template (if it's just "selfsigned-issuer")
        issuer_pattern = r"name:\s*selfsigned-issuer"
        issuer_replacement = f'name: {{{{ include "{chart_name}.fullname" . }}}}-selfsigned-issuer'
        content = re.sub(issuer_pattern, issuer_replacement, content)

    # Fix commonName: Remove fullname prefix if it exists (for llmisvc certificate)
    # Pattern: commonName: {{ include "chart.fullname" . }}-service-name.{{ .Release.Namespace }}.svc
    # Should be: commonName: service-name.{{ .Release.Namespace }}.svc (for llmisvc)
    # OR add fullname prefix if missing (for kserve certificate)
    chart_name_escaped = chart_name.replace('.', r'\.')

    # First, try to remove fullname prefix from commonName (for llmisvc)
    commonname_with_prefix_pattern = (
        r'commonName:\s*\{\{\s*include\s+"' + chart_name_escaped + r'\.fullname"\s+\.\s*\}\}\s*-\s*'
        + re.escape(service_name) + r'\.\{\{\s*\.Release\.Namespace\s*\}\}\.svc'
    )
    commonname_without_prefix = f"commonName: {service_name}.{{{{ .Release.Namespace }}}}.svc"
    content = re.sub(commonname_with_prefix_pattern, commonname_without_prefix, content)

    # For llmisvc: Also convert hardcoded namespace to template (if it's still hardcoded)
    # Pattern: commonName: service-name.namespace.svc -> commonName: service-name.{{ .Release.Namespace }}.svc
    if remove_fullname_prefix:
        hardcoded_namespace_pattern = rf'commonName:\s*{re.escape(service_name)}\.\w+\.svc'
        templated_namespace = f"commonName: {service_name}.{{{{ .Release.Namespace }}}}.svc"
        # Only replace if it's a hardcoded value (not already templated)
        if re.search(hardcoded_namespace_pattern, content) and '{{' not in re.search(hardcoded_namespace_pattern, content).group(0):
            content = re.sub(hardcoded_namespace_pattern, templated_namespace, content)

    # Then, fix commonName to add fullname pattern if it's hardcoded without template (for kserve)
    # Pattern matches hardcoded service names like "kserve-webhook-server-service.kserve.svc"
    # Only apply if the commonName doesn't already have a template
    # Skip this for llmisvc certificates (remove_fullname_prefix=True) - they should stay hardcoded
    # Only for hardcoded service names and kserve certificates
    if not remove_fullname_prefix and '{{' not in service_name:
        commonname_hardcoded_pattern = rf"commonName:\s*{service_name}\.\w+\.svc"
        # Only replace if it's a hardcoded value (not already templated)
        if re.search(commonname_hardcoded_pattern, content) and '{{' not in re.search(commonname_hardcoded_pattern, content).group(0):
            commonname_replacement = f"commonName: {{{{ include \"{chart_name}.fullname\" . }}}}-{service_name}.{{{{ .Release.Namespace }}}}.svc"
            content = re.sub(commonname_hardcoded_pattern, commonname_replacement, content)

    with open(filepath, 'w', encoding='utf-8') as f:
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
    # Remove fullname prefix from both commonName and dnsNames (service name doesn't have prefix)
    fix_certificate_file(
        'charts/kserve-resources/templates/llmisvc-serving-cert.yaml',
        'llmisvc-webhook-server-service',
        'kserve-resources',
        remove_fullname_prefix=True
    )

    # Fix LLMISvc controller certificate in kserve-llmisvc-resources
    # Remove fullname prefix from both commonName and dnsNames (service name doesn't have prefix)
    # Also fix issuerRef to use fullname template
    fix_certificate_file(
        'charts/kserve-llmisvc-resources/templates/llmisvc-serving-cert.yaml',
        'llmisvc-webhook-server-service',
        'kserve-llmisvc-resources',
        remove_fullname_prefix=True
    )
