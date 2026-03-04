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

"""Tests for definition_parser module."""

import sys
from pathlib import Path
import pytest

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import definition_parser  # noqa: E402


def test_parse_definition_basic(tmp_path):
    """Test parsing basic definition file."""
    definition_content = """
FILE_NAME: test-install
DESCRIPTION: Test installation
METHOD: helm
EMBED_MANIFESTS: false
COMPONENTS:
  - cert-manager
  - gateway-api-crd
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert result["file_name"] == "test-install"
    assert result["description"] == "Test installation"
    assert result["method"] == "helm"
    assert result["embed_manifests"] is False
    assert len(result["components"]) == 2
    assert result["components"][0] == {"name": "cert-manager", "env": {}}
    assert result["components"][1] == {"name": "gateway-api-crd", "env": {}}


def test_parse_definition_with_env(tmp_path):
    """Test parsing definition with component env variables."""
    definition_content = """
FILE_NAME: test-install
COMPONENTS:
  - name: kserve-helm
    env:
      ENABLE_LLMISVC: "true"
      NAMESPACE: "custom-ns"
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert len(result["components"]) == 1
    assert result["components"][0]["name"] == "kserve-helm"
    assert result["components"][0]["env"]["ENABLE_LLMISVC"] == "true"
    assert result["components"][0]["env"]["NAMESPACE"] == "custom-ns"


def test_parse_definition_with_global_env(tmp_path):
    """Test parsing definition with global env."""
    definition_content = """
FILE_NAME: test-install
GLOBAL_ENV:
  KSERVE_NAMESPACE: kserve
  CERT_MANAGER_VERSION: v1.13.0
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert result["global_env"]["KSERVE_NAMESPACE"] == "kserve"
    assert result["global_env"]["CERT_MANAGER_VERSION"] == "v1.13.0"


def test_parse_definition_with_tools(tmp_path):
    """Test parsing definition with tools."""
    definition_content = """
FILE_NAME: test-install
TOOLS:
  - kubectl
  - helm
  - kustomize
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert result["tools"] == ["kubectl", "helm", "kustomize"]


def test_parse_definition_embed_manifests_true(tmp_path):
    """Test parsing definition with EMBED_MANIFESTS=true."""
    definition_content = """
FILE_NAME: test-install
EMBED_MANIFESTS: true
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert result["embed_manifests"] is True


def test_parse_definition_missing_components(tmp_path):
    """Test parsing definition without COMPONENTS raises error."""
    definition_content = """
FILE_NAME: test-install
DESCRIPTION: Test
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    with pytest.raises(ValueError, match="COMPONENTS not found"):
        definition_parser.parse_definition(definition_file)


def test_parse_definition_default_values(tmp_path):
    """Test parsing definition uses default values."""
    definition_content = """
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    # FILE_NAME defaults to stem
    assert result["file_name"] == "test"
    # DESCRIPTION has default
    assert result["description"] == "Install infrastructure components"
    # METHOD defaults to helm
    assert result["method"] == "helm"
    # EMBED_MANIFESTS defaults to False
    assert result["embed_manifests"] is False
    # RELEASE defaults to False
    assert result["release"] is False
    # TOOLS defaults to empty list
    assert result["tools"] == []
    # GLOBAL_ENV defaults to empty dict
    assert result["global_env"] == {}


def test_parse_definition_release_true(tmp_path):
    """Test parsing definition with RELEASE=true."""
    definition_content = """
FILE_NAME: test-install
RELEASE: true
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert result["release"] is True


# ============================================================================
# Unit Tests for Helper Functions
# ============================================================================

def test_resolve_definition_path_relative(tmp_path):
    """Test resolving relative path."""
    base_file = tmp_path / "subdir" / "main.definition"
    base_file.parent.mkdir(parents=True)
    base_file.touch()

    result = definition_parser.resolve_definition_path("./other.definition", base_file)
    expected = (tmp_path / "subdir" / "other.definition").resolve()

    assert result == expected


def test_resolve_definition_path_parent_dir(tmp_path):
    """Test resolving parent directory path."""
    base_file = tmp_path / "subdir" / "main.definition"
    base_file.parent.mkdir(parents=True)
    base_file.touch()

    result = definition_parser.resolve_definition_path("../common/base.definition", base_file)
    expected = (tmp_path / "common" / "base.definition").resolve()

    assert result == expected


def test_resolve_definition_path_absolute(tmp_path):
    """Test resolving absolute path."""
    base_file = tmp_path / "main.definition"
    base_file.touch()

    absolute_path = "/absolute/path/to/file.definition"
    result = definition_parser.resolve_definition_path(absolute_path, base_file)

    assert result == Path(absolute_path)


