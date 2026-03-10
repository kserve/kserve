#!/usr/bin/env python3

# Copyright 2025 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Kustomize manifest building and processing utilities."""

import subprocess
from pathlib import Path
from typing import Any


YAML_SEPARATOR = '---\n'


def run_kustomize_build(kustomize_dir: Path) -> str:
    """Run kustomize build on a directory.

    Args:
        kustomize_dir: Directory containing kustomization.yaml

    Returns:
        Kustomize build output as string

    Raises:
        subprocess.CalledProcessError: If kustomize build fails
        FileNotFoundError: If kustomize command not found
    """
    try:
        result = subprocess.run(
            ["kustomize", "build", str(kustomize_dir)],
            capture_output=True,
            text=True,
            check=True,
            cwd=kustomize_dir.parent
        )
        return result.stdout
    except subprocess.CalledProcessError as e:
        raise RuntimeError(
            f"Failed to run kustomize build on {kustomize_dir}: {e}\n"
            f"stderr: {e.stderr}"
        )
    except FileNotFoundError:
        raise FileNotFoundError("kustomize command not found. Please install kustomize.")


def filter_out_crds(manifest: str) -> str:
    """Filter out CustomResourceDefinition from manifest.

    Args:
        manifest: YAML manifest string

    Returns:
        Manifest with CRDs removed
    """
    documents = manifest.split(YAML_SEPARATOR)
    filtered_documents = []

    for doc in documents:
        if not doc.strip():
            continue

        is_crd = any('kind:' in line and 'CustomResourceDefinition' in line
                     for line in doc.split('\n'))

        if not is_crd:
            filtered_documents.append(doc)

    return YAML_SEPARATOR.join(filtered_documents)


def get_llmisvc_value(config: dict[str, Any], components: list[dict[str, Any]]) -> str:
    """Extract ENABLE_LLMISVC value from definition config.

    Priority:
    1. kserve-helm component env
    2. kserve-kustomize component env
    3. GLOBAL_ENV
    4. Default: "false"

    Args:
        config: Parsed definition config
        components: List of component configs

    Returns:
        ENABLE_LLMISVC value ("true" or "false")
    """
    llmisvc = "false"

    # Check kserve-helm or kserve-kustomize component env first
    for comp in components:
        comp_name = comp.get("name", "")
        if comp_name in ["kserve-helm", "kserve-kustomize"]:
            llmisvc = comp.get("env", {}).get("ENABLE_LLMISVC", llmisvc)
            if llmisvc == "true":
                break

    # If not found in component, check GLOBAL_ENV
    if llmisvc == "false":
        llmisvc = config.get("global_env", {}).get("ENABLE_LLMISVC", "false")

    return llmisvc


def select_kserve_directories(repo_root: Path, llmisvc: str) -> tuple[list[Path], list[Path]]:
    """Select KServe CRD and config directories based on ENABLE_LLMISVC.

    Args:
        repo_root: Repository root path
        llmisvc: ENABLE_LLMISVC value ("true" or "false")

    Returns:
        Tuple of (crd_dirs, config_dirs)
    """
    if llmisvc == "true":
        crd_dirs = [repo_root / "config/crd/full/llmisvc"]
        config_dirs = [repo_root / "config/overlays/standalone/llmisvc"]
    else:
        crd_dirs = [
            repo_root / "config/crd/full",
            repo_root / "config/crd/full/llmisvc",
            repo_root / "config/crd/full/localmodel",
        ]
        config_dirs = [repo_root / "config/overlays/all"]

    return crd_dirs, config_dirs


def build_kserve_manifests(repo_root: Path,
                           config: dict[str, Any],
                           components: list[dict[str, Any]]) -> tuple[str, str, str, str]:
    """Build KServe CRD, core, runtime, and llmisvcconfig manifests.

    Args:
        repo_root: Repository root path
        config: Parsed definition config
        components: List of component configs

    Returns:
        Tuple of (crd_manifest, core_manifest, runtime_manifest, llmisvcconfig_manifest)
    """
    llmisvc = get_llmisvc_value(config, components)
    crd_dirs, config_dirs = select_kserve_directories(repo_root, llmisvc)

    # Build CRD manifests from all CRD directories
    crd_manifests = []
    for crd_dir in crd_dirs:
        manifest = run_kustomize_build(crd_dir)
        crd_manifests.append(manifest)

    # Combine all CRD manifests
    crd_manifest = YAML_SEPARATOR.join(crd_manifests)

    # Build core manifests from all config directories
    core_manifests = []
    for config_dir in config_dirs:
        full_manifest = run_kustomize_build(config_dir)
        core_manifest_part = filter_out_crds(full_manifest)
        core_manifests.append(core_manifest_part)

    # Combine all core manifests
    core_manifest = YAML_SEPARATOR.join(core_manifests)

    # Build runtime and llmisvcconfig manifests
    runtime_manifest = build_kserve_runtime_manifests(repo_root)
    llmisvcconfig_manifest = build_kserve_llmisvcconfig_manifests(repo_root)

    return crd_manifest, core_manifest, runtime_manifest, llmisvcconfig_manifest


