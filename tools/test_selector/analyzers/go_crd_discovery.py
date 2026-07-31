"""Discover CRD types from Go source using pattern matching.

Scans Go source files for:
- +kubebuilder:object:root=true markers (root CRD types)
- SetupWithManager + For()/Owns()/Watches() (controller-CRD bindings)
- var _ ComponentImplementation = &Spec{} (framework specs)
- PredictorSpec struct fields with JSON tags (framework names)
"""

from __future__ import annotations

import re
from pathlib import Path

from ..mapping.schema import ControllerInfo, CRDTypeInfo

_KUBEBUILDER_ROOT_RE = re.compile(r"//\s*\+kubebuilder:object:root=true")
_TYPE_STRUCT_RE = re.compile(r"^type\s+(\w+)\s+struct\s*\{", re.MULTILINE)

_FOR_RE = re.compile(r"For\(\s*&(\w+)\.(\w+)\{", re.MULTILINE)
_WATCHES_RE = re.compile(r"Watches\(\s*&(\w+)\.(\w+)\{", re.MULTILINE)

_COMPONENT_IMPL_RE = re.compile(
    r"var\s+_\s+ComponentImplementation\s*=\s*&(\w+)\{", re.MULTILINE
)

_PREDICTOR_FIELD_RE = re.compile(
    r"^\s+\w+\s+\*(\w+Spec)\s+`json:\"(\w+),omitempty\"`",
    re.MULTILINE,
)

_COMPONENT_FIELD_RE = re.compile(
    r"^\s+(\w+)\s+\*?(Predictor|Transformer|Explainer)\w*Spec\s+`json:\"(\w+)",
    re.MULTILINE,
)


def discover_root_crd_types(repo_root: Path) -> dict[str, list[str]]:
    """Find CRD root types via +kubebuilder:object:root=true markers.

    Returns: {api_version: [type_names]}
    """
    apis_dir = repo_root / "pkg" / "apis" / "serving"
    if not apis_dir.is_dir():
        return {}

    result: dict[str, list[str]] = {}

    for version_dir in sorted(apis_dir.iterdir()):
        if not version_dir.is_dir():
            continue
        version = version_dir.name

        for go_file in sorted(version_dir.glob("*.go")):
            if go_file.name.endswith("_test.go"):
                continue
            content = go_file.read_text()

            lines = content.splitlines()
            for i, line in enumerate(lines):
                if _KUBEBUILDER_ROOT_RE.search(line):
                    for j in range(i + 1, len(lines)):
                        if not lines[j].startswith("//"):
                            m = _TYPE_STRUCT_RE.search(lines[j])
                            if m:
                                type_name = m.group(1)
                                if not type_name.endswith("List"):
                                    result.setdefault(version, []).append(type_name)
                            break

    return result


def discover_controllers(
    repo_root: Path, internal_packages: list[str], module_prefix: str
) -> list[ControllerInfo]:
    """Find controller-to-CRD bindings via SetupWithManager/For/Owns/Watches."""
    controllers: list[ControllerInfo] = []
    controller_dir = repo_root / "pkg" / "controller"
    if not controller_dir.is_dir():
        return controllers

    for go_file in sorted(controller_dir.rglob("*.go")):
        if go_file.name.endswith("_test.go"):
            continue
        content = go_file.read_text()

        if "SetupWithManager" not in content:
            continue

        for_matches = _FOR_RE.findall(content)
        if not for_matches:
            continue

        rel_path = go_file.relative_to(repo_root)
        pkg_path = str(rel_path.parent)
        go_pkg = module_prefix + pkg_path

        secondary_matches = _WATCHES_RE.findall(content)
        watched = [
            CRDTypeInfo(
                kind=type_name,
                api_version=_infer_api_version(pkg_alias, content),
            )
            for pkg_alias, type_name in secondary_matches
        ]

        for pkg_alias, type_name in for_matches:
            crd = CRDTypeInfo(
                kind=type_name,
                api_version=_infer_api_version(pkg_alias, content),
            )
            controller = ControllerInfo(
                package=go_pkg,
                primary_crd=crd,
                watched_crds=watched,
            )
            controllers.append(controller)

    return controllers


