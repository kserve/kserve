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

"""Tests for bash_parser module."""

import unittest
import tempfile
from pathlib import Path
import sys

# Add parent directory to path to import modules
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import bash_parser  # noqa: E402


class TestBashParser(unittest.TestCase):
    """Test cases for bash_parser module."""

    def test_extract_bash_function_simple(self):
        """Test extracting simple bash function."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("install() {\n")
            f.write("    echo 'Installing...'\n")
            f.write("}\n")
            temp_path = Path(f.name)

        try:
            func_code = bash_parser.extract_bash_function(temp_path, "install")
            expected = "install() {\n    echo 'Installing...'\n}"
            self.assertEqual(func_code, expected)
        finally:
            temp_path.unlink()

    def test_extract_bash_function_nested_braces(self):
        """Test extracting function with nested braces."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("install() {\n")
            f.write("    if [ true ]; then\n")
            f.write("        echo 'nested'\n")
            f.write("    fi\n")
            f.write("}\n")
            temp_path = Path(f.name)

        try:
            func_code = bash_parser.extract_bash_function(temp_path, "install")
            self.assertIn("install() {", func_code)
            self.assertIn("if [ true ]; then", func_code)
            self.assertIn("}", func_code)
        finally:
            temp_path.unlink()

    def test_extract_bash_function_not_found(self):
        """Test extracting non-existent function."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("other_func() {\n")
            f.write("    echo 'test'\n")
            f.write("}\n")
            temp_path = Path(f.name)

        try:
            func_code = bash_parser.extract_bash_function(temp_path, "install")
            self.assertEqual(func_code, "")
        finally:
            temp_path.unlink()

    def test_extract_bash_function_multiple_functions(self):
        """Test extracting specific function when multiple exist."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("uninstall() {\n")
            f.write("    echo 'Uninstalling'\n")
            f.write("}\n")
            f.write("\n")
            f.write("install() {\n")
            f.write("    echo 'Installing'\n")
            f.write("}\n")
            temp_path = Path(f.name)

        try:
            func_code = bash_parser.extract_bash_function(temp_path, "install")
            self.assertIn("install() {", func_code)
            self.assertIn("Installing", func_code)
            self.assertNotIn("Uninstalling", func_code)
        finally:
            temp_path.unlink()

    def test_rename_bash_function_simple(self):
        """Test renaming bash function."""
        func_code = "install() {\n    echo 'test'\n}"
        renamed = bash_parser.rename_bash_function(func_code, "install", "install_istio")
        self.assertEqual(renamed, "install_istio() {\n    echo 'test'\n}")

    def test_rename_bash_function_empty(self):
        """Test renaming empty function."""
        renamed = bash_parser.rename_bash_function("", "install", "install_istio")
        self.assertEqual(renamed, "")

    def test_rename_bash_function_preserve_body(self):
        """Test that function body is preserved during rename."""
        func_code = "install() {\n    log_info 'Installing'\n    kubectl apply -f manifest.yaml\n}"
        renamed = bash_parser.rename_bash_function(func_code, "install", "install_component")
        self.assertIn("install_component() {", renamed)
        self.assertIn("log_info 'Installing'", renamed)
        self.assertIn("kubectl apply -f manifest.yaml", renamed)

    def test_rename_bash_function_with_function_calls(self):
        """Test renaming function calls within function body."""
        func_code = """install() {
    if [ "$REINSTALL" = true ]; then
        uninstall
    fi
    echo 'Installing'
}"""
        renamed = bash_parser.rename_bash_function(func_code, "install", "install_external_lb")
        self.assertIn("install_external_lb() {", renamed)
        # Original uninstall call should remain unchanged
        self.assertIn("uninstall", renamed)

    def test_rename_bash_function_rename_cross_function_calls(self):
        """Test renaming calls to other functions within function body."""
        func_code = """install() {
    if [ "$REINSTALL" = true ]; then
        uninstall
    fi
    echo 'Installing'
}"""
        # First rename install to install_external_lb
        renamed = bash_parser.rename_bash_function(func_code, "install", "install_external_lb")
        # Then rename uninstall calls to uninstall_external_lb
        renamed = bash_parser.rename_bash_function(renamed, "uninstall", "uninstall_external_lb")

        self.assertIn("install_external_lb() {", renamed)
        self.assertIn("uninstall_external_lb", renamed)
        # Original uninstall should be gone
        self.assertNotIn("uninstall\n", renamed)

    def test_rename_bash_function_word_boundary(self):
        """Test that rename respects word boundaries."""
        func_code = """install() {
    echo 'uninstall_old'
    uninstall
    echo 'reinstall'
}"""
        renamed = bash_parser.rename_bash_function(func_code, "uninstall", "uninstall_component")

        # Should rename standalone 'uninstall' call
        self.assertIn("uninstall_component", renamed)
        # Should NOT rename 'uninstall' when part of other words
        self.assertIn("uninstall_old", renamed)
        self.assertIn("reinstall", renamed)

    def test_rename_bash_function_preserve_command_arguments(self):
        """Test that rename doesn't affect function names used as command arguments."""
        func_code = """install() {
    if [ "$REINSTALL" = true ]; then
        uninstall
    fi
    helm install cert-manager jetstack/cert-manager
    kubectl apply -f install.yaml
    helm uninstall old-release
}"""
        renamed = bash_parser.rename_bash_function(func_code, "install", "install_cert_manager")

        # Should rename function definition
        self.assertIn("install_cert_manager() {", renamed)
        # uninstall call should remain unchanged (we're only renaming install)
        self.assertIn("uninstall", renamed)
        # Should NOT rename 'install' in 'helm install'
        self.assertIn("helm install cert-manager", renamed)
        # Should NOT rename 'install' in 'install.yaml'
        self.assertIn("kubectl apply -f install.yaml", renamed)

        # Now rename uninstall calls
        renamed = bash_parser.rename_bash_function(renamed, "uninstall", "uninstall_cert_manager")
        # Should rename standalone uninstall call
        self.assertIn("uninstall_cert_manager", renamed)
        # The standalone 'uninstall' at the start of a line should now be renamed
        self.assertIn("        uninstall_cert_manager", renamed)
        # But 'uninstall' as part of 'helm uninstall' should NOT be renamed
        self.assertIn("helm uninstall old-release", renamed)
        # The original standalone 'uninstall' should be gone
        self.assertNotIn("        uninstall\n", renamed)

    def test_deduplicate_variables_simple(self):
        """Test deduplicating variable declarations."""
        variables = [
            "NAMESPACE=kserve",
            "VERSION=1.0",
            "NAMESPACE=istio"
        ]
        result = bash_parser.deduplicate_variables(variables)
        self.assertEqual(result, ["NAMESPACE=kserve", "VERSION=1.0"])

    def test_deduplicate_variables_array_multi_line(self):
        """Test deduplicating multi-line array declaration."""
        variables = [
            "TARGET_POD_LABELS=(",
            '"control-plane=kserve-controller-manager"',
            '"app.kubernetes.io/name=kserve-localmodel-controller-manager"',
            '"app.kubernetes.io/name=llmisvc-controller-manager"',
            ")",
            "VERSION=1.0"
        ]
        result = bash_parser.deduplicate_variables(variables)
        self.assertEqual(len(result), 6)
        self.assertEqual(result[0], "TARGET_POD_LABELS=(")
        self.assertEqual(result[1], '"control-plane=kserve-controller-manager"')
        self.assertEqual(result[4], ")")
        self.assertEqual(result[5], "VERSION=1.0")

    def test_deduplicate_variables_duplicate_array(self):
        """Test deduplicating when array is declared twice."""
        variables = [
            "TARGET_POD_LABELS=(",
            '"label1"',
            ")",
            "VERSION=1.0",
            "TARGET_POD_LABELS=(",
            '"label2"',
            ")"
        ]
        result = bash_parser.deduplicate_variables(variables)
        # Should keep first array declaration only
        self.assertEqual(len(result), 4)
        self.assertEqual(result[0], "TARGET_POD_LABELS=(")
        self.assertEqual(result[1], '"label1"')
        self.assertEqual(result[2], ")")
        self.assertEqual(result[3], "VERSION=1.0")

    def test_extract_common_functions_with_markers(self):
        """Test extracting common functions section."""
        content = """#!/bin/bash
# Some header

# Utility Functions
function log_info() {
    echo "INFO: $1"
}

function wait_for_pods() {
    kubectl wait --for=condition=ready pods
}

# Auto-initialization
REPO_ROOT=$(find_repo_root)
"""
        result = bash_parser.extract_common_functions(content)
        self.assertIn("# Utility Functions", result)
        self.assertIn("function log_info()", result)
        self.assertIn("function wait_for_pods()", result)
        self.assertNotIn("# Auto-initialization", result)
        self.assertNotIn("REPO_ROOT=", result)

    def test_extract_common_functions_no_end_marker(self):
        """Test extracting when no end marker exists."""
        content = """#!/bin/bash
# Utility Functions
function test() {
    echo "test"
}
"""
        result = bash_parser.extract_common_functions(content)
        self.assertIn("# Utility Functions", result)
        self.assertIn("function test()", result)

    def test_extract_common_functions_no_markers(self):
        """Test extracting when no markers exist."""
        content = """#!/bin/bash
function test() {
    echo "test"
}
"""
        result = bash_parser.extract_common_functions(content)
        # Should return original content
        self.assertEqual(result, content)


if __name__ == '__main__':
    unittest.main()
