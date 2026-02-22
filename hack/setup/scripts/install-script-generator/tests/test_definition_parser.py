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
      LLMISVC: "true"
      NAMESPACE: "custom-ns"
"""
    definition_file = tmp_path / "test.definition"
    definition_file.write_text(definition_content)

    result = definition_parser.parse_definition(definition_file)

    assert len(result["components"]) == 1
    assert result["components"][0]["name"] == "kserve-helm"
    assert result["components"][0]["env"]["LLMISVC"] == "true"
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
