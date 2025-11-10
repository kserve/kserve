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

"""Tests for file_reader module."""

import unittest
import tempfile
import yaml
from pathlib import Path
import sys

# Add parent directory to path to import modules
sys.path.insert(0, str(Path(__file__).parent.parent))

from pkg import file_reader  # noqa: E402


class TestFileReader(unittest.TestCase):
    """Test cases for file_reader module."""

    def test_find_git_root_success(self):
        """Test finding git root from current directory."""
        # Should find the kserve repo root
        current_dir = Path(__file__).parent
        repo_root = file_reader.find_git_root(current_dir)
        self.assertTrue((repo_root / ".git").exists())

    def test_find_git_root_failure(self):
        """Test finding git root from non-git directory."""
        with self.assertRaises(RuntimeError):
            file_reader.find_git_root(Path("/"))

    def test_read_env_file_basic(self):
        """Test reading basic env file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.env', delete=False) as f:
            f.write("VAR1=value1\n")
            f.write("VAR2=value2\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.read_env_file(temp_path)
            self.assertEqual(lines, ["VAR1=value1", "VAR2=value2"])
        finally:
            temp_path.unlink()

    def test_read_env_file_with_comments(self):
        """Test reading env file with comments."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.env', delete=False) as f:
            f.write("# This is a comment\n")
            f.write("VAR1=value1\n")
            f.write("# Another comment\n")
            f.write("VAR2=value2\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.read_env_file(temp_path)
            self.assertEqual(lines, ["VAR1=value1", "VAR2=value2"])
        finally:
            temp_path.unlink()

    def test_read_env_file_skip_empty(self):
        """Test reading env file skipping empty lines."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.env', delete=False) as f:
            f.write("VAR1=value1\n")
            f.write("\n")
            f.write("VAR2=value2\n")
            f.write("\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.read_env_file(temp_path)
            self.assertEqual(lines, ["VAR1=value1", "VAR2=value2"])
        finally:
            temp_path.unlink()

    def test_read_env_file_require_assignment(self):
        """Test reading env file requiring assignment."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.env', delete=False) as f:
            f.write("VAR1=value1\n")
            f.write("JUST_TEXT\n")
            f.write("VAR2=value2\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.read_env_file(temp_path, require_assignment=True)
            self.assertEqual(lines, ["VAR1=value1", "VAR2=value2"])
        finally:
            temp_path.unlink()

    def test_read_env_file_not_exists(self):
        """Test reading non-existent env file."""
        lines = file_reader.read_env_file(Path("/nonexistent.env"))
        self.assertEqual(lines, [])

    def test_read_yaml_file_simple(self):
        """Test reading simple YAML file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            yaml.dump({"key1": "value1", "key2": "value2"}, f)
            temp_path = Path(f.name)

        try:
            data = file_reader.read_yaml_file(temp_path)
            self.assertEqual(data, {"key1": "value1", "key2": "value2"})
        finally:
            temp_path.unlink()

    def test_read_yaml_file_nested(self):
        """Test reading nested YAML file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            data = {
                "COMPONENTS": ["comp1", "comp2"],
                "GLOBAL_ENV": {"VAR": "value"}
            }
            yaml.dump(data, f)
            temp_path = Path(f.name)

        try:
            result = file_reader.read_yaml_file(temp_path)
            self.assertEqual(result["COMPONENTS"], ["comp1", "comp2"])
            self.assertEqual(result["GLOBAL_ENV"]["VAR"], "value")
        finally:
            temp_path.unlink()

    def test_read_yaml_file_empty(self):
        """Test reading empty YAML file."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write("")
            temp_path = Path(f.name)

        try:
            data = file_reader.read_yaml_file(temp_path)
            self.assertEqual(data, {})
        finally:
            temp_path.unlink()

    def test_extract_marked_section_simple(self):
        """Test extracting marked section."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("# VARIABLES\n")
            f.write("VAR1=value1\n")
            f.write("VAR2=value2\n")
            f.write("# VARIABLES END\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.extract_marked_section(
                temp_path,
                "# VARIABLES",
                "# VARIABLES END"
            )
            self.assertEqual(lines, ["VAR1=value1", "VAR2=value2"])
        finally:
            temp_path.unlink()

    def test_extract_marked_section_preserve_indent(self):
        """Test extracting marked section with preserved indentation."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("# START\n")
            f.write("    indented line 1\n")
            f.write("    indented line 2\n")
            f.write("# END\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.extract_marked_section(
                temp_path,
                "# START",
                "# END",
                preserve_indent=True
            )
            self.assertEqual(lines, ["    indented line 1", "    indented line 2"])
        finally:
            temp_path.unlink()

    def test_extract_marked_section_skip_empty(self):
        """Test extracting marked section skipping empty lines."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("# START\n")
            f.write("line1\n")
            f.write("\n")
            f.write("line2\n")
            f.write("# END\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.extract_marked_section(
                temp_path,
                "# START",
                "# END",
                skip_empty=True
            )
            self.assertEqual(lines, ["line1", "line2"])
        finally:
            temp_path.unlink()

    def test_extract_marked_section_not_found(self):
        """Test extracting marked section when markers not found."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.sh', delete=False) as f:
            f.write("some content\n")
            temp_path = Path(f.name)

        try:
            lines = file_reader.extract_marked_section(
                temp_path,
                "# NOTFOUND",
                "# NOTFOUND END"
            )
            self.assertEqual(lines, [])
        finally:
            temp_path.unlink()


if __name__ == '__main__':
    unittest.main()
