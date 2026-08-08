"""Build the unified mapping from all analyzers."""

from __future__ import annotations

import sys
from pathlib import Path

from ..analyzers.config_discovery import discover_config_to_crds
from ..analyzers.e2e_mapper import analyze_e2e_tests
from ..analyzers.go_crd_discovery import (
    discover_components,
    discover_controllers,
    discover_framework_packages,
    discover_framework_specs,
    discover_root_crd_types,
)
from ..analyzers.go_deps import (
    build_go_dependency_info,
    discover_entrypoints,
    get_module_prefix,
)
from ..analyzers.python_imports import (
    analyze_python_imports,
    build_python_file_to_package,
    discover_python_packages,
)
from ..mapping.schema import CRDTypeInfo, EntrypointInfo, Mapping


def build_mapping(repo_root: Path) -> Mapping:
    """Run all analyzers and build the full mapping."""
    print("Building mapping...", file=sys.stderr)

    module_prefix = get_module_prefix(repo_root)
    print(f"  Module: {module_prefix.rstrip('/')}", file=sys.stderr)

    # Step 1: Discover Go entrypoints
    print("Step 1: Discovering Go entrypoints...", file=sys.stderr)
    entrypoint_targets = discover_entrypoints(repo_root)
    print(f"  Found {len(entrypoint_targets)} entrypoints", file=sys.stderr)

    # Step 2: Build Go dependency graph (single `go list -json ./...` call)
    print("Step 2: Building Go dependency graph...", file=sys.stderr)
    dep_graph, go_packages, file_to_package, ep_packages = build_go_dependency_info(
        repo_root, module_prefix, entrypoint_targets
    )
    print(f"  {len(go_packages)} internal packages", file=sys.stderr)

    # Step 3: Discover CRD types and controllers
    print("Step 3: Discovering CRD types...", file=sys.stderr)
    root_crd_types = discover_root_crd_types(repo_root)
    controllers = discover_controllers(
        repo_root, list(go_packages.keys()), module_prefix
    )
    spec_names, framework_json_tags = discover_framework_specs(repo_root)
    components = discover_components(repo_root)
    framework_packages = discover_framework_packages(repo_root, module_prefix)

    print(
        f"  CRD types: {sum(len(v) for v in root_crd_types.values())}, "
        f"Controllers: {len(controllers)}, "
        f"Frameworks: {len(framework_json_tags)}",
        file=sys.stderr,
    )

    # Step 4: Build entrypoint info
    print("Step 4: Building entrypoint-CRD map...", file=sys.stderr)
    entrypoints: dict[str, EntrypointInfo] = {}
    known_crd_kinds = {t for types in root_crd_types.values() for t in types}

    for target in entrypoint_targets:
        ep_deps = ep_packages.get(target, [])

        ep_controllers = []
        for ctrl in controllers:
            if ctrl.package in ep_deps or ctrl.package == module_prefix + target.lstrip(
                "./"
            ):
                ep_controllers.append(ctrl)

        crd_types: list[CRDTypeInfo] = []
        watched_crd_types: list[CRDTypeInfo] = []
        primary_kinds: set[str] = set()
        watched_kinds: set[str] = set()
        for ctrl in ep_controllers:
            crd = ctrl.primary_crd
            if crd.kind == "InferenceService" and crd.api_version == "v1beta1":
                crd.components = components or ["predictor", "transformer", "explainer"]
                crd.frameworks = sorted(framework_json_tags.keys())
            crd_types.append(crd)
            primary_kinds.add(crd.kind)

            for watched in ctrl.watched_crds:
                if (
                    watched.kind in known_crd_kinds
                    and watched.kind not in primary_kinds
                    and watched.kind not in watched_kinds
                ):
                    watched_crd_types.append(watched)
                    watched_kinds.add(watched.kind)

        entrypoints[target] = EntrypointInfo(
            path=target,
            go_package=module_prefix + target.lstrip("./"),
            dep_packages=ep_deps,
            crd_types=crd_types,
            watched_crd_types=watched_crd_types,
            is_controller=bool(ep_controllers),
        )

    # Step 5: Build reverse dependency map
    go_reverse_deps: dict[str, list[str]] = {}
    for node in dep_graph.nodes:
        dependents = dep_graph.dependents_of(node)
        if dependents:
            go_reverse_deps[node] = sorted(dependents)

    # Step 6: Build package-to-entrypoints map
    pkg_to_entrypoints: dict[str, list[str]] = {}
    for target, deps in ep_packages.items():
        for dep in deps:
            pkg_to_entrypoints.setdefault(dep, [])
            if target not in pkg_to_entrypoints[dep]:
                pkg_to_entrypoints[dep].append(target)

    # Step 7: Analyze e2e tests
    print("Step 5: Analyzing e2e tests...", file=sys.stderr)
    test_files = analyze_e2e_tests(repo_root, set(framework_json_tags.keys()))
    print(f"  {len(test_files)} test files analyzed", file=sys.stderr)

    # Step 8: Analyze Python packages
    print("Step 6: Analyzing Python packages...", file=sys.stderr)
    python_packages = discover_python_packages(repo_root)
    analyze_python_imports(repo_root, python_packages)
    python_file_to_package = build_python_file_to_package(python_packages)
    print(f"  {len(python_packages)} Python packages", file=sys.stderr)

    # Step 9: Discover config/chart/hack -> CRD mappings
    print("Step 7: Discovering config/chart/hack CRD mappings...", file=sys.stderr)
    config_to_crds = discover_config_to_crds(repo_root)
    print(f"  {len(config_to_crds)} directory mappings", file=sys.stderr)

    # Step 10: Build CRD-to-markers mapping from test file data
    print("Step 8: Building CRD-to-marker mapping...", file=sys.stderr)
    crd_to_markers, all_e2e_markers = _build_crd_marker_maps(test_files)
    print(
        f"  {len(crd_to_markers)} CRDs mapped to markers, "
        f"{len(all_e2e_markers)} total e2e markers",
        file=sys.stderr,
    )

    # Step 11: Build suite directory -> markers mapping from test file data
    suite_dir_to_markers = _build_suite_dir_markers(test_files)

    # Step 12: Discover server -> framework mapping from runtime YAMLs
    server_to_frameworks = _discover_server_frameworks(repo_root, python_packages)

    mapping = Mapping(
        entrypoints=entrypoints,
        go_file_to_package=file_to_package,
        go_reverse_deps=go_reverse_deps,
        go_package_to_entrypoints=pkg_to_entrypoints,
        test_files=test_files,
        python_packages=python_packages,
        python_file_to_package=python_file_to_package,
        framework_packages=framework_packages,
        config_to_crds=config_to_crds,
        crd_to_markers=crd_to_markers,
        all_e2e_markers=all_e2e_markers,
        suite_dir_to_markers=suite_dir_to_markers,
        server_to_frameworks=server_to_frameworks,
    )

    print("Mapping complete.", file=sys.stderr)
    return mapping


