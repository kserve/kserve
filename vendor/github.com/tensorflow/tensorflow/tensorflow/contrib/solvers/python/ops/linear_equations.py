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
"""Solvers for linear equations."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections

from tensorflow.contrib.solvers.python.ops import util
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import linalg_ops
from tensorflow.python.ops import math_ops


def conjugate_gradient(operator,
                       rhs,
                       preconditioner=None,
                       x=None,
                       tol=1e-4,
                       max_iter=20,
                       name="conjugate_gradient"):
  r"""Conjugate gradient solver.

  Solves a linear system of equations `A*x = rhs` for selfadjoint, positive
  definite matrix `A` and right-hand side vector `rhs`, using an iterative,
  matrix-free algorithm where the action of the matrix A is represented by
  `operator`. The iteration terminates when either the number of iterations
  exceeds `max_iter` or when the residual norm has been reduced to `tol`
  times its initial value, i.e. \\(||rhs - A x_k|| <= tol ||rhs||\\).

  Args:
    operator: An object representing a linear operator with attributes:
      - shape: Either a list of integers or a 1-D `Tensor` of type `int32` of
        length 2. `shape[0]` is the dimension on the domain of the operator,
        `shape[1]` is the dimension of the co-domain of the operator. On other
        words, if operator represents an N x N matrix A, `shape` must contain
        `[N, N]`.
      - dtype: The datatype of input to and output from `apply`.
      - apply: Callable object taking a vector `x` as input and returning a
        vector with the result of applying the operator to `x`, i.e. if
       `operator` represents matrix `A`, `apply` should return `A * x`.
    rhs: A rank-1 `Tensor` of shape `[N]` containing the right-hand size vector.
    preconditioner: An object representing a linear operator, see `operator`
      for detail. The preconditioner should approximate the inverse of `A`.
      An efficient preconditioner could dramatically improve the rate of
      convergence. If `preconditioner` represents matrix `M`(`M` approximates
      `A^{-1}`), the algorithm uses `preconditioner.apply(x)` to estimate
      `A^{-1}x`. For this to be useful, the cost of applying `M` should be
      much lower than computing `A^{-1}` directly.
    x: A rank-1 `Tensor` of shape `[N]` containing the initial guess for the
      solution.
    tol: A float scalar convergence tolerance.
    max_iter: An integer giving the maximum number of iterations.
    name: A name scope for the operation.

  Returns:
    output: A namedtuple representing the final state with fields:
      - i: A scalar `int32` `Tensor`. Number of iterations executed.
      - x: A rank-1 `Tensor` of shape `[N]` containing the computed solution.
      - r: A rank-1 `Tensor` of shape `[M]` containing the residual vector.
      - p: A rank-1 `Tensor` of shape `[N]`. `A`-conjugate basis vector.
      - gamma: \\(r \dot M \dot r\\), equivalent to  \\(||r||_2^2\\) when
        `preconditioner=None`.
  """
  # ephemeral class holding CG state.
  cg_state = collections.namedtuple("CGState", ["i", "x", "r", "p", "gamma"])

  def stopping_criterion(i, state):
    return math_ops.logical_and(i < max_iter, linalg_ops.norm(state.r) > tol)

  def cg_step(i, state):  # pylint: disable=missing-docstring
    z = operator.apply(state.p)
    alpha = state.gamma / util.dot(state.p, z)
    x = state.x + alpha * state.p
    r = state.r - alpha * z
    if preconditioner is None:
      gamma = util.dot(r, r)
      beta = gamma / state.gamma
      p = r + beta * state.p
    else:
      q = preconditioner.apply(r)
      gamma = util.dot(r, q)
      beta = gamma / state.gamma
      p = q + beta * state.p
    return i + 1, cg_state(i + 1, x, r, p, gamma)

  with ops.name_scope(name):
    n = operator.shape[1:]
    rhs = array_ops.expand_dims(rhs, -1)
    if x is None:
      x = array_ops.expand_dims(
          array_ops.zeros(n, dtype=rhs.dtype.base_dtype), -1)
      r0 = rhs
    else:
      x = array_ops.expand_dims(x, -1)
      r0 = rhs - operator.apply(x)
    if preconditioner is None:
      p0 = r0
    else:
      p0 = preconditioner.apply(r0)
    gamma0 = util.dot(r0, p0)
    tol *= linalg_ops.norm(r0)
    i = constant_op.constant(0, dtype=dtypes.int32)
    state = cg_state(i=i, x=x, r=r0, p=p0, gamma=gamma0)
    _, state = control_flow_ops.while_loop(stopping_criterion, cg_step,
                                           [i, state])
    return cg_state(
        state.i,
        x=array_ops.squeeze(state.x),
        r=array_ops.squeeze(state.r),
        p=array_ops.squeeze(state.p),
        gamma=state.gamma)
