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

"""Bash script parsing and function extraction utilities."""

import re
from pathlib import Path


def extract_bash_function(script_file: Path, func_name: str) -> str:
    """Extract bash function from script file.

    Args:
        script_file: Path to bash script
        func_name: Function name to extract (e.g., "install", "uninstall")

    Returns:
        Complete function code as string (empty if not found)
    """
    lines = []
    in_function = False
    brace_count = 0

    with open(script_file) as f:
        for line in f:
            if line.startswith(f"{func_name}() {{"):
                in_function = True
                brace_count = 0

            if in_function:
                lines.append(line.rstrip())
                brace_count += line.count("{") - line.count("}")
                if brace_count == 0 and len(lines) > 1:
                    break

    return "\n".join(lines)


def rename_bash_function(func_code: str, old_name: str, new_name: str) -> str:
    """Rename bash function and its calls within the code.

    Args:
        func_code: Original function code
        old_name: Current function name
        new_name: New function name

    Returns:
        Function code with renamed function definition and calls
    """
    if not func_code:
        return func_code

    lines = func_code.split('\n')

    # Rename function definition
    if lines and lines[0].startswith(f"{old_name}() {{"):
        lines[0] = f"{new_name}() {{"

    # Rename function calls within the body
    # Pattern matches function calls that appear:
    # - At start of line (with optional leading whitespace)
    # - After command separators: ; | & ( etc. (with optional whitespace)
    # But NOT after other commands (like 'helm install' or 'kubectl apply')
    # The pattern captures the prefix separately to preserve it
    pattern = re.compile(
        r'(^[ \t]*|[;|&(][ \t]*)(' + re.escape(old_name) + r')\b(?=\s|$|;|\||&|\))'
    )

    def replace_func(match):
        # Preserve the prefix (whitespace/separator) and replace only the function name
        return match.group(1) + new_name

    for i in range(1, len(lines)):  # Skip first line (function definition)
        lines[i] = pattern.sub(replace_func, lines[i])

    return '\n'.join(lines)


def deduplicate_variables(variables: list[str]) -> list[str]:
    """Remove duplicate variable declarations.

    Keeps first occurrence of each variable name.
    Handles multi-line array declarations (e.g., VAR=( ... )).

    Args:
        variables: List of variable declarations (e.g., ["VAR1=value", "VAR2=value"])

    Returns:
        List with duplicates removed
    """
    seen = set()
    result = []
    in_array = None  # Track if we're inside an array declaration

    for var in variables:
        # Check if this is the start of a variable declaration
        match = re.match(r"^([A-Z_][A-Z0-9_]*)=", var)

        if match:
            var_name = match.group(1)
            if var_name not in seen:
                seen.add(var_name)
                result.append(var)
                # Check if this starts an array declaration
                if '=(' in var and ')' not in var:
                    in_array = var_name
            elif in_array == var_name:
                # Skip duplicate array declaration start
                in_array = None
        elif in_array:
            # This is a continuation of an array (indented lines or closing paren)
            result.append(var)
            # Check if array ends
            if ')' in var:
                in_array = None

    return result


def inject_code_into_function(func_code: str, code_to_inject: str) -> str:
    """Inject code at the beginning of a bash function body.

    Args:
        func_code: Original function code
        code_to_inject: Code to inject at the start of function body

    Returns:
        Function code with injected code
    """
    if not func_code or not code_to_inject:
        return func_code

    lines = func_code.split('\n')
    if not lines:
        return func_code

    # Find the opening brace line (e.g., "install_kserve_helm() {")
    if not lines[0].endswith('{'):
        return func_code

    # Insert code after the opening brace
    # Preserve indentation of first real line if exists
    indent = "    "  # Default 4 spaces
    if len(lines) > 1 and lines[1]:
        # Extract indentation from first line
        match = re.match(r'^(\s*)', lines[1])
        if match:
            indent = match.group(1)

    # Indent the injected code
    injected_lines = code_to_inject.split('\n')
    indented_injection = '\n'.join(indent + line if line.strip() else line
                                   for line in injected_lines)

    # Insert after opening brace
    result = [lines[0], indented_injection] + lines[1:]
    return '\n'.join(result)


def extract_common_functions(content: str) -> str:
    """Extract utility functions from common.sh.

    Extracts content between '# Utility Functions' and '# Auto-initialization'.

    Args:
        content: Content of common.sh file

    Returns:
        Extracted utility functions section
    """
    lines = content.split('\n')

    start_idx = next((i for i, line in enumerate(lines)
                     if '# Utility Functions' in line), None)
    end_idx = next((i for i, line in enumerate(lines)
                   if '# Auto-initialization' in line), None)

    if start_idx is not None:
        return '\n'.join(lines[start_idx:end_idx] if end_idx else lines[start_idx:])

    return content
