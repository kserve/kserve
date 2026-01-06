#!/usr/bin/env python3

"""Generate kserve-deps.env from go.mod"""

import os
import re
import subprocess
import json
from pathlib import Path


# Configuration
DEPENDENCIES = {
    "KEDA_VERSION": ("github.com/kedacore/keda/v2", "kedacore", "keda"),
    "ISTIO_VERSION": ("istio.io/api", "istio", "base"),
    "OPENTELEMETRY_OPERATOR_VERSION": (
        "github.com/open-telemetry/opentelemetry-operator",
        "open-telemetry",
        "opentelemetry-operator",
    ),
    "GATEWAY_API_VERSION": ("sigs.k8s.io/gateway-api", None, None),
    "LWS_VERSION": ("sigs.k8s.io/lws", None, None),
    "GIE_VERSION": ("sigs.k8s.io/gateway-api-inference-extension", None, None),
}

HELM_REPOS = {
    "istio": "https://istio-release.storage.googleapis.com/charts",
    "kedacore": "https://kedacore.github.io/charts",
    "open-telemetry": "https://open-telemetry.github.io/opentelemetry-helm-charts",
}


def run(cmd):
    """Run command and return stdout"""
    return subprocess.run(cmd, shell=True, capture_output=True, text=True, check=True).stdout


def extract_all_versions_from_gomod(go_mod_path, packages):
    """Extract all versions from go.mod at once"""
    content = go_mod_path.read_text()
    versions = {}

    for package in packages:
        match = re.search(rf"^\s+{re.escape(package)}\s+(\S+)", content, re.MULTILINE)
        if not match:
            raise ValueError(f"Package {package} not found in go.mod")
        versions[package] = match.group(1).lstrip("v")

    return versions


def get_helm_versions(repo, chart):
    """Get all available helm chart versions with caching"""
    cache_file = f"/tmp/{repo.replace('/', '_')}__{chart.replace('/', '_')}.json"

    # Use cache if exists
    if os.path.exists(cache_file):
        with open(cache_file, "r") as f:
            return json.load(f)

    # Fetch from helm and cache
    output = run(f"helm search repo {repo}/{chart} --versions --devel -o json")
    versions = json.loads(output)

    with open(cache_file, "w") as f:
        json.dump(versions, f)

    return versions


def parse_semver(ver_str):
    """Parse semantic version string to (major, minor, patch) tuple"""
    ver = ver_str.lstrip("v")
    # Remove pre-release and build metadata
    ver = ver.split("-")[0].split("+")[0]
    parts = ver.split(".")
    major = int(parts[0]) if len(parts) > 0 and parts[0].isdigit() else 0
    minor = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 0
    patch = int(parts[2]) if len(parts) > 2 and parts[2].isdigit() else 0
    return (major, minor, patch)


def find_best_chart_version(versions, requested_app_version):
    """Find best matching chart version for requested app version"""
    # Filter out versions without app_version
    valid = [v for v in versions if v.get("app_version") and v["app_version"] != "-"]

    if not valid:
        return versions[0]["version"]  # Return newest

    # Try exact match
    for v in valid:
        if v["app_version"].lstrip("v") == requested_app_version:
            return v["version"]

    # Find closest version (compare major.minor.patch)
    try:
        target_major, target_minor, target_patch = parse_semver(requested_app_version)
        target_num = target_major * 10000 + target_minor * 100 + target_patch

        def version_distance(v):
            try:
                app_major, app_minor, app_patch = parse_semver(v["app_version"])
                app_num = app_major * 10000 + app_minor * 100 + app_patch
                return abs(target_num - app_num)
            except Exception:
                return float("inf")

        closest = min(valid, key=version_distance)
        print(f"⚠️  Using {closest['version']} (app: {closest['app_version']}) for requested {requested_app_version}")
        return closest["version"]
    except Exception:
        return valid[0]["version"]


def ensure_helm_repo(name, url):
    """Add helm repo only if it doesn't exist"""
    result = run("helm repo list -o json || echo '[]'")
    repos = json.loads(result)
    if any(r.get("name") == name for r in repos):
        return  # Already exists

    run(f"helm repo add {name} {url}")


def main():
    # Setup paths
    repo_root = Path(__file__).resolve().parent.parent.parent.parent
    go_mod = repo_root / "go.mod"
    output_file = repo_root / "kserve-deps.env"

    print("📦 Extracting versions from go.mod...")

    # Extract all packages from go.mod at once (single file read)
    packages = [pkg for pkg, _, _ in DEPENDENCIES.values()]
    gomod_versions = extract_all_versions_from_gomod(go_mod, packages)

    # Ensure helm repos exist
    for name, url in HELM_REPOS.items():
        ensure_helm_repo(name, url)

    # Resolve versions
    versions = {}
    for var_name, (package, helm_repo, helm_chart) in DEPENDENCIES.items():
        app_version = gomod_versions[package]

        if helm_repo and helm_chart:
            # Resolve helm chart version
            helm_versions = get_helm_versions(helm_repo, helm_chart)
            chart_version = find_best_chart_version(helm_versions, app_version)
            versions[var_name] = chart_version
        else:
            # Use go.mod version directly (with 'v' prefix)
            versions[var_name] = f"v{app_version}"

    # Update output file
    lines = output_file.read_text().splitlines(keepends=True)
    start = next(i for i, line in enumerate(lines) if "# START" in line)
    end = next(i for i, line in enumerate(lines) if "# END" in line)

    new_section = [
        "# START\n",
        "# Serverless dependencies\n",
        f"ISTIO_VERSION={versions['ISTIO_VERSION']}\n",
        "\n",
        "# KEDA dependencies\n",
        f"KEDA_VERSION={versions['KEDA_VERSION']}\n",
        f"OPENTELEMETRY_OPERATOR_VERSION={versions['OPENTELEMETRY_OPERATOR_VERSION']}\n",
        "\n",
        "# LLMISvc dependencies\n",
        f"LWS_VERSION={versions['LWS_VERSION']}\n",
        f"GATEWAY_API_VERSION={versions['GATEWAY_API_VERSION']}\n",
        f"GIE_VERSION={versions['GIE_VERSION']}\n",
        "# END\n",
    ]

    output_file.write_text("".join(lines[:start] + new_section + lines[end + 1 :]))

    print(f"\n✅ Updated {output_file.name}\n")
    # Print in specific order
    output_order = [
        "ISTIO_VERSION",
        "KEDA_VERSION",
        "GATEWAY_API_VERSION",
        "GIE_VERSION",
        "LWS_VERSION",
        "OPENTELEMETRY_OPERATOR_VERSION",
    ]
    for var in output_order:
        print(f"  {var}={versions[var]}")


if __name__ == "__main__":
    main()
