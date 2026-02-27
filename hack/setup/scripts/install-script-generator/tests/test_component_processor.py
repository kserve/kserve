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

"""Tests for component_processor module."""

import sys
from pathlib import Path
import pytest

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import component_processor  # noqa: E402


def test_find_component_script_direct(tmp_path):
    """Test finding component script in direct location."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    # Create script at manage.{component}.sh
    script_file = infra_dir / "manage.cert-manager.sh"
    script_file.write_text("#!/bin/bash\n")

    result = component_processor.find_component_script("cert-manager", infra_dir)
    assert result == script_file


def test_find_component_script_in_subdir(tmp_path):
    """Test finding component script in component subdirectory."""
    infra_dir = tmp_path / "infra"
    component_dir = infra_dir / "cert-manager"
    component_dir.mkdir(parents=True)

    # Create script at {component}/manage.{component}.sh
    script_file = component_dir / "manage.cert-manager.sh"
    script_file.write_text("#!/bin/bash\n")

    result = component_processor.find_component_script("cert-manager", infra_dir)
    assert result == script_file


def test_find_component_script_helm_variant(tmp_path):
    """Test finding component script with -helm suffix when method is specified."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    # Create script at manage.{component}-helm.sh
    script_file = infra_dir / "manage.kserve-helm.sh"
    script_file.write_text("#!/bin/bash\n")

    result = component_processor.find_component_script("kserve", infra_dir, "helm")
    assert result == script_file


def test_find_component_script_with_method_priority(tmp_path):
    """Test that method-specific script has priority over base script."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    # Create both base and method-specific scripts
    base_script = infra_dir / "manage.kserve.sh"
    base_script.write_text("#!/bin/bash\n# Base\n")

    helm_script = infra_dir / "manage.kserve-helm.sh"
    helm_script.write_text("#!/bin/bash\n# Helm\n")

    # When method is specified, should return method-specific script
    result = component_processor.find_component_script("kserve", infra_dir, "helm")
    assert result == helm_script

    # When method is not specified, should return base script
    result = component_processor.find_component_script("kserve", infra_dir)
    assert result == base_script


def test_find_component_script_method_fallback(tmp_path):
    """Test fallback to base script when method-specific not found."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    # Create only base script
    base_script = infra_dir / "manage.cert-manager.sh"
    base_script.write_text("#!/bin/bash\n")

    # Should fall back to base script even when method is specified
    result = component_processor.find_component_script("cert-manager", infra_dir, "helm")
    assert result == base_script


def test_find_component_script_not_found(tmp_path):
    """Test finding non-existent component script returns None."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    result = component_processor.find_component_script("nonexistent", infra_dir)
    assert result is None


def test_find_component_script_priority(tmp_path):
    """Test that direct path has priority over subdirectory."""
    infra_dir = tmp_path / "infra"
    component_dir = infra_dir / "cert-manager"
    component_dir.mkdir(parents=True)

    # Create scripts in both locations
    direct_script = component_dir / "manage.cert-manager.sh"
    direct_script.write_text("#!/bin/bash\n# Direct\n")

    root_script = infra_dir / "manage.cert-manager.sh"
    root_script.write_text("#!/bin/bash\n# Root\n")

    # Should return the one in subdirectory first (first in search paths)
    result = component_processor.find_component_script("cert-manager", infra_dir)
    assert result == direct_script


def test_process_component_basic(tmp_path):
    """Test basic component processing."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    # Create a simple component script
    script_content = """#!/bin/bash

# VARIABLES
NAMESPACE="cert-manager"
VERSION="v1.13.0"
# VARIABLES END

install() {
    echo "Installing cert-manager"
}

uninstall() {
    echo "Uninstalling cert-manager"
}
"""
    script_file = infra_dir / "manage.cert-manager.sh"
    script_file.write_text(script_content)

    comp_config = {"name": "cert-manager", "env": {}}
    result = component_processor.process_component(comp_config, infra_dir)

    assert result["name"] == "cert-manager"
    assert result["install_func"] == "install_cert_manager"
    assert result["uninstall_func"] == "uninstall_cert_manager"
    assert "install_cert_manager() {" in result["install_code"]
    assert "uninstall_cert_manager() {" in result["uninstall_code"]
    assert len(result["variables"]) == 2
    assert result["env"] == {}


def test_process_component_with_env(tmp_path):
    """Test component processing with environment variables."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    script_content = """#!/bin/bash

# VARIABLES
NAMESPACE="default"
# VARIABLES END

install() {
    echo "Installing"
}

uninstall() {
    echo "Uninstalling"
}
"""
    script_file = infra_dir / "manage.test.sh"
    script_file.write_text(script_content)

    comp_config = {
        "name": "test",
        "env": {"NAMESPACE": "custom-ns", "VERSION": "v1.0.0"}
    }
    result = component_processor.process_component(comp_config, infra_dir)

    assert result["env"] == {"NAMESPACE": "custom-ns", "VERSION": "v1.0.0"}


def test_process_component_with_include_section(tmp_path):
    """Test component processing with include section."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    script_content = """#!/bin/bash

# VARIABLES
VAR="value"
# VARIABLES END

# INCLUDE_IN_GENERATED_SCRIPT_START
# Custom helper function
helper() {
    echo "Helper"
}
# INCLUDE_IN_GENERATED_SCRIPT_END

install() {
    echo "Installing"
}

uninstall() {
    echo "Uninstalling"
}
"""
    script_file = infra_dir / "manage.test.sh"
    script_file.write_text(script_content)

    comp_config = {"name": "test", "env": {}}
    result = component_processor.process_component(comp_config, infra_dir)

    assert len(result["include_section"]) > 0
    assert any("helper()" in line for line in result["include_section"])


def test_process_component_script_not_found(tmp_path):
    """Test processing component when script not found raises error."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    comp_config = {"name": "nonexistent", "env": {}}

    with pytest.raises(RuntimeError, match="Script not found"):
        component_processor.process_component(comp_config, infra_dir)


def test_process_component_missing_install_function(tmp_path):
    """Test processing component missing install() function raises error."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    script_content = """#!/bin/bash

uninstall() {
    echo "Uninstalling"
}
"""
    script_file = infra_dir / "manage.test.sh"
    script_file.write_text(script_content)

    comp_config = {"name": "test", "env": {}}

    with pytest.raises(RuntimeError, match="install.*not found"):
        component_processor.process_component(comp_config, infra_dir)


def test_process_component_missing_uninstall_function(tmp_path):
    """Test processing component missing uninstall() function raises error."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    script_content = """#!/bin/bash

install() {
    echo "Installing"
}
"""
    script_file = infra_dir / "manage.test.sh"
    script_file.write_text(script_content)

    comp_config = {"name": "test", "env": {}}

    with pytest.raises(RuntimeError, match="uninstall.*not found"):
        component_processor.process_component(comp_config, infra_dir)


def test_process_component_name_with_hyphens(tmp_path):
    """Test component with hyphens in name gets properly converted."""
    infra_dir = tmp_path / "infra"
    infra_dir.mkdir()

    script_content = """#!/bin/bash

# VARIABLES
# VARIABLES END

install() {
    echo "Installing"
}

uninstall() {
    echo "Uninstalling"
}
"""
    script_file = infra_dir / "manage.gateway-api-crd.sh"
    script_file.write_text(script_content)

    comp_config = {"name": "gateway-api-crd", "env": {}}
    result = component_processor.process_component(comp_config, infra_dir)

    # Hyphens should be converted to underscores in function names
    assert result["install_func"] == "install_gateway_api_crd"
    assert result["uninstall_func"] == "uninstall_gateway_api_crd"
