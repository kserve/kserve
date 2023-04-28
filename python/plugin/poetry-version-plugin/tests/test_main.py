import shutil
import subprocess
from pathlib import Path

import pkginfo

testing_assets = Path(__file__).parent / "assets"
plugin_source_dir = Path(__file__).parent.parent / "poetry_version_plugin"


def copy_assets(source_name: str, testing_dir: Path):
    package_path = testing_assets / source_name
    shutil.copytree(package_path, testing_dir)


def build_package(testing_dir: Path):
    result = subprocess.run(
        [
            "coverage",
            "run",
            "--source",
            str(plugin_source_dir),
            "--parallel-mode",
            "-m",
            "poetry",
            "build",
        ],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    coverage_path = list(testing_dir.glob(".coverage*"))[0]
    dst_coverage_path = Path(__file__).parent.parent / coverage_path.name
    dst_coverage_path.write_bytes(coverage_path.read_bytes())
    return result


def test_defaults(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("no_packages", testing_dir)
    result = build_package(testing_dir=testing_dir)

    assert (
        "poetry-version-plugin: Using __init__.py file at "
        "test_custom_version/__init__.py for dynamic version" in result.stdout
    )
    assert (
        "poetry-version-plugin: Setting package dynamic version to __version__ "
        "variable from __init__.py: 0.0.1" in result.stdout
    )
    assert "Built test_custom_version-0.0.1-py3-none-any.whl" in result.stdout
    wheel_path = testing_dir / "dist" / "test_custom_version-0.0.1-py3-none-any.whl"
    info = pkginfo.get_metadata(str(wheel_path))
    assert info.version == "0.0.1"


def test_custom_packages(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("custom_packages", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: Using __init__.py file at custom_package/__init__.py "
        "for dynamic version" in result.stdout
    )
    assert (
        "poetry-version-plugin: Setting package dynamic version to __version__ "
        "variable from __init__.py: 0.0.2" in result.stdout
    )
    assert "Built test_custom_version-0.0.2-py3-none-any.whl" in result.stdout
    wheel_path = testing_dir / "dist" / "test_custom_version-0.0.2-py3-none-any.whl"
    info = pkginfo.get_metadata(str(wheel_path))
    assert info.version == "0.0.2"


def test_variations(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("variations", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: Using __init__.py file at "
        "test_custom_version/__init__.py for dynamic version" in result.stdout
    )
    assert (
        "poetry-version-plugin: Setting package dynamic version to __version__ "
        "variable from __init__.py: 0.0.3" in result.stdout
    )
    assert "Built test_custom_version-0.0.3-py3-none-any.whl" in result.stdout
    wheel_path = testing_dir / "dist" / "test_custom_version-0.0.3-py3-none-any.whl"
    info = pkginfo.get_metadata(str(wheel_path))
    assert info.version == "0.0.3"


def test_no_version_var(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("no_version_var", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: No valid __version__ variable found in __init__.py, "
        "cannot extract dynamic version" in result.stderr
    )
    assert result.returncode != 0


def test_no_standard_dir(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("no_standard_dir", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert "poetry-version-plugin: __init__.py file not found at" in result.stderr
    assert result.returncode != 0


def test_multiple_packages(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("multiple_packages", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: More than one package set, cannot extract "
        "dynamic version" in result.stderr
    )
    assert result.returncode != 0


def test_no_config(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("no_config", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert "Built test_custom_version-0-py3-none-any.whl" in result.stdout
    assert result.returncode == 0


def test_no_config_source(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("no_config_source", testing_dir)
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: No source configuration found in "
        "[tool.poetry-version-plugin] in pyproject.toml, not extracting dynamic version"
    ) in result.stderr
    assert result.returncode != 0


def test_git_tag(tmp_path: Path):
    testing_dir = tmp_path / "testing_package"
    copy_assets("git_tag", testing_dir)
    result = result = subprocess.run(
        [
            "git",
            "init",
        ],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = result = subprocess.run(
        ["git", "config", "user.email", "tester@example.com"],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = result = subprocess.run(
        ["git", "config", "user.name", "Tester"],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = result = subprocess.run(
        ["git", "add", "."],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = result = subprocess.run(
        ["git", "commit", "-m", "release"],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = build_package(testing_dir=testing_dir)
    assert "No Git tag found, not extracting dynamic version" in result.stderr
    assert result.returncode != 0
    result = result = subprocess.run(
        [
            "git",
            "tag",
            "0.0.9",
        ],
        cwd=testing_dir,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        encoding="utf-8",
    )
    assert result.returncode == 0
    result = build_package(testing_dir=testing_dir)
    assert (
        "poetry-version-plugin: Git tag found, setting dynamic version to: 0.0.9"
        in result.stdout
    )
    assert "Built test_custom_version-0.0.9-py3-none-any.whl" in result.stdout
    wheel_path = testing_dir / "dist" / "test_custom_version-0.0.9-py3-none-any.whl"
    info = pkginfo.get_metadata(str(wheel_path))
    assert info.version == "0.0.9"
