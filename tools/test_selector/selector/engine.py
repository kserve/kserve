"""Core selection engine: changed files -> test suites."""

from __future__ import annotations

import re
from collections import deque
from pathlib import Path

from .rules import (
    IGNORABLE_PATTERNS,
    PYTHON_ALL_E2E_PACKAGES,
)
from ..analyzers.config_mapper import load_overrides, match_override
from ..mapping.schema import (
    Mapping,
    TestSelection,
)


def select_tests(
    mapping: Mapping,
    changed_files: list[str],
    repo_root: Path,
) -> TestSelection:
    """Determine which tests to run for the given changed files."""
    sel = TestSelection()
    overrides = load_overrides(repo_root)

    for f in changed_files:
        f = f.strip().lstrip("./")
        if not f:
            continue

        _process_file(f, mapping, overrides, sel, repo_root)

    _finalize(sel)
    return sel


def _process_file(
    file_path: str,
    mapping: Mapping,
    overrides: list[dict],
    sel: TestSelection,
    repo_root: Path,
) -> None:
    """Classify and process a single changed file."""
    matched_overrides = match_override(file_path, overrides)
    for ov in matched_overrides:
        _apply_override(ov, file_path, sel, mapping)

    if file_path.endswith(".go"):
        _process_go_file(file_path, mapping, sel)
    elif file_path.startswith("python/"):
        _process_python_file(file_path, mapping, sel)
    elif file_path.startswith("test/e2e/"):
        _process_e2e_test_file(file_path, mapping, sel)
    elif file_path.startswith(("config/", "charts/", "hack/")):
        _process_config_or_infra_file(file_path, mapping, sel)
    elif file_path.endswith(".py"):
        pass  # non-package Python file, skip
    elif not matched_overrides:
        if _is_ignorable(file_path):
            return
        sel.reasons.append(f"{file_path} -> unclassified -> conservative:all")
        _trigger_all(sel, mapping)


def _process_go_file(file_path: str, mapping: Mapping, sel: TestSelection) -> None:
    """Process a changed Go source file."""
    go_pkg = mapping.go_file_to_package.get(file_path)
    if not go_pkg:
        sel.reasons.append(f"{file_path} -> unknown Go package -> conservative:all_go")
        sel.go_tests.run = True
        sel.go_tests.all = True
        return

    sel.go_tests.run = True

    affected_pkgs = _reverse_walk(go_pkg, mapping)
    for pkg in affected_pkgs:
        rel = _pkg_to_rel(pkg)
        sel.go_tests.packages.append(f"./{rel}...")

    affected_entrypoints = _find_entrypoints(affected_pkgs, mapping)

    file_frameworks = _detect_framework_from_file(file_path, go_pkg, mapping)

    if file_frameworks:
        markers = _narrow_markers_by_framework(file_frameworks, mapping)
        reason_fw = ",".join(sorted(file_frameworks))
        sel.reasons.append(
            f"{file_path} -> pkg:{_pkg_to_rel(go_pkg)} -> "
            f"framework:{reason_fw} -> e2e:{','.join(markers)}"
        )
        for m in markers:
            _add_markers(sel, [m])
    else:
        controller_entrypoints = {
            name for name, ep in mapping.entrypoints.items() if ep.crd_types
        }
        if controller_entrypoints and affected_entrypoints >= controller_entrypoints:
            sel.go_tests.all = True
            sel.reasons.append(
                f"{file_path} -> pkg:{_pkg_to_rel(go_pkg)} -> all_entrypoints -> all_e2e"
            )
            _trigger_all_e2e(sel, mapping)
            return

        for ep_name in affected_entrypoints:
            ep = mapping.entrypoints.get(ep_name)
            if not ep:
                continue

            for crd in ep.crd_types:
                markers = _get_crd_markers(crd.kind, mapping)
                sel.reasons.append(
                    f"{file_path} -> pkg:{_pkg_to_rel(go_pkg)} -> "
                    f"entrypoint:{ep_name} -> crd:{crd.kind} -> "
                    f"e2e:{','.join(markers)}"
                )
                _add_markers(sel, markers)

            for crd in ep.watched_crd_types:
                markers = _get_watched_crd_markers(crd.kind, mapping)
                if markers:
                    sel.reasons.append(
                        f"{file_path} -> pkg:{_pkg_to_rel(go_pkg)} -> "
                        f"entrypoint:{ep_name} -> watches:{crd.kind} -> "
                        f"e2e:{','.join(markers)}"
                    )
                    _add_markers(sel, markers)


