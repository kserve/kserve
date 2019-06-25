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
# ==============================================================================
"""Model script to test TF-TensorRT integration."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.contrib.tensorrt.test import tf_trt_integration_test_base as trt_test
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import gen_array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import test


class BatchMatMulTest(trt_test.TfTrtIntegrationTestBase):

  def GetParams(self):
    """Testing conversion of BatchMatMul in TF-TRT conversion."""
    dtype = dtypes.float32
    input_name = "input"
    input_dims = [12, 5, 8, 12]
    output_name = "output"
    w1_name = "matmul_w1"
    w1_dims = [12, 5, 12, 7]
    w2_name = "matmul_w2"
    w2_dims = [12, 12, 7]
    g = ops.Graph()
    with g.as_default():
      inp = array_ops.placeholder(
          dtype=dtype, shape=[None] + input_dims[1:], name=input_name)
      w1 = array_ops.placeholder(dtype=dtype, shape=w1_dims, name=w1_name)
      w2 = array_ops.placeholder(dtype=dtype, shape=w2_dims, name=w2_name)
      with g.device("/GPU:0"):
        b = constant_op.constant(np.random.randn(12, 5, 12, 7), dtype=dtype)
        x1 = math_ops.matmul(inp, b)
        c = constant_op.constant(np.random.randn(5, 1, 1), dtype=dtype)
        x1 = x1 + c

        x2 = math_ops.matmul(inp, w1)
        d = constant_op.constant(np.random.randn(5, 1, 1), dtype=dtype)
        x2 = x2 * d

        e = self.trt_incompatible_op(inp)
        e = gen_array_ops.reshape(e, [12, 40, 12])
        x3 = math_ops.matmul(e, w2)
        f = constant_op.constant(np.random.randn(40, 1), dtype=dtype)
        x3 = x3 + f
        x3 = gen_array_ops.reshape(x3, [12, 5, 8, 7])
        x3 = self.trt_incompatible_op(x3)

        out = x1 + x2 + x3
      array_ops.squeeze(out, name=output_name)
    return trt_test.TfTrtIntegrationTestParams(
        gdef=g.as_graph_def(),
        input_names=[input_name, w1_name, w2_name],
        input_dims=[input_dims, w1_dims, w2_dims],
        output_names=[output_name],
        expected_output_dims=[(12, 5, 8, 7)])

  def ExpectedEnginesToBuild(self, run_params):
    """Return the expected engines to build."""
    if (run_params.dynamic_engine and
        not trt_test.IsQuantizationMode(run_params.precision_mode)):
      return ["TRTEngineOp_0", "TRTEngineOp_1"]
    return ["TRTEngineOp_1"]

  def ExpectedEnginesToRun(self, run_params):
    """Return the expected engines to run."""
    return ["TRTEngineOp_1"]

  def ShouldRunTest(self, run_params):
    """Whether to run the test."""
    # TODO(aaroey): Trt library will fail like:
    #
    # ../builder/cudnnBuilder2.cpp:685:
    # virtual std::vector<nvinfer1::query::Ports<
    #     nvinfer1::query::TensorRequirements>>
    # nvinfer1::builder::Node::getSupportedFormats(
    #     const nvinfer1::query::Ports<nvinfer1::query::AbstractTensor>&,
    #     const nvinfer1::cudnn::HardwareContext&,
    #     nvinfer1::builder::Format::Type,
    #     const nvinfer1::builder::FormatTypeHack&) const:
    # Assertion `sf' failed.
    #
    # To reproduce, run:
    # bazel test -c opt --copt=-mavx \
    #   --test_arg=BatchMatMulTest.testTfTrt_ToolConversion_INT8_DynamicEngine \
    #   tensorflow/contrib/tensorrt:batch_matmul_test
    #
    # Investigate and fix it.
    return not trt_test.IsQuantizationMode(run_params.precision_mode)


if __name__ == "__main__":
  test.main()
