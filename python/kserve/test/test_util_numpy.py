import numpy as np
import pytest
from kserve.utils.numpy_codec import from_np_dtype


def test_from_np_dtype_arrow_marked():
    # INT8
    assert from_np_dtype(np.int8) == "INT8"

    # INT16
    assert from_np_dtype(np.int16) == "INT16"

    # UINT16
    assert from_np_dtype(np.uint16) == "UINT16"

    # UINT64
    assert from_np_dtype(np.uint64) == "UINT64"

    # FP64
    assert from_np_dtype(np.float64) == "FP64"

    # None (unknown dtype)
    unknown_dtype = np.dtype("complex64")
    assert from_np_dtype(unknown_dtype) is None
