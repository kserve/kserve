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
    if np_dtype == bool:
        return "BOOL"
    elif np_dtype == np.int8:
        return "INT8"
    elif np_dtype == np.int16:
        return "INT16"
    elif np_dtype == np.int32:
        return "INT32"
    elif np_dtype == np.int64:
        return "INT64"
    elif np_dtype == np.uint8:
        return "UINT8"
    elif np_dtype == np.uint16:
        return "UINT16"
    elif np_dtype == np.uint32:
        return "UINT32"
    elif np_dtype == np.uint64:
        return "UINT64"
    elif np_dtype == np.float16:
        return "FP16"
    elif np_dtype == np.float32:
        return "FP32"
    elif np_dtype == np.float64:
        return "FP64"
    elif np_dtype == np.object_ or np_dtype.type == np.bytes_:
        return "BYTES"
    return None


def from_triton_type_to_np_type(dtype: str):
    dtype_map = {
        "TYPE_BOOL": bool,
        "TYPE_UINT8": np.uint8,
        "TYPE_UINT16": np.uint16,
        "TYPE_UINT32": np.uint32,
        "TYPE_UINT64": np.uint64,
        "TYPE_INT8": np.int8,
        "TYPE_INT16": np.int16,
        "TYPE_INT32": np.int32,
        "TYPE_INT64": np.int64,
        "TYPE_FP16": np.float16,
        "TYPE_FP32": np.float32,
        "TYPE_FP64": np.float64,
        "TYPE_STRING": np.object_,
    }
    return dtype_map.get(dtype, None)
