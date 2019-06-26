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

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes as dtypes_lib
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import gradient_checker
from tensorflow.python.ops import gradients_impl
from tensorflow.python.platform import test
from tensorflow.python.platform import tf_logging


class MatrixDiagTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testVector(self):
    with self.session(use_gpu=True):
      v = np.array([1.0, 2.0, 3.0])
      mat = np.diag(v)
      v_diag = array_ops.matrix_diag(v)
      self.assertEqual((3, 3), v_diag.get_shape())
      self.assertAllEqual(v_diag.eval(), mat)

  def _testBatchVector(self, dtype):
    with self.cached_session(use_gpu=True):
      v_batch = np.array([[1.0, 0.0, 3.0], [4.0, 5.0, 6.0]]).astype(dtype)
      mat_batch = np.array([[[1.0, 0.0, 0.0], [0.0, 0.0, 0.0], [0.0, 0.0, 3.0]],
                            [[4.0, 0.0, 0.0], [0.0, 5.0, 0.0],
                             [0.0, 0.0, 6.0]]]).astype(dtype)
      v_batch_diag = array_ops.matrix_diag(v_batch)
      self.assertEqual((2, 3, 3), v_batch_diag.get_shape())
      self.assertAllEqual(v_batch_diag.eval(), mat_batch)

  @test_util.run_deprecated_v1
  def testBatchVector(self):
    self._testBatchVector(np.float32)
    self._testBatchVector(np.float64)
    self._testBatchVector(np.int32)
    self._testBatchVector(np.int64)
    self._testBatchVector(np.bool)

  @test_util.run_deprecated_v1
  def testInvalidShape(self):
    with self.assertRaisesRegexp(ValueError, "must be at least rank 1"):
      array_ops.matrix_diag(0)

  @test_util.run_deprecated_v1
  def testInvalidShapeAtEval(self):
    with self.session(use_gpu=True):
      v = array_ops.placeholder(dtype=dtypes_lib.float32)
      with self.assertRaisesOpError("input must be at least 1-dim"):
        array_ops.matrix_diag(v).eval(feed_dict={v: 0.0})

  @test_util.run_deprecated_v1
  def testGrad(self):
    shapes = ((3,), (7, 4))
    with self.session(use_gpu=True):
      for shape in shapes:
        x = constant_op.constant(np.random.rand(*shape), np.float32)
        y = array_ops.matrix_diag(x)
        error = gradient_checker.compute_gradient_error(x,
                                                        x.get_shape().as_list(),
                                                        y,
                                                        y.get_shape().as_list())
        self.assertLess(error, 1e-4)


class MatrixSetDiagTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testSquare(self):
    with self.session(use_gpu=True):
      v = np.array([1.0, 2.0, 3.0])
      mat = np.array([[0.0, 1.0, 0.0], [1.0, 0.0, 1.0], [1.0, 1.0, 1.0]])
      mat_set_diag = np.array([[1.0, 1.0, 0.0], [1.0, 2.0, 1.0],
                               [1.0, 1.0, 3.0]])
      output = array_ops.matrix_set_diag(mat, v)
      self.assertEqual((3, 3), output.get_shape())
      self.assertAllEqual(mat_set_diag, self.evaluate(output))

  @test_util.run_deprecated_v1
  def testRectangular(self):
    with self.session(use_gpu=True):
      v = np.array([3.0, 4.0])
      mat = np.array([[0.0, 1.0, 0.0], [1.0, 0.0, 1.0]])
      expected = np.array([[3.0, 1.0, 0.0], [1.0, 4.0, 1.0]])
      output = array_ops.matrix_set_diag(mat, v)
      self.assertEqual((2, 3), output.get_shape())
      self.assertAllEqual(expected, self.evaluate(output))

      v = np.array([3.0, 4.0])
      mat = np.array([[0.0, 1.0], [1.0, 0.0], [1.0, 1.0]])
      expected = np.array([[3.0, 1.0], [1.0, 4.0], [1.0, 1.0]])
      output = array_ops.matrix_set_diag(mat, v)
      self.assertEqual((3, 2), output.get_shape())
      self.assertAllEqual(expected, self.evaluate(output))

  def _testSquareBatch(self, dtype):
    with self.cached_session(use_gpu=True):
      v_batch = np.array([[-1.0, 0.0, -3.0], [-4.0, -5.0, -6.0]]).astype(dtype)
      mat_batch = np.array([[[1.0, 0.0, 3.0], [0.0, 2.0, 0.0], [1.0, 0.0, 3.0]],
                            [[4.0, 0.0, 4.0], [0.0, 5.0, 0.0],
                             [2.0, 0.0, 6.0]]]).astype(dtype)

      mat_set_diag_batch = np.array([[[-1.0, 0.0, 3.0], [0.0, 0.0, 0.0],
                                      [1.0, 0.0, -3.0]],
                                     [[-4.0, 0.0, 4.0], [0.0, -5.0, 0.0],
                                      [2.0, 0.0, -6.0]]]).astype(dtype)

      output = array_ops.matrix_set_diag(mat_batch, v_batch)
      self.assertEqual((2, 3, 3), output.get_shape())
      self.assertAllEqual(mat_set_diag_batch, self.evaluate(output))

  @test_util.run_deprecated_v1
  def testSquareBatch(self):
    self._testSquareBatch(np.float32)
    self._testSquareBatch(np.float64)
    self._testSquareBatch(np.int32)
    self._testSquareBatch(np.int64)
    self._testSquareBatch(np.bool)

  @test_util.run_deprecated_v1
  def testRectangularBatch(self):
    with self.session(use_gpu=True):
      v_batch = np.array([[-1.0, -2.0], [-4.0, -5.0]])
      mat_batch = np.array([[[1.0, 0.0, 3.0], [0.0, 2.0, 0.0]],
                            [[4.0, 0.0, 4.0], [0.0, 5.0, 0.0]]])

      mat_set_diag_batch = np.array([[[-1.0, 0.0, 3.0], [0.0, -2.0, 0.0]],
                                     [[-4.0, 0.0, 4.0], [0.0, -5.0, 0.0]]])
      output = array_ops.matrix_set_diag(mat_batch, v_batch)
      self.assertEqual((2, 2, 3), output.get_shape())
      self.assertAllEqual(mat_set_diag_batch, self.evaluate(output))

  @test_util.run_deprecated_v1
  def testInvalidShape(self):
    with self.assertRaisesRegexp(ValueError, "must be at least rank 2"):
      array_ops.matrix_set_diag(0, [0])
    with self.assertRaisesRegexp(ValueError, "must be at least rank 1"):
      array_ops.matrix_set_diag([[0]], 0)

  @test_util.run_deprecated_v1
  def testInvalidShapeAtEval(self):
    with self.session(use_gpu=True):
      v = array_ops.placeholder(dtype=dtypes_lib.float32)
      with self.assertRaisesOpError("input must be at least 2-dim"):
        array_ops.matrix_set_diag(v, [v]).eval(feed_dict={v: 0.0})
      with self.assertRaisesOpError(
          r"but received input shape: \[1,1\] and diagonal shape: \[\]"):
        array_ops.matrix_set_diag([[v]], v).eval(feed_dict={v: 0.0})

  @test_util.run_deprecated_v1
  def testGrad(self):
    shapes = ((3, 4, 4), (3, 3, 4), (3, 4, 3), (7, 4, 8, 8))
    with self.session(use_gpu=True):
      for shape in shapes:
        x = constant_op.constant(
            np.random.rand(*shape), dtype=dtypes_lib.float32)
        diag_shape = shape[:-2] + (min(shape[-2:]),)
        x_diag = constant_op.constant(
            np.random.rand(*diag_shape), dtype=dtypes_lib.float32)
        y = array_ops.matrix_set_diag(x, x_diag)
        error_x = gradient_checker.compute_gradient_error(
            x,
            x.get_shape().as_list(), y,
            y.get_shape().as_list())
        self.assertLess(error_x, 1e-4)
        error_x_diag = gradient_checker.compute_gradient_error(
            x_diag,
            x_diag.get_shape().as_list(), y,
            y.get_shape().as_list())
        self.assertLess(error_x_diag, 1e-4)

  @test_util.run_deprecated_v1
  def testGradWithNoShapeInformation(self):
    with self.session(use_gpu=True) as sess:
      v = array_ops.placeholder(dtype=dtypes_lib.float32)
      mat = array_ops.placeholder(dtype=dtypes_lib.float32)
      grad_input = array_ops.placeholder(dtype=dtypes_lib.float32)
      output = array_ops.matrix_set_diag(mat, v)
      grads = gradients_impl.gradients(output, [mat, v], grad_ys=grad_input)
      grad_input_val = np.random.rand(3, 3).astype(np.float32)
      grad_vals = sess.run(
          grads,
          feed_dict={
              v: 2 * np.ones(3),
              mat: np.ones((3, 3)),
              grad_input: grad_input_val
          })
      self.assertAllEqual(np.diag(grad_input_val), grad_vals[1])
      self.assertAllEqual(grad_input_val - np.diag(np.diag(grad_input_val)),
                          grad_vals[0])


class MatrixDiagPartTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testSquare(self):
    with self.session(use_gpu=True):
      v = np.array([1.0, 2.0, 3.0])
      mat = np.diag(v)
      mat_diag = array_ops.matrix_diag_part(mat)
      self.assertEqual((3,), mat_diag.get_shape())
      self.assertAllEqual(mat_diag.eval(), v)

  @test_util.run_deprecated_v1
  def testRectangular(self):
    with self.session(use_gpu=True):
      mat = np.array([[1.0, 2.0, 3.0], [4.0, 5.0, 6.0]])
      mat_diag = array_ops.matrix_diag_part(mat)
      self.assertAllEqual(mat_diag.eval(), np.array([1.0, 5.0]))
      mat = np.array([[1.0, 2.0], [3.0, 4.0], [5.0, 6.0]])
      mat_diag = array_ops.matrix_diag_part(mat)
      self.assertAllEqual(mat_diag.eval(), np.array([1.0, 4.0]))

  def _testSquareBatch(self, dtype):
    with self.cached_session(use_gpu=True):
      v_batch = np.array([[1.0, 0.0, 3.0], [4.0, 5.0, 6.0]]).astype(dtype)
      mat_batch = np.array([[[1.0, 0.0, 0.0], [0.0, 0.0, 0.0], [0.0, 0.0, 3.0]],
                            [[4.0, 0.0, 0.0], [0.0, 5.0, 0.0],
                             [0.0, 0.0, 6.0]]]).astype(dtype)
      self.assertEqual(mat_batch.shape, (2, 3, 3))
      mat_batch_diag = array_ops.matrix_diag_part(mat_batch)
      self.assertEqual((2, 3), mat_batch_diag.get_shape())
      self.assertAllEqual(mat_batch_diag.eval(), v_batch)

  @test_util.run_deprecated_v1
  def testSquareBatch(self):
    self._testSquareBatch(np.float32)
    self._testSquareBatch(np.float64)
    self._testSquareBatch(np.int32)
    self._testSquareBatch(np.int64)
    self._testSquareBatch(np.bool)

  @test_util.run_deprecated_v1
  def testRectangularBatch(self):
    with self.session(use_gpu=True):
      v_batch = np.array([[1.0, 2.0], [4.0, 5.0]])
      mat_batch = np.array([[[1.0, 0.0, 0.0], [0.0, 2.0, 0.0]],
                            [[4.0, 0.0, 0.0], [0.0, 5.0, 0.0]]])
      self.assertEqual(mat_batch.shape, (2, 2, 3))
      mat_batch_diag = array_ops.matrix_diag_part(mat_batch)
      self.assertEqual((2, 2), mat_batch_diag.get_shape())
      self.assertAllEqual(mat_batch_diag.eval(), v_batch)

  @test_util.run_deprecated_v1
  def testInvalidShape(self):
    with self.assertRaisesRegexp(ValueError, "must be at least rank 2"):
      array_ops.matrix_diag_part(0)

  @test_util.run_deprecated_v1
  def testInvalidShapeAtEval(self):
    with self.session(use_gpu=True):
      v = array_ops.placeholder(dtype=dtypes_lib.float32)
      with self.assertRaisesOpError("input must be at least 2-dim"):
        array_ops.matrix_diag_part(v).eval(feed_dict={v: 0.0})

  @test_util.run_deprecated_v1
  def testGrad(self):
    shapes = ((3, 3), (2, 3), (3, 2), (5, 3, 3))
    with self.session(use_gpu=True):
      for shape in shapes:
        x = constant_op.constant(np.random.rand(*shape), dtype=np.float32)
        y = array_ops.matrix_diag_part(x)
        error = gradient_checker.compute_gradient_error(x,
                                                        x.get_shape().as_list(),
                                                        y,
                                                        y.get_shape().as_list())
        self.assertLess(error, 1e-4)


