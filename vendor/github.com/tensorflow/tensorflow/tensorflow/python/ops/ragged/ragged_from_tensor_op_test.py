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
"""Tests for RaggedTensor.from_tensor."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops.ragged import ragged_test_util
from tensorflow.python.ops.ragged.ragged_tensor import RaggedTensor
from tensorflow.python.platform import googletest


@test_util.run_all_in_graph_and_eager_modes
class RaggedTensorToSparseOpTest(ragged_test_util.RaggedTensorTestCase,
                                 parameterized.TestCase):

  def testDocStringExamples(self):
    # The examples from RaggedTensor.from_tensor.__doc__.
    dt = constant_op.constant([[5, 7, 0], [0, 3, 0], [6, 0, 0]])
    self.assertRaggedEqual(
        RaggedTensor.from_tensor(dt), [[5, 7, 0], [0, 3, 0], [6, 0, 0]])

    self.assertRaggedEqual(
        RaggedTensor.from_tensor(dt, lengths=[1, 0, 3]), [[5], [], [6, 0, 0]])

    self.assertRaggedEqual(
        RaggedTensor.from_tensor(dt, padding=0), [[5, 7], [0, 3], [6]])

  @parameterized.parameters(
      # 2D test cases, no length or padding.
      {
          'tensor': [[]],
          'expected': [[]],
      },
      {
          'tensor': [[1]],
          'expected': [[1]],
      },
      {
          'tensor': [[1, 2]],
          'expected': [[1, 2]],
      },
      {
          'tensor': [[1], [2], [3]],
          'expected': [[1], [2], [3]],
      },
      {
          'tensor': [[1, 2, 3], [4, 5, 6], [7, 8, 9]],
          'expected': [[1, 2, 3], [4, 5, 6], [7, 8, 9]],
      },
      # 3D test cases, no length or padding
      {
          'tensor': [[[]]],
          'expected': [[[]]],
      },
      {
          'tensor': [[[]]],
          'expected': [[[]]],
          'ragged_rank': 1,
      },
      {
          'tensor': [[[1]]],
          'expected': [[[1]]],
      },
      {
          'tensor': [[[1, 2]]],
          'expected': [[[1, 2]]],
      },
      {
          'tensor': [[[1, 2], [3, 4]]],
          'expected': [[[1, 2], [3, 4]]],
      },
      {
          'tensor': [[[1, 2]], [[3, 4]], [[5, 6]], [[7, 8]]],
          'expected': [[[1, 2]], [[3, 4]], [[5, 6]], [[7, 8]]],
      },
      {
          'tensor': [[[1], [2]], [[3], [4]], [[5], [6]], [[7], [8]]],
          'expected': [[[1], [2]], [[3], [4]], [[5], [6]], [[7], [8]]],
      },
      # 2D test cases, with length
      {
          'tensor': [[1]],
          'lengths': [1],
          'expected': [[1]]
      },
      {
          'tensor': [[1]],
          'lengths': [0],
          'expected': [[]]
      },
      {
          'tensor': [[1, 2, 3], [4, 5, 6], [7, 8, 9]],
          'lengths': [0, 1, 2],
          'expected': [[], [4], [7, 8]]
      },
      {
          'tensor': [[1, 2, 3], [4, 5, 6], [7, 8, 9]],
          'lengths': [0, 0, 0],
          'expected': [[], [], []]
      },
      {
          'tensor': [[1, 2], [3, 4]],
          'lengths': [2, 2],
          'expected': [[1, 2], [3, 4]]
      },
      {
          'tensor': [[1, 2], [3, 4]],
          'lengths': [7, 8],  # lengths > ncols: truncated to ncols
          'expected': [[1, 2], [3, 4]]
      },
      {
          'tensor': [[1, 2], [3, 4]],
          'lengths': [-2, -1],  # lengths < 0: treated as zero
          'expected': [[], []]
      },
      # 3D test cases, with length
      {
          'tensor': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
          'lengths': [0, 0],
          'expected': [[], []]
      },
      {
          'tensor': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
          'lengths': [1, 2],
          'expected': [[[1, 2]], [[5, 6], [7, 8]]]
      },
      {
          'tensor': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
          'lengths': [2, 2],
          'expected': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]]
      },
      # 2D test cases, with padding
      {
          'tensor': [[1]],
          'padding': 0,
          'expected': [[1]]
      },
      {
          'tensor': [[0]],
          'padding': 0,
          'expected': [[]]
      },
      {
          'tensor': [[0, 1]],
          'padding': 0,
          'expected': [[0, 1]]
      },
      {
          'tensor': [[1, 0]],
          'padding': 0,
          'expected': [[1]]
      },
      {
          'tensor': [[1, 0, 1, 0, 0, 1, 0, 0]],
          'padding': 0,
          'expected': [[1, 0, 1, 0, 0, 1]]
      },
      {
          'tensor': [[3, 7, 0, 0], [2, 0, 0, 0], [5, 0, 0, 0]],
          'padding': 0,
          'expected': [[3, 7], [2], [5]]
      },
      # 3D test cases, with padding
      {
          'tensor': [[[1]]],
          'padding': [0],
          'expected': [[[1]]]
      },
      {
          'tensor': [[[0]]],
          'padding': [0],
          'expected': [[]]
      },
      {
          'tensor': [[[0, 0], [1, 2]], [[3, 4], [0, 0]]],
          'padding': [0, 0],
          'expected': [[[0, 0], [1, 2]], [[3, 4]]]
      },
      # 4D test cases, with padding
      {
          'tensor': [
              [[[1, 2], [3, 4]], [[0, 0], [0, 0]], [[0, 0], [0, 0]]],
              [[[0, 0], [0, 0]], [[5, 6], [7, 8]], [[0, 0], [0, 0]]],
              [[[0, 0], [0, 0]], [[0, 0], [0, 0]], [[0, 0], [0, 0]]]
          ],
          'padding': [[0, 0], [0, 0]],
          'expected': [
              [[[1, 2], [3, 4]]],
              [[[0, 0], [0, 0]], [[5, 6], [7, 8]]],
              []
          ]
      },
      # 3D test cases, with ragged_rank=2.
      {
          'tensor': [[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
          'ragged_rank': 2,
          'expected': [[[1, 0], [2, 3]], [[0, 0], [4, 0]]]
      },
      {
          'tensor': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
          'ragged_rank': 2,
          'lengths': [2, 0, 2, 1],
          'expected': [[[1, 2], []], [[5, 6], [7]]]
      },
      {
          'tensor': [[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
          'ragged_rank': 2,
          'padding': 0,
          'expected': [[[1], [2, 3]], [[], [4]]]
      },
      # 4D test cases, with ragged_rank>1
      {
          'tensor': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                     [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]],
          'ragged_rank': 2,
          'expected': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                       [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]]
      },
      {
          'tensor': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                     [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]],
          'ragged_rank': 3,
          'expected': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                       [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]]
      },
      {
          'tensor': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                     [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]],
          'ragged_rank': 2,
          'padding': [0, 0],
          'expected': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                       [[[5, 6], [7, 0]], [[0, 8]]]]
      },
      {
          'tensor': [[[[1, 0], [2, 3]], [[0, 0], [4, 0]]],
                     [[[5, 6], [7, 0]], [[0, 8], [0, 0]]]],
          'ragged_rank': 3,
          'padding': 0,
          'expected': [[[[1], [2, 3]], [[], [4]]],
                       [[[5, 6], [7]], [[0, 8], []]]]
      },
  )  # pyformat: disable
  def testRaggedFromTensor(self,
                           tensor,
                           expected,
                           lengths=None,
                           padding=None,
                           ragged_rank=1):
    dt = constant_op.constant(tensor)
    rt = RaggedTensor.from_tensor(dt, lengths, padding, ragged_rank)
    self.assertEqual(type(rt), RaggedTensor)
    self.assertEqual(rt.ragged_rank, ragged_rank)
    self.assertTrue(
        dt.shape.is_compatible_with(rt.shape),
        '%s is incompatible with %s' % (dt.shape, rt.shape))
    self.assertRaggedEqual(rt, expected)

  def testHighDimensions(self):
    # Use distinct prime numbers for all dimension shapes in this test, so
    # we can see any errors that are caused by mixing up dimension sizes.
    dt = array_ops.reshape(
        math_ops.range(3 * 5 * 7 * 11 * 13 * 17), [3, 5, 7, 11, 13, 17])
    for ragged_rank in range(1, 4):
      rt = RaggedTensor.from_tensor(dt, ragged_rank=ragged_rank)
      self.assertEqual(type(rt), RaggedTensor)
      self.assertEqual(rt.ragged_rank, ragged_rank)
      self.assertTrue(
          dt.shape.is_compatible_with(rt.shape),
          '%s is incompatible with %s' % (dt.shape, rt.shape))
      self.assertRaggedEqual(rt, self.evaluate(dt).tolist())

  @parameterized.parameters(
      # With no padding or lengths
      {
          'dt_shape': [0, 0],
          'expected': []
      },
      {
          'dt_shape': [0, 3],
          'expected': []
      },
      {
          'dt_shape': [3, 0],
          'expected': [[], [], []]
      },
      {
          'dt_shape': [0, 2, 3],
          'expected': []
      },
      {
          'dt_shape': [2, 0, 3],
          'expected': [[], []]
      },
      {
          'dt_shape': [2, 3, 0],
          'expected': [[[], [], []], [[], [], []]]
      },
      {
          'dt_shape': [2, 3, 0, 1],
          'expected': [[[], [], []], [[], [], []]]
      },
      {
          'dt_shape': [2, 3, 1, 0],
          'expected': [[[[]], [[]], [[]]], [[[]], [[]], [[]]]]
      },
      # With padding
      {
          'dt_shape': [0, 0],
          'padding': 0,
          'expected': []
      },
      {
          'dt_shape': [0, 3],
          'padding': 0,
          'expected': []
      },
      {
          'dt_shape': [3, 0],
          'padding': 0,
          'expected': [[], [], []]
      },
      {
          'dt_shape': [0, 2, 3],
          'padding': [0, 0, 0],
          'expected': []
      },
      {
          'dt_shape': [2, 0, 3],
          'padding': [0, 0, 0],
          'expected': [[], []]
      },
      {
          'dt_shape': [2, 3, 0],
          'padding': [],
          'expected': [[], []]
      },
      # With lengths
      {
          'dt_shape': [0, 0],
          'lengths': [],
          'expected': []
      },
      {
          'dt_shape': [0, 3],
          'lengths': [],
          'expected': []
      },
      {
          'dt_shape': [3, 0],
          'lengths': [0, 0, 0],
          'expected': [[], [], []]
      },
      {
          'dt_shape': [3, 0],
          'lengths': [2, 3, 4],  # lengths > ncols: truncated to ncols
          'expected': [[], [], []]
      },
      {
          'dt_shape': [0, 2, 3],
          'lengths': [],
          'expected': []
      },
      {
          'dt_shape': [2, 0, 3],
          'lengths': [0, 0],
          'expected': [[], []]
      },
      {
          'dt_shape': [2, 3, 0],
          'lengths': [0, 0],
          'expected': [[], []]
      },
  )
  def testEmpty(self, dt_shape, expected, lengths=None, padding=None):
    dt = array_ops.zeros(dt_shape)
    rt = RaggedTensor.from_tensor(dt, lengths, padding)
    self.assertEqual(type(rt), RaggedTensor)
    self.assertEqual(rt.ragged_rank, 1)
    self.assertTrue(dt.shape.is_compatible_with(rt.shape))
    self.assertRaggedEqual(rt, expected)

  @parameterized.parameters(
      {
          'tensor': [[1]],
          'lengths': [0],
          'padding': 0,
          'error': (ValueError, 'Specify lengths or padding, but not both')
      },
      {
          'tensor': [[1]],
          'lengths': [0.5],
          'error': (TypeError, 'lengths must be an integer tensor')
      },
      {
          'tensor': [[1]],
          'padding': 'a',
          'error': (TypeError, '.*')
      },
      {
          'tensor': [[1]],
          'padding': [1],
          'error': (ValueError, r'Shapes \(1,\) and \(\) are incompatible')
      },
      {
          'tensor': [[[1]]],
          'padding': 1,
          'error': (ValueError, r'Shapes \(\) and \(1,\) are incompatible')
      },
      {
          'tensor': [[1]],
          'ragged_rank': 'bad',
          'error': (TypeError, r'ragged_rank expected int, got \'bad\'')
      },
      {
          'tensor': [[1]],
          'ragged_rank': 0,
          'error': (ValueError, r'ragged_rank must be greater than 0; got 0')
      },
      {
          'tensor': [[1]],
          'ragged_rank': -1,
          'error': (ValueError, r'ragged_rank must be greater than 0; got -1')
      },
  )
  def testErrors(self,
                 tensor,
                 lengths=None,
                 padding=None,
                 ragged_rank=1,
                 error=None):
    dt = constant_op.constant(tensor)
    self.assertRaisesRegexp(error[0], error[1], RaggedTensor.from_tensor, dt,
                            lengths, padding, ragged_rank)


if __name__ == '__main__':
  googletest.main()
