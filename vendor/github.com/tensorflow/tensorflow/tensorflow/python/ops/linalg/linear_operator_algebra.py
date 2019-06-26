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

"""Registration mechanisms for various n-ary operations on LinearOperators."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import itertools

from tensorflow.python.framework import ops
from tensorflow.python.util import tf_inspect


_CHOLESKY_DECOMPS = {}
_MATMUL = {}


def _registered_function(type_list, registry):
  """Given a list of classes, finds the most specific function registered."""
  enumerated_hierarchies = [enumerate(tf_inspect.getmro(t)) for t in type_list]
  # Get all possible combinations of hierarchies.
  cls_combinations = list(itertools.product(*enumerated_hierarchies))

  def hierarchy_distance(cls_combination):
    candidate_distance = sum(c[0] for c in cls_combination)
    if tuple(c[1] for c in cls_combination) in registry:
      return candidate_distance
    return 10000

  registered_combination = min(cls_combinations, key=hierarchy_distance)
  return registry.get(tuple(r[1] for r in registered_combination), None)


def _registered_cholesky(type_a):
  """Get the Cholesky function registered for class a."""
  return _registered_function([type_a], _CHOLESKY_DECOMPS)


def _registered_matmul(type_a, type_b):
  """Get the Matmul function registered for classes a and b."""
  return _registered_function([type_a, type_b], _MATMUL)


def cholesky(lin_op_a, name=None):
  """Get the Cholesky factor associated to lin_op_a.

  Args:
    lin_op_a: The LinearOperator to decompose.
    name: Name to use for this operation.

  Returns:
    A LinearOperator that represents the lower Cholesky factor of `lin_op_a`.

  Raises:
    NotImplementedError: If no Cholesky method is defined for the LinearOperator
      type of `lin_op_a`.
  """
  cholesky_fn = _registered_cholesky(type(lin_op_a))
  if cholesky_fn is None:
    raise ValueError("No cholesky decomposition registered for {}".format(
        type(lin_op_a)))

  with ops.name_scope(name, "Cholesky"):
    return cholesky_fn(lin_op_a)


def matmul(lin_op_a, lin_op_b, name=None):
  """Compute lin_op_a.matmul(lin_op_b).

  Args:
    lin_op_a: The LinearOperator on the left.
    lin_op_b: The LinearOperator on the right.
    name: Name to use for this operation.

  Returns:
    A LinearOperator that represents the matmul between `lin_op_a` and
      `lin_op_b`.

  Raises:
    NotImplementedError: If no matmul method is defined between types of
      `lin_op_a` and `lin_op_b`.
  """
  matmul_fn = _registered_matmul(type(lin_op_a), type(lin_op_b))
  if matmul_fn is None:
    raise ValueError("No matmul registered for {}.matmul({})".format(
        type(lin_op_a), type(lin_op_b)))

  with ops.name_scope(name, "Matmul"):
    return matmul_fn(lin_op_a, lin_op_b)


class RegisterCholesky(object):
  """Decorator to register a Cholesky implementation function.

  Usage:

  @linear_operator_algebra.RegisterCholesky(lin_op.LinearOperatorIdentity)
  def _cholesky_identity(lin_op_a):
    # Return the identity matrix.
  """

  def __init__(self, lin_op_cls_a):
    """Initialize the LinearOperator registrar.

    Args:
      lin_op_cls_a: the class of the LinearOperator to decompose.
    """
    self._key = (lin_op_cls_a,)

  def __call__(self, cholesky_fn):
    """Perform the Cholesky registration.

    Args:
      cholesky_fn: The function to use for the Cholesky.

    Returns:
      cholesky_fn

    Raises:
      TypeError: if cholesky_fn is not a callable.
      ValueError: if a Cholesky function has already been registered for
        the given argument classes.
    """
    if not callable(cholesky_fn):
      raise TypeError(
          "cholesky_fn must be callable, received: {}".format(cholesky_fn))
    if self._key in _CHOLESKY_DECOMPS:
      raise ValueError("Cholesky({}) has already been registered to: {}".format(
          self._key[0].__name__, _CHOLESKY_DECOMPS[self._key]))
    _CHOLESKY_DECOMPS[self._key] = cholesky_fn
    return cholesky_fn


class RegisterMatmul(object):
  """Decorator to register a Matmul implementation function.

  Usage:

  @linear_operator_algebra.RegisterMatmul(
    lin_op.LinearOperatorIdentity,
    lin_op.LinearOperatorIdentity)
  def _matmul_identity(a, b):
    # Return the identity matrix.
  """

  def __init__(self, lin_op_cls_a, lin_op_cls_b):
    """Initialize the LinearOperator registrar.

    Args:
      lin_op_cls_a: the class of the LinearOperator to multiply.
      lin_op_cls_b: the class of the second LinearOperator to multiply.
    """
    self._key = (lin_op_cls_a, lin_op_cls_b)

  def __call__(self, matmul_fn):
    """Perform the Matmul registration.

    Args:
      matmul_fn: The function to use for the Matmul.

    Returns:
      matmul_fn

    Raises:
      TypeError: if matmul_fn is not a callable.
      ValueError: if a Matmul function has already been registered for
        the given argument classes.
    """
    if not callable(matmul_fn):
      raise TypeError(
          "matmul_fn must be callable, received: {}".format(matmul_fn))
    if self._key in _MATMUL:
      raise ValueError("Matmul({}, {}) has already been registered.".format(
          self._key[0].__name__,
          self._key[1].__name__))
    _MATMUL[self._key] = matmul_fn
    return matmul_fn
