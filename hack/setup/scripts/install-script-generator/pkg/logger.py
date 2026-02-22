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

"""Logging utilities with color support."""

import sys


class Colors:
    """ANSI color codes for terminal output."""
    BLUE = "\033[94m"
    GREEN = "\033[92m"
    RED = "\033[91m"
    RESET = "\033[0m"


def log_info(msg: str):
    """Log informational message in blue.

    Args:
        msg: Message to log
    """
    print(f"{Colors.BLUE}[INFO]{Colors.RESET} {msg}")


def log_success(msg: str):
    """Log success message in green.

    Args:
        msg: Message to log
    """
    print(f"{Colors.GREEN}[SUCCESS]{Colors.RESET} {msg}")


def log_error(msg: str):
    """Log error message in red to stderr.

    Args:
        msg: Message to log
    """
    print(f"{Colors.RED}[ERROR]{Colors.RESET} {msg}", file=sys.stderr)
