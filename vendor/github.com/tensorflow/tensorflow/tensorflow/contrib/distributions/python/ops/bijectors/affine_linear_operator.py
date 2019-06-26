# Copyright 2016 The TensorFlow Authors. All Rights Reserved.
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
"""AffineLinearOperator bijector."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.distributions.python.ops.shape import _DistributionShape
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.ops.distributions import bijector
from tensorflow.python.ops.linalg import linear_operator
from tensorflow.python.util import deprecation


__all__ = [
    "AffineLinearOperator",
]


class AffineLinearOperator(bijector.Bijector):
  """Compute `Y = g(X; shift, scale) = scale @ X + shift`.

  `shift` is a numeric `Tensor` and `scale` is a `LinearOperator`.

  If `X` is a scalar then the forward transformation is: `scale * X + shift`
  where `*` denotes the scalar product.

  Note: we don't always simply transpose `X` (but write it this way for
  brevity). Actually the input `X` undergoes the following transformation
  before being premultiplied by `scale`:

  1. If there are no sample dims, we call `X = tf.expand_dims(X, 0)`, i.e.,
     `new_sample_shape = [1]`. Otherwise do nothing.
  2. The sample shape is flattened to have one dimension, i.e.,
     `new_sample_shape = [n]` where `n = tf.reduce_prod(old_sample_shape)`.
  3. The sample dim is cyclically rotated left by 1, i.e.,
     `new_shape = [B1,...,Bb, k, n]` where `n` is as above, `k` is the
     event_shape, and `B1,...,Bb` are the batch shapes for each of `b` batch
     dimensions.

  (For more details see `shape.make_batch_of_event_sample_matrices`.)

  The result of the above transformation is that `X` can be regarded as a batch
  of matrices where each column is a draw from the distribution. After
  premultiplying by `scale`, we take the inverse of this procedure. The input
  `Y` also undergoes the same transformation before/after premultiplying by
  `inv(scale)`.

  Example Use:

  ```python
  linalg = tf.linalg

  x = [1., 2, 3]

  shift = [-1., 0., 1]
  diag = [1., 2, 3]
  scale = linalg.LinearOperatorDiag(diag)
  affine = AffineLinearOperator(shift, scale)
  # In this case, `forward` is equivalent to:
  # y = scale @ x + shift
  y = affine.forward(x)  # [0., 4, 10]

  shift = [2., 3, 1]
  tril = [[1., 0, 0],
          [2, 1, 0],
          [3, 2, 1]]
  scale = linalg.LinearOperatorLowerTriangular(tril)
  affine = AffineLinearOperator(shift, scale)
  # In this case, `forward` is equivalent to:
  # np.squeeze(np.matmul(tril, np.expand_dims(x, -1)), -1) + shift
  y = affine.forward(x)  # [3., 7, 11]
  ```

  """

  @deprecation.deprecated(
      "2018-10-01",
      "The TensorFlow Distributions library has moved to "
      "TensorFlow Probability "
      "(https://github.com/tensorflow/probability). You "
      "should update all references to use `tfp.distributions` "
      "instead of `tf.contrib.distributions`.",
      warn_once=True)
  def __init__(self,
               shift=None,
               scale=None,
               validate_args=False,
               name="affine_linear_operator"):
    """Instantiates the `AffineLinearOperator` bijector.

    Args:
      shift: Floating-point `Tensor`.
      scale:  Subclass of `LinearOperator`. Represents the (batch) positive
        definite matrix `M` in `R^{k x k}`.
      validate_args: Python `bool` indicating whether arguments should be
        checked for correctness.
      name: Python `str` name given to ops managed by this object.

    Raises:
      TypeError: if `scale` is not a `LinearOperator`.
      TypeError: if `shift.dtype` does not match `scale.dtype`.
      ValueError: if not `scale.is_non_singular`.
    """
    self._graph_parents = []
    self._name = name
    self._validate_args = validate_args
    graph_parents = []
    with self._name_scope("init", values=[shift]):
      # In the absence of `loc` and `scale`, we'll assume `dtype` is `float32`.
      dtype = dtypes.float32

      if shift is not None:
        shift = ops.convert_to_tensor(shift, name="shift")
        graph_parents += [shift]
        dtype = shift.dtype.base_dtype
      self._shift = shift

      if scale is not None:
        if (shift is not None and
            shift.dtype.base_dtype != scale.dtype.base_dtype):
          raise TypeError(
              "shift.dtype({}) is incompatible with scale.dtype({}).".format(
                  shift.dtype, scale.dtype))
        if not isinstance(scale, linear_operator.LinearOperator):
          raise TypeError("scale is not an instance of tf.LinearOperator")
        if validate_args and not scale.is_non_singular:
          raise ValueError("Scale matrix must be non-singular.")
        graph_parents += scale.graph_parents
        if scale.tensor_rank is not None:
          batch_ndims = scale.tensor_rank - 2
        else:
          batch_ndims = scale.tensor_rank_tensor() - 2
          graph_parents += [batch_ndims]
        if scale.dtype is not None:
          dtype = scale.dtype.base_dtype
      else:
        batch_ndims = 0  # We won't need shape inference when scale is None.
      self._scale = scale
      self._shaper = _DistributionShape(
          batch_ndims=batch_ndims,
          event_ndims=1,
          validate_args=validate_args)
      super(AffineLinearOperator, self).__init__(
          forward_min_event_ndims=1,
          graph_parents=graph_parents,
          is_constant_jacobian=True,
          dtype=dtype,
          validate_args=validate_args,
          name=name)

  @property
  def shift(self):
    """The `shift` `Tensor` in `Y = scale @ X + shift`."""
    return self._shift

  @property
  def scale(self):
    """The `scale` `LinearOperator` in `Y = scale @ X + shift`."""
    return self._scale

  def _forward(self, x):
    y = x
    if self.scale is not None:
      y, sample_shape = self._shaper.make_batch_of_event_sample_matrices(
          y, expand_batch_dim=False)
      with ops.control_dependencies(self._maybe_collect_assertions() if
                                    self.validate_args else []):
        y = self.scale.matmul(y)
      y = self._shaper.undo_make_batch_of_event_sample_matrices(
          y, sample_shape, expand_batch_dim=False)
    if self.shift is not None:
      y += self.shift
    return y

  def _inverse(self, y):
    x = y
    if self.shift is not None:
      x -= self.shift
    if self.scale is not None:
      x, sample_shape = self._shaper.make_batch_of_event_sample_matrices(
          x, expand_batch_dim=False)
      # Solve fails if the op is singular so we may safely skip this assertion.
      x = self.scale.solve(x)
      x = self._shaper.undo_make_batch_of_event_sample_matrices(
          x, sample_shape, expand_batch_dim=False)
    return x

  def _forward_log_det_jacobian(self, x):
    # is_constant_jacobian = True for this bijector, hence the
    # `log_det_jacobian` need only be specified for a single input, as this will
    # be tiled to match `event_ndims`.
    if self.scale is None:
      return constant_op.constant(0., dtype=x.dtype.base_dtype)

    with ops.control_dependencies(self._maybe_collect_assertions() if
                                  self.validate_args else []):
      return self.scale.log_abs_determinant()

  def _maybe_collect_assertions(self):
    try:
      return [self.scale.assert_non_singular()]
    except NotImplementedError:
      pass
    return []
