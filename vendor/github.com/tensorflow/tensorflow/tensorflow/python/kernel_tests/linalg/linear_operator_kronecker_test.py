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

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops.linalg import linalg as linalg_lib
from tensorflow.python.ops.linalg import linear_operator_kronecker as kronecker
from tensorflow.python.ops.linalg import linear_operator_lower_triangular as lower_triangular
from tensorflow.python.ops.linalg import linear_operator_test_util
from tensorflow.python.ops.linalg import linear_operator_util
from tensorflow.python.platform import test

linalg = linalg_lib
rng = np.random.RandomState(0)


def _kronecker_dense(factors):
  """Convert a list of factors, into a dense Kronecker product."""
  product = factors[0]
  for factor in factors[1:]:
    product = product[..., array_ops.newaxis, :, array_ops.newaxis]
    factor_to_mul = factor[..., array_ops.newaxis, :, array_ops.newaxis, :]
    product *= factor_to_mul
    product = array_ops.reshape(
        product,
        shape=array_ops.concat(
            [array_ops.shape(product)[:-4],
             [array_ops.shape(product)[-4] * array_ops.shape(product)[-3],
              array_ops.shape(product)[-2] * array_ops.shape(product)[-1]]
            ], axis=0))

  return product


class KroneckerDenseTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testKroneckerDenseMatrix(self):
    x = ops.convert_to_tensor([[2., 3.], [1., 2.]], dtype=dtypes.float32)
    y = ops.convert_to_tensor([[1., 2.], [5., -1.]], dtype=dtypes.float32)
    # From explicitly writing out the kronecker product of x and y.
    z = ops.convert_to_tensor([
        [2., 4., 3., 6.],
        [10., -2., 15., -3.],
        [1., 2., 2., 4.],
        [5., -1., 10., -2.]], dtype=dtypes.float32)
    # From explicitly writing out the kronecker product of y and x.
    w = ops.convert_to_tensor([
        [2., 3., 4., 6.],
        [1., 2., 2., 4.],
        [10., 15., -2., -3.],
        [5., 10., -1., -2.]], dtype=dtypes.float32)

    with self.cached_session():
      self.assertAllClose(_kronecker_dense([x, y]).eval(), self.evaluate(z))
      self.assertAllClose(_kronecker_dense([y, x]).eval(), self.evaluate(w))


