# Copyright 2018 The TensorFlow Authors. All Rights Reserved.
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
# =============================================================================
"""Exposes the python wrapper for TensorRT graph transforms."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# pylint: disable=unused-import,line-too-long
from tensorflow.contrib.tensorrt.python.ops import trt_engine_op
from tensorflow.contrib.tensorrt.python.trt_convert import add_test_value
from tensorflow.contrib.tensorrt.python.trt_convert import calib_graph_to_infer_graph
from tensorflow.contrib.tensorrt.python.trt_convert import clear_test_values
from tensorflow.contrib.tensorrt.python.trt_convert import create_inference_graph
from tensorflow.contrib.tensorrt.python.trt_convert import enable_test_value
from tensorflow.contrib.tensorrt.python.trt_convert import get_test_value
from tensorflow.contrib.tensorrt.python.trt_convert import is_tensorrt_enabled
# pylint: enable=unused-import,line-too-long
