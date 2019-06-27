# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
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
"""Tests for Clip Operations."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import clip_ops
from tensorflow.python.ops import numerics
from tensorflow.python.platform import test


class ClipOpsTest(test.TestCase):

  def __init__(self, method_name="runTest"):
    super(ClipOpsTest, self).__init__(method_name)

  def _testClipTensorByNorm(self, inputs, max_norm, expected):
    with self.cached_session() as sess:
      input_op = constant_op.constant(inputs)
      clipped = clip_ops.clip_by_norm(input_op, max_norm)
      check_op = numerics.add_check_numerics_ops()
      result, _ = self.evaluate([clipped, check_op])
    self.assertAllClose(result, expected)

  def _testClipIndexedSlicesByNorm(self, values, indices, shape, max_norm,
                                   axes):
    with self.cached_session() as sess:
      values = constant_op.constant(values)
      indices = constant_op.constant(indices)
      shape = constant_op.constant(shape)
      # IndexedSlices mode
      indixed_slices = ops.IndexedSlices(values, indices, shape)
      clipped = clip_ops.clip_by_norm(indixed_slices, max_norm, axes)
      # clipped should be IndexedSlices
      self.assertIsInstance(clipped, ops.IndexedSlices)
      clipped = ops.convert_to_tensor(clipped)

      # Tensor mode
      dense_tensor = ops.convert_to_tensor(indixed_slices)
      dense_clipped = clip_ops.clip_by_norm(dense_tensor, max_norm, axes)
      result, expected = self.evaluate([clipped, dense_clipped])
    self.assertAllClose(result, expected)

  @test_util.run_deprecated_v1
  def testClipTensorByNorm(self):
    # Simple example
    self._testClipTensorByNorm([[-3.0, 0.0, 0.0], [4.0, 0.0, 0.0]], 4.0,
                               [[-2.4, 0.0, 0.0], [3.2, 0.0, 0.0]])
    # Zero norm
    self._testClipTensorByNorm([[0.0, 0.0, 0.0], [0.0, 0.0, 0.0]], 4.0,
                               [[0.0, 0.0, 0.0], [0.0, 0.0, 0.0]])

  def testClipIndexedSlicesByNorm(self):
    values = [[[-3.0, 0.0, 0.0], [4.0, 0.0, 0.0]],
              [[0.0, 2.0, 0.0], [0.0, 0.0, -1.0]]]
    indices = [2, 6]
    shape = [10, 2, 3]
    # Axes == None
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, None)

    # Axes == 0
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, 0)

    # Axes == 1
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, 1)

    # Axes == 2
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, 1)

    # Axes == [0, 1]
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, [0, 1])

    # Axes == [0, 1]
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, [0, 2])

    # Axes == [0, 1]
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, [1, 2])

    # Axes == [0, 1]
    self._testClipIndexedSlicesByNorm(values, indices, shape, 4.0, [0, 1, 2])


if __name__ == "__main__":
  test.main()
