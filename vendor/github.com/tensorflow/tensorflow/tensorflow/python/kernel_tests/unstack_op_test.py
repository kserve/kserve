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
"""Functional tests for Unstack Op."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np
from six.moves import xrange  # pylint: disable=redefined-builtin

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import gradient_checker
from tensorflow.python.platform import test


def np_split_squeeze(array, axis):
  axis_len = array.shape[axis]
  return [
      np.squeeze(
          arr, axis=(axis,)) for arr in np.split(
              array, axis_len, axis=axis)
  ]


class UnstackOpTest(test.TestCase):

  def testSimple(self):
    np.random.seed(7)
    with test_util.use_gpu():
      for shape in (2,), (3,), (2, 3), (3, 2), (4, 3, 2):
        for dtype in [
            np.bool, np.float16, np.float32, np.float64, np.int32, np.int64
        ]:
          data = np.random.randn(*shape).astype(dtype)
          # Convert data to a single tensorflow tensor
          x = constant_op.constant(data)
          # Unstack into a list of tensors
          cs = array_ops.unstack(x, num=shape[0])
          self.assertEqual(type(cs), list)
          self.assertEqual(len(cs), shape[0])
          cs = [self.evaluate(c) for c in cs]
          self.assertAllEqual(cs, data)

  def testSimpleGpu(self):
    if not test_util.is_gpu_available():
      self.skipTest('No GPU available')

    np.random.seed(7)
    with test_util.force_gpu():
      for shape in (2,), (3,), (2, 3), (3, 2), (4, 3, 2):
        for dtype in [np.float16, np.float32, np.float64, np.int32, np.int64]:
          data = np.random.randn(*shape).astype(dtype)
          # Convert data to a single tensorflow tensor
          x = constant_op.constant(data)
          # Unstack into a list of tensors
          cs = array_ops.unstack(x, num=shape[0])
          self.assertEqual(type(cs), list)
          self.assertEqual(len(cs), shape[0])
          cs = [self.evaluate(c) for c in cs]
          self.assertAllEqual(cs, data)

  @test_util.run_deprecated_v1
  def testGradientsAxis0(self):
    for shape in (2,), (3,), (2, 3), (3, 2), (4, 3, 2):
      data = np.random.randn(*shape)
      shapes = [shape[1:]] * shape[0]
      for i in xrange(shape[0]):
        with self.cached_session(use_gpu=True):
          x = constant_op.constant(data)
          cs = array_ops.unstack(x, num=shape[0])
          err = gradient_checker.compute_gradient_error(x, shape, cs[i],
                                                        shapes[i])
          self.assertLess(err, 1e-6)

  @test_util.run_deprecated_v1
  def testGradientsAxis1(self):
    for shape in (2, 3), (3, 2), (4, 3, 2):
      data = np.random.randn(*shape)
      out_shape = list(shape)
      del out_shape[1]
      for i in xrange(shape[1]):
        with self.cached_session(use_gpu=True):
          x = constant_op.constant(data)
          cs = array_ops.unstack(x, num=shape[1], axis=1)
          err = gradient_checker.compute_gradient_error(x, shape, cs[i],
                                                        out_shape)
          self.assertLess(err, 1e-6)

  @test_util.run_deprecated_v1
  def testInferNum(self):
    with self.cached_session():
      for shape in (2,), (3,), (2, 3), (3, 2), (4, 3, 2):
        x = array_ops.placeholder(np.float32, shape=shape)
        cs = array_ops.unstack(x)
        self.assertEqual(type(cs), list)
        self.assertEqual(len(cs), shape[0])

  @test_util.run_deprecated_v1
  def testCannotInferNumFromUnknownShape(self):
    x = array_ops.placeholder(np.float32)
    with self.assertRaisesRegexp(ValueError,
                                 r'Cannot infer num from shape <unknown>'):
      array_ops.unstack(x)

  @test_util.run_deprecated_v1
  def testUnknownShapeOkWithNum(self):
    x = array_ops.placeholder(np.float32)
    array_ops.unstack(x, num=2)

  @test_util.run_deprecated_v1
  def testCannotInferNumFromNoneShape(self):
    x = array_ops.placeholder(np.float32, shape=(None,))
    with self.assertRaisesRegexp(ValueError,
                                 r'Cannot infer num from shape \((\?|None),\)'):
      array_ops.unstack(x)

  def testAgainstNumpy(self):
    # For 1 to 5 dimensions.
    for i in range(1, 6):
      a = np.random.random(np.random.permutation(i) + 1)

      # For all the possible axis to split it, including negative indices.
      for j in range(-i, i):
        expected = np_split_squeeze(a, j)

        actual_unstack = self.evaluate(array_ops.unstack(a, axis=j))

        self.assertAllEqual(expected, actual_unstack)

  def testAxis0Default(self):
    a = constant_op.constant([[1, 2, 3], [4, 5, 6]], name='a')
    unstacked = self.evaluate(array_ops.unstack(a))

    self.assertEqual(len(unstacked), 2)
    self.assertAllEqual(unstacked[0], [1, 2, 3])
    self.assertAllEqual(unstacked[1], [4, 5, 6])

  def testAxisOutOfRange(self):
    a = constant_op.constant([[1, 2, 3], [4, 5, 6]], name='a')
    with self.assertRaisesRegexp(ValueError, r'axis = 2 not in \[-2, 2\)'):
      array_ops.unstack(a, axis=2)

  def testAxisOutOfNegativeRange(self):
    a = constant_op.constant([[1, 2, 3], [4, 5, 6]], name='a')
    with self.assertRaisesRegexp(ValueError, r'axis = -3 not in \[-2, 2\)'):
      array_ops.unstack(a, axis=-3)

  def testZeroLengthDim(self):
    x = array_ops.zeros(shape=(0, 1, 2))
    y = self.evaluate(array_ops.unstack(x, axis=1)[0])
    self.assertEqual(y.shape, (0, 2))


if __name__ == '__main__':
  test.main()