def test_merge_tools_no_duplicates():
    """Test merging tools without duplicates."""
    base = ["helm", "kubectl"]
    new = ["kustomize", "yq"]

    result = definition_parser.merge_tools(base, new)

    assert result == ["helm", "kubectl", "kustomize", "yq"]


def test_merge_tools_with_duplicates():
    """Test merging tools with duplicates (last-wins)."""
    base = ["helm", "kubectl", "yq"]
    new = ["helm", "kustomize"]  # helm is duplicate

    result = definition_parser.merge_tools(base, new)

    # helm should be removed from base and added at end
    assert result == ["kubectl", "yq", "helm", "kustomize"]


def test_merge_tools_case_insensitive():
    """Test merging tools with case-insensitive duplicate detection."""
    base = ["Helm", "kubectl"]
    new = ["helm", "kustomize"]  # Helm vs helm

    result = definition_parser.merge_tools(base, new)

    # Helm should be treated as duplicate of helm
    assert result == ["kubectl", "helm", "kustomize"]


def test_merge_components_no_duplicates():
    """Test merging components without duplicates."""
    base = [
        {"name": "cert-manager", "env": {}},
        {"name": "istio", "env": {}}
    ]
    new = [
        {"name": "kserve-helm", "env": {}}
    ]

    result = definition_parser.merge_components(base, new)

    assert len(result) == 3
    assert result[0]["name"] == "cert-manager"
    assert result[1]["name"] == "istio"
    assert result[2]["name"] == "kserve-helm"


def test_merge_components_with_duplicates():
    """Test merging components with duplicates (last-wins)."""
    base = [
        {"name": "cert-manager", "env": {}},
        {"name": "kserve-helm", "env": {"NAMESPACE": "kserve"}},
        {"name": "istio", "env": {}}
    ]
    new = [
        {"name": "kserve-helm", "env": {"NAMESPACE": "custom"}}  # duplicate - override
    ]

    result = definition_parser.merge_components(base, new)

    # kserve-helm should be removed from base and new one added at end
    assert len(result) == 3
    assert result[0]["name"] == "cert-manager"
    assert result[1]["name"] == "istio"
    assert result[2]["name"] == "kserve-helm"
    assert result[2]["env"]["NAMESPACE"] == "custom"  # new env value


def test_merge_components_overwrites_env():
    """Test that merging components overwrites env (last-wins)."""
    base = [
        {"name": "component-a", "env": {"VAR1": "base-value", "VAR2": "keep"}}
    ]
    new = [
        {"name": "component-a", "env": {"VAR1": "new-value", "VAR3": "added"}}
    ]

    result = definition_parser.merge_components(base, new)

    assert len(result) == 1
    assert result[0]["name"] == "component-a"
    # Entire component is replaced - env is not merged, but replaced
    assert result[0]["env"] == {"VAR1": "new-value", "VAR3": "added"}


# ============================================================================
# Integration Tests for INCLUDE_DEFINITIONS
# ============================================================================

def test_include_single_file(tmp_path):
    """Test including a single definition file."""
    # Create base definition
    base_content = """
COMPONENTS:
  - cert-manager
  - istio
TOOLS:
  - helm
  - kubectl
"""
    base_file = tmp_path / "base.definition"
    base_file.write_text(base_content)

    # Create main definition that includes base
    main_content = """
INCLUDE_DEFINITIONS:
  - ./base.definition
COMPONENTS:
  - kserve-helm
TOOLS:
  - kustomize
"""
    main_file = tmp_path / "main.definition"
    main_file.write_text(main_content)

    result = definition_parser.parse_definition(main_file)

    # Tools should be merged
    assert result["tools"] == ["helm", "kubectl", "kustomize"]

    # Components should be merged
    assert len(result["components"]) == 3
    assert result["components"][0]["name"] == "cert-manager"
    assert result["components"][1]["name"] == "istio"
    assert result["components"][2]["name"] == "kserve-helm"


def test_include_multiple_files(tmp_path):
    """Test including multiple definition files."""
    # Create first base
    base1_content = """
COMPONENTS:
  - cert-manager
TOOLS:
  - helm
"""
    base1_file = tmp_path / "base1.definition"
    base1_file.write_text(base1_content)

    # Create second base
    base2_content = """
COMPONENTS:
  - istio
TOOLS:
  - kubectl
"""
    base2_file = tmp_path / "base2.definition"
    base2_file.write_text(base2_content)

    # Create main definition
    main_content = """
INCLUDE_DEFINITIONS:
  - ./base1.definition
  - ./base2.definition
COMPONENTS:
  - kserve-helm
TOOLS:
  - kustomize
"""
    main_file = tmp_path / "main.definition"
    main_file.write_text(main_content)

    result = definition_parser.parse_definition(main_file)

    # Check merged results
    assert result["tools"] == ["helm", "kubectl", "kustomize"]
    assert len(result["components"]) == 3
    assert [c["name"] for c in result["components"]] == ["cert-manager", "istio", "kserve-helm"]


