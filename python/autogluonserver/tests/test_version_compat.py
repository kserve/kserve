# Copyright 2026 The KServe Authors.
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

import pytest
from unittest.mock import MagicMock

from kserve.errors import InferenceError
from packaging.version import Version

from autogluonserver.version_compat import (
    load_predictor_tolerating_patch_mismatch,
    _read_saved_version,
    _get_installed_version,
)

pytestmark = pytest.mark.autogluon

# ---------------------------------------------------------------------------
# _read_saved_version
# ---------------------------------------------------------------------------


def test_read_saved_version_reads_version_txt(tmp_path):
    (tmp_path / "version.txt").write_text("1.5.0")
    assert _read_saved_version(str(tmp_path)) == Version("1.5.0")


def test_read_saved_version_reads_local_label(tmp_path):
    (tmp_path / "version.txt").write_text("1.5.0+rhaiv.2")
    assert _read_saved_version(str(tmp_path)) == Version("1.5.0+rhaiv.2")


def test_read_saved_version_falls_back_to_legacy_filename(tmp_path):
    """autogluon.tabular <= 1.1.0 wrote the version to __version__, not version.txt."""
    (tmp_path / "__version__").write_text("0.6.0")
    assert _read_saved_version(str(tmp_path)) == Version("0.6.0")


def test_read_saved_version_prefers_version_txt_over_legacy(tmp_path):
    (tmp_path / "version.txt").write_text("1.5.0")
    (tmp_path / "__version__").write_text("0.6.0")
    assert _read_saved_version(str(tmp_path)) == Version("1.5.0")


def test_read_saved_version_returns_none_when_no_file(tmp_path):
    assert _read_saved_version(str(tmp_path)) is None


def test_read_saved_version_returns_none_on_invalid_content(tmp_path):
    (tmp_path / "version.txt").write_text("not-a-version")
    assert _read_saved_version(str(tmp_path)) is None


# ---------------------------------------------------------------------------
# _get_installed_version
# ---------------------------------------------------------------------------


def test_get_installed_version_uses_module_package(monkeypatch):
    mock_cls = MagicMock()
    mock_cls.__module__ = "autogluon.tabular.predictor.predictor"
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda pkg: "1.5.1"
        if pkg == "autogluon.tabular"
        else (_ for _ in ()).throw(Exception()),
    )
    assert _get_installed_version(mock_cls) == Version("1.5.1")


def test_get_installed_version_returns_none_when_package_not_found(monkeypatch):
    import importlib.metadata

    mock_cls = MagicMock()
    mock_cls.__module__ = "autogluon.tabular.predictor.predictor"
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        MagicMock(side_effect=importlib.metadata.PackageNotFoundError),
    )
    assert _get_installed_version(mock_cls) is None


# ---------------------------------------------------------------------------
# load_predictor_tolerating_patch_mismatch
# ---------------------------------------------------------------------------


def _make_mock_cls():
    mock_cls = MagicMock()
    mock_cls.__module__ = "autogluon.tabular.predictor.predictor"
    mock_cls.load.return_value = MagicMock()
    return mock_cls


def test_matching_versions_load_normally(tmp_path, monkeypatch):
    (tmp_path / "version.txt").write_text("1.5.0")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.5.0",
    )

    load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_called_once_with(str(tmp_path))


def test_patch_version_mismatch_loads_without_version_check(tmp_path, monkeypatch):
    (tmp_path / "version.txt").write_text("1.5.0")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.5.1",
    )

    load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_called_once_with(str(tmp_path), require_version_match=False)


def test_local_version_label_loads_without_version_check(tmp_path, monkeypatch):
    """Predictor saved with a locally patched build (e.g. 1.5.0+rhaiv.2) must load
    against the matching public release — local labels do not affect compatibility."""
    (tmp_path / "version.txt").write_text("1.5.0+rhaiv.2")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.5.0",
    )

    load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_called_once_with(str(tmp_path), require_version_match=False)


def test_minor_version_mismatch_raises_inference_error(tmp_path, monkeypatch):
    (tmp_path / "version.txt").write_text("1.4.0")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.5.0",
    )

    with pytest.raises(InferenceError, match="major or minor version mismatch"):
        load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_not_called()


def test_multi_digit_minor_version_mismatch_raises_inference_error(
    tmp_path, monkeypatch
):
    """1.10.0 vs 1.5.0 must be treated as a minor mismatch (10 != 5),
    not confused by lexicographic ordering where '10' < '5'."""
    (tmp_path / "version.txt").write_text("1.10.0")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.5.0",
    )

    with pytest.raises(InferenceError, match="major or minor version mismatch"):
        load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_not_called()


def test_major_version_mismatch_raises_inference_error(tmp_path, monkeypatch):
    (tmp_path / "version.txt").write_text("0.6.0")
    mock_cls = _make_mock_cls()
    monkeypatch.setattr(
        "autogluonserver.version_compat.importlib.metadata.version",
        lambda _: "1.0.0",
    )

    with pytest.raises(InferenceError, match="major or minor version mismatch"):
        load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_not_called()


def test_missing_version_file_delegates_to_load(tmp_path):
    """When version.txt is absent, fall through to load() so AutoGluon's own checks run."""
    mock_cls = MagicMock()
    mock_cls.__module__ = "autogluon.tabular.predictor.predictor"
    mock_cls.load.return_value = MagicMock()

    load_predictor_tolerating_patch_mismatch(mock_cls, str(tmp_path))

    mock_cls.load.assert_called_once_with(str(tmp_path))
