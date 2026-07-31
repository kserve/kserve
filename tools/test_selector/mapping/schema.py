"""Dataclass definitions for all mapping structures."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class CRDTypeInfo:
    kind: str
    api_version: str
    components: list[str] = field(default_factory=list)
    frameworks: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        d: dict = {"kind": self.kind, "api_version": self.api_version}
        if self.components:
            d["components"] = self.components
        if self.frameworks:
            d["frameworks"] = self.frameworks
        return d

    @classmethod
    def from_dict(cls, d: dict) -> CRDTypeInfo:
        return cls(
            kind=d["kind"],
            api_version=d["api_version"],
            components=d.get("components", []),
            frameworks=d.get("frameworks", []),
        )


@dataclass
class ControllerInfo:
    package: str
    primary_crd: CRDTypeInfo
    watched_crds: list[CRDTypeInfo] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "package": self.package,
            "primary_crd": self.primary_crd.to_dict(),
            "watched_crds": [c.to_dict() for c in self.watched_crds],
        }

    @classmethod
    def from_dict(cls, d: dict) -> ControllerInfo:
        return cls(
            package=d["package"],
            primary_crd=CRDTypeInfo.from_dict(d["primary_crd"]),
            watched_crds=[CRDTypeInfo.from_dict(c) for c in d.get("watched_crds", [])],
        )


@dataclass
class EntrypointInfo:
    path: str
    go_package: str
    dep_packages: list[str] = field(default_factory=list)
    crd_types: list[CRDTypeInfo] = field(default_factory=list)
    watched_crd_types: list[CRDTypeInfo] = field(default_factory=list)
    is_controller: bool = True

    def to_dict(self) -> dict:
        d = {
            "path": self.path,
            "go_package": self.go_package,
            "dep_packages": self.dep_packages,
            "crd_types": [c.to_dict() for c in self.crd_types],
            "is_controller": self.is_controller,
        }
        if self.watched_crd_types:
            d["watched_crd_types"] = [c.to_dict() for c in self.watched_crd_types]
        return d

    @classmethod
    def from_dict(cls, d: dict) -> EntrypointInfo:
        return cls(
            path=d["path"],
            go_package=d["go_package"],
            dep_packages=d.get("dep_packages", []),
            crd_types=[CRDTypeInfo.from_dict(c) for c in d.get("crd_types", [])],
            watched_crd_types=[
                CRDTypeInfo.from_dict(c) for c in d.get("watched_crd_types", [])
            ],
            is_controller=d.get("is_controller", True),
        )


@dataclass
class TestFileInfo:
    path: str
    markers: list[str] = field(default_factory=list)
    crd_kinds: list[str] = field(default_factory=list)
    crd_versions: list[str] = field(default_factory=list)
    frameworks: list[str] = field(default_factory=list)
    model_formats: list[str] = field(default_factory=list)
    components: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        d: dict = {"path": self.path}
        if self.markers:
            d["markers"] = self.markers
        if self.crd_kinds:
            d["crd_kinds"] = self.crd_kinds
        if self.crd_versions:
            d["crd_versions"] = self.crd_versions
        if self.frameworks:
            d["frameworks"] = self.frameworks
        if self.model_formats:
            d["model_formats"] = self.model_formats
        if self.components:
            d["components"] = self.components
        return d

    @classmethod
    def from_dict(cls, d: dict) -> TestFileInfo:
        return cls(
            path=d["path"],
            markers=d.get("markers", []),
            crd_kinds=d.get("crd_kinds", []),
            crd_versions=d.get("crd_versions", []),
            frameworks=d.get("frameworks", []),
            model_formats=d.get("model_formats", []),
            components=d.get("components", []),
        )


@dataclass
class PythonPackageInfo:
    name: str
    path: str
    files: list[str] = field(default_factory=list)
    intra_repo_imports: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "name": self.name,
            "path": self.path,
            "files": self.files,
            "intra_repo_imports": self.intra_repo_imports,
        }

    @classmethod
    def from_dict(cls, d: dict) -> PythonPackageInfo:
        return cls(
            name=d["name"],
            path=d["path"],
            files=d.get("files", []),
            intra_repo_imports=d.get("intra_repo_imports", []),
        )


@dataclass
class Mapping:
    entrypoints: dict[str, EntrypointInfo] = field(default_factory=dict)
    go_file_to_package: dict[str, str] = field(default_factory=dict)
    go_reverse_deps: dict[str, list[str]] = field(default_factory=dict)
    go_package_to_entrypoints: dict[str, list[str]] = field(default_factory=dict)
    test_files: dict[str, TestFileInfo] = field(default_factory=dict)
    python_packages: dict[str, PythonPackageInfo] = field(default_factory=dict)
    python_file_to_package: dict[str, str] = field(default_factory=dict)
    framework_packages: dict[str, list[str]] = field(default_factory=dict)
    config_to_crds: dict[str, list[str]] = field(default_factory=dict)
    crd_to_markers: dict[str, list[str]] = field(default_factory=dict)
    all_e2e_markers: list[str] = field(default_factory=list)
    suite_dir_to_markers: dict[str, list[str]] = field(default_factory=dict)
    server_to_frameworks: dict[str, list[str]] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "entrypoints": {k: v.to_dict() for k, v in self.entrypoints.items()},
            "go_file_to_package": self.go_file_to_package,
            "go_reverse_deps": self.go_reverse_deps,
            "go_package_to_entrypoints": self.go_package_to_entrypoints,
            "test_files": {k: v.to_dict() for k, v in self.test_files.items()},
            "python_packages": {
                k: v.to_dict() for k, v in self.python_packages.items()
            },
            "python_file_to_package": self.python_file_to_package,
            "framework_packages": self.framework_packages,
            "config_to_crds": self.config_to_crds,
            "crd_to_markers": self.crd_to_markers,
            "all_e2e_markers": self.all_e2e_markers,
            "suite_dir_to_markers": self.suite_dir_to_markers,
            "server_to_frameworks": self.server_to_frameworks,
        }

    @classmethod
    def from_dict(cls, d: dict) -> Mapping:
        return cls(
            entrypoints={
                k: EntrypointInfo.from_dict(v)
                for k, v in d.get("entrypoints", {}).items()
            },
            go_file_to_package=d.get("go_file_to_package", {}),
            go_reverse_deps=d.get("go_reverse_deps", {}),
            go_package_to_entrypoints=d.get("go_package_to_entrypoints", {}),
            test_files={
                k: TestFileInfo.from_dict(v) for k, v in d.get("test_files", {}).items()
            },
            python_packages={
                k: PythonPackageInfo.from_dict(v)
                for k, v in d.get("python_packages", {}).items()
            },
            python_file_to_package=d.get("python_file_to_package", {}),
            framework_packages=d.get("framework_packages", {}),
            config_to_crds=d.get("config_to_crds", {}),
            crd_to_markers=d.get("crd_to_markers", {}),
            all_e2e_markers=d.get("all_e2e_markers", []),
            suite_dir_to_markers=d.get("suite_dir_to_markers", {}),
            server_to_frameworks=d.get("server_to_frameworks", {}),
        )


@dataclass
class TestSuiteSelection:
    run: bool = False
    markers: list[str] = field(default_factory=list)
    suites: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        d: dict = {"run": self.run}
        if self.markers:
            d["markers"] = sorted(set(self.markers))
        if self.suites:
            d["suites"] = sorted(set(self.suites))
        return d


@dataclass
class GoTestSelection:
    run: bool = False
    packages: list[str] = field(default_factory=list)
    all: bool = False

    def to_dict(self) -> dict:
        d: dict = {"run": self.run}
        if self.all:
            d["all"] = True
        elif self.packages:
            d["packages"] = sorted(set(self.packages))
        return d


@dataclass
class PythonTestSelection:
    run: bool = False
    packages: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        d: dict = {"run": self.run}
        if self.packages:
            d["packages"] = sorted(set(self.packages))
        return d


@dataclass
class TestSelection:
    go_tests: GoTestSelection = field(default_factory=GoTestSelection)
    python_tests: PythonTestSelection = field(default_factory=PythonTestSelection)
    e2e_tests: TestSuiteSelection = field(default_factory=TestSuiteSelection)
    reasons: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "go_tests": self.go_tests.to_dict(),
            "python_tests": self.python_tests.to_dict(),
            "e2e_tests": self.e2e_tests.to_dict(),
            "reasons": self.reasons,
        }