def _build_crd_marker_maps(
    test_files: dict[str, object],
) -> tuple[dict[str, list[str]], list[str]]:
    """Build CRD-to-markers mapping and flat e2e marker list from test files.

    Scans all analyzed test files (including helpers like conftest.py and
    fixtures.py) to discover which CRD kinds each file constructs and which
    pytest markers it carries.
    """
    crd_markers: dict[str, set[str]] = {}
    all_markers: set[str] = set()

    for info in test_files.values():
        all_markers.update(info.markers)
        for kind in info.crd_kinds:
            crd_markers.setdefault(kind, set()).update(info.markers)

    crd_to_markers = {k: sorted(v) for k, v in sorted(crd_markers.items())}
    return crd_to_markers, sorted(all_markers)


def _build_suite_dir_markers(
    test_files: dict[str, object],
) -> dict[str, list[str]]:
    """Build test/e2e/<dir>/ -> markers mapping from analyzed test files."""
    dir_markers: dict[str, set[str]] = {}
    for path, info in test_files.items():
        parts = path.split("/")
        if (
            len(parts) >= 4
            and parts[0] == "test"
            and parts[1] == "e2e"
            and info.markers
        ):
            dir_markers.setdefault(parts[2], set()).update(info.markers)
    return {k: sorted(v) for k, v in sorted(dir_markers.items())}


def _discover_server_frameworks(
    repo_root: Path,
    python_packages: dict[str, object],
) -> dict[str, list[str]]:
    """Discover server -> framework mapping from runtime YAMLs.

    Parses config/runtimes/kserve-*.yaml for the server-type annotation and
    supportedModelFormats names. Only includes servers that have a matching
    Python package directory.
    """
    import re

    runtimes_dir = repo_root / "config" / "runtimes"
    if not runtimes_dir.is_dir():
        return {}

    server_type_re = re.compile(r"serving\.kserve\.io/server-type:\s*(\S+)")
    format_name_re = re.compile(r"^\s+-\s*name:\s*(\S+)")

    result: dict[str, list[str]] = {}
    for yaml_file in sorted(runtimes_dir.glob("kserve-*.yaml")):
        server_type = None
        formats: list[str] = []
        in_formats = False

        for line in yaml_file.read_text().splitlines():
            m = server_type_re.search(line)
            if m:
                server_type = m.group(1)
                continue

            if "supportedModelFormats:" in line:
                in_formats = True
                continue

            if in_formats:
                m = format_name_re.match(line)
                if m:
                    formats.append(m.group(1))
                elif line.strip() and not line.startswith(" "):
                    in_formats = False

        if server_type and formats and server_type in python_packages:
            result[server_type] = formats

    return result
