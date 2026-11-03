import numpy as np
from PIL import Image

_DatatypeToNumpy = {
    "BOOL": "bool",
    "UINT8": "uint8",
    "UINT16": "uint16",
    "UINT32": "uint32",
    "UINT64": "uint64",
    "INT8": "int8",
    "INT16": "int16",
    "INT32": "int32",
    "INT64": "int64",
    "FP16": "float16",
    "FP32": "float32",
    "FP64": "float64",
    "BYTES": "byte",
}

_NumpyToDatatype = {value: key for key, value in _DatatypeToNumpy.items()}

# NOTE: numpy has more types than v2 protocol
_NumpyToDatatype["object"] = "BYTES"


def _to_datatype(dtype: np.dtype) -> str:
    """
    Converts numpy datatype to KServe datatype
    """
    as_str = str(dtype)
    datatype = _NumpyToDatatype[as_str]

    return datatype


def check_image_with_pil(path):
    """
    Check if input file is an image
    """
    try:
        Image.open(path)
    except IOError:
        return False
    return True
