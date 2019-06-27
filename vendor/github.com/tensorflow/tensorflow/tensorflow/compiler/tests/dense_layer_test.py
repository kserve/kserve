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
"""Tests for DenseLayer JIT compilation on the CPU and GPU devices."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os
import numpy as np

from tensorflow.compiler.tests import test_utils
from tensorflow.contrib.compiler import jit
from tensorflow.core.protobuf import config_pb2
from tensorflow.python.layers import layers
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test

jit_scope = jit.experimental_jit_scope

def GetRunMetadataLabels(run_metadata):
  """Returns all labels in run_metadata."""
  labels = []
  for dev_stats in run_metadata.step_stats.dev_stats:
    for node_stats in dev_stats.node_stats:
      labels.append(node_stats.timeline_label)
  return labels


def InLabels(labels, substr):
  """Returns true iff one of the labels contains substr."""
  return any(substr in x for x in labels)


class DenseLayerTest(test.TestCase):

  def countXlaOps(self, labels):
    """Count how many XlaCompile/XlaRun labels are present."""
    xla_compile_count = sum("XlaCompile(" in x for x in labels)
    xla_run_count = sum("XlaRun(" in x for x in labels)
    self.assertEqual(xla_compile_count, xla_run_count)
    return xla_run_count


  def testDenseLayerAutoJit(self):
    """Tests dense layer compilation in auto-jit mode.

    Dense layer should be compiled into a single XlaCompile/XlaRun op pair in
    auto-jit mode.
    """

    os.environ["TF_XLA_FLAGS"] = (
        "--tf_xla_cpu_global_jit " + os.environ.get("TF_XLA_FLAGS", ""))
    config = config_pb2.ConfigProto()
    config.graph_options.optimizer_options.global_jit_level = (
        config_pb2.OptimizerOptions.ON_1)

    with self.session(config=config) as sess:
      x = array_ops.placeholder(shape=[None, None, 3], dtype=np.float32)
      y = layers.dense(x, 3)

      self.evaluate(variables.initialize_all_variables())
      run_metadata = config_pb2.RunMetadata()
      test_utils.RunWithWarmup(
          sess,
          y, {x: np.array([[[1, 2, 3], [4, 5, 6]], [[1, 2, 3], [4, 5, 6]]])},
          run_metadata=run_metadata,
          options=config_pb2.RunOptions(
              trace_level=config_pb2.RunOptions.FULL_TRACE))

    labels = GetRunMetadataLabels(run_metadata)
    self.assertEqual(1, self.countXlaOps(labels))
    self.assertFalse(InLabels(labels, "MatMult"))

  def testDenseLayerJitScopeDefinedShape(self):
    """Tests that the dense layer node is properly compiled in jit scope.

    Dense layer with static shape input tensor should be compiled into a single
    XlaCompile/XlaRun op pair by XLA.
    """

    with self.cached_session() as sess:
      x = array_ops.placeholder(shape=[2, 2, 3], dtype=np.float32)
      with jit_scope():
        y = layers.dense(x, 3)

      self.evaluate(variables.initialize_all_variables())
      run_metadata = config_pb2.RunMetadata()
      test_utils.RunWithWarmup(
          sess,
          y, {x: np.array([[[1, 2, 3], [4, 5, 6]], [[1, 2, 3], [4, 5, 6]]])},
          run_metadata=run_metadata,
          options=config_pb2.RunOptions(
              trace_level=config_pb2.RunOptions.FULL_TRACE))

    labels = GetRunMetadataLabels(run_metadata)
    self.assertEqual(1, self.countXlaOps(labels))
    # No need to check whether ListDiff is compiled or not because ListDiff op
    # is not used when input tensor shape is fully defined.

  def testDenseLayerJitScopeUndefinedShape(self):
    """Tests that the dense layer node is properly compiled in jit scope.

    Dense layer uses shape op to get shape of input tensor if its shape is not
    fully defined. XLA does not cluster shape op with other operators. But in
    experimental_jit_scope, XLA is forced to compile shape op into its own
    cluster, causing dense layer to be split into TWO XlaCompile/XlaRun op
    pairs.
    """

    with self.cached_session() as sess:
      x = array_ops.placeholder(shape=[None, None, 3], dtype=np.float32)
      with jit_scope():
        y = layers.dense(x, 3)

      self.evaluate(variables.initialize_all_variables())
      run_metadata = config_pb2.RunMetadata()
      test_utils.RunWithWarmup(
          sess,
          y, {x: np.array([[[1, 2, 3], [4, 5, 6]], [[1, 2, 3], [4, 5, 6]]])},
          run_metadata=run_metadata,
          options=config_pb2.RunOptions(
              trace_level=config_pb2.RunOptions.FULL_TRACE))

    labels = GetRunMetadataLabels(run_metadata)
    self.assertEqual(2, self.countXlaOps(labels))
    self.assertFalse(InLabels(labels, "MatMult"))


if __name__ == "__main__":
  os.environ["TF_XLA_FLAGS"] = ("--tf_xla_enable_lazy_compilation=true " +
                                os.environ.get("TF_XLA_FLAGS", ""))
  test.main()
