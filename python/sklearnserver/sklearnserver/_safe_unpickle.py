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

"""Restricted loading of joblib/pickle sklearn artifacts.

``joblib.load`` (like ``pickle``) executes arbitrary Python embedded in the
artifact through ``__reduce__``, so loading a model whose bytes an attacker can
influence (e.g. an InferenceService ``storageUri`` pointing at attacker-written
data) can run arbitrary code in the model-server process.

To make the default path safe without requiring models to be re-serialized, the
artifact is loaded through a restricted unpickler. Restricting only by
*module prefix* is not sufficient: the scientific stack itself exposes callables
that re-enter an unrestricted deserializer (e.g. ``pandas.read_pickle``,
``numpy.load(..., allow_pickle=True)``), so an allowlist that permits whole
packages can be bypassed. Instead the unpickler permits:

  * any **class** defined under the scientific-stack packages (estimators,
    ``numpy``/``scipy`` array and dtype types, ``pandas`` containers, joblib's
    array wrapper). Their reduce arguments are themselves restricted, so they
    cannot smuggle a dangerous callable; and
  * a small, curated set of **reconstruction-helper functions** that genuine
    artifacts need (``copyreg``/``numpy`` reconstructors, the Cython
    ``__pyx_unpickle_*`` / ``newObj`` helpers, ``numpy.random`` ctors); plus
  * a few safe builtin/stdlib **names** (container/scalar types,
    ``copyreg``/``_codecs`` reconstruction helpers).

Every other global -- including any non-helper function reachable from the
scientific stack, and anything from modules such as ``os``/``subprocess``/
``builtins.eval`` -- is refused before it can be resolved or called, so a
poisoned artifact cannot execute code.

Operators who fully trust the artifact and its source can restore the original
unrestricted ``joblib.load`` by setting ``KSERVE_ALLOW_UNSAFE_DESERIALIZATION``.
"""

import os
import threading
from typing import Any

import joblib
import joblib.numpy_pickle as _joblib_numpy_pickle
from kserve.logging import logger

ENV_ALLOW_UNSAFE_DESERIALIZATION = "KSERVE_ALLOW_UNSAFE_DESERIALIZATION"

# Top-level packages a genuine sklearn artifact references. Classes defined under
# these packages are allowed; callables under them are allowed only if they are
# recognised reconstruction helpers (see below). Importantly, being under one of
# these prefixes is NOT by itself sufficient for a *function* to be allowed --
# that is what lets e.g. ``pandas.read_pickle`` / ``numpy.load`` be refused.
_ALLOWED_MODULE_PREFIXES = (
    "sklearn",
    "numpy",
    "scipy",
    "pandas",
    "joblib",
)

# Reconstruction-helper *functions* in the scientific stack that genuine
# artifacts need (these are not exposed as classes). They only rebuild an object
# of an already-restricted type and cannot be repurposed to run arbitrary code.
_ALLOWED_RECONSTRUCTORS = frozenset(
    {
        # numpy ndarray/scalar reconstruction (``numpy.core`` pre-2.0,
        # ``numpy._core`` on numpy >= 2.0).
        ("numpy.core.multiarray", "_reconstruct"),
        ("numpy.core.multiarray", "scalar"),
        ("numpy.core.multiarray", "_frombuffer"),
        ("numpy._core.multiarray", "_reconstruct"),
        ("numpy._core.multiarray", "scalar"),
        ("numpy._core.multiarray", "_frombuffer"),
        # numpy.random generator/state reconstruction.
        ("numpy.random._pickle", "__bit_generator_ctor"),
        ("numpy.random._pickle", "__generator_ctor"),
        ("numpy.random._pickle", "__randomstate_ctor"),
        ("numpy.random.bit_generator", "__pyx_unpickle_SeedSequence"),
    }
)

# Safe individual names from stdlib modules that also expose dangerous callables,
# so the whole module cannot be allowed. (e.g. ``builtins`` also holds ``eval``.)
# These are container/scalar *types* plus the standard reconstruction helpers.
_ALLOWED_QUALIFIED_NAMES = {
    "builtins": frozenset(
        {
            "object",
            "list",
            "dict",
            "set",
            "frozenset",
            "tuple",
            "bytearray",
            "bytes",
            "str",
            "int",
            "float",
            "bool",
            "complex",
            "slice",
            "range",
        }
    ),
    "collections": frozenset({"OrderedDict", "defaultdict", "Counter", "deque"}),
    "copyreg": frozenset({"_reconstructor", "__newobj__", "__newobj_ex__"}),
    "_codecs": frozenset({"encode"}),
}

