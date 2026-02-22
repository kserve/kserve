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

"""
Installation Script Convention Validator

Validates that installation scripts follow the required template structure
and conventions defined in SCRIPT_GUIDELINES.md.

This ensures scripts can be properly integrated into the quick-install
generator system.

Usage:
    # Validate all scripts
    ./validate-install-scripts.py

    # Validate specific files
    ./validate-install-scripts.py hack/setup/infra/manage.cert-manager-helm.sh

    # Validate all scripts in a directory
    ./validate-install-scripts.py hack/setup/infra

    # Validate multiple directories and files
    ./validate-install-scripts.py hack/setup/infra hack/setup/infra/knative

    # CI mode (exit with error if any validation fails)
    ./validate-install-scripts.py --strict

Exit codes:
    0 - All validations passed
    1 - Critical errors found (in strict mode)
    2 - Script usage error
"""

import sys
import re
from pathlib import Path
from dataclasses import dataclass
from typing import List, Optional
from enum import Enum


class Severity(Enum):
    ERROR = "❌ ERROR"
    WARNING = "⚠️  WARNING"
    INFO = "ℹ️  INFO"


@dataclass
class ValidationResult:
    rule_id: str
    severity: Severity
    message: str

    def __str__(self):
        return f"{self.severity.value:12} [{self.rule_id}] {self.message}"