class DiagTest(test.TestCase):

  def _diagOp(self, diag, dtype, expected_ans, use_gpu):
    with self.cached_session(use_gpu=use_gpu):
      tf_ans = array_ops.diag(ops.convert_to_tensor(diag.astype(dtype)))
      out = self.evaluate(tf_ans)
      tf_ans_inv = array_ops.diag_part(expected_ans)
      inv_out = self.evaluate(tf_ans_inv)
    self.assertAllClose(out, expected_ans)
    self.assertAllClose(inv_out, diag)
    self.assertShapeEqual(expected_ans, tf_ans)
    self.assertShapeEqual(diag, tf_ans_inv)

  def diagOp(self, diag, dtype, expected_ans):
    self._diagOp(diag, dtype, expected_ans, False)
    self._diagOp(diag, dtype, expected_ans, True)

  def testEmptyTensor(self):
    x = np.array([])
    expected_ans = np.empty([0, 0])
    self.diagOp(x, np.int32, expected_ans)

  def testRankOneIntTensor(self):
    x = np.array([1, 2, 3])
    expected_ans = np.array([[1, 0, 0], [0, 2, 0], [0, 0, 3]])
    self.diagOp(x, np.int32, expected_ans)
    self.diagOp(x, np.int64, expected_ans)

  def testRankOneFloatTensor(self):
    x = np.array([1.1, 2.2, 3.3])
    expected_ans = np.array([[1.1, 0, 0], [0, 2.2, 0], [0, 0, 3.3]])
    self.diagOp(x, np.float32, expected_ans)
    self.diagOp(x, np.float64, expected_ans)

  def testRankOneComplexTensor(self):
    for dtype in [np.complex64, np.complex128]:
      x = np.array([1.1 + 1.1j, 2.2 + 2.2j, 3.3 + 3.3j], dtype=dtype)
      expected_ans = np.array(
          [[1.1 + 1.1j, 0 + 0j, 0 + 0j], [0 + 0j, 2.2 + 2.2j, 0 + 0j],
           [0 + 0j, 0 + 0j, 3.3 + 3.3j]],
          dtype=dtype)
      self.diagOp(x, dtype, expected_ans)

  def testRankTwoIntTensor(self):
    x = np.array([[1, 2, 3], [4, 5, 6]])
    expected_ans = np.array([[[[1, 0, 0], [0, 0, 0]], [[0, 2, 0], [0, 0, 0]],
                              [[0, 0, 3], [0, 0, 0]]],
                             [[[0, 0, 0], [4, 0, 0]], [[0, 0, 0], [0, 5, 0]],
                              [[0, 0, 0], [0, 0, 6]]]])
    self.diagOp(x, np.int32, expected_ans)
    self.diagOp(x, np.int64, expected_ans)

  def testRankTwoFloatTensor(self):
    x = np.array([[1.1, 2.2, 3.3], [4.4, 5.5, 6.6]])
    expected_ans = np.array(
        [[[[1.1, 0, 0], [0, 0, 0]], [[0, 2.2, 0], [0, 0, 0]],
          [[0, 0, 3.3], [0, 0, 0]]], [[[0, 0, 0], [4.4, 0, 0]],
                                      [[0, 0, 0], [0, 5.5, 0]], [[0, 0, 0],
                                                                 [0, 0, 6.6]]]])
    self.diagOp(x, np.float32, expected_ans)
    self.diagOp(x, np.float64, expected_ans)

  def testRankTwoComplexTensor(self):
    for dtype in [np.complex64, np.complex128]:
      x = np.array(
          [[1.1 + 1.1j, 2.2 + 2.2j, 3.3 + 3.3j],
           [4.4 + 4.4j, 5.5 + 5.5j, 6.6 + 6.6j]],
          dtype=dtype)
      expected_ans = np.array(
          [[[[1.1 + 1.1j, 0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j, 0 + 0j]], [
              [0 + 0j, 2.2 + 2.2j, 0 + 0j], [0 + 0j, 0 + 0j, 0 + 0j]
          ], [[0 + 0j, 0 + 0j, 3.3 + 3.3j], [0 + 0j, 0 + 0j, 0 + 0j]]], [[
              [0 + 0j, 0 + 0j, 0 + 0j], [4.4 + 4.4j, 0 + 0j, 0 + 0j]
          ], [[0 + 0j, 0 + 0j, 0 + 0j], [0 + 0j, 5.5 + 5.5j, 0 + 0j]
             ], [[0 + 0j, 0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j, 6.6 + 6.6j]]]],
          dtype=dtype)
      self.diagOp(x, dtype, expected_ans)

  def testRankThreeFloatTensor(self):
    x = np.array([[[1.1, 2.2], [3.3, 4.4]], [[5.5, 6.6], [7.7, 8.8]]])
    expected_ans = np.array([[[[[[1.1, 0], [0, 0]], [[0, 0], [0, 0]]],
                               [[[0, 2.2], [0, 0]], [[0, 0], [0, 0]]]],
                              [[[[0, 0], [3.3, 0]], [[0, 0], [0, 0]]],
                               [[[0, 0], [0, 4.4]], [[0, 0], [0, 0]]]]],
                             [[[[[0, 0], [0, 0]], [[5.5, 0], [0, 0]]],
                               [[[0, 0], [0, 0]], [[0, 6.6], [0, 0]]]],
                              [[[[0, 0], [0, 0]], [[0, 0], [7.7, 0]]],
                               [[[0, 0], [0, 0]], [[0, 0], [0, 8.8]]]]]])
    self.diagOp(x, np.float32, expected_ans)
    self.diagOp(x, np.float64, expected_ans)

  def testRankThreeComplexTensor(self):
    for dtype in [np.complex64, np.complex128]:
      x = np.array(
          [[[1.1 + 1.1j, 2.2 + 2.2j], [3.3 + 3.3j, 4.4 + 4.4j]],
           [[5.5 + 5.5j, 6.6 + 6.6j], [7.7 + 7.7j, 8.8 + 8.8j]]],
          dtype=dtype)
      expected_ans = np.array(
          [[[[[[1.1 + 1.1j, 0 + 0j], [0 + 0j, 0 + 0j]], [[0 + 0j, 0 + 0j], [
              0 + 0j, 0 + 0j
          ]]], [[[0 + 0j, 2.2 + 2.2j], [0 + 0j, 0 + 0j]], [[0 + 0j, 0 + 0j], [
              0 + 0j, 0 + 0j
          ]]]], [[[[0 + 0j, 0 + 0j], [3.3 + 3.3j, 0 + 0j]], [[0 + 0j, 0 + 0j], [
              0 + 0j, 0 + 0j
          ]]], [[[0 + 0j, 0 + 0j], [0 + 0j, 4.4 + 4.4j]], [[0 + 0j, 0 + 0j], [
              0 + 0j, 0 + 0j
          ]]]]], [[[[[0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j]], [
              [5.5 + 5.5j, 0 + 0j], [0 + 0j, 0 + 0j]
          ]], [[[0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j]], [[0 + 0j, 6.6 + 6.6j], [
              0 + 0j, 0 + 0j
          ]]]], [[[[0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j]], [[0 + 0j, 0 + 0j], [
              7.7 + 7.7j, 0 + 0j
          ]]], [[[0 + 0j, 0 + 0j], [0 + 0j, 0 + 0j]],
                [[0 + 0j, 0 + 0j], [0 + 0j, 8.8 + 8.8j]]]]]],
          dtype=dtype)
      self.diagOp(x, dtype, expected_ans)

  def testRankFourNumberTensor(self):
    for dtype in [np.float32, np.float64, np.int64, np.int32]:
      # Input with shape [2, 1, 2, 3]
      x = np.array(
          [[[[1, 2, 3], [4, 5, 6]]], [[[7, 8, 9], [10, 11, 12]]]], dtype=dtype)
      # Output with shape [2, 1, 2, 3, 2, 1, 2, 3]
      expected_ans = np.array(
          [[[[[[[[1, 0, 0], [0, 0, 0]]], [[[0, 0, 0], [0, 0, 0]]]], [
              [[[0, 2, 0], [0, 0, 0]]], [[[0, 0, 0], [0, 0, 0]]]
          ], [[[[0, 0, 3], [0, 0, 0]]], [[[0, 0, 0], [0, 0, 0]]]]], [[
              [[[0, 0, 0], [4, 0, 0]]], [[[0, 0, 0], [0, 0, 0]]]
          ], [[[[0, 0, 0], [0, 5, 0]]], [[[0, 0, 0], [0, 0, 0]]]], [
              [[[0, 0, 0], [0, 0, 6]]], [[[0, 0, 0], [0, 0, 0]]]
          ]]]], [[[[[[[0, 0, 0], [0, 0, 0]]], [[[7, 0, 0], [0, 0, 0]]]], [
              [[[0, 0, 0], [0, 0, 0]]], [[[0, 8, 0], [0, 0, 0]]]
          ], [[[[0, 0, 0], [0, 0, 0]]], [[[0, 0, 9], [0, 0, 0]]]]], [[
              [[[0, 0, 0], [0, 0, 0]]], [[[0, 0, 0], [10, 0, 0]]]
          ], [[[[0, 0, 0], [0, 0, 0]]], [[[0, 0, 0], [0, 11, 0]]]
             ], [[[[0, 0, 0], [0, 0, 0]]], [[[0, 0, 0], [0, 0, 12]]]]]]]],
          dtype=dtype)
      self.diagOp(x, dtype, expected_ans)

  @test_util.run_deprecated_v1
  def testInvalidRank(self):
    with self.assertRaisesRegexp(ValueError, "must be at least rank 1"):
      array_ops.diag(0.0)


