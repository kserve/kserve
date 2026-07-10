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

import pickle

import pytest
from kserve.errors import InferenceError

from autogluonserver.safe_deserialize import (
    assert_safe_autogluon_artifact,
    scan_pickle_for_blocked_globals,
)

pytestmark = pytest.mark.autogluon


def test_scan_pickle_for_blocked_globals_detects_os_system(tmp_path):
    payload = b"cos\nsystem\n(Vecho hacked\ntR."
    model_file = tmp_path / "predictor.pkl"
    model_file.write_bytes(payload)

    findings = scan_pickle_for_blocked_globals(model_file)

    assert findings == ["os.system"]


def test_scan_pickle_for_blocked_globals_allows_benign_payload(tmp_path):
    payload = pickle.dumps({"k": [1, 2, 3]})
    model_file = tmp_path / "predictor.pkl"
    model_file.write_bytes(payload)

    findings = scan_pickle_for_blocked_globals(model_file)

    assert findings == []


def test_assert_safe_autogluon_artifact_raises_on_blocked_global(tmp_path):
    payload = b"csubprocess\nPopen\n(Vecho blocked\ntR."
    model_file = tmp_path / "nested" / "predictor.pkl"
    model_file.parent.mkdir()
    model_file.write_bytes(payload)

    with pytest.raises(InferenceError, match="Blocked unsafe pickle GLOBAL"):
        assert_safe_autogluon_artifact(str(tmp_path))


def test_assert_safe_autogluon_artifact_can_be_disabled(tmp_path, monkeypatch):
    payload = b"cos\nsystem\n(Vecho disabled\ntR."
    model_file = tmp_path / "predictor.pkl"
    model_file.write_bytes(payload)
    monkeypatch.setenv("AUTOGLUON_SAFE_PICKLE_SCAN", "false")

    assert_safe_autogluon_artifact(str(tmp_path))
