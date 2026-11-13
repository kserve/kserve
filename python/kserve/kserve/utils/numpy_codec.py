# Copyright 2021 The KServe Authors.
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


def to_np_dtype(dtype):
    dtype_map = {
        "BOOL": bool,
        "INT8": np.int8,
        "INT16": np.int16,
        "INT32": np.int32,
        "INT64": np.int64,
        "UINT8": np.uint8,
        "UINT16": np.uint16,
        "UINT32": np.uint32,
        "UINT64": np.uint64,
        "FP16": np.float16,
        "FP32": np.float32,
        "FP64": np.float64,
        "BYTES": np.object_,
    }
    return dtype_map.get(dtype, None)


def from_np_dtype(np_dtype):
    if np_dtype is None:
        return None
    try:
        dt = np.dtype(np_dtype)
    except (TypeError, ValueError):
        return None

    if dt == np.bool_:
        return "BOOL"
    elif dt == np.int8:
        return "INT8"
    elif dt == np.int16:
        return "INT16"
    elif dt == np.int32:
        return "INT32"
    elif dt == np.int64:
        return "INT64"
    elif dt == np.uint8:
        return "UINT8"
    elif dt == np.uint16:
        return "UINT16"
    elif dt == np.uint32:
        return "UINT32"
    elif dt == np.uint64:
        return "UINT64"
    elif dt == np.float16:
        return "FP16"
    elif dt == np.float32:
        return "FP32"
    elif dt == np.float64:
        return "FP64"
    elif (
        dt == np.object_
        or dt.type == np.bytes_
        or dt.type == np.str_
        or np.issubdtype(dt, np.datetime64)
    ):
        return "BYTES"
    return None