def _process_python_file(file_path: str, mapping: Mapping, sel: TestSelection) -> None:
    """Process a changed Python source file under python/."""
    if "/kserve/models/" in file_path and not file_path.endswith("__init__.py"):
        crd_kind = _extract_crd_from_model_file(file_path, mapping)
        if crd_kind:
            sel.python_tests.run = True
            sel.python_tests.packages.append("kserve")
            markers = _get_crd_markers(crd_kind, mapping)
            sel.reasons.append(
                f"{file_path} -> python:model:{crd_kind} -> e2e:{','.join(markers)}"
            )
            _add_markers(sel, markers)
            return

    pkg_name = mapping.python_file_to_package.get(file_path)

    if not pkg_name:
        parts = file_path.split("/")
        if len(parts) >= 2:
            pkg_name = parts[1]

    if not pkg_name:
        return

    sel.python_tests.run = True
    sel.python_tests.packages.append(pkg_name)

    if pkg_name in PYTHON_ALL_E2E_PACKAGES:
        sel.reasons.append(f"{file_path} -> python:{pkg_name} -> all_e2e")
        _trigger_all_e2e(sel, mapping)
        return

    frameworks = mapping.server_to_frameworks.get(pkg_name)
    if frameworks:
        all_markers: set[str] = set()
        for fw in frameworks:
            all_markers.update(_get_markers_for_framework(fw, mapping))
        sorted_markers = sorted(all_markers)
        sel.reasons.append(
            f"{file_path} -> python:{pkg_name} -> "
            f"framework:{','.join(frameworks)} -> "
            f"e2e:{','.join(sorted_markers)}"
        )
        for m in sorted_markers:
            _add_markers(sel, [m])


def _process_config_or_infra_file(
    file_path: str, mapping: Mapping, sel: TestSelection
) -> None:
    """Process a changed file under config/, charts/, or hack/ using dynamic CRD mapping."""
    crds = _lookup_config_crds(file_path, mapping)
    if crds:
        markers_all: list[str] = []
        for kind in crds:
            markers = _get_crd_markers(kind, mapping)
            markers_all.extend(markers)
            _add_markers(sel, markers)
        sel.reasons.append(
            f"{file_path} -> config_discovery:{','.join(crds)} -> "
            f"e2e:{','.join(sorted(set(markers_all)))}"
        )
    else:
        sel.reasons.append(f"{file_path} -> no config mapping -> conservative:all")
        _trigger_all(sel, mapping)


def _lookup_config_crds(file_path: str, mapping: Mapping) -> list[str]:
    """Find CRD kinds for a config/chart/hack file by walking up its directory path."""
    parts = file_path.split("/")
    for i in range(len(parts), 0, -1):
        dir_path = "/".join(parts[:i])
        crds = mapping.config_to_crds.get(dir_path)
        if crds:
            return crds
    return []


def _process_e2e_test_file(
    file_path: str, mapping: Mapping, sel: TestSelection
) -> None:
    """If a test file changed, always run its suite."""
    test_info = mapping.test_files.get(file_path)
    if test_info:
        for m in test_info.markers:
            _add_markers(sel, [m])
        sel.reasons.append(
            f"{file_path} -> test_changed -> e2e:{','.join(test_info.markers)}"
        )
    else:
        markers = _infer_markers_from_test_dir(file_path, mapping)
        if markers:
            _add_markers(sel, markers)
            sel.reasons.append(
                f"{file_path} -> test_changed -> e2e:{','.join(markers)}"
            )


def _apply_override(
    override: dict, file_path: str, sel: TestSelection, mapping: Mapping
) -> None:
    """Apply a single override rule."""
    trigger = override.get("trigger")
    reason_suffix = override.get("reason", override.get("pattern", ""))

    if trigger == "all":
        sel.reasons.append(f"{file_path} -> override:{reason_suffix} -> all")
        _trigger_all(sel, mapping)
    elif isinstance(trigger, dict):
        parts = []
        if trigger.get("go"):
            sel.go_tests.run = True
            sel.go_tests.all = True
            parts.append("go")
        e2e_markers = trigger.get("e2e", [])
        for m in e2e_markers:
            _add_markers(sel, [m])
        if e2e_markers:
            parts.append(f"e2e:{','.join(e2e_markers)}")
        if parts:
            sel.reasons.append(
                f"{file_path} -> override:{reason_suffix} -> {','.join(parts)}"
            )


def _reverse_walk(start_pkg: str, mapping: Mapping) -> set[str]:
    """BFS reverse dependency walk from a Go package."""
    visited = {start_pkg}
    queue = deque([start_pkg])
    while queue:
        pkg = queue.popleft()
        for dep in mapping.go_reverse_deps.get(pkg, []):
            if dep not in visited:
                visited.add(dep)
                queue.append(dep)
    return visited


def _find_entrypoints(affected_pkgs: set[str], mapping: Mapping) -> set[str]:
    """Find which entrypoints have any of the affected packages in their dep tree."""
    result: set[str] = set()
    for pkg in affected_pkgs:
        eps = mapping.go_package_to_entrypoints.get(pkg, [])
        result.update(eps)
    return result


def _detect_framework_from_file(
    file_path: str, go_pkg: str, mapping: Mapping
) -> set[str]:
    """Check if a Go file is framework-specific (e.g., predictor_sklearn.go)."""
    frameworks: set[str] = set()
    for fw, pkg_list in mapping.framework_packages.items():
        if go_pkg in pkg_list:
            file_name = file_path.rsplit("/", 1)[-1]
            if fw in file_name or f"predictor_{fw}" in file_name:
                frameworks.add(fw)
    return frameworks


