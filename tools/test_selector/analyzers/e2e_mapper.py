"""AST-parse e2e test files to extract CRD types, frameworks, and markers."""

from __future__ import annotations

import ast
import sys
from pathlib import Path

import re

from ..mapping.schema import TestFileInfo
from ..selector.rules import ALL_CRD_KINDS

_VERSION_PREFIX_RE = re.compile(r"^V(1(?:alpha|beta)\d+)(.+)$")

_COMPONENT_KWARG_NAMES = {"predictor", "transformer", "explainer"}

_PREDICTOR_SPEC_NAMES = {"V1beta1PredictorSpec"}
_ISVC_SPEC_NAMES = {"V1beta1InferenceServiceSpec"}

_SKIP_DIRS = {"__pycache__", ".pytest_cache", "data"}

_SKIP_MARKERS = {
    "asyncio",
    "parametrize",
    "skip",
    "skipif",
    "xfail",
    "usefixtures",
    "filterwarnings",
    "timeout",
}


class _E2EVisitor(ast.NodeVisitor):
    """Walk a test file's AST to extract CRD usage and markers."""

    def __init__(self, framework_kwarg_names: set[str]) -> None:
        self.markers: set[str] = set()
        self.crd_kinds: set[str] = set()
        self.crd_versions: set[str] = set()
        self.frameworks: set[str] = set()
        self.model_formats: set[str] = set()
        self.components: set[str] = set()
        self._framework_kwarg_names = framework_kwarg_names

    def visit_FunctionDef(self, node: ast.FunctionDef) -> None:
        self._extract_markers(node.decorator_list)
        self.generic_visit(node)

    visit_AsyncFunctionDef = visit_FunctionDef

    def visit_Call(self, node: ast.Call) -> None:
        func_name = self._get_call_name(node)

        crd = _parse_crd_constructor(func_name)
        if crd:
            self.crd_kinds.add(crd[0])
            self.crd_versions.add(crd[1])

        if func_name in _PREDICTOR_SPEC_NAMES:
            self._extract_framework_kwargs(node)

        if func_name in _ISVC_SPEC_NAMES:
            self._extract_component_kwargs(node)

        if func_name == "V1beta1ModelFormat":
            self._extract_model_format(node)

        self.generic_visit(node)

    def visit_Expr(self, node: ast.Expr) -> None:
        """Detect item.add_marker(pytest.mark.X) in conftest hooks."""
        if isinstance(node.value, ast.Call):
            call = node.value
            if (
                isinstance(call.func, ast.Attribute)
                and call.func.attr == "add_marker"
                and call.args
            ):
                marker = self._get_pytest_marker(call.args[0])
                if marker and marker not in _SKIP_MARKERS:
                    self.markers.add(marker)
        self.generic_visit(node)

    def visit_Dict(self, node: ast.Dict) -> None:
        """Detect dict-literal CRD construction like {"kind": "LLMInferenceServiceConfig"}."""
        for key, value in zip(node.keys, node.values, strict=True):
            if (
                isinstance(key, ast.Constant)
                and key.value == "kind"
                and isinstance(value, ast.Constant)
                and isinstance(value.value, str)
                and value.value in ALL_CRD_KINDS
            ):
                self.crd_kinds.add(value.value)
        self.generic_visit(node)

    def _extract_markers(self, decorators: list[ast.expr]) -> None:
        for dec in decorators:
            marker = self._get_pytest_marker(dec)
            if marker and marker not in _SKIP_MARKERS:
                self.markers.add(marker)

    def _get_pytest_marker(self, node: ast.expr) -> str | None:
        """Extract marker name from @pytest.mark.X or @pytest.mark.X(...)."""
        target = node
        if isinstance(node, ast.Call):
            target = node.func

        if isinstance(target, ast.Attribute):
            parts = self._get_dotted_name(target)
            if (
                parts
                and len(parts) >= 3
                and parts[0] == "pytest"
                and parts[1] == "mark"
            ):
                return parts[2]
        return None

    def _extract_framework_kwargs(self, node: ast.Call) -> None:
        for kw in node.keywords:
            if kw.arg in self._framework_kwarg_names:
                self.frameworks.add(kw.arg)

    def _extract_component_kwargs(self, node: ast.Call) -> None:
        for kw in node.keywords:
            if kw.arg in _COMPONENT_KWARG_NAMES:
                self.components.add(kw.arg)

    def _extract_model_format(self, node: ast.Call) -> None:
        for kw in node.keywords:
            if kw.arg == "name" and isinstance(kw.value, ast.Constant):
                if isinstance(kw.value.value, str):
                    self.model_formats.add(kw.value.value)

    def _get_call_name(self, node: ast.Call) -> str:
        if isinstance(node.func, ast.Name):
            return node.func.id
        if isinstance(node.func, ast.Attribute):
            return node.func.attr
        return ""

    def _get_dotted_name(self, node: ast.expr) -> list[str]:
        if isinstance(node, ast.Name):
            return [node.id]
        if isinstance(node, ast.Attribute):
            parent = self._get_dotted_name(node.value)
            if parent:
                return parent + [node.attr]
        return []