def test_include_nested_files(tmp_path):
    """Test nested includes (A includes B, B includes C)."""
    # Create deepest file
    c_content = """
COMPONENTS:
  - cert-manager
"""
    c_file = tmp_path / "c.definition"
    c_file.write_text(c_content)

    # Create middle file
    b_content = """
INCLUDE_DEFINITIONS:
  - ./c.definition
COMPONENTS:
  - istio
"""
    b_file = tmp_path / "b.definition"
    b_file.write_text(b_content)

    # Create top file
    a_content = """
INCLUDE_DEFINITIONS:
  - ./b.definition
COMPONENTS:
  - kserve-helm
"""
    a_file = tmp_path / "a.definition"
    a_file.write_text(a_content)

    result = definition_parser.parse_definition(a_file)

    # Check that all components are included in order
    assert len(result["components"]) == 3
    assert [c["name"] for c in result["components"]] == ["cert-manager", "istio", "kserve-helm"]


def test_include_cycle_detection(tmp_path):
    """Test that circular dependencies are detected."""
    # Create A that includes B
    a_content = """
INCLUDE_DEFINITIONS:
  - ./b.definition
COMPONENTS:
  - component-a
"""
    a_file = tmp_path / "a.definition"
    a_file.write_text(a_content)

    # Create B that includes A (circular)
    b_content = """
INCLUDE_DEFINITIONS:
  - ./a.definition
COMPONENTS:
  - component-b
"""
    b_file = tmp_path / "b.definition"
    b_file.write_text(b_content)

    # Should raise ValueError about circular dependency
    with pytest.raises(ValueError, match="Circular dependency detected"):
        definition_parser.parse_definition(a_file)


def test_include_file_not_found(tmp_path):
    """Test error when included file doesn't exist."""
    main_content = """
INCLUDE_DEFINITIONS:
  - ./nonexistent.definition
COMPONENTS:
  - cert-manager
"""
    main_file = tmp_path / "main.definition"
    main_file.write_text(main_content)

    with pytest.raises(ValueError, match="Included definition file not found"):
        definition_parser.parse_definition(main_file)


def test_include_with_component_env_override(tmp_path):
    """Test that component env can be overridden (last-wins)."""
    # Create base with env
    base_content = """
COMPONENTS:
  - name: kserve-helm
    env:
      NAMESPACE: kserve
      VERSION: v1.0
"""
    base_file = tmp_path / "base.definition"
    base_file.write_text(base_content)

    # Create main that overrides env
    main_content = """
INCLUDE_DEFINITIONS:
  - ./base.definition
COMPONENTS:
  - name: kserve-helm
    env:
      NAMESPACE: custom
      DEPLOY: "true"
"""
    main_file = tmp_path / "main.definition"
    main_file.write_text(main_content)

    result = definition_parser.parse_definition(main_file)

    # Should have only one kserve-helm component with new env
    assert len(result["components"]) == 1
    assert result["components"][0]["name"] == "kserve-helm"
    assert result["components"][0]["env"] == {"NAMESPACE": "custom", "DEPLOY": "true"}


def test_empty_include_list(tmp_path):
    """Test that empty INCLUDE_DEFINITIONS list works."""
    content = """
INCLUDE_DEFINITIONS: []
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(content)

    result = definition_parser.parse_definition(definition_file)

    assert len(result["components"]) == 1
    assert result["components"][0]["name"] == "cert-manager"


def test_no_include_field(tmp_path):
    """Test that missing INCLUDE_DEFINITIONS field works (backward compatibility)."""
    content = """
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(content)

    result = definition_parser.parse_definition(definition_file)

    assert len(result["components"]) == 1
    assert result["components"][0]["name"] == "cert-manager"


def test_include_invalid_type(tmp_path):
    """Test that INCLUDE_DEFINITIONS must be a list."""
    content = """
INCLUDE_DEFINITIONS: "not-a-list"
COMPONENTS:
  - cert-manager
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(content)

    with pytest.raises(ValueError, match="INCLUDE_DEFINITIONS must be a list"):
        definition_parser.parse_definition(definition_file)