def discover_framework_specs(repo_root: Path) -> tuple[list[str], dict[str, str]]:
    """Find framework specs via ComponentImplementation assertions and PredictorSpec fields.

    Returns:
        - framework_spec_names: list of spec type names (e.g., ["SKLearnSpec"])
        - framework_json_tags: {json_tag: spec_type_name} (e.g., {"sklearn": "SKLearnSpec"})
    """
    apis_v1beta1 = repo_root / "pkg" / "apis" / "serving" / "v1beta1"
    if not apis_v1beta1.is_dir():
        return [], {}

    spec_names: list[str] = []
    json_tags: dict[str, str] = {}

    for go_file in sorted(apis_v1beta1.glob("*.go")):
        if go_file.name.endswith("_test.go"):
            continue
        content = go_file.read_text()

        for m in _COMPONENT_IMPL_RE.finditer(content):
            spec_names.append(m.group(1))

    predictor_file = apis_v1beta1 / "predictor.go"
    if predictor_file.exists():
        content = predictor_file.read_text()
        predictor_struct = _extract_struct_body(content, "PredictorSpec")
        if predictor_struct:
            for m in _PREDICTOR_FIELD_RE.finditer(predictor_struct):
                spec_type = m.group(1)
                json_tag = m.group(2)
                non_framework = {"WorkerSpec", "StorageSpec", "ConfidentialSpec"}
                if spec_type not in non_framework:
                    json_tags[json_tag] = spec_type

    return spec_names, json_tags


def discover_components(repo_root: Path) -> list[str]:
    """Find InferenceServiceSpec component fields (predictor, transformer, explainer)."""
    isvc_file = (
        repo_root / "pkg" / "apis" / "serving" / "v1beta1" / "inference_service.go"
    )
    if not isvc_file.exists():
        return []

    content = isvc_file.read_text()
    components = []
    for m in _COMPONENT_FIELD_RE.finditer(content):
        json_tag = m.group(3)
        components.append(json_tag)

    if not components:
        for name in ["predictor", "transformer", "explainer"]:
            if f'"{name}' in content:
                components.append(name)

    return components


def discover_framework_packages(
    repo_root: Path, module_prefix: str
) -> dict[str, list[str]]:
    """Map framework JSON tag to Go packages that define the framework spec.

    Returns: {json_tag: [go_package_import_paths]}
    """
    _, json_tags = discover_framework_specs(repo_root)
    apis_v1beta1 = repo_root / "pkg" / "apis" / "serving" / "v1beta1"

    result: dict[str, list[str]] = {}
    for json_tag, spec_type in json_tags.items():
        for go_file in apis_v1beta1.glob("*.go"):
            if go_file.name.endswith("_test.go"):
                continue
            content = go_file.read_text()
            if (
                f"type {spec_type} struct" in content
                or f"func (s *{spec_type})" in content
            ):
                pkg = module_prefix + str(go_file.parent.relative_to(repo_root))
                result.setdefault(json_tag, [])
                if pkg not in result[json_tag]:
                    result[json_tag].append(pkg)

    return result


def _extract_struct_body(content: str, struct_name: str) -> str | None:
    """Extract the body of a Go struct definition (between { and })."""
    pattern = re.compile(rf"^type\s+{struct_name}\s+struct\s*\{{", re.MULTILINE)
    m = pattern.search(content)
    if not m:
        return None

    start = m.end()
    depth = 1
    pos = start
    while pos < len(content) and depth > 0:
        if content[pos] == "{":
            depth += 1
        elif content[pos] == "}":
            depth -= 1
        pos += 1

    return content[start : pos - 1]


def _infer_api_version(pkg_alias: str, file_content: str) -> str:
    """Try to infer the API version from import alias usage in the file."""
    alias_map = {
        "v1beta1": "v1beta1",
        "v1alpha1": "v1alpha1",
        "v1alpha2": "v1alpha2",
    }

    for version_str, version in alias_map.items():
        if version_str in pkg_alias:
            return version

    import_re = re.compile(rf'{pkg_alias}\s+"([^"]+)"', re.MULTILINE)
    m = import_re.search(file_content)
    if m:
        import_path = m.group(1)
        for version_str, version in alias_map.items():
            if version_str in import_path:
                return version

    return "unknown"