def build_kserve_runtime_manifests(repo_root: Path) -> str:
    """Build KServe runtime manifests.

    Args:
        repo_root: Repository root path

    Returns:
        Combined runtime manifest as string

    Raises:
        RuntimeError: If kustomize build fails
    """
    runtime_dir = repo_root / "config/runtimes"
    if not runtime_dir.exists():
        return ""

    manifest = run_kustomize_build(runtime_dir)
    return manifest


def build_kserve_llmisvcconfig_manifests(repo_root: Path) -> str:
    """Build KServe LLMISvcConfig manifests.

    Args:
        repo_root: Repository root path

    Returns:
        Combined LLMISvcConfig manifest as string

    Raises:
        RuntimeError: If kustomize build fails
    """
    llmisvcconfig_dir = repo_root / "config/llmisvcconfig"
    if not llmisvcconfig_dir.exists():
        return ""

    manifest = run_kustomize_build(llmisvcconfig_dir)
    return manifest


def generate_manifest_functions(crd_manifest: str,
                                core_manifest: str,
                                runtime_manifest: str = "",
                                llmisvcconfig_manifest: str = "") -> str:
    """Generate bash functions for embedded manifests.

    Args:
        crd_manifest: CRD manifest content
        core_manifest: Core manifest content
        runtime_manifest: Runtime manifest content (optional)
        llmisvcconfig_manifest: LLMISvcConfig manifest content (optional)

    Returns:
        Bash function definitions as string
    """

    # Generate runtime getter function (only if manifest exists)
    runtime_getter = ""
    if runtime_manifest:
        runtime_getter = f'''get_kserve_runtime_manifests() {{
    cat <<'KSERVE_RUNTIME_MANIFEST_EOF'
{runtime_manifest}KSERVE_RUNTIME_MANIFEST_EOF
}}

'''

    # Generate llmisvcconfig getter function (only if manifest exists)
    llmisvcconfig_getter = ""
    if llmisvcconfig_manifest:
        llmisvcconfig_getter = f'''get_kserve_llmisvcconfig_manifests() {{
    cat <<'KSERVE_LLMISVCCONFIG_MANIFEST_EOF'
{llmisvcconfig_manifest}KSERVE_LLMISVCCONFIG_MANIFEST_EOF
}}

'''

    # Generate create functions (always generate)
    runtime_create = '''create_kserve_runtime_manifests() {
    get_kserve_runtime_manifests | kubectl apply --server-side -f -
}

'''

    llmisvcconfig_create = '''create_kserve_llmisvcconfig_manifests() {
    get_kserve_llmisvcconfig_manifests | kubectl apply --server-side -f -
}

'''

    return f'''# ============================================================================
# KServe Manifest Functions (EMBED_MANIFESTS MODE)
# ============================================================================

install_kserve_manifest() {{
    log_info "Installing KServe CRDs..."
    get_kserve_crd_manifest | kubectl apply --server-side -f -

    log_info "Installing KServe core components..."
    get_kserve_core_manifest | kubectl apply --server-side -f -

    log_success "KServe CRD and core components installed successfully!"
}}

uninstall_kserve_manifest() {{
    # Uninstall in reverse order of dependencies
    log_info "Uninstalling KServe LLMISvcConfig manifests..."
    if [ "${{LLMISVC:-false}}" = "true" ] && type get_kserve_llmisvcconfig_manifests &>/dev/null; then
        get_kserve_llmisvcconfig_manifests | kubectl delete -f - || true
    fi

    log_info "Uninstalling KServe runtime manifests..."
    if [ "${{INSTALL_RUNTIMES:-false}}" = "true" ] && type get_kserve_runtime_manifests &>/dev/null; then
        get_kserve_runtime_manifests | kubectl delete -f - || true
    fi

    log_info "Uninstalling KServe core components..."
    get_kserve_core_manifest | kubectl delete -f - || true

    log_info "Uninstalling KServe CRDs..."
    get_kserve_crd_manifest | kubectl delete -f - || true

    log_success "KServe manifests uninstalled successfully!"
}}

{runtime_getter}{llmisvcconfig_getter}{runtime_create}{llmisvcconfig_create}get_kserve_crd_manifest() {{
    cat <<'KSERVE_CRD_MANIFEST_EOF'
{crd_manifest}KSERVE_CRD_MANIFEST_EOF
}}

get_kserve_core_manifest() {{
    cat <<'KSERVE_CORE_MANIFEST_EOF'
{core_manifest}KSERVE_CORE_MANIFEST_EOF
}}

'''
