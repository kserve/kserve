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

import joblib
import numpy as np
import pytest
from sklearn.linear_model import LogisticRegression

from sklearnserver import SKLearnModel
from sklearnserver._safe_unpickle import (
    ENV_ALLOW_UNSAFE_DESERIALIZATION,
    UnsafeArtifactError,
    safe_joblib_load,
)


def _dump(obj, model_dir):
    joblib.dump(obj, os.path.join(model_dir, "model.joblib"))
    return str(model_dir)


def _benign_model():
    return LogisticRegression().fit(np.array([[0.0], [1.0]]), np.array([0, 1]))


class _PoisonedArtifact:
    """Reduces to ``os.system`` — a poisoned artifact whose load would run code."""

    def __init__(self, sentinel):
        self._sentinel = sentinel

    def __reduce__(self):
        return (os.system, (f"touch {self._sentinel}",))


class _NonAllowlistedButHarmless:
    """Reduces to a non-allowlisted global (``os.getpid``) that does no harm.

    Used to show the gate flips: refused by default, permitted under the
    explicit opt-out — without running anything destructive.
    """

    def __reduce__(self):
        return (os.getpid, ())


def test_load_benign_sklearn_model(tmp_path):
    model = SKLearnModel("m", _dump(_benign_model(), tmp_path))
    assert model.load() is True
    assert model.ready is True


def test_poisoned_artifact_is_refused_and_payload_not_executed(
    tmp_path, monkeypatch
):
    monkeypatch.delenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, raising=False)
    sentinel = tmp_path / "pwned"
    model = SKLearnModel("m", _dump(_PoisonedArtifact(str(sentinel)), tmp_path))
    with pytest.raises(UnsafeArtifactError):
        model.load()
    assert not sentinel.exists()  # os.system must never have run
    assert model.ready is False


def test_non_allowlisted_global_refused_by_default(tmp_path, monkeypatch):
    monkeypatch.delenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, raising=False)
    path = tmp_path / "model.joblib"
    joblib.dump(_NonAllowlistedButHarmless(), path)
    with pytest.raises(UnsafeArtifactError):
        safe_joblib_load(path)


def test_opt_out_restores_unrestricted_loading(tmp_path, monkeypatch):
    monkeypatch.setenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, "true")
    path = tmp_path / "model.joblib"
    joblib.dump(_NonAllowlistedButHarmless(), path)
    assert isinstance(safe_joblib_load(path), int)


def test_opt_out_still_loads_benign_model(tmp_path, monkeypatch):
    monkeypatch.setenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, "true")
    model = SKLearnModel("m", _dump(_benign_model(), tmp_path))
    assert model.load() is True


# --- in-allowlist gadgets: callables reachable from the scientific stack that
#     re-enter an unrestricted deserializer must still be refused. ---


class _ReadPickleGadget:
    """Reduces to ``pandas.read_pickle`` (under the allowed ``pandas`` prefix)."""

    def __init__(self, target):
        self._target = target

    def __reduce__(self):
        import pandas

        return (pandas.read_pickle, (self._target,))


class _NumpyLoadGadget:
    """Reduces to ``numpy.load(..., allow_pickle=True)`` (allowed ``numpy``)."""

    def __init__(self, target):
        self._target = target

    def __reduce__(self):
        return (np.load, (self._target, None, True))


def test_pandas_read_pickle_gadget_refused(tmp_path, monkeypatch):
    monkeypatch.delenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, raising=False)
    sentinel = tmp_path / "pwned"
    stage2 = tmp_path / "stage2.pkl"
    joblib.dump(_PoisonedArtifact(str(sentinel)), stage2)
    path = tmp_path / "model.joblib"
    joblib.dump(_ReadPickleGadget(str(stage2)), path)
    with pytest.raises(UnsafeArtifactError):
        safe_joblib_load(path)
    assert not sentinel.exists()


def test_numpy_load_gadget_refused(tmp_path, monkeypatch):
    monkeypatch.delenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, raising=False)
    sentinel = tmp_path / "pwned"
    npy = tmp_path / "stage2.npy"
    np.save(npy, np.array([_PoisonedArtifact(str(sentinel))], dtype=object), allow_pickle=True)
    path = tmp_path / "model.joblib"
    joblib.dump(_NumpyLoadGadget(str(npy)), path)
    with pytest.raises(UnsafeArtifactError):
        safe_joblib_load(path)
    assert not sentinel.exists()


def test_joblib_load_gadget_refused(tmp_path, monkeypatch):
    monkeypatch.delenv(ENV_ALLOW_UNSAFE_DESERIALIZATION, raising=False)
    sentinel = tmp_path / "pwned"
    stage2 = tmp_path / "stage2.joblib"
    joblib.dump(_PoisonedArtifact(str(sentinel)), stage2)

    class _JoblibLoadGadget:
        def __reduce__(self):
            return (joblib.load, (str(stage2),))

    path = tmp_path / "model.joblib"
    joblib.dump(_JoblibLoadGadget(), path)
    with pytest.raises(UnsafeArtifactError):
        safe_joblib_load(path)
    assert not sentinel.exists()
