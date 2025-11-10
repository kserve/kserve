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

"""Tests for template_engine module."""

import unittest
import tempfile
from pathlib import Path
import sys

# Add parent directory to path to import modules
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import template_engine  # noqa: E402


class TestTemplateEngine(unittest.TestCase):
    """Test cases for template_engine module."""

    def test_replace_placeholders_simple(self):
        """Test simple placeholder replacement."""
        template = "Hello {{NAME}}, version {{VERSION}}"
        replacements = {"NAME": "World", "VERSION": "1.0"}
        result = template_engine.replace_placeholders(template, replacements)
        self.assertEqual(result, "Hello World, version 1.0")

    def test_replace_placeholders_multiple(self):
        """Test multiple occurrences of same placeholder."""
        template = "{{NAME}} says {{NAME}} again"
        replacements = {"NAME": "Test"}
        result = template_engine.replace_placeholders(template, replacements)
        self.assertEqual(result, "Test says Test again")

    def test_replace_placeholders_empty(self):
        """Test replacement with empty value."""
        template = "Value: {{VALUE}}"
        replacements = {"VALUE": ""}
        result = template_engine.replace_placeholders(template, replacements)
        self.assertEqual(result, "Value: ")

    def test_replace_placeholders_no_match(self):
        """Test template without matching placeholders."""
        template = "No placeholders here"
        replacements = {"NAME": "Test"}
        result = template_engine.replace_placeholders(template, replacements)
        self.assertEqual(result, "No placeholders here")

    def test_replace_placeholders_partial(self):
        """Test partial placeholder replacement."""
        template = "{{FOUND}} and {{NOT_REPLACED}}"
        replacements = {"FOUND": "Value"}
        result = template_engine.replace_placeholders(template, replacements)
        self.assertEqual(result, "Value and {{NOT_REPLACED}}")

    def test_read_template(self):
        """Test reading template from file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.template', delete=False) as f:
            f.write("Test template {{VAR}}")
            temp_path = Path(f.name)

        try:
            content = template_engine.read_template(temp_path)
            self.assertEqual(content, "Test template {{VAR}}")
        finally:
            temp_path.unlink()

    def test_read_template_not_found(self):
        """Test reading non-existent template file."""
        with self.assertRaises(FileNotFoundError):
            template_engine.read_template(Path("/nonexistent/template.txt"))

    def test_generate_from_template(self):
        """Test complete template generation."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.template', delete=False) as f:
            f.write("Name: {{NAME}}\nVersion: {{VERSION}}")
            temp_path = Path(f.name)

        try:
            replacements = {"NAME": "TestApp", "VERSION": "2.0"}
            result = template_engine.generate_from_template(temp_path, replacements)
            self.assertEqual(result, "Name: TestApp\nVersion: 2.0")
        finally:
            temp_path.unlink()


if __name__ == '__main__':
    unittest.main()
