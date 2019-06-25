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

"""Gradient checker for functions.

The gradient checker verifies numerically that an function properly
computes the gradients
"""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.eager import backprop
from tensorflow.python.eager import context
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.util.tf_export import tf_export


def _product(t):
  if isinstance(t, int):
    return t
  else:
    y = 1
    for x in t:
      y *= x
    return y


def _eval_indexed_slices(a):
  """Converts IndexedSlices to IndexedSlicesValue with numpy indices/values.

  When eager execution is enabled, converts IndexedSlices
  to IndexedSlicesValue with numpy indices/values.

  Args:
    a: any value.

  Returns:
    If a is IndexedSlices and eager execution is enabled, calls numpy() on a's
    fields. Otherwise returns a unchanged.
  """
  if isinstance(a, ops.IndexedSlices) and context.executing_eagerly():
    return ops.IndexedSlicesValue(
        indices=[x.numpy() for x in a.indices],
        values=[x.numpy() for x in a.values],
        dense_shape=a.dense_shape)
  return a


def _to_numpy(a):
  """Converts Tensors and EagerTensors to numpy arrays.

  Args:
    a: any value.

  Returns:
    If a is EagerTensor or Tensor, returns the evaluation of a by calling
    numpy() or run(). Otherwise returns a unchanged.
  """
  if isinstance(a, ops.EagerTensor):
    return a.numpy()
  if isinstance(a, ops.Tensor):
    sess = ops.get_default_session()
    return sess.run(a)
  return a


def _prepare(f, xs_dtypes):
  """Return a function that executes 'f'.

    In TF 2.x, this is the same as `f`.
    In TF 1.x, returns a Python function that executes the graph defined by `f`
    in a Session.

  Args:
    f: the function.
    xs_dtypes: dtypes of f's arguments.

  Returns:
    a function that will be evaluated in both graph and eager mode
  """
  if context.executing_eagerly():

    def decorated_eager(*xs_data):
      return f(*map(ops.convert_to_tensor, xs_data))

    return decorated_eager
  xs = [array_ops.placeholder(x_dtype) for x_dtype in xs_dtypes]
  y = f(*xs)
  sess = ops.get_default_session()
  def decorated_graph(*xs_data):
    xs_data = [_to_numpy(a) for a in xs_data]
    return sess.run(y, feed_dict=dict(zip(xs, xs_data)))
  return decorated_graph


def _compute_theoretical_jacobian(f, y_shape, y_dtype, xs, param):
  """Computes the theoretical Jacobian for f regarding xs[param].

  One can think of the relation among f, xs and y as y = f(xs).

  Args:
    f: the function.
    y_shape: the shape of the result.
    y_dtype: the dtype of the result.
    xs: a list of tensors.
    param: the index of the target parameter.

  Returns:
    A 2-d numpy array representing the Jacobian. It has "x_size" rows
    and "y_size" columns where "x_size" is the number of elements in xs[param]
    and "y_size" is the number of elements in the result.

  Raises:
    ValueError: If result is empty but the gradient is nonzero.
  """
  x = xs[param]
  # Complex vectors are treated as vectors of twice as many reals.
  x_shape = tuple(x.shape) + (2,) if x.dtype.is_complex else x.shape
  y_factor = 2 if y_dtype.is_complex else 1

  # To compute the jacobian, we treat x and y as one-dimensional vectors.
  x_size = _product(x_shape)
  x_val_size = _product(x_shape[1:])  # This is used for sparse gradients
  y_size = _product(y_shape) * y_factor

  # Allocate 2-D Jacobian, with x dimensions smashed into the first
  # dimension and y dimensions smashed into the second.
  jacobian = np.zeros((x_size, y_size), dtype=x.dtype.real_dtype.as_numpy_dtype)

  # For each of the entry of dy, we set this to be 1 and
  # everything else to be 0 and compute the gradients -- this will give us one
  # one column of the Jacobian matrix.
  dy_data = np.zeros(y_shape, dtype=y_dtype.as_numpy_dtype)
  dy_data_flat = dy_data.ravel().view(y_dtype.real_dtype.as_numpy_dtype)
  grad_fn_unprep = backprop.gradients_function(f, [param])
  grad_fn = _prepare(lambda dy, *xs: grad_fn_unprep(*xs, dy=dy),
                     [y_dtype] + [x.dtype for x in xs])
  for col in range(y_size):
    dy_data_flat[col] = 1
    grad = _to_numpy(grad_fn(dy_data, *xs)[0])
    grad = _eval_indexed_slices(grad)
    dy_data_flat[col] = 0
    if isinstance(grad, ops.IndexedSlicesValue):
      for i, v in zip(grad.indices, grad.values):
        r_begin = i * x_val_size
        r_end = r_begin + x_val_size
        jacobian[r_begin:r_end, col] += v.flat
    else:
      jacobian[:, col] = grad.ravel().view(jacobian.dtype)

  # If the output is empty, run the gradients at least once and make sure
  # they produce zeros.
  if y_size == 0:  # don't use 'not y_size', because y_size may not be an int
    grad = _to_numpy(grad_fn(dy_data, *xs)[0])
    if grad.shape != x.shape:
      raise ValueError("Empty gradient has wrong shape: expected %s, got %s" %
                       (x.shape, grad.shape))
    if np.any(grad):
      raise ValueError("Empty tensor with nonzero gradients")

  logging.vlog(1, "Theoretical Jacobian =\n%s", jacobian)
  return jacobian


