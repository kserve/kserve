#!/usr/bin/env python3

"""Generate kserve-deps.env from go.mod"""

import os
import re
import subprocess
import json
import urllib.request
from urllib.request import Request
from pathlib import Path


# Configuration: (go_package, helm_repo, helm_chart, (github_repo, download_file))
DEPENDENCIES = {
    "KEDA_VERSION": ("github.com/kedacore/keda/v2", "kedacore", "keda", None),
    "ISTIO_VERSION": ("istio.io/api", "istio", "base", None),
    "OPENTELEMETRY_OPERATOR_VERSION": (
        "github.com/open-telemetry/opentelemetry-operator",
        "open-telemetry",
        "opentelemetry-operator",
        None,
    ),
    "GATEWAY_API_VERSION": (
        "sigs.k8s.io/gateway-api",
        None,
        None,
        ("kubernetes-sigs/gateway-api", "standard-install.yaml"),
    ),
    "LWS_VERSION": (
        "sigs.k8s.io/lws",
        None,
        None,
        ("kubernetes-sigs/lws", "manifests.yaml"),
    ),
    "GIE_VERSION": (
        "sigs.k8s.io/gateway-api-inference-extension",
        None,
        None,
        ("kubernetes-sigs/gateway-api-inference-extension", "manifests.yaml"),
    ),
}

HELM_REPOS = {
    "istio": "https://istio-release.storage.googleapis.com/charts",
    "kedacore": "https://kedacore.github.io/charts",
    "open-telemetry": "https://open-telemetry.github.io/opentelemetry-helm-charts",
}


def run(cmd):
    return subprocess.run(cmd, shell=True, capture_output=True, text=True, check=True).stdout


def extract_all_versions_from_gomod(go_mod_path, packages):
    content = go_mod_path.read_text()
    versions = {}

    for package in packages:
        match = re.search(rf"^\s+{re.escape(package)}\s+(\S+)", content, re.MULTILINE)
        if not match:
            raise ValueError(f"Package {package} not found in go.mod")
        versions[package] = match.group(1).lstrip("v")

    return versions


def get_helm_versions(repo, chart):
    cache_file = f"/tmp/{repo.replace('/', '_')}__{chart.replace('/', '_')}.json"
    if os.path.exists(cache_file):
        with open(cache_file, "r") as f:
            return json.load(f)

    output = run(f"helm search repo {repo}/{chart} --versions --devel -o json")
    versions = json.loads(output)
    with open(cache_file, "w") as f:
        json.dump(versions, f)
    return versions


def strip_pseudo_version(version):
    """Strip pseudo-version to base version (v1.3.1-0.xxx -> v1.3.1)"""
    match = re.match(r'^(v[0-9]+\.[0-9]+\.[0-9]+)-0\.[0-9]{14}-', version)
    if match:
        return match.group(1)
    return version


def check_url_exists(url, timeout=5):
    """Check if URL exists via HEAD request"""
    try:
        req = Request(url, method='HEAD')
        with urllib.request.urlopen(req, timeout=timeout) as response:
            return 200 <= response.status < 400
    except Exception:
        return False


def build_release_url(github_repo, version, filename):
    return f"https://github.com/{github_repo}/releases/download/{version}/{filename}"


def find_available_version_with_url(base_version, github_repo, filename):
    """Find closest version in same minor version (X.Y.z) with existing URL"""
    if check_url_exists(build_release_url(github_repo, base_version, filename)):
        return base_version, True

    try:
        major, minor, patch = parse_semver(base_version)
    except Exception:
        return base_version, False

    try:
        api_url = f"https://api.github.com/repos/{github_repo}/releases"
        with urllib.request.urlopen(api_url, timeout=10) as response:
            releases = json.loads(response.read())

        same_minor_versions = []
        for release in releases:
            tag = release.get("tag_name", "")
            try:
                r_major, r_minor, r_patch = parse_semver(tag)
                if r_major == major and r_minor == minor:
                    same_minor_versions.append((tag, r_patch))
            except Exception:
                continue

        if not same_minor_versions:
            return base_version, False

        same_minor_versions.sort(key=lambda x: abs(x[1] - patch))

        for ver, _ in same_minor_versions:
            if check_url_exists(build_release_url(github_repo, ver, filename)):
                return ver, True

        return base_version, False

    except Exception as e:
        print(f"⚠️  Failed to fetch releases from {github_repo}: {e}")
        return base_version, False


def parse_semver(ver_str):
    """Parse version to (major, minor, patch) tuple"""
    ver = ver_str.lstrip("v").split("-")[0].split("+")[0]
    parts = ver.split(".")
    major = int(parts[0]) if len(parts) > 0 and parts[0].isdigit() else 0
    minor = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 0
    patch = int(parts[2]) if len(parts) > 2 and parts[2].isdigit() else 0
    return (major, minor, patch)


def find_best_chart_version(versions, requested_app_version):
    valid = [v for v in versions if v.get("app_version") and v["app_version"] != "-"]
    if not valid:
        return versions[0]["version"]

    for v in valid:
        if v["app_version"].lstrip("v") == requested_app_version:
            return v["version"]

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
    result = run("helm repo list -o json || echo '[]'")
    repos = json.loads(result)
    if any(r.get("name") == name for r in repos):
        return
    run(f"helm repo add {name} {url}")


def main():
    repo_root = Path(__file__).resolve().parent.parent.parent.parent
    go_mod = repo_root / "go.mod"
    output_file = repo_root / "kserve-deps.env"

    print("📦 Extracting versions from go.mod...")

    packages = [item[0] for item in DEPENDENCIES.values()]
    gomod_versions = extract_all_versions_from_gomod(go_mod, packages)

    for name, url in HELM_REPOS.items():
        ensure_helm_repo(name, url)

    versions = {}
    for var_name, dependency_info in DEPENDENCIES.items():
        package = dependency_info[0]
        helm_repo = dependency_info[1] if len(dependency_info) > 1 else None
        helm_chart = dependency_info[2] if len(dependency_info) > 2 else None
        url_verify = dependency_info[3] if len(dependency_info) > 3 else None

        app_version = gomod_versions[package]

        if helm_repo and helm_chart:
            helm_versions = get_helm_versions(helm_repo, helm_chart)
            chart_version = find_best_chart_version(helm_versions, app_version)
            versions[var_name] = chart_version
        else:
            raw_version = f"v{app_version}"
            base_version = strip_pseudo_version(raw_version)

            if url_verify:
                github_repo, filename = url_verify
                final_version, url_found = find_available_version_with_url(
                    base_version, github_repo, filename
                )
                if url_found:
                    if final_version == base_version:
                        print(f"✅ {var_name}: {final_version} (URL verified)")
                    else:
                        print(f"⚠️  {var_name}: Using {final_version} for requested {base_version}")
                else:
                    print(f"⚠️  {var_name}: No available URL found, using {base_version}")
                versions[var_name] = final_version
            else:
                versions[var_name] = base_version

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
    for var in ["ISTIO_VERSION", "KEDA_VERSION", "GATEWAY_API_VERSION",
                "GIE_VERSION", "LWS_VERSION", "OPENTELEMETRY_OPERATOR_VERSION"]:
        print(f"  {var}={versions[var]}")


if __name__ == "__main__":
    main()
