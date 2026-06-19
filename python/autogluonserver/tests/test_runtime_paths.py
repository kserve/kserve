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

import os

import pytest

from autogluonserver.runtime_paths import ensure_autogluon_runtime_paths


@pytest.fixture
def clean_runtime_env(monkeypatch):
    """Isolate MPLCONFIGDIR, TMPDIR, and HOME for runtime path tests."""
    for key in ("MPLCONFIGDIR", "TMPDIR", "HOME"):
        monkeypatch.delenv(key, raising=False)
    yield


@pytest.fixture
def restore_cwd():
    """Prevent cross-test leakage from process-wide chdir."""
    original_cwd = os.getcwd()
    try:
        yield
    finally:
        os.chdir(original_cwd)


def test_sets_mplconfigdir_when_unset(clean_runtime_env):
    ensure_autogluon_runtime_paths()
    assert os.environ["MPLCONFIGDIR"] == "/tmp/matplotlib"


def test_preserves_existing_mplconfigdir(monkeypatch, clean_runtime_env):
    monkeypatch.setenv("MPLCONFIGDIR", "/custom/mpl")
    ensure_autogluon_runtime_paths()
    assert os.environ["MPLCONFIGDIR"] == "/custom/mpl"


def test_chdirs_to_writable_tmpdir(
    monkeypatch, tmp_path, clean_runtime_env, restore_cwd
):
    writable = tmp_path / "work"
    writable.mkdir()
    monkeypatch.setenv("TMPDIR", str(writable))

    ensure_autogluon_runtime_paths()

    assert os.getcwd() == str(writable.resolve())


def test_skips_home_when_home_is_root(
    monkeypatch, tmp_path, clean_runtime_env, restore_cwd
):
    writable = tmp_path / "tmpdir"
    writable.mkdir()
    monkeypatch.setenv("TMPDIR", str(writable))
    monkeypatch.setenv("HOME", "/")

    ensure_autogluon_runtime_paths()

    assert os.getcwd() == str(writable.resolve())


def test_prefers_tmpdir_over_home(
    monkeypatch, tmp_path, clean_runtime_env, restore_cwd
):
    tmpdir = tmp_path / "tmp"
    tmpdir.mkdir()
    home = tmp_path / "home"
    home.mkdir()
    monkeypatch.setenv("TMPDIR", str(tmpdir))
    monkeypatch.setenv("HOME", str(home))

    ensure_autogluon_runtime_paths()

    assert os.getcwd() == str(tmpdir.resolve())


def test_falls_back_to_home_when_tmp_not_writable(
    monkeypatch, tmp_path, clean_runtime_env
):
    tmpdir = tmp_path / "tmp"
    tmpdir.mkdir()
    home = tmp_path / "home"
    home.mkdir()
    monkeypatch.setenv("TMPDIR", str(tmpdir))
    monkeypatch.setenv("HOME", str(home))

    chosen = []
    blocked = str(tmpdir.resolve())
    target_home = str(home.resolve())
    real_chdir = os.chdir

    def fake_chdir(path):
        resolved = str(os.path.realpath(path))
        if resolved == blocked:
            raise OSError("simulated chdir failure")
        chosen.append(resolved)
        real_chdir(path)

    monkeypatch.setattr("autogluonserver.runtime_paths.os.chdir", fake_chdir)

    ensure_autogluon_runtime_paths()

    assert chosen == [target_home]
    assert os.getcwd() == target_home
