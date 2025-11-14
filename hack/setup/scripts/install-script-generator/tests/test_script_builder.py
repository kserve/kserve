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

"""Tests for script_builder module."""

import sys
from pathlib import Path

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import script_builder  # noqa: E402


def test_build_component_variables_basic():
    """Test building component variables section."""
    components = [
        {
            "name": "cert-manager",
            "variables": [
                'NAMESPACE="cert-manager"',
                'VERSION="v1.13.0"'
            ]
        },
        {
            "name": "istio",
            "variables": [
                'ISTIO_VERSION="1.27.1"',
                'NAMESPACE="istio-system"'
            ]
        }
    ]
    global_env = {}

    result = script_builder.build_component_variables(components, global_env)

    assert 'NAMESPACE="cert-manager"' in result
    assert 'VERSION="v1.13.0"' in result
    assert 'ISTIO_VERSION="1.27.1"' in result


def test_build_component_variables_with_global_env():
    """Test that global env variables are excluded from component variables."""
    components = [
        {
            "name": "cert-manager",
            "variables": [
                'NAMESPACE="cert-manager"',
                'VERSION="v1.13.0"'
            ]
        }
    ]
    global_env = {"NAMESPACE": "custom-namespace"}

    result = script_builder.build_component_variables(components, global_env)

    # NAMESPACE should be excluded as it's in global_env
    assert 'NAMESPACE=' not in result
    assert 'VERSION="v1.13.0"' in result


def test_build_component_variables_deduplication():
    """Test that duplicate variables are removed."""
    components = [
        {
            "name": "comp1",
            "variables": [
                'VERSION="v1.0.0"',
                'NAMESPACE="default"'
            ]
        },
        {
            "name": "comp2",
            "variables": [
                'VERSION="v2.0.0"',  # Duplicate, should keep first
                'IMAGE="test"'
            ]
        }
    ]
    global_env = {}

    result = script_builder.build_component_variables(components, global_env)

    # First VERSION should be kept
    assert 'VERSION="v1.0.0"' in result
    assert 'VERSION="v2.0.0"' not in result
    assert 'NAMESPACE="default"' in result
    assert 'IMAGE="test"' in result


def test_build_component_functions():
    """Test building component functions section."""
    components = [
        {
            "name": "cert-manager",
            "install_code": "install_cert_manager() {\n    echo 'Installing'\n}",
            "uninstall_code": "uninstall_cert_manager() {\n    echo 'Uninstalling'\n}"
        },
        {
            "name": "istio",
            "install_code": "install_istio() {\n    echo 'Installing Istio'\n}",
            "uninstall_code": "uninstall_istio() {\n    echo 'Uninstalling Istio'\n}"
        }
    ]

    result = script_builder.build_component_functions(components)

    assert "# Component: cert-manager" in result
    assert "install_cert_manager() {" in result
    assert "uninstall_cert_manager() {" in result
    assert "# Component: istio" in result
    assert "install_istio() {" in result
    assert "uninstall_istio() {" in result


def test_build_definition_global_env():
    """Test building global env section."""
    global_env = {
        "KSERVE_NAMESPACE": "kserve",
        "CERT_MANAGER_VERSION": "v1.13.0"
    }

    result = script_builder.build_definition_global_env(global_env)

    assert 'export KSERVE_NAMESPACE="${KSERVE_NAMESPACE:-kserve}"' in result
    assert 'export CERT_MANAGER_VERSION="${CERT_MANAGER_VERSION:-v1.13.0}"' in result


def test_build_definition_global_env_empty():
    """Test building global env with empty dict."""
    result = script_builder.build_definition_global_env({})
    assert result == ""


def test_build_tool_install_calls(tmp_path):
    """Test building tool installation calls."""
    # Create repo structure
    repo_root = tmp_path
    cli_dir = repo_root / "hack/setup/cli"
    cli_dir.mkdir(parents=True)

    # Create tool scripts
    (cli_dir / "install-kubectl.sh").write_text("#!/bin/bash\n")
    (cli_dir / "install-helm.sh").write_text("#!/bin/bash\n")

    tools = ["kubectl", "helm", "nonexistent"]

    result = script_builder.build_tool_install_calls(tools, repo_root)

    assert "install-kubectl.sh" in result
    assert "install-helm.sh" in result
    assert "log_warning" in result
    assert "Tool installation script not found: install-nonexistent.sh" in result


def test_build_tool_install_calls_empty():
    """Test building tool install calls with empty list."""
    result = script_builder.build_tool_install_calls([], Path("/tmp"))
    assert result == ""


def test_build_install_calls_basic():
    """Test building basic install calls."""
    components = [
        {
            "name": "cert-manager",
            "install_func": "install_cert_manager",
            "env": {},
            "variables": [],
            "include_section": []
        },
        {
            "name": "istio",
            "install_func": "install_istio",
            "env": {},
            "variables": [],
            "include_section": []
        }
    ]
    global_env = {}

    result = script_builder.build_install_calls(components, global_env)

    assert "install_cert_manager" in result
    assert "install_istio" in result


def test_build_install_calls_with_env():
    """Test building install calls with environment variables."""
    components = [
        {
            "name": "kserve-helm",
            "install_func": "install_kserve_helm",
            "env": {"LLMISVC": "true", "NAMESPACE": "kserve"},
            "variables": ['LLMISVC="${LLMISVC:-false}"', 'NAMESPACE="${NAMESPACE:-default}"'],
            "include_section": []
        }
    ]
    global_env = {"NAMESPACE": "global-ns"}

    result = script_builder.build_install_calls(components, global_env)

    assert "set_env_with_priority" in result
    assert "LLMISVC" in result
    assert "NAMESPACE" in result
    assert "install_kserve_helm" in result


def test_build_install_calls_with_include_section():
    """Test building install calls with include section."""
    components = [
        {
            "name": "test",
            "install_func": "install_test",
            "env": {},
            "variables": [],
            "include_section": ["# Helper function", "helper() { echo 'help'; }"]
        }
    ]
    global_env = {}

    result = script_builder.build_install_calls(components, global_env)

    assert "# Helper function" in result
    assert "helper() { echo 'help'; }" in result
    assert "install_test" in result


def test_build_uninstall_calls():
    """Test building uninstall calls in reverse order."""
    components = [
        {
            "name": "cert-manager",
            "uninstall_func": "uninstall_cert_manager"
        },
        {
            "name": "istio",
            "uninstall_func": "uninstall_istio"
        },
        {
            "name": "keda",
            "uninstall_func": "uninstall_keda"
        }
    ]

    result = script_builder.build_uninstall_calls(components)

    # Uninstall should be in reverse order
    lines = result.split("\n")
    assert "uninstall_keda" in lines[0]
    assert "uninstall_istio" in lines[1]
    assert "uninstall_cert_manager" in lines[2]


def test_build_uninstall_calls_single():
    """Test building uninstall calls with single component."""
    components = [
        {
            "name": "cert-manager",
            "uninstall_func": "uninstall_cert_manager"
        }
    ]

    result = script_builder.build_uninstall_calls(components)

    assert "uninstall_cert_manager" in result


def test_build_component_variables_empty_components():
    """Test building variables with empty components list."""
    result = script_builder.build_component_variables([], {})
    assert result == ""


def test_build_component_functions_empty_components():
    """Test building functions with empty components list."""
    result = script_builder.build_component_functions([])
    assert result == ""


def test_build_uninstall_calls_empty_components():
    """Test building uninstall calls with empty components list."""
    result = script_builder.build_uninstall_calls([])
    assert result == ""