class ScriptValidator:
    """Validates a single installation script"""

    def __init__(self, script_path: Path):
        self.script_path = script_path
        self.content = script_path.read_text()
        self.lines = self.content.split("\n")
        self.results: List[ValidationResult] = []

    def validate(self) -> List[ValidationResult]:
        """Run all validations"""
        self.validate_template_structure()
        self.validate_required_functions()
        self.validate_variables_section()
        self.validate_include_section()
        self.validate_standard_patterns()
        return self.results

    def add_error(self, rule_id: str, message: str):
        self.results.append(ValidationResult(rule_id, Severity.ERROR, message))

    def add_warning(self, rule_id: str, message: str):
        self.results.append(ValidationResult(rule_id, Severity.WARNING, message))

    def add_info(self, rule_id: str, message: str):
        self.results.append(ValidationResult(rule_id, Severity.INFO, message))

    # === Critical Validations (Template Structure) ===

    def validate_template_structure(self):
        """Validate INITIALIZE SECTION template structure"""
        begin_marker = "# INIT"
        end_marker = "# INIT END"

        # Check markers exist
        if begin_marker not in self.content:
            self.add_error("TEMPLATE-01", "Missing BEGIN INITIALIZE SECTION marker")
            return

        if end_marker not in self.content:
            self.add_error("TEMPLATE-02", "Missing END INITIALIZE SECTION marker")
            return

        # Extract section content
        begin_idx = self.content.index(begin_marker)
        end_idx = self.content.index(end_marker)
        section = self.content[begin_idx:end_idx]

        # Required patterns (allowing path variations for subdirectories)
        required_patterns = {
            "SCRIPT_DIR": r"SCRIPT_DIR=",
            "common.sh": r'source "\$\{SCRIPT_DIR\}/(\.\./)+common\.sh"',
            "REINSTALL": r'REINSTALL="\$\{REINSTALL:-false\}"',
            "UNINSTALL": r'UNINSTALL="\$\{UNINSTALL:-false\}"',
            "uninstall-check": r'if \[\[ "\$\*" == \*"--uninstall"\* \]\]',
            "reinstall-check": r'if \[\[ "\$\*" == \*"--reinstall"\* \]\]',
        }

        for name, pattern in required_patterns.items():
            if not re.search(pattern, section):
                self.add_error("TEMPLATE-03", f"INITIALIZE SECTION missing required element: {name}")

    def validate_required_functions(self):
        """Validate install/uninstall functions exist"""
        install_pattern = r"^install\(\s*\)\s*\{"
        uninstall_pattern = r"^uninstall\(\s*\)\s*\{"

        has_install = any(re.match(install_pattern, line) for line in self.lines)
        has_uninstall = any(re.match(uninstall_pattern, line) for line in self.lines)

        if not has_install:
            self.add_error("FUNC-01", "Missing install() function")

        if not has_uninstall:
            self.add_error("FUNC-02", "Missing uninstall() function")

    def validate_variables_section(self):
        """Validate VARIABLES section structure"""
        begin_marker = "# VARIABLES"
        end_marker = "# VARIABLES END"

        has_begin = begin_marker in self.content
        has_end = end_marker in self.content

        if not has_begin and not has_end:
            # Optional section, just skip
            return

        if has_begin and not has_end:
            self.add_error("VAR-01", "VARIABLES section has start marker but missing end marker")
            return

        if not has_begin and has_end:
            self.add_error("VAR-02", "VARIABLES section has end marker but missing start marker")
            return

        # Both markers exist - validate content
        begin_idx = self.content.index(begin_marker)
        end_idx = self.content.index(end_marker)

        if begin_idx >= end_idx:
            self.add_error("VAR-03", "VARIABLES END marker appears before VARIABLES marker")
            return

        section = self.content[begin_idx:end_idx]

        # Check that VARIABLES section doesn't contain control flow (if/then/fi)
        # These should be in INCLUDE_IN_GENERATED_SCRIPT section instead
        if re.search(r'\bif\s*\[', section):
            self.add_warning("VAR-04", "VARIABLES section contains if statement - consider moving to INCLUDE_IN_GENERATED_SCRIPT section")

        self.add_info("VAR-INFO", "Has VARIABLES section ✓")

    def validate_include_section(self):
        """Validate INCLUDE_IN_GENERATED_SCRIPT section structure"""
        begin_marker = "# INCLUDE_IN_GENERATED_SCRIPT_START"
        end_marker = "# INCLUDE_IN_GENERATED_SCRIPT_END"

        has_begin = begin_marker in self.content
        has_end = end_marker in self.content

        if not has_begin and not has_end:
            # Optional section, just skip
            return

        if has_begin and not has_end:
            self.add_error("INC-01", "INCLUDE_IN_GENERATED_SCRIPT section has start marker but missing end marker")
            return

        if not has_begin and has_end:
            self.add_error("INC-02", "INCLUDE_IN_GENERATED_SCRIPT section has end marker but missing start marker")
            return

        # Both markers exist - validate position
        begin_idx = self.content.index(begin_marker)
        end_idx = self.content.index(end_marker)

        if begin_idx >= end_idx:
            self.add_error("INC-03", "INCLUDE_IN_GENERATED_SCRIPT_END marker appears before START marker")
            return

        # Check that it's after VARIABLES END
        if "# VARIABLES END" in self.content:
            var_end_idx = self.content.index("# VARIABLES END")
            if begin_idx < var_end_idx:
                self.add_warning("INC-04", "INCLUDE_IN_GENERATED_SCRIPT section should appear after VARIABLES END")

        self.add_info("INC-INFO", "Has INCLUDE_IN_GENERATED_SCRIPT section ✓")

    # === Code Quality Validations ===

    def validate_standard_patterns(self):
        """Informational checks for standard patterns"""
        # CLI dependency check
        if "check_cli_exist" in self.content:
            self.add_info("PATTERN-01", "Uses CLI dependency check ✓")

        # Logging functions
        log_funcs = ["log_info", "log_success", "log_error"]
        if any(func in self.content for func in log_funcs):
            self.add_info("PATTERN-02", "Uses standard logging functions ✓")