class DiagPartOpTest(test.TestCase):

  def setUp(self):
    np.random.seed(0)

  def _diagPartOp(self, tensor, dtype, expected_ans, use_gpu):
    with self.cached_session(use_gpu=use_gpu):
      tensor = ops.convert_to_tensor(tensor.astype(dtype))
      tf_ans_inv = array_ops.diag_part(tensor)
      inv_out = self.evaluate(tf_ans_inv)
    self.assertAllClose(inv_out, expected_ans)
    self.assertShapeEqual(expected_ans, tf_ans_inv)

  def diagPartOp(self, tensor, dtype, expected_ans):
    self._diagPartOp(tensor, dtype, expected_ans, False)
    self._diagPartOp(tensor, dtype, expected_ans, True)

  def testRankTwoFloatTensor(self):
    x = np.random.rand(3, 3)
    i = np.arange(3)
    expected_ans = x[i, i]
    self.diagPartOp(x, np.float32, expected_ans)
    self.diagPartOp(x, np.float64, expected_ans)

  def testRankFourFloatTensorUnknownShape(self):
    x = np.random.rand(3, 3)
    i = np.arange(3)
    expected_ans = x[i, i]
    for shape in None, (None, 3), (3, None):
      with self.cached_session(use_gpu=False):
        t = ops.convert_to_tensor(x.astype(np.float32))
        t.set_shape(shape)
        tf_ans = array_ops.diag_part(t)
        out = self.evaluate(tf_ans)
      self.assertAllClose(out, expected_ans)
      self.assertShapeEqual(expected_ans, tf_ans)

  def testRankFourFloatTensor(self):
    x = np.random.rand(2, 3, 2, 3)
    i = np.arange(2)[:, None]
    j = np.arange(3)
    expected_ans = x[i, j, i, j]
    self.diagPartOp(x, np.float32, expected_ans)
    self.diagPartOp(x, np.float64, expected_ans)

  def testRankSixFloatTensor(self):
    x = np.random.rand(2, 2, 2, 2, 2, 2)
    i = np.arange(2)[:, None, None]
    j = np.arange(2)[:, None]
    k = np.arange(2)
    expected_ans = x[i, j, k, i, j, k]
    self.diagPartOp(x, np.float32, expected_ans)
    self.diagPartOp(x, np.float64, expected_ans)

  def testRankEightComplexTensor(self):
    x = np.random.rand(2, 2, 2, 3, 2, 2, 2, 3)
    i = np.arange(2)[:, None, None, None]
    j = np.arange(2)[:, None, None]
    k = np.arange(2)[:, None]
    l = np.arange(3)
    expected_ans = x[i, j, k, l, i, j, k, l]
    self.diagPartOp(x, np.complex64, expected_ans)
    self.diagPartOp(x, np.complex128, expected_ans)

  @test_util.run_deprecated_v1
  def testOddRank(self):
    w = np.random.rand(2)
    x = np.random.rand(2, 2, 2)
    self.assertRaises(ValueError, self.diagPartOp, w, np.float32, 0)
    self.assertRaises(ValueError, self.diagPartOp, x, np.float32, 0)
    with self.assertRaises(ValueError):
      array_ops.diag_part(0.0)

  @test_util.run_deprecated_v1
  def testUnevenDimensions(self):
    w = np.random.rand(2, 5)
    x = np.random.rand(2, 1, 2, 3)
    self.assertRaises(ValueError, self.diagPartOp, w, np.float32, 0)
    self.assertRaises(ValueError, self.diagPartOp, x, np.float32, 0)


class DiagGradOpTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testDiagGrad(self):
    np.random.seed(0)
    shapes = ((3,), (3, 3), (3, 3, 3))
    dtypes = (dtypes_lib.float32, dtypes_lib.float64)
    with self.session(use_gpu=False):
      errors = []
      for shape in shapes:
        for dtype in dtypes:
          x1 = constant_op.constant(np.random.rand(*shape), dtype=dtype)
          y = array_ops.diag(x1)
          error = gradient_checker.compute_gradient_error(
              x1,
              x1.get_shape().as_list(), y,
              y.get_shape().as_list())
          tf_logging.info("error = %f", error)
          self.assertLess(error, 1e-4)


class DiagGradPartOpTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testDiagPartGrad(self):
    np.random.seed(0)
    shapes = ((3, 3), (3, 3, 3, 3))
    dtypes = (dtypes_lib.float32, dtypes_lib.float64)
    with self.session(use_gpu=False):
      errors = []
      for shape in shapes:
        for dtype in dtypes:
          x1 = constant_op.constant(np.random.rand(*shape), dtype=dtype)
          y = array_ops.diag_part(x1)
          error = gradient_checker.compute_gradient_error(
              x1,
              x1.get_shape().as_list(), y,
              y.get_shape().as_list())
          tf_logging.info("error = %f", error)
          self.assertLess(error, 1e-4)


if __name__ == "__main__":
  test.main()