def _parse_crd_constructor(name: str) -> tuple[str, str] | None:
    """Parse a SDK constructor like V1beta1InferenceService into (kind, version).

    Returns None if the name doesn't match or the derived kind isn't a known CRD.
    """
    m = _VERSION_PREFIX_RE.match(name)
    if not m:
        return None
    kind = m.group(2)
    if kind not in ALL_CRD_KINDS:
        return None
    version = "v" + m.group(1)
    return kind, version


def analyze_e2e_tests(
    repo_root: Path,
    framework_kwarg_names: set[str] | None = None,
) -> dict[str, TestFileInfo]:
    """Analyze all e2e test files and return per-file CRD/marker info.

    Two-pass approach: first collect explicit markers per directory, then assign
    directory markers to files that have no explicit markers of their own.
    """
    if framework_kwarg_names is None:
        framework_kwarg_names = set()
    test_dir = repo_root / "test" / "e2e"
    if not test_dir.is_dir():
        return {}

    parsed: list[tuple[str, Path, _E2EVisitor]] = []
    dir_markers: dict[str, set[str]] = {}

    for py_file in sorted(test_dir.rglob("*.py")):
        if any(part in _SKIP_DIRS for part in py_file.parts):
            continue
        if py_file.name.startswith("__"):
            continue

        rel_path = str(py_file.relative_to(repo_root))

        try:
            tree = ast.parse(py_file.read_text(), filename=rel_path)
        except SyntaxError:
            print(f"  Warning: SyntaxError in {rel_path}", file=sys.stderr)
            continue

        visitor = _E2EVisitor(framework_kwarg_names)
        visitor.visit(tree)

        if not (visitor.markers or visitor.crd_kinds):
            if not py_file.name.startswith("test_"):
                continue

        parsed.append((rel_path, py_file, visitor))

        if visitor.markers:
            suite_dir = _get_suite_dir(rel_path)
            if suite_dir:
                dir_markers.setdefault(suite_dir, set()).update(visitor.markers)

    results: dict[str, TestFileInfo] = {}
    for rel_path, _py_file, visitor in parsed:
        if not visitor.markers:
            suite_dir = _get_suite_dir(rel_path)
            if suite_dir and suite_dir in dir_markers:
                visitor.markers.update(dir_markers[suite_dir])

        results[rel_path] = TestFileInfo(
            path=rel_path,
            markers=sorted(visitor.markers),
            crd_kinds=sorted(visitor.crd_kinds),
            crd_versions=sorted(visitor.crd_versions),
            frameworks=sorted(visitor.frameworks),
            model_formats=sorted(visitor.model_formats),
            components=sorted(visitor.components),
        )

    return results


def _get_suite_dir(rel_path: str) -> str | None:
    """Extract the suite directory name from a test/e2e/<dir>/... path."""
    parts = rel_path.split("/")
    if len(parts) >= 4 and parts[0] == "test" and parts[1] == "e2e":
        return parts[2]
    return None