_PATCH_LOCK = threading.Lock()


class UnsafeArtifactError(Exception):
    """Raised when an artifact references a global outside the safelist."""


def _is_cython_reconstructor(name: str) -> bool:
    """Cython generates these helpers to reconstruct extension-type instances.

    They live under the scientific-stack packages, only call ``cls.__new__`` /
    rebuild a fixed extension type, and so cannot be used to run arbitrary code.
    Matching them by shape avoids having to enumerate every estimator's helper.
    """
    return name == "newObj" or name.startswith("__pyx_unpickle_")


def _resolve_disallowed(module: str, name: str) -> "UnsafeArtifactError":
    return UnsafeArtifactError(
        f"Refusing to deserialize disallowed global '{module}.{name}' while "
        f"loading the model. The artifact references code outside the allowed "
        f"safelist; loading it could execute arbitrary code. If you fully trust "
        f"this artifact and the location it was read from, set "
        f"{ENV_ALLOW_UNSAFE_DESERIALIZATION}=true to load it with the "
        f"unrestricted joblib loader."
    )


class _RestrictedNumpyUnpickler(_joblib_numpy_pickle.NumpyUnpickler):
    """``NumpyUnpickler`` that refuses to resolve non-allowlisted globals.

    ``find_class`` is invoked by the unpickler for every ``GLOBAL`` /
    ``STACK_GLOBAL`` opcode, including the callable a ``REDUCE`` (``__reduce__``)
    is about to invoke. Refusing the global here stops the gadget from being
    constructed, so no attacker code runs. A global is only resolved (imported)
    once it has passed the module/name gate, so artifacts cannot trigger imports
    of arbitrary modules either.
    """

    def find_class(self, module: str, name: str) -> Any:
        top_level = module.split(".", 1)[0]

        if top_level in _ALLOWED_MODULE_PREFIXES:
            obj = super().find_class(module, name)
            # Classes are safe: their reduce args are restricted in turn, so a
            # class cannot be used to smuggle a dangerous callable.
            if isinstance(obj, type):
                return obj
            # Otherwise it is a function/other callable: only the curated
            # reconstruction helpers are allowed. This refuses scientific-stack
            # functions that re-enter an unrestricted deserializer, such as
            # ``pandas.read_pickle`` or ``numpy.load``.
            if (module, name) in _ALLOWED_RECONSTRUCTORS or _is_cython_reconstructor(
                name
            ):
                return obj
            raise _resolve_disallowed(module, name)

        # Modules outside the scientific stack: only a fixed set of safe names
        # (container/scalar types and standard reconstruction helpers). Anything
        # else is refused without importing the module.
        allowed_names = _ALLOWED_QUALIFIED_NAMES.get(module)
        if allowed_names is not None and name in allowed_names:
            return super().find_class(module, name)
        raise _resolve_disallowed(module, name)


def _allow_unsafe_deserialization() -> bool:
    value = os.environ.get(ENV_ALLOW_UNSAFE_DESERIALIZATION, "false")
    return value.strip().lower() in ("1", "true", "yes", "on")


def safe_joblib_load(filename: Any) -> Any:
    """Load a joblib/pickle model artifact with restricted globals by default.

    Args:
        filename: Path (or file object) of the model artifact to load.

    Returns:
        The deserialized model object.

    Raises:
        UnsafeArtifactError: If the artifact references a global outside the
            safelist and ``KSERVE_ALLOW_UNSAFE_DESERIALIZATION`` is not set.
    """
    if _allow_unsafe_deserialization():
        logger.warning(
            "%s is enabled; loading the model with the UNRESTRICTED joblib "
            "loader, which executes arbitrary code embedded in the artifact. "
            "Only use this for artifacts from a fully trusted source.",
            ENV_ALLOW_UNSAFE_DESERIALIZATION,
        )
        return joblib.load(filename)

    # joblib.load constructs ``NumpyUnpickler`` internally and exposes no hook to
    # swap it, so the module-global unpickler is temporarily replaced with the
    # restricted subclass for the duration of the load. This keeps joblib's
    # compression / numpy-array handling intact and stays correct across joblib
    # versions (the ``NumpyUnpickler`` symbol is stable). The lock serializes
    # concurrent loads (e.g. the V2 ``/load`` endpoint) that share the global.
    with _PATCH_LOCK:
        original_unpickler = _joblib_numpy_pickle.NumpyUnpickler
        _joblib_numpy_pickle.NumpyUnpickler = _RestrictedNumpyUnpickler
        try:
            return joblib.load(filename)
        finally:
            _joblib_numpy_pickle.NumpyUnpickler = original_unpickler
