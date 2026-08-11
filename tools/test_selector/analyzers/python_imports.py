"""Python import analysis for intra-repo dependencies."""

from __future__ import annotations

import ast
import sys
from pathlib import Path

from ..mapping.schema import PythonPackageInfo

_SKIP_DIRS = {"__pycache__", ".pytest_cache", ".venv", "venv", "node_modules", ".git"}


def discover_python_packages(repo_root: Path) -> dict[str, PythonPackageInfo]:
    """Walk python/ directory and classify files into packages.

    A package is a direct subdirectory of python/ that contains a setup.py,
    setup.cfg, or pyproject.toml.
    """
    python_dir = repo_root / "python"
    if not python_dir.is_dir():
        return {}

    packages: dict[str, PythonPackageInfo] = {}

    for sub in sorted(python_dir.iterdir()):
        if not sub.is_dir() or sub.name in _SKIP_DIRS or sub.name.startswith("."):
            continue

        has_setup = (
            (sub / "setup.py").exists()
            or (sub / "setup.cfg").exists()
            or (sub / "pyproject.toml").exists()
        )
        if not has_setup:
            continue

        pkg_name = sub.name
        rel_path = str(sub.relative_to(repo_root))

        py_files = []
        for f in sorted(sub.rglob("*.py")):
            if any(part in _SKIP_DIRS for part in f.parts):
                continue
            py_files.append(str(f.relative_to(repo_root)))

        packages[pkg_name] = PythonPackageInfo(
            name=pkg_name,
            path=rel_path,
            files=py_files,
            intra_repo_imports=[],
        )

    return packages


def build_python_file_to_package(
    packages: dict[str, PythonPackageInfo],
) -> dict[str, str]:
    """Map each Python file to its package name."""
    result: dict[str, str] = {}
    for pkg_name, pkg_info in packages.items():
        for f in pkg_info.files:
            result[f] = pkg_name
    return result


def analyze_python_imports(
    repo_root: Path,
    packages: dict[str, PythonPackageInfo],
) -> None:
    """Parse Python imports and find intra-repo dependencies between packages."""
    known_modules: dict[str, str] = {}
    for pkg_name, pkg_info in packages.items():
        known_modules[pkg_name] = pkg_name
        pkg_dir = repo_root / pkg_info.path
        for sub in pkg_dir.iterdir():
            if sub.is_dir() and (sub / "__init__.py").exists():
                known_modules[sub.name] = pkg_name

    for pkg_name, pkg_info in packages.items():
        imports_set: set[str] = set()
        for f in pkg_info.files:
            file_path = repo_root / f
            if not file_path.exists():
                continue
            try:
                tree = ast.parse(file_path.read_text(), filename=f)
            except SyntaxError:
                print(f"  Warning: SyntaxError in {f}", file=sys.stderr)
                continue

            for node in ast.walk(tree):
                if isinstance(node, ast.Import):
                    for alias in node.names:
                        top = alias.name.split(".")[0]
                        if top in known_modules and known_modules[top] != pkg_name:
                            imports_set.add(known_modules[top])
                elif isinstance(node, ast.ImportFrom):
                    if node.module:
                        top = node.module.split(".")[0]
                        if top in known_modules and known_modules[top] != pkg_name:
                            imports_set.add(known_modules[top])

        pkg_info.intra_repo_imports = sorted(imports_set)