class SquareLinearOperatorKroneckerTest(
    linear_operator_test_util.SquareLinearOperatorDerivedClassTest):
  """Most tests done in the base class LinearOperatorDerivedClassTest."""

  def setUp(self):
    # Increase from 1e-6 to 1e-4
    self._atol[dtypes.float32] = 1e-4
    self._atol[dtypes.complex64] = 1e-4
    self._rtol[dtypes.float32] = 1e-4
    self._rtol[dtypes.complex64] = 1e-4

  @property
  def _operator_build_infos(self):
    build_info = linear_operator_test_util.OperatorBuildInfo
    return [
        build_info((1, 1), factors=[(1, 1), (1, 1)]),
        build_info((8, 8), factors=[(2, 2), (2, 2), (2, 2)]),
        build_info((12, 12), factors=[(2, 2), (3, 3), (2, 2)]),
        build_info((1, 3, 3), factors=[(1, 1), (1, 3, 3)]),
        build_info((3, 6, 6), factors=[(3, 1, 1), (1, 2, 2), (1, 3, 3)]),
    ]

  @property
  def _tests_to_skip(self):
    return ["det", "solve", "solve_with_broadcast"]

  def _operator_and_matrix(
      self, build_info, dtype, use_placeholder,
      ensure_self_adjoint_and_pd=False):
    # Kronecker products constructed below will be from symmetric
    # positive-definite matrices.
    del ensure_self_adjoint_and_pd
    shape = list(build_info.shape)
    expected_factors = build_info.__dict__["factors"]
    matrices = [
        linear_operator_test_util.random_positive_definite_matrix(
            block_shape, dtype, force_well_conditioned=True)
        for block_shape in expected_factors
    ]

    lin_op_matrices = matrices

    if use_placeholder:
      lin_op_matrices = [
          array_ops.placeholder_with_default(m, shape=None) for m in matrices]

    operator = kronecker.LinearOperatorKronecker(
        [linalg.LinearOperatorFullMatrix(
            l,
            is_square=True,
            is_self_adjoint=True,
            is_positive_definite=True)
         for l in lin_op_matrices])

    matrices = linear_operator_util.broadcast_matrix_batch_dims(matrices)

    kronecker_dense = _kronecker_dense(matrices)

    if not use_placeholder:
      kronecker_dense.set_shape(shape)

    return operator, kronecker_dense

  def test_is_x_flags(self):
    # Matrix with two positive eigenvalues, 1, and 1.
    # The matrix values do not effect auto-setting of the flags.
    matrix = [[1., 0.], [1., 1.]]
    operator = kronecker.LinearOperatorKronecker(
        [linalg.LinearOperatorFullMatrix(matrix),
         linalg.LinearOperatorFullMatrix(matrix)],
        is_positive_definite=True,
        is_non_singular=True,
        is_self_adjoint=False)
    self.assertTrue(operator.is_positive_definite)
    self.assertTrue(operator.is_non_singular)
    self.assertFalse(operator.is_self_adjoint)

  def test_is_non_singular_auto_set(self):
    # Matrix with two positive eigenvalues, 11 and 8.
    # The matrix values do not effect auto-setting of the flags.
    matrix = [[11., 0.], [1., 8.]]
    operator_1 = linalg.LinearOperatorFullMatrix(matrix, is_non_singular=True)
    operator_2 = linalg.LinearOperatorFullMatrix(matrix, is_non_singular=True)

    operator = kronecker.LinearOperatorKronecker(
        [operator_1, operator_2],
        is_positive_definite=False,  # No reason it HAS to be False...
        is_non_singular=None)
    self.assertFalse(operator.is_positive_definite)
    self.assertTrue(operator.is_non_singular)

    with self.assertRaisesRegexp(ValueError, "always non-singular"):
      kronecker.LinearOperatorKronecker(
          [operator_1, operator_2], is_non_singular=False)

  def test_name(self):
    matrix = [[11., 0.], [1., 8.]]
    operator_1 = linalg.LinearOperatorFullMatrix(matrix, name="left")
    operator_2 = linalg.LinearOperatorFullMatrix(matrix, name="right")

    operator = kronecker.LinearOperatorKronecker([operator_1, operator_2])

    self.assertEqual("left_x_right", operator.name)

  def test_different_dtypes_raises(self):
    operators = [
        linalg.LinearOperatorFullMatrix(rng.rand(2, 3, 3)),
        linalg.LinearOperatorFullMatrix(rng.rand(2, 3, 3).astype(np.float32))
    ]
    with self.assertRaisesRegexp(TypeError, "same dtype"):
      kronecker.LinearOperatorKronecker(operators)

  def test_empty_or_one_operators_raises(self):
    with self.assertRaisesRegexp(ValueError, ">=1 operators"):
      kronecker.LinearOperatorKronecker([])

  def test_kronecker_cholesky_type(self):
    matrix = [[1., 0.], [0., 1.]]
    operator = kronecker.LinearOperatorKronecker(
        [
            linalg.LinearOperatorFullMatrix(
                matrix,
                is_positive_definite=True,
                is_self_adjoint=True,
            ),
            linalg.LinearOperatorFullMatrix(
                matrix,
                is_positive_definite=True,
                is_self_adjoint=True,
            ),
        ],
        is_positive_definite=True,
        is_self_adjoint=True,
    )
    cholesky_factor = operator.cholesky()
    self.assertTrue(isinstance(
        cholesky_factor,
        kronecker.LinearOperatorKronecker))
    self.assertEqual(2, len(cholesky_factor.operators))
    self.assertTrue(
        isinstance(
            cholesky_factor.operators[0],
            lower_triangular.LinearOperatorLowerTriangular)
    )
    self.assertTrue(
        isinstance(
            cholesky_factor.operators[1],
            lower_triangular.LinearOperatorLowerTriangular)
    )


if __name__ == "__main__":
  test.main()
