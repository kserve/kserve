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

"""Tests for manifest_builder module."""

import sys
from pathlib import Path
import pytest

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import manifest_builder  # noqa: E402


def test_build_kserve_runtime_manifests_success(tmp_path, monkeypatch):
    """Test building runtime manifests successfully."""
    runtime_dir = tmp_path / "config/runtimes"
    runtime_dir.mkdir(parents=True)

    expected_manifest = "apiVersion: v1\nkind: ClusterServingRuntime\n"

    def mock_run_kustomize_build(path):
        return expected_manifest

    monkeypatch.setattr(manifest_builder, "run_kustomize_build", mock_run_kustomize_build)

    result = manifest_builder.build_kserve_runtime_manifests(tmp_path)

    assert result == expected_manifest


def test_build_kserve_runtime_manifests_directory_not_exists(tmp_path):
    """Test building runtime manifests when directory doesn't exist."""
    result = manifest_builder.build_kserve_runtime_manifests(tmp_path)
    assert result == ""


def test_build_kserve_llmisvcconfig_manifests_success(tmp_path, monkeypatch):
    """Test building llmisvcconfig manifests successfully."""
    llmisvcconfig_dir = tmp_path / "config/llmisvcconfig"
    llmisvcconfig_dir.mkdir(parents=True)

    expected_manifest = "apiVersion: v1\nkind: ConfigMap\n"

    def mock_run_kustomize_build(path):
        return expected_manifest

    monkeypatch.setattr(manifest_builder, "run_kustomize_build", mock_run_kustomize_build)

    result = manifest_builder.build_kserve_llmisvcconfig_manifests(tmp_path)

    assert result == expected_manifest


def test_build_kserve_llmisvcconfig_manifests_directory_not_exists(tmp_path):
    """Test building llmisvcconfig manifests when directory doesn't exist."""
    result = manifest_builder.build_kserve_llmisvcconfig_manifests(tmp_path)
    assert result == ""


def test_build_kserve_manifests_returns_four_values(tmp_path, monkeypatch):
    """Test that build_kserve_manifests returns 4 values."""
    (tmp_path / "config/crd/full").mkdir(parents=True)
    (tmp_path / "config/overlays/all").mkdir(parents=True)
    (tmp_path / "config/runtimes").mkdir(parents=True)
    (tmp_path / "config/llmisvcconfig").mkdir(parents=True)

    def mock_run_kustomize_build(path):
        if "crd" in str(path):
            return "kind: CustomResourceDefinition\n"
        elif "overlays" in str(path):
            return "kind: Deployment\n"
        elif "runtimes" in str(path):
            return "kind: ClusterServingRuntime\n"
        elif "llmisvcconfig" in str(path):
            return "kind: ConfigMap\n"
        return ""

    monkeypatch.setattr(manifest_builder, "run_kustomize_build", mock_run_kustomize_build)

    config = {"global_env": {}}
    components = []

    crd, core, runtime, llmisvc = manifest_builder.build_kserve_manifests(
        tmp_path, config, components
    )

    assert isinstance(crd, str) and "CustomResourceDefinition" in crd
    assert isinstance(core, str)
    assert isinstance(runtime, str) and "ClusterServingRuntime" in runtime
    assert isinstance(llmisvc, str) and "ConfigMap" in llmisvc


def test_build_kserve_manifests_without_optional_dirs(tmp_path, monkeypatch):
    """Test build_kserve_manifests when runtime/llmisvcconfig dirs don't exist."""
    (tmp_path / "config/crd/full").mkdir(parents=True)
    (tmp_path / "config/overlays/all").mkdir(parents=True)

    def mock_run_kustomize_build(path):
        if "crd" in str(path):
            return "kind: CustomResourceDefinition\n"
        elif "overlays" in str(path):
            return "kind: Deployment\n"
        return ""

    monkeypatch.setattr(manifest_builder, "run_kustomize_build", mock_run_kustomize_build)

    config = {"global_env": {}}
    components = []

    crd, core, runtime, llmisvc = manifest_builder.build_kserve_manifests(
        tmp_path, config, components
    )

    assert "CustomResourceDefinition" in crd
    assert runtime == ""
    assert llmisvc == ""


def test_generate_manifest_functions_with_all_manifests():
    """Test generating manifest functions with all manifests."""
    crd = "kind: CRD\n"
    core = "kind: Deployment\n"
    runtime = "kind: ClusterServingRuntime\n"
    llmisvc = "kind: ConfigMap\n"

    result = manifest_builder.generate_manifest_functions(crd, core, runtime, llmisvc)

    assert "get_kserve_crd_manifest()" in result
    assert "get_kserve_core_manifest()" in result
    assert "get_kserve_runtime_manifests()" in result
    assert "get_kserve_llmisvcconfig_manifests()" in result
    assert "create_kserve_runtime_manifests()" in result
    assert "create_kserve_llmisvcconfig_manifests()" in result
    assert "install_kserve_manifest()" in result
    assert "uninstall_kserve_manifest()" in result


def test_generate_manifest_functions_without_optional_manifests():
    """Test generating manifest functions without runtime/llmisvcconfig."""
    crd = "kind: CRD\n"
    core = "kind: Deployment\n"

    result = manifest_builder.generate_manifest_functions(crd, core, "", "")

    assert "get_kserve_crd_manifest()" in result
    assert "get_kserve_core_manifest()" in result
    # Getter functions should NOT be generated when manifests are empty
    assert "get_kserve_runtime_manifests()" not in result
    assert "get_kserve_llmisvcconfig_manifests()" not in result
    # Create functions should still be generated
    assert "create_kserve_runtime_manifests()" in result
    assert "create_kserve_llmisvcconfig_manifests()" in result


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