def _compute_numeric_jacobian(f, y_size, y_dtype, xs, param,
                              delta):
  """Computes the numeric Jacobian for f regarding xs[param].

  One can think of the relation among f, xs and y as y = f(xs).

  Args:
    f: the function.
    y_size: the number of elements of the result.
    y_dtype: the dtype of the result.
    xs: a list of tensors.
    param: the index of the target parameter.
    delta: the amount of perturbation we give to the input.

  Returns:
    A 2-d numpy array representing the Jacobian. It has "x_size" rows
    and "y_size" columns where "x_size" is the number of elements in xs[param]
    and "y_size" is the number of elements in the result.
  """
  # bfloat16 doesn't have enough bits to represent high precision numbers such
  # as delta. Convert to float32 here. Since numeric_jacobian is expected to
  # be the groundtruth to compare against, it shouldn't lose any information.
  x_shape = xs[param].shape
  x_dtype = xs[param].dtype
  if y_dtype == dtypes.bfloat16:
    f = lambda *xs: math_ops.cast(f(*xs), dtypes.float32)
    y_dtype = dtypes.float32

  # To compute the jacobian, we treat x and y as one-dimensional vectors
  x_size = _product(x_shape) * (2 if x_dtype.is_complex else 1)
  y_size = y_size * (2 if y_dtype.is_complex else 1)
  x_dtype = x_dtype.real_dtype.as_numpy_dtype
  y_dtype = y_dtype.real_dtype.as_numpy_dtype

  xs_dtypes = [x.dtype for x in xs]
  # Converts xs to numpy arrays to do in-place perturbation.
  # Calls asarray() to avoid copying in ravel() later.
  xs = [np.asarray(_to_numpy(x)) for x in xs]
  x = xs[param]

  # Make sure we have the right types
  scale = np.asarray(2 * delta, dtype=y_dtype)[()]

  jacobian = np.zeros((x_size, y_size), dtype=x_dtype)
  # For each of the entry of x, we slightly perturbs this by adding and
  # subtracting a delta and then compute difference between the outputs. This
  # will give us one row of the Jacobian matrix.

  f = _prepare(f, xs_dtypes)
  for row in range(x_size):
    original = x.ravel().view(x_dtype)[row]
    x.ravel().view(x_dtype)[row] += delta
    y_pos = _to_numpy(f(*xs))
    x.ravel().view(x_dtype)[row] = original
    x.ravel().view(x_dtype)[row] -= delta
    y_neg = _to_numpy(f(*xs))
    x.ravel().view(x_dtype)[row] = original
    diff = (y_pos - y_neg) / scale
    jacobian[row, :] = diff.ravel().view(y_dtype)

  logging.vlog(1, "Numeric Jacobian =\n%s", jacobian)
  return jacobian


def _compute_gradient(f,
                      y_shape,
                      y_dtype,
                      xs,
                      param,
                      delta):
  """Computes the theoretical and numerical jacobian."""
  x = xs[param]
  t = x.dtype
  allowed_types = [dtypes.float16, dtypes.bfloat16, dtypes.float32,
                   dtypes.float64, dtypes.complex64, dtypes.complex128]
  assert t.base_dtype in allowed_types, ("Cannot compute gradient for"
                                         "unsupported type %s of argument %s" %
                                         (t.name, param))
  t2 = y_dtype
  assert t2.base_dtype in allowed_types, ("Cannot compute gradient for"
                                          "unsupported type %s of y" % t2.name)
  y_size = _product(y_shape)
  jacob_t = _compute_theoretical_jacobian(f, y_shape, y_dtype,
                                          xs, param)
  jacob_n = _compute_numeric_jacobian(f, y_size, y_dtype, xs,
                                      param, delta)
  return jacob_t, jacob_n


def _compute_gradient_list(f, xs, delta):
  """Compute gradients for a list of x values."""
  # convert xs to tensors so that dtype and shape have uniform types
  xs = list(map(ops.convert_to_tensor, xs))
  # run the function to get info of the result
  xs_dtypes = [x.dtype for x in xs]
  f_temp = _prepare(f, xs_dtypes)
  y = f_temp(*xs)
  return zip(*[_compute_gradient(f, y.shape, dtypes.as_dtype(y.dtype),
                                 xs, i, delta) for i in range(len(xs))])


@tf_export("test.compute_gradient", v1=[])
def compute_gradient(f, x, delta=1e-3):
  """Computes the theoretical and numeric Jacobian of f.

  With y = f(x), computes the theoretical and numeric Jacobian dy/dx.

  Args:
    f: the function.
    x: a list of tensors.
    delta: (optional) perturbation used to compute numeric Jacobian.

  Returns:
    A pair of lists, where the first is a list of 2-d numpy arrays representing
    the theoretical Jacobians for each argument, and the second list is the
    numerical ones. Each 2-d array has "x_size" rows
    and "y_size" columns where "x_size" is the number of elements in the
    corresponding argument and "y_size" is the number of elements in f(x).

  Raises:
    ValueError: If result is empty but the gradient is nonzero.
  """
  if not isinstance(x, list):
    raise ValueError(
        "`x` must be a list of Tensors (arguments to `f`), not a %s" % type(x))
  return _compute_gradient_list(f, x, delta)


def max_error(grad1, grad2):
  """Computes maximum elementwise gap.

  Computes the maximum elementwise gap between two lists of tensors of the same
  shape.

  Args:
    grad1: a lists of tensors.
    grad2: a lists of tensors with the same shape as grad1.

  Returns:
    The maximum elementwise gap between the two.
  """
  error = 0
  for j_t, j_n in zip(grad1, grad2):
    if j_t.size or j_n.size:  # Handle zero size tensors correctly
      error = np.maximum(error, np.fabs(j_t - j_n).max())
  return error