def _narrow_markers_by_framework(frameworks: set[str], mapping: Mapping) -> list[str]:
    """When a change is framework-specific, find test markers that exercise those frameworks."""
    matched_markers: set[str] = set()
    for test_info in mapping.test_files.values():
        test_frameworks = set(test_info.frameworks) | set(test_info.model_formats)
        if test_frameworks & frameworks:
            matched_markers.update(test_info.markers)

    if not matched_markers:
        matched_markers.add("predictor")

    return sorted(matched_markers)


def _get_markers_for_framework(framework: str, mapping: Mapping) -> list[str]:
    """Find e2e test markers that exercise a given framework."""
    markers: set[str] = set()
    for test_info in mapping.test_files.values():
        if framework in test_info.frameworks or framework in test_info.model_formats:
            markers.update(test_info.markers)

    if not markers:
        markers.add("predictor")

    return sorted(markers)


def _get_crd_markers(kind: str, mapping: Mapping) -> list[str]:
    """Get e2e markers for a CRD kind, falling back to all e2e markers."""
    return mapping.crd_to_markers.get(kind, mapping.all_e2e_markers)


def _get_watched_crd_markers(kind: str, mapping: Mapping) -> list[str]:
    """Get e2e markers for a watched CRD.

    Unlike primary CRDs, watched CRDs don't fall back to all markers when the
    kind has no entry in crd_to_markers.
    """
    return mapping.crd_to_markers.get(kind, [])


def _add_markers(sel: TestSelection, markers: list[str]) -> None:
    """Add markers to e2e test selection."""
    sel.e2e_tests.run = True
    sel.e2e_tests.markers.extend(markers)


def _trigger_all(sel: TestSelection, mapping: Mapping) -> None:
    """Escalate to running everything."""
    sel.go_tests.run = True
    sel.go_tests.all = True
    sel.python_tests.run = True
    _trigger_all_e2e(sel, mapping)


def _trigger_all_e2e(sel: TestSelection, mapping: Mapping) -> None:
    """Escalate to running all e2e tests."""
    sel.e2e_tests.run = True
    sel.e2e_tests.markers.extend(mapping.all_e2e_markers)


def _finalize(sel: TestSelection) -> None:
    """Deduplicate markers and suites."""
    sel.e2e_tests.markers = sorted(set(sel.e2e_tests.markers))
    sel.go_tests.packages = sorted(set(sel.go_tests.packages))
    sel.python_tests.packages = sorted(set(sel.python_tests.packages))

    seen_reasons: list[str] = []
    for r in sel.reasons:
        if r not in seen_reasons:
            seen_reasons.append(r)
    sel.reasons = seen_reasons


def _pkg_to_rel(import_path: str) -> str:
    """Convert a Go import path to a relative path within the repo.

    Example: github.com/kserve/kserve/pkg/apis -> pkg/apis
    """
    prefix = "github.com/kserve/kserve/"
    if import_path.startswith(prefix):
        return import_path[len(prefix) :]
    return import_path


def _infer_markers_from_test_dir(file_path: str, mapping: Mapping) -> list[str]:
    """Infer markers from the test file's directory using auto-discovered data."""
    parts = file_path.split("/")
    if len(parts) >= 4 and parts[0] == "test" and parts[1] == "e2e":
        return mapping.suite_dir_to_markers.get(parts[2], [])
    return []


def _is_ignorable(file_path: str) -> bool:
    """Check if a file can be safely ignored (docs, CI, etc.)."""
    for pattern in IGNORABLE_PATTERNS:
        if file_path.endswith(pattern) or file_path.startswith(pattern):
            return True
    return False


_VERSION_PREFIX_RE = re.compile(r"^v\d+(?:alpha\d+|beta\d+)?_")


def _camel_to_snake(name: str) -> str:
    s = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1_\2", name)
    s = re.sub(r"([a-z\d])([A-Z])", r"\1_\2", s)
    return s.lower()


def _extract_crd_from_model_file(file_path: str, mapping: Mapping) -> str | None:
    """Match a Python SDK model filename to a known CRD kind.

    Model files are named like v1alpha2_llm_inference_service.py.
    Related types (Spec, Status, List) share the root CRD as a prefix.
    """
    basename = file_path.rsplit("/", 1)[-1].removesuffix(".py")
    m = _VERSION_PREFIX_RE.match(basename)
    if not m:
        return None
    rest = basename[m.end() :]
    snake_to_kind = {_camel_to_snake(kind): kind for kind in mapping.crd_to_markers}
    for snake, kind in sorted(
        snake_to_kind.items(), key=lambda x: len(x[0]), reverse=True
    ):
        if rest == snake or rest.startswith(snake + "_"):
            return kind
    return None
