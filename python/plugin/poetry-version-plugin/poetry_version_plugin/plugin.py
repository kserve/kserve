# Copyright 2023 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import ast
import subprocess
from pathlib import Path
from typing import Any, Dict, Optional

from cleo.io.io import IO
from poetry.plugins.plugin import Plugin
from poetry.poetry import Poetry
from poetry.core.utils.helpers import module_name


class VersionPlugin(Plugin):  # type: ignore
    def activate(self, poetry: Poetry, io: IO) -> None:
        poetry_version_config: Optional[Dict[str, Any]] = poetry.pyproject.data.get(
            "tool", {}
        ).get("poetry-version-plugin")
        if poetry_version_config is None:
            return
        version_source = poetry_version_config.get("source")
        version_file_path = poetry_version_config.get("file_path")
        if not version_source:
            message = (
                "<b>poetry-version-plugin</b>: No <b>source</b> configuration found in "
                "[tool.poetry-version-plugin] in pyproject.toml, not extracting "
                "dynamic version"
            )
            io.write_error_line(message)
            raise RuntimeError(message)

        # Update version using file
        if version_source == "file" and version_file_path:
            with open(version_file_path, "r") as version_file:
                version = version_file.read().strip()
                poetry.package._set_version(version)
                io.write_line(
                    "<b>poetry-version-plugin</b>: New package "
                    "version is updated from given file "
                    f"version is: <b>{version}</b>"
                )
                content: dict[str, Any] = poetry.file.read()
                poetry_content = content["tool"]["poetry"]
                poetry_content["version"] = version
                poetry.file.write(content)
                io.write_line(
                    "<b>poetry-version-plugin</b>: New version updated in toml file"
                )

        if version_source == "init":
            packages = poetry.local_config.get("packages")
            if packages:
                if len(packages) == 1:
                    package_name = packages[0]["include"]
                else:
                    message = (
                        "<b>poetry-version-plugin</b>: More than one package set, "
                        "cannot extract dynamic version"
                    )
                    io.write_error_line(message)
                    raise RuntimeError(message)
            else:
                package_name = module_name(poetry.package.name)
            init_path = Path(package_name) / "__init__.py"
            if not init_path.is_file():
                message = (
                    "<b>poetry-version-plugin</b>: __init__.py file not found at "
                    f"{init_path} cannot extract dynamic version"
                )
                io.write_error_line(message)
                raise RuntimeError(message)
            else:
                io.write_line(
                    "<b>poetry-version-plugin</b>: Using __init__.py file at "
                    f"{init_path} for dynamic version"
                )
            tree = ast.parse(init_path.read_text())
            for el in tree.body:
                if isinstance(el, ast.Assign):
                    if len(el.targets) == 1:
                        target = el.targets[0]
                        if isinstance(target, ast.Name):
                            if target.id == "__version__":
                                value_node = el.value
                                if isinstance(value_node, ast.Constant):
                                    version = value_node.value
                                elif isinstance(value_node, ast.Str):
                                    version = value_node.s
                                else:  # pragma: nocover
                                    # This is actually covered by tests, but can't be
                                    # reported by Coverage
                                    # Ref: https://github.com/nedbat/coveragepy/issues/198
                                    continue
                                io.write_line(
                                    "<b>poetry-version-plugin</b>: Setting package "
                                    "dynamic version to __version__ "
                                    f"variable from __init__.py: <b>{version}</b>"
                                )
                                poetry.package._set_version(version)
                                return
            message = (
                "<b>poetry-version-plugin</b>: No valid __version__ variable found "
                "in __init__.py, cannot extract dynamic version"
            )
            io.write_error_line(message)
            raise RuntimeError(message)
        elif version_source == "git-tag":
            result = subprocess.run(
                ["git", "describe", "--exact-match", "--tags", "HEAD"],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                universal_newlines=True,
            )
            if result.returncode == 0:
                tag = result.stdout.strip()
                io.write_line(
                    "<b>poetry-version-plugin</b>: Git tag found, setting "
                    f"dynamic version to: {tag}"
                )
                poetry.package._set_version(tag)
                return
            else:
                message = (
                    "<b>poetry-version-plugin</b>: No Git tag found, not "
                    "extracting dynamic version"
                )
                io.write_error_line(message)
                raise RuntimeError(message)
