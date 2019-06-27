# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ==============================================================================

"""Ops for building quantized models."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# pylint: disable=unused-import,wildcard-import,g-bad-import-order
from tensorflow.contrib.quantization.python import array_ops as quantized_array_ops
from tensorflow.contrib.quantization.python.math_ops import *
from tensorflow.contrib.quantization.python.nn_ops import *

from tensorflow.python.ops import gen_array_ops as quantized_gen_array_ops
from tensorflow.python.ops.gen_array_ops import dequantize
from tensorflow.python.ops.gen_array_ops import quantize_v2
from tensorflow.python.ops.gen_array_ops import quantized_concat
# pylint: enable=unused-import,wildcard-import,g-bad-import-order
