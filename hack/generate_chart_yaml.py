#!/usr/bin/env python3
"""
Generate Chart.yaml files for all KServe Helm charts.

This script generates Chart.yaml files with consistent metadata and structure
matching the format used in kserve/kserve:master.
"""

import os
import sys
from typing import Optional


# Chart metadata templates
CHART_METADATA = {
    "kserve-crd": {
        "apiVersion": "v1",
        "name": "kserve-crd",
        "description": "Helm chart for deploying kserve crds ",
        "keywords": ["kserve", "modelmesh"],
        "sources": ["http://github.com/kserve/kserve"],
    },
    "kserve-crd-minimal": {
        "apiVersion": "v1",
        "name": "kserve-crd-minimal",
        "description": "Helm chart for deploying minimal kserve crds without validation",
        "keywords": ["kserve", "modelmesh"],
        "sources": ["http://github.com/kserve/kserve"],
    },
    "kserve-llmisvc-crd": {
        "apiVersion": "v1",
        "name": "kserve-llmisvc-crd",
        "description": "Helm chart for deploying LLMInferenceService crds ",
        "keywords": [
            "kserve",
            "llm",
            "llm-d",
            "inference",
            "generative-ai",
            "machine-learning",
            "model-serving",
        ],
        "home": "https://kserve.github.io/website/",
        "sources": ["https://github.com/kserve/kserve"],
        "maintainers": [{"name": "KServe Team", "url": "https://github.com/kserve/kserve"}],
        "icon": "https://raw.githubusercontent.com/kserve/website/main/docs/images/logo.png",
        "annotations": {"category": "AI/Machine Learning"},
    },
    "kserve-llmisvc-crd-minimal": {
        "apiVersion": "v1",
        "name": "kserve-llmisvc-crd-minimal",
        "description": "Helm chart for deploying LLMInferenceService minimal crds ",
        "keywords": [
            "kserve",
            "llm",
            "llm-d",
            "inference",
            "generative-ai",
            "machine-learning",
            "model-serving",
        ],
        "home": "https://kserve.github.io/website/",
        "sources": ["https://github.com/kserve/kserve"],
        "maintainers": [{"name": "KServe Team", "url": "https://github.com/kserve/kserve"}],
        "icon": "https://raw.githubusercontent.com/kserve/website/main/docs/images/logo.png",
        "annotations": {"category": "AI/Machine Learning"},
    },
    "kserve-llmisvc-resources": {
        "apiVersion": "v2",
        "name": "kserve-llmisvc-resources",
        "description": "Helm chart for deploying KServe LLMInferenceService resources",
        "type": "application",
        "keywords": [
            "kserve",
            "llm",
            "llm-d",
            "inference",
            "generative-ai",
            "machine-learning",
            "model-serving",
        ],
        "home": "https://kserve.github.io/website/",
        "sources": ["https://github.com/kserve/kserve"],
        "maintainers": [{"name": "KServe Team", "url": "https://github.com/kserve/kserve"}],
        "icon": "https://raw.githubusercontent.com/kserve/website/main/docs/images/logo.png",
        "annotations": {"category": "AI/Machine Learning"},
    },
    "kserve-resources": {
        "apiVersion": "v1",
        "name": "kserve ",  # Note: trailing space matches master
        "description": "Helm chart for deploying kserve resources",
        "keywords": ["kserve", "modelmesh"],
        "sources": ["http://github.com/kserve/kserve"],
    },
}


def generate_chart_yaml(chart_name: str, version: str, app_version: Optional[str] = None) -> str:
    """
    Generate Chart.yaml content for a given chart.

    Args:
        chart_name: Name of the chart (e.g., 'kserve-crd')
        version: Chart version (e.g., 'v0.16.0')
        app_version: Application version (only for v2 charts)

    Returns:
        Chart.yaml content as a string
    """
    if chart_name not in CHART_METADATA:
        raise ValueError(f"Unknown chart: {chart_name}")

    metadata = CHART_METADATA[chart_name]
    lines = []

    # Required fields (order matters!)
    lines.append(f"apiVersion: {metadata['apiVersion']}")
    lines.append(f"name: {metadata['name']}")
    lines.append(f"version: {version}")

    # For API v2, add appVersion after version
    if metadata["apiVersion"] == "v2" and app_version:
        lines.append(f"appVersion: {app_version}")

    lines.append(f"description: {metadata['description']}")

    # Type (only for v2)
    if "type" in metadata:
        lines.append(f"type: {metadata['type']}")

    # Keywords
    if "keywords" in metadata:
        lines.append("keywords:")
        for keyword in metadata["keywords"]:
            lines.append(f"  - {keyword}")

    # Home (optional)
    if "home" in metadata:
        lines.append(f"home: {metadata['home']}")

    # Sources
    if "sources" in metadata:
        lines.append("sources:")
        for source in metadata["sources"]:
            lines.append(f"  - {source}")

    # Maintainers (optional)
    if "maintainers" in metadata:
        lines.append("maintainers:")
        for maintainer in metadata["maintainers"]:
            lines.append(f"  - name: {maintainer['name']}")
            lines.append(f"    url: {maintainer['url']}")

    # Icon (optional)
    if "icon" in metadata:
        lines.append(f"icon: {metadata['icon']}")

    # Annotations (optional)
    if "annotations" in metadata:
        lines.append("annotations:")
        for key, value in metadata["annotations"].items():
            lines.append(f"  {key}: {value}")

    return "\n".join(lines) + "\n"


def main():
    """Generate Chart.yaml files for specified charts."""
    if len(sys.argv) < 3:
        print("Usage: generate_chart_yaml.py <version> <chart1> [chart2] [...]")
        print("Example: generate_chart_yaml.py v0.16.0 kserve-resources kserve-llmisvc-resources")
        sys.exit(1)

    version = sys.argv[1]
    chart_names = sys.argv[2:]

    for chart_name in chart_names:
        if chart_name not in CHART_METADATA:
            print(f"Error: Unknown chart '{chart_name}'", file=sys.stderr)
            print(f"Available charts: {', '.join(CHART_METADATA.keys())}", file=sys.stderr)
            sys.exit(1)

        # Determine app version (only for v2 charts)
        metadata = CHART_METADATA[chart_name]
        app_version = version if metadata["apiVersion"] == "v2" else None

        # Generate Chart.yaml content
        content = generate_chart_yaml(chart_name, version, app_version)

        # Write to file
        chart_path = f"charts/{chart_name}/Chart.yaml"
        os.makedirs(os.path.dirname(chart_path), exist_ok=True)
        with open(chart_path, "w") as f:
            f.write(content)

        print(f"Generated {chart_path}")


if __name__ == "__main__":
    main()