class ValidatorRunner:
    """Runs validation on multiple scripts"""

    def __init__(self, strict_mode: bool = False):
        self.strict_mode = strict_mode
        self.stats = {
            "total": 0,
            "passed": 0,
            "failed": 0,
            "errors": 0,
            "warnings": 0,
        }

    def find_scripts(self, path: Optional[Path] = None) -> List[Path]:
        """Find all installation scripts to validate"""
        if path and path.is_file():
            return [path]

        # Find all manage.*.sh in infra/
        script_dir = Path(__file__).parent.parent
        infra_dir = script_dir / "infra"

        if not infra_dir.exists():
            return []

        scripts = list(infra_dir.glob("manage.*.sh"))

        # Include subdirectories
        for subdir in infra_dir.iterdir():
            if subdir.is_dir():
                scripts.extend(subdir.glob("manage.*.sh"))

        return sorted(scripts)

    def validate_all(self, scripts: List[Path]):
        """Validate all scripts and print results"""
        print("=" * 70)
        print("KServe Installation Script Validator")
        print("=" * 70)
        print()

        for script in scripts:
            self.validate_one(script)

        self.print_summary()

        # Exit with appropriate code
        if self.strict_mode and self.stats["errors"] > 0:
            sys.exit(1)
        sys.exit(0)

    def validate_one(self, script_path: Path):
        """Validate a single script"""
        self.stats["total"] += 1

        # Show relative path
        try:
            rel_path = script_path.relative_to(Path.cwd())
        except ValueError:
            rel_path = script_path

        print(f"📄 {rel_path}")

        validator = ScriptValidator(script_path)
        results = validator.validate()

        # Separate by severity
        errors = [r for r in results if r.severity == Severity.ERROR]
        warnings = [r for r in results if r.severity == Severity.WARNING]
        infos = [r for r in results if r.severity == Severity.INFO]

        self.stats["errors"] += len(errors)
        self.stats["warnings"] += len(warnings)

        # Print results
        if errors:
            self.stats["failed"] += 1
            for r in errors:
                print(f"   {r}")
            for r in warnings:
                print(f"   {r}")
        else:
            self.stats["passed"] += 1
            if warnings or infos:
                for r in warnings:
                    print(f"   {r}")
                for r in infos:
                    print(f"   {r}")
            else:
                print("   ✅ All checks passed")

        print()

    def print_summary(self):
        """Print validation summary"""
        print("=" * 70)
        print("Summary")
        print("=" * 70)
        print(f"Total files:  {self.stats['total']}")
        print(f"✅ Passed:    {self.stats['passed']}")
        print(f"❌ Failed:    {self.stats['failed']}")
        print(f"Errors:       {self.stats['errors']}")
        print(f"Warnings:     {self.stats['warnings']}")
        print()

        if self.stats["failed"] == 0:
            print("🎉 All scripts follow conventions!")
        else:
            print("⚠️  Some scripts need fixes. See errors above.")
            if not self.strict_mode:
                print("   (Run with --strict to fail CI on errors)")


def main():
    import argparse

    parser = argparse.ArgumentParser(
        description="Validate installation script conventions",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("paths", nargs="*", help="Files or directories to validate (default: all manage.*.sh in infra/)")
    parser.add_argument("--strict", action="store_true", help="Exit with error code if any validation fails (for CI)")

    args = parser.parse_args()

    runner = ValidatorRunner(strict_mode=args.strict)

    if args.paths:
        scripts = []
        for path_str in args.paths:
            path = Path(path_str)
            if not path.exists():
                print(f"Error: Path not found: {path}", file=sys.stderr)
                sys.exit(2)

            if path.is_file():
                # Single file
                scripts.append(path)
            elif path.is_dir():
                # Directory: find all manage.*.sh files
                scripts.extend(path.glob("manage.*.sh"))
                # Also check subdirectories
                for subdir in path.iterdir():
                    if subdir.is_dir():
                        scripts.extend(subdir.glob("manage.*.sh"))
            else:
                print(f"Error: Invalid path: {path}", file=sys.stderr)
                sys.exit(2)

        scripts = sorted(set(scripts))  # Remove duplicates and sort
    else:
        scripts = runner.find_scripts()

    if not scripts:
        print("No scripts found to validate")
        sys.exit(2)

    runner.validate_all(scripts)


if __name__ == "__main__":
    main()
