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
import zipfile
from io import BytesIO
from kserve.errors import InferenceError

from autogluonserver.safe_deserialize import (
    ENV_SAFE_LOAD_MODE,
    SafeLoadMode,
    get_safe_load_mode,
    scan_model_artifacts,
    validate_model_artifacts_for_safe_load,
)

pytestmark = pytest.mark.autogluon


def _write_global_pickle(path, module: str, name: str) -> None:
    path.write_bytes(f"c{module}\n{name}\n.".encode("utf-8"))


def _write_stack_global_pickle(path, module: str, name: str) -> None:
    module_b = module.encode("utf-8")
    name_b = name.encode("utf-8")
    payload = (
        b"\x80\x04"
        + bytes([0x8C, len(module_b)])
        + module_b
        + bytes([0x8C, len(name_b)])
        + name_b
        + b"\x93."
    )
    path.write_bytes(payload)


def _write_zip_with_pickle_entry(path, entry_name: str, payload: bytes) -> None:
    buff = BytesIO()
    with zipfile.ZipFile(buff, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr(entry_name, payload)
    path.write_bytes(buff.getvalue())


def test_safe_load_mode_defaults_to_off(monkeypatch):
    monkeypatch.delenv(ENV_SAFE_LOAD_MODE, raising=False)
    assert get_safe_load_mode() is SafeLoadMode.OFF


def test_safe_load_mode_invalid_value_falls_back_to_off(monkeypatch):
    monkeypatch.setenv(ENV_SAFE_LOAD_MODE, "invalid")
    assert get_safe_load_mode() is SafeLoadMode.OFF


def test_scan_reports_forbidden_refs(tmp_path):
    _write_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    result = scan_model_artifacts(
        str(tmp_path),
        allow_module_prefixes=("autogluon", "builtins"),
        scan_patterns=("*.pkl",),
    )
    assert result.status == "forbidden_refs"
    assert len(result.forbidden_refs) == 1
    assert result.forbidden_refs[0].module == "evil.module"
    assert result.forbidden_refs[0].name == "BadClass"


def test_scan_extracts_stack_global_refs(tmp_path):
    _write_stack_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    result = scan_model_artifacts(
        str(tmp_path),
        allow_module_prefixes=("autogluon",),
        scan_patterns=("*.pkl",),
    )
    assert result.status == "forbidden_refs"
    assert len(result.forbidden_refs) == 1
    assert result.forbidden_refs[0].module == "evil.module"


def test_validate_enforce_raises_for_forbidden_refs(tmp_path, monkeypatch):
    _write_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    monkeypatch.setenv(ENV_SAFE_LOAD_MODE, "enforce")
    with pytest.raises(InferenceError, match="Safe-load validation"):
        validate_model_artifacts_for_safe_load(str(tmp_path))


def test_validate_permissive_allows_forbidden_refs(tmp_path, monkeypatch):
    _write_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    monkeypatch.setenv(ENV_SAFE_LOAD_MODE, "permissive")
    validate_model_artifacts_for_safe_load(str(tmp_path))


def test_validate_off_skips_checking(tmp_path, monkeypatch):
    _write_global_pickle(tmp_path / "predictor.pkl", "evil.module", "BadClass")
    monkeypatch.setenv(ENV_SAFE_LOAD_MODE, "off")
    validate_model_artifacts_for_safe_load(str(tmp_path))


def test_scan_reports_forbidden_refs_inside_zip_pickle_container(tmp_path):
    _write_zip_with_pickle_entry(
        tmp_path / "model-internals.pkl",
        "archive/data.pkl",
        b"cevil.module\nBadClass\n.",
    )
    result = scan_model_artifacts(
        str(tmp_path),
        allow_module_prefixes=("autogluon", "builtins"),
        scan_patterns=("*.pkl",),
    )
    assert result.status == "forbidden_refs"
    assert len(result.forbidden_refs) == 1
    assert result.forbidden_refs[0].module == "evil.module"


def test_scan_ignores_non_pickle_entries_in_zip_container(tmp_path):
    _write_zip_with_pickle_entry(
        tmp_path / "model-internals.pkl", "weights.bin", b"\x00\x01\x02"
    )
    result = scan_model_artifacts(
        str(tmp_path),
        allow_module_prefixes=("autogluon", "builtins"),
        scan_patterns=("*.pkl",),
    )
    assert result.status == "ok"
