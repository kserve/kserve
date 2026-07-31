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

import numpy as np
import pytest

from kserve.utils.numpy_codec import from_np_dtype, to_np_dtype


@pytest.mark.parametrize(
    "array, expected",
    [
        (np.array([True, False]), "BOOL"),
        (np.array([1, 2], dtype=np.int8), "INT8"),
        (np.array([1, 2], dtype=np.int16), "INT16"),
        (np.array([1, 2], dtype=np.int32), "INT32"),
        (np.array([1, 2], dtype=np.int64), "INT64"),
        (np.array([1, 2], dtype=np.uint8), "UINT8"),
        (np.array([1, 2], dtype=np.float32), "FP32"),
        (np.array([1, 2], dtype=np.float64), "FP64"),
        (np.array(["a", "bb"]), "BYTES"),  # unicode string dtype (kind "U")
        (np.array([b"a", b"bb"]), "BYTES"),  # byte string dtype (kind "S")
        (np.array(["a"], dtype=object), "BYTES"),  # object dtype
        (np.array(["2020-01-01"], dtype="datetime64[s]"), "BYTES"),
    ],
)
def test_from_np_dtype(array, expected):
    assert from_np_dtype(array.dtype) == expected


def test_from_np_dtype_unicode_is_bytes():
    """Regression: numpy unicode string dtype must map to BYTES, not None.

    ``np.array(["a", "b"])`` yields a ``<U`` (unicode) dtype. Previously only
    byte-string (``|S``) and object dtypes were recognised, so a unicode string
    array produced ``None`` and an InferOutput with ``datatype=None``.
    """
    assert from_np_dtype(np.array(["cat", "dog"]).dtype) == "BYTES"


@pytest.mark.parametrize(
    "datatype",
    ["BOOL", "INT8", "INT16", "INT32", "INT64", "UINT8", "FP16", "FP32", "FP64"],
)
def test_np_dtype_round_trip(datatype):
    """Numeric datatypes round-trip through to_np_dtype / from_np_dtype."""
    assert from_np_dtype(np.dtype(to_np_dtype(datatype))) == datatype
