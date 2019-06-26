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
"""Control Flow Operations.

See the [autograph](https://www.tensorflow.org/guide/autographs) guide.
"""
# pylint: disable=g-bad-name
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import abc
import collections
import functools

import six

from tensorflow.core.framework import attr_value_pb2
from tensorflow.core.protobuf import control_flow_pb2
from tensorflow.python.eager import context
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import tensor_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_util as util
from tensorflow.python.ops import gen_array_ops
from tensorflow.python.ops import gen_control_flow_ops
from tensorflow.python.ops import gen_data_flow_ops
from tensorflow.python.ops import gen_logging_ops
from tensorflow.python.ops import gen_resource_variable_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import tensor_array_ops
# go/tf-wildcard-import
# pylint: disable=wildcard-import,undefined-variable
from tensorflow.python.ops.gen_control_flow_ops import *
# pylint: enable=wildcard-import
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.util import compat
from tensorflow.python.util import deprecation
from tensorflow.python.util import nest
from tensorflow.python.util import tf_should_use
from tensorflow.python.util.lazy_loader import LazyLoader
from tensorflow.python.util.tf_export import tf_export

# This is to avoid a circular dependency:
# cond_v2 -> gradients_impl -> control_flow_ops
cond_v2 = LazyLoader("cond_v2", globals(),
                     "tensorflow.python.ops.cond_v2")

# This is to avoid circular dependencies:
# while_v2 -> control_flow_ops
# while_v2 -> gradients_impl -> control_flow_ops
while_v2 = LazyLoader("while_v2", globals(),
                      "tensorflow.python.ops.while_v2")

# We override the 'tuple' for a control flow op, so we keep python's
# existing 'tuple' for later use in this module.
_basetuple = tuple


def _summarize_eager(tensor, summarize=None):
  """Returns a summarized string representation of eager `tensor`.

  Args:
    tensor: EagerTensor to summarize
    summarize: Include these many first elements of `array`
  """
  # Emulate the behavior of Tensor::SummarizeValue()
  if summarize is None:
    summarize = 3
  elif summarize < 0:
    summarize = array_ops.size(tensor)
  # reshape((-1,)) is the fastest way to get a flat array view
  if tensor._rank():  # pylint: disable=protected-access
    flat = tensor.numpy().reshape((-1,))
    lst = [str(x) for x in flat[:summarize]]
    if len(lst) < flat.size:
      lst.append("...")
  else:
    # tensor.numpy() returns a scalar for zero dimensional arrays
    if summarize != 0:
      lst = [str(tensor.numpy())]
    else:
      lst = []

  return ", ".join(lst)


# pylint: disable=protected-access


# Assert and Print are special symbols in python, so we must
# use an upper-case version of them.
@tf_export("debugging.Assert", "Assert")
@tf_should_use.should_use_result
def Assert(condition, data, summarize=None, name=None):
  """Asserts that the given condition is true.

  If `condition` evaluates to false, print the list of tensors in `data`.
  `summarize` determines how many entries of the tensors to print.

  NOTE: In graph mode, to ensure that Assert executes, one usually attaches
  a dependency:

  ```python
  # Ensure maximum element of x is smaller or equal to 1
  assert_op = tf.Assert(tf.less_equal(tf.reduce_max(x), 1.), [x])
  with tf.control_dependencies([assert_op]):
    ... code using x ...
  ```

  Args:
    condition: The condition to evaluate.
    data: The tensors to print out when condition is false.
    summarize: Print this many entries of each tensor.
    name: A name for this operation (optional).

  Returns:
    assert_op: An `Operation` that, when executed, raises a
    `tf.errors.InvalidArgumentError` if `condition` is not true.
    @compatibility{eager} returns None.

  Raises:
    @compatibility{eager} `tf.errors.InvalidArgumentError` if `condition`
    is not true
  """
  if context.executing_eagerly():
    if not condition:
      xs = ops.convert_n_to_tensor(data)
      data_str = [_summarize_eager(x, summarize) for x in xs]
      raise errors.InvalidArgumentError(
          node_def=None,
          op=None,
          message="Expected '%s' to be true. Summarized data: %s" %
          (condition, "\n".join(data_str)))
    return

  with ops.name_scope(name, "Assert", [condition, data]) as name:
    xs = ops.convert_n_to_tensor(data)
    if all(x.dtype in {dtypes.string, dtypes.int32} for x in xs):
      # As a simple heuristic, we assume that string and int32 are
      # on host to avoid the need to use cond. If it is not case,
      # we will pay the price copying the tensor to host memory.
      return gen_logging_ops._assert(condition, data, summarize, name="Assert")
    else:
      condition = ops.convert_to_tensor(condition, name="Condition")

      def true_assert():
        return gen_logging_ops._assert(
            condition, data, summarize, name="Assert")

      guarded_assert = cond(condition, no_op, true_assert, name="AssertGuard")
      if context.executing_eagerly():
        return
      return guarded_assert.op


def _Identity(data, name=None):
  """Return a tensor with the same shape and contents as the input tensor.

  Args:
    data: A Tensor.
    name: A name for this operation (optional).

  Returns:
    A Tensor with the same type and value as the input Tensor.
  """
  data = ops.internal_convert_to_tensor_or_indexed_slices(data, as_ref=True)
  if isinstance(data, ops.Tensor):
    if data.dtype._is_ref_dtype:  # pylint: disable=protected-access
      return gen_array_ops.ref_identity(data, name=name)
    else:
      return array_ops.identity(data, name=name)
  else:
    if not isinstance(data, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      raise TypeError("Type %s not supported" % type(data))
    values = _Identity(data.values, name=name)
    indices = array_ops.identity(data.indices, name="indices")
    if isinstance(data, ops.IndexedSlices):
      dense_shape = data.dense_shape
      if dense_shape is not None:
        dense_shape = array_ops.identity(dense_shape, name="dense_shape")
      return ops.IndexedSlices(values, indices, dense_shape)
    else:
      dense_shape = array_ops.identity(data.dense_shape, name="dense_shape")
      return sparse_tensor.SparseTensor(indices, values, dense_shape)


def _NextIteration(data, name=None):
  data = ops.internal_convert_to_tensor_or_indexed_slices(data, as_ref=True)
  if isinstance(data, ops.Tensor):
    if data.dtype._is_ref_dtype:  # pylint: disable=protected-access
      return ref_next_iteration(data, name=name)
    else:
      return next_iteration(data, name=name)
  else:
    if not isinstance(data, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      raise TypeError("Type %s not supported" % type(data))
    values = _NextIteration(data.values, name=name)
    indices = next_iteration(data.indices, name="indices")
    if isinstance(data, ops.IndexedSlices):
      dense_shape = data.dense_shape
      if dense_shape is not None:
        dense_shape = next_iteration(dense_shape, name="dense_shape")
      return ops.IndexedSlices(values, indices, dense_shape)
    else:
      dense_shape = next_iteration(data.dense_shape, name="dense_shape")
      return sparse_tensor.SparseTensor(indices, values, dense_shape)


def _Enter(data,
           frame_name,
           is_constant=False,
           parallel_iterations=10,
           use_ref=True,
           use_input_shape=True,
           name=None):
  """Creates or finds a child frame, and makes `data` available to it.

  The unique `frame_name` is used by the `Executor` to identify frames. If
  `is_constant` is true, `data` is a constant in the child frame; otherwise
  it may be changed in the child frame. At most `parallel_iterations`
  iterations are run in parallel in the child frame.

  Args:
    data: The tensor to be made available to the child frame.
    frame_name: The name of the child frame.
    is_constant: If true, the output is constant within the child frame.
    parallel_iterations: The number of iterations allowed to run in parallel.
    use_ref: If true, use ref_enter if data is of ref type.
    name: A name for this operation (optional).

  Returns:
    The same tensor as `data`.
  """
  data = ops.internal_convert_to_tensor_or_indexed_slices(data, as_ref=True)
  if isinstance(data, ops.Tensor):
    if data.dtype._is_ref_dtype and use_ref:  # pylint: disable=protected-access
      result = gen_control_flow_ops.ref_enter(
          data, frame_name, is_constant, parallel_iterations, name=name)
    else:
      result = gen_control_flow_ops.enter(
          data, frame_name, is_constant, parallel_iterations, name=name)
    if use_input_shape:
      result.set_shape(data.get_shape())
    return result
  else:
    if not isinstance(data, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      raise TypeError("Type %s not supported" % type(data))
    values = _Enter(
        data.values,
        frame_name,
        is_constant,
        parallel_iterations=parallel_iterations,
        use_input_shape=use_input_shape,
        name=name)
    indices = gen_control_flow_ops.enter(
        data.indices,
        frame_name,
        is_constant,
        parallel_iterations,
        name="indices")
    if use_input_shape:
      indices.set_shape(data.indices.get_shape())
    if isinstance(data, ops.IndexedSlices):
      dense_shape = data.dense_shape
      if dense_shape is not None:
        dense_shape = gen_control_flow_ops.enter(
            dense_shape,
            frame_name,
            is_constant,
            parallel_iterations,
            name="dense_shape")
        if use_input_shape:
          dense_shape.set_shape(data.dense_shape.get_shape())
      return ops.IndexedSlices(values, indices, dense_shape)
    else:
      dense_shape = gen_control_flow_ops.enter(
          data.dense_shape,
          frame_name,
          is_constant,
          parallel_iterations,
          name="dense_shape")
      if use_input_shape:
        dense_shape.set_shape(data.dense_shape.get_shape())
      return sparse_tensor.SparseTensor(indices, values, dense_shape)


def exit(data, name=None):  # pylint: disable=redefined-builtin
  """Exits the current frame to its parent frame.

  Exit makes its input `data` available to the parent frame.

  Args:
    data: The tensor to be made available to the parent frame.
    name: A name for this operation (optional).

  Returns:
    The same tensor as `data`.
  """
  data = ops.internal_convert_to_tensor_or_indexed_slices(data, as_ref=True)
  if isinstance(data, ops.Tensor):
    if data.dtype._is_ref_dtype:  # pylint: disable=protected-access
      return gen_control_flow_ops.ref_exit(data, name)
    else:
      return gen_control_flow_ops._exit(data, name)
  else:
    if not isinstance(data, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      raise TypeError("Type %s not supported" % type(data))
    values = exit(data.values, name=name)
    indices = gen_control_flow_ops._exit(data.indices, name="indices")
    if isinstance(data, ops.IndexedSlices):
      dense_shape = data.dense_shape
      if dense_shape is not None:
        dense_shape = gen_control_flow_ops._exit(dense_shape, name)
      return ops.IndexedSlices(values, indices, dense_shape)
    else:
      dense_shape = gen_control_flow_ops._exit(data.dense_shape, name)
      return sparse_tensor.SparseTensor(indices, values, dense_shape)


def switch(data, pred, dtype=None, name=None):
  """Forwards `data` to an output determined by `pred`.

  If `pred` is false, the `data` input is forwarded to the first output.
  Otherwise, the data goes to the second output.

  This op handles `Tensor`s and `IndexedSlices`.

  Args:
    data: The tensor to be forwarded to the appropriate output.
    pred: A scalar that specifies which output port will receive data.
    dtype: Optional element type for the returned tensor. If missing, the type
      is inferred from the type of `value`.
    name: A name for this operation (optional).

  Returns:
    `(output_false, output_true)`: If `pred` is true, data will be forwarded
    to `output_true`, otherwise it goes to `output_false`.
  """
  with ops.name_scope(name, "Switch", [data, pred]) as name:
    data = ops.internal_convert_to_tensor_or_indexed_slices(
        data, dtype=dtype, name="data", as_ref=True)
    pred = ops.convert_to_tensor(pred, name="pred")
    if isinstance(data, ops.Tensor):
      return gen_control_flow_ops.switch(data, pred, name=name)
    else:
      if not isinstance(data, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
        raise TypeError("Type %s not supported" % type(data))
      val, ind = data.values, data.indices
      val_f, val_t = gen_control_flow_ops.switch(val, pred, name=name)
      ind_f, ind_t = gen_control_flow_ops.switch(ind, pred, name="indices")
      if isinstance(data, ops.IndexedSlices):
        dense_shape = data.dense_shape
        if dense_shape is not None:
          dense_shape_f, dense_shape_t = gen_control_flow_ops.switch(
              dense_shape, pred, name="dense_shape")
        else:
          dense_shape_f, dense_shape_t = None, None
        return (ops.IndexedSlices(val_f, ind_f, dense_shape_f),
                ops.IndexedSlices(val_t, ind_t, dense_shape_t))
      else:
        dense_shape = data.dense_shape
        dense_shape_f, dense_shape_t = gen_control_flow_ops.switch(
            data.dense_shape, pred, name="dense_shape")
        return (sparse_tensor.SparseTensor(ind_f, val_f, dense_shape_f),
                sparse_tensor.SparseTensor(ind_t, val_t, dense_shape_t))


def _SwitchRefOrTensor(data, pred, name="Switch"):
  """Forwards `data` to an output determined by `pred`.

  If `pred` is false, the `data` input is forwarded to the first output.
  Otherwise, the data goes to the second output.

  This op handles `Tensor`s and `IndexedSlices`.

  Args:
    data: The tensor to be forwarded to the appropriate output.
    pred: A scalar that specifies which output port will receive data.
    name: A name for this operation (optional).

  Returns:
    `(output_false, output_true)`: If `pred` is true, data will be forwarded to
    `output_true`, otherwise it goes to `output_false`.

  Raises:
    TypeError: if data is not a Tensor or IndexedSlices
  """
  data = ops.convert_to_tensor_or_indexed_slices(data, name="data")
  # NOTE(vrv): ops.colocate_with(data, ignore_existing=True) below
  # addresses the following scenario.
  #
  # Assume you execute Optimizer.apply_gradients() in a branch of a cond().
  #
  # 1. The update op is created inside a `with ops.colocate(var):` block
  #
  # 2. Some tensor `data` is captured and a switch is created in a
  #    `with ops.colocate_with(data):` block.
  #
  # with ops.colocate_with(var):
  #  with ops.colocate_with(data):
  #    op = ...
  #
  # var and data may be pinned to different devices, so we want to ops
  # created within ops.colocate_with(data) to ignore the existing stack.
  with ops.colocate_with(data, ignore_existing=True):
    if isinstance(data, ops.Tensor):
      if data.dtype._is_ref_dtype:  # pylint: disable=protected-access
        return ref_switch(data, pred, name=name)
    return switch(data, pred, name=name)


def merge(inputs, name=None):
  """Returns the value of an available element of `inputs`.

  This op tests each of the tensors in `inputs` in turn to determine if any of
  them is available. If it finds an available tensor, it returns it and its
  index in `inputs`.

  It is an error if more than one tensor in `inputs` is available. If no tensor
  in `inputs` is available, the returned tensor and index are not set.

  This op handles both `Tensor`s and `IndexedSlices`. If inputs has a mix of
  `Tensor`s and `IndexedSlices`, all inputs are converted to IndexedSlices
  before merging.

  Args:
    inputs: The input tensors, at most one of which is available.
    name: A name for this operation (optional).

  Returns:
    A tuple containing the chosen input tensor and its index in `inputs`.

  Raises:
    ValueError: If any of the inputs is None, or inputs are IndexedSlices and
      some but not all have a dense_shape property.
  """
  if any(inp is None for inp in inputs):
    raise ValueError("At least one of the merge inputs is None: %s" % inputs)
  with ops.name_scope(name, "Merge", inputs) as name:
    inputs = [
        ops.internal_convert_to_tensor_or_indexed_slices(inp, as_ref=True)
        for inp in inputs
    ]
    if all(isinstance(v, ops.Tensor) for v in inputs):
      if all(v.dtype._is_ref_dtype for v in inputs):  # pylint: disable=protected-access
        return gen_control_flow_ops.ref_merge(inputs, name)
      else:
        return gen_control_flow_ops.merge(inputs, name)
    elif all(isinstance(v, sparse_tensor.SparseTensor) for v in inputs):
      # Only handle the case when all inputs are SparseTensor.
      values, _ = merge([inp.values for inp in inputs], name=name)
      indices, chosen_index = gen_control_flow_ops.merge(
          [inp.indices for inp in inputs], name="indices")
      dense_shape, _ = gen_control_flow_ops.merge(
          [inp.dense_shape for inp in inputs], name="dense_shape")
      return (sparse_tensor.SparseTensor(indices, values, dense_shape),
              chosen_index)
    else:
      # For now convert all the inputs as IndexedSlices.
      inputs = math_ops._as_indexed_slices_list(inputs, optimize=False)
      values, _ = merge([inp.values for inp in inputs], name=name)
      indices, chosen_index = gen_control_flow_ops.merge(
          [inp.indices for inp in inputs], name="indices")
      if any(inp.dense_shape is not None for inp in inputs):
        if any(inp.dense_shape is None for inp in inputs):
          raise ValueError("Either all merged IndexedSlices must have a "
                           "dense_shape, or none must have a dense_shape.")
        dense_shape, _ = gen_control_flow_ops.merge(
            [inp.dense_shape for inp in inputs], name="dense_shape")
      else:
        dense_shape = None
      return ops.IndexedSlices(values, indices, dense_shape), chosen_index


# pylint: enable=protected-access


def _convert_tensorarray_to_flow(tensor_or_tensor_array):
  if isinstance(tensor_or_tensor_array, tensor_array_ops.TensorArray):
    return tensor_or_tensor_array.flow
  else:
    return tensor_or_tensor_array


def _make_tensor_array(ta, t_or_flow):
  # pylint: disable=protected-access
  new_ta = tensor_array_ops.TensorArray(
      dtype=ta.dtype,
      handle=ta.handle,
      flow=t_or_flow,
      infer_shape=ta._infer_shape,
      colocate_with_first_write_call=ta._colocate_with_first_write_call)
  new_ta._colocate_with = ta._colocate_with
  new_ta._element_shape = ta._element_shape
  # pylint: enable=protected-access
  return new_ta


def _convert_flows_to_tensorarrays(tensors_or_tensorarrays, tensors_or_flows):
  if len(tensors_or_tensorarrays) != len(tensors_or_flows):
    raise ValueError(
        "Lengths of original Tensor list and new list do not match: %d vs. %d" %
        (len(tensors_or_tensorarrays), len(tensors_or_flows)))
  return [
      _make_tensor_array(ta, t_or_flow) if isinstance(
          ta, tensor_array_ops.TensorArray) else t_or_flow
      for (ta, t_or_flow) in zip(tensors_or_tensorarrays, tensors_or_flows)
  ]


def _ShapeLessThanOrEqual(shape1, shape2):
  if shape2.dims is None:
    return True
  if shape1.ndims != shape2.ndims:
    return False
  for dim1, dim2 in zip(shape1.dims, shape2.dims):
    if dim2.value is not None and dim1.value != dim2.value:
      return False
  return True


def _SetShapeInvariants(input_vars, enter_vars, shapes):
  """Set the shapes of the tensors in `enter_vars` to `shapes`.

  Args:
    input_vars: A list of tensors that are inputs to `enter_vars`.
    enter_vars: A list of tensors whose shapes will be set.
    shapes: A (possibly nested) list of shapes.

  Raises:
    ValueError: If any tensor in `enter_vars` has a less specific shape
      than its corresponding shape in `shapes`.
  """
  if shapes is None:
    return
  flat_shapes = nest.flatten(shapes)
  if not all(isinstance(s, tensor_shape.TensorShape) for s in flat_shapes):
    raise ValueError("`shapes` must be a (possibly nested) list of shapes.")
  # Check that the shapes of the inputs are less than the shape invariants,
  # and set the shapes of `enter_vars` to the shape invariants.
  for inp, var, shape in zip(input_vars, enter_vars, flat_shapes):
    if isinstance(var, ops.Tensor):
      if not _ShapeLessThanOrEqual(inp.get_shape(), shape):
        raise ValueError(
            "The shape invariant specified for %s is not compatible with "
            "the initial shape of the loop variable. It enters the loop "
            "with shape %s, but the specified shape invariant is %s." %
            (inp.name, inp.get_shape(), shape))
      var.set_shape(shape)
    else:
      if not isinstance(var, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
        raise TypeError("Type %s not supported" % type(var))
      if isinstance(var, ops.IndexedSlices):
        if not _ShapeLessThanOrEqual(inp.values.get_shape(), shape):
          raise ValueError(
              "The shape invariant specified for %s is not compatible with "
              "the initial shape of the values tensor of this IndexedSlices. "
              "It enters the loop with shape %s, but the specified shape "
              "invariant is %s." % (inp.values.name, inp.values.get_shape(),
                                    shape))
        var.values.set_shape(shape)
        var.indices.set_shape(tensor_shape.TensorShape([shape[0]]))
        if var.dense_shape is not None:
          var.dense_shape.set_shape(tensor_shape.TensorShape([shape.ndims]))
      else:
        if not _ShapeLessThanOrEqual(inp.dense_shape.get_shape(), shape):
          raise ValueError(
              "The shape invariant specified for %s is not compatible with "
              "the initial shape of the shape tensor of this SparseTensor. "
              "It enters the loop with shape %s, but the specified shape "
              "invariant is %s." % (inp.dense_shape.name,
                                    inp.dense_shape.get_shape(), shape))
        var.values.set_shape(tensor_shape.TensorShape([None]))
        var.indices.set_shape(tensor_shape.TensorShape([None, shape.ndims]))
        var.dense_shape.set_shape(shape)


def _EnforceShapeInvariant(merge_var, next_var):
  """Check if the shapes of the loops variables are invariants.

  Args:
    merge_var: The list of tensors representing the initial values of the loop
      variables.
    next_var: The list of tensors representing the values of the loop variables
      after one loop iteration.

  Raises:
    ValueError: If any tensor in `merge_var` has a more specific shape than
      its correspnding tensor in `next_var`.
  """
  if isinstance(merge_var, ops.Tensor):
    m_shape = merge_var.get_shape()
    n_shape = next_var.get_shape()
    if not _ShapeLessThanOrEqual(n_shape, m_shape):
      enter = merge_var.op.inputs[0].op
      assert util.IsLoopEnter(enter)
      input_t = enter.inputs[0]
      raise ValueError(
          "Input tensor '%s' enters the loop with shape %s, but has shape %s "
          "after one iteration. To allow the shape to vary across iterations, "
          "use the `shape_invariants` argument of tf.while_loop to specify a "
          "less-specific shape." % (input_t.name, input_t.shape, n_shape))
  else:
    if not isinstance(merge_var,
                      (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      raise TypeError("Type %s not supported" % type(merge_var))
    if isinstance(merge_var, ops.IndexedSlices):
      m_values_shape = merge_var.values.get_shape()
      m_indices_shape = merge_var.indices.get_shape()
      m_shape_shape = tensor_shape.TensorShape(None)
      if merge_var.dense_shape is not None:
        m_shape_shape = merge_var.dense_shape.get_shape()
      n_values_shape = next_var.values.get_shape()
      n_indices_shape = next_var.indices.get_shape()
      n_shape_shape = tensor_shape.TensorShape(None)
      if next_var.dense_shape is not None:
        n_shape_shape = next_var.dense_shape.get_shape()
      if (not _ShapeLessThanOrEqual(n_values_shape, m_values_shape) or
          not _ShapeLessThanOrEqual(n_indices_shape, m_indices_shape)):
        if not _ShapeLessThanOrEqual(n_values_shape, m_values_shape):
          raise ValueError(
              "The shape for %s is not an invariant for the loop. It enters "
              "the loop with shape (%s, %s, %s), but has shape (%s, %s, %s) "
              "after one iteration. Provide shape invariants using either the "
              "`shape_invariants` argument of tf.while_loop or set_shape() "
              "on the loop variables." %
              (merge_var.name, m_values_shape, m_indices_shape, m_shape_shape,
               n_values_shape, n_indices_shape, n_shape_shape))
    else:
      m_values_shape = merge_var.values.get_shape()
      m_indices_shape = merge_var.indices.get_shape()
      m_shape_shape = merge_var.dense_shape.get_shape()
      n_values_shape = next_var.values.get_shape()
      n_indices_shape = next_var.indices.get_shape()
      n_shape_shape = next_var.dense_shape.get_shape()
      if (not _ShapeLessThanOrEqual(n_values_shape, m_values_shape) or
          not _ShapeLessThanOrEqual(n_indices_shape, m_indices_shape) or
          not _ShapeLessThanOrEqual(n_shape_shape, m_shape_shape)):
        raise ValueError(
            "The shape for %s is not an invariant for the loop. It enters "
            "the loop with shape (%s, %s, %s), but has shape (%s, %s, %s) "
            "after one iteration. Provide shape invariants using either "
            "the `shape_invariants` argument of tf.while_loop or set_shape() "
            "on the loop variables." %
            (merge_var.name, m_values_shape, m_indices_shape, m_shape_shape,
             n_values_shape, n_indices_shape, n_shape_shape))


def _AddNextAndBackEdge(m, v, enforce_shape_invariant=True):
  """Add NextIteration and back edge from v to m."""
  if isinstance(m, ops.Tensor):
    v = ops.convert_to_tensor(v)
    v = _NextIteration(v)
    if enforce_shape_invariant:
      # Make sure the shapes of loop outputs are correct. We do this before
      # calling _update_input, which will raise a less-helpful error message if
      # the types don't match.
      # TODO(skyewm): call this for other cases below (needs testing)
      _EnforceShapeInvariant(m, v)
    m.op._update_input(1, v)  # pylint: disable=protected-access
  elif isinstance(m, ops.IndexedSlices):
    # pylint: disable=protected-access
    v = math_ops._as_indexed_slices(v, optimize=False)
    v = _NextIteration(v)
    m.values.op._update_input(1, v.values)
    m.indices.op._update_input(1, v.indices)
    # pylint: enable=protected-access
    if m.dense_shape is not None:
      if v.dense_shape is None:
        raise ValueError("Must have dense shape: %s" % v.name)
      m.dense_shape.op._update_input(1, v.dense_shape)
  elif isinstance(m, sparse_tensor.SparseTensor):
    if not isinstance(v, sparse_tensor.SparseTensor):
      raise ValueError("Must be a sparse tensor: %s" % v.name)
    v = _NextIteration(v)
    # pylint: disable=protected-access
    m.values.op._update_input(1, v.values)
    m.indices.op._update_input(1, v.indices)
    m.dense_shape.op._update_input(1, v.dense_shape)
    # pylint: enable=protected-access
  else:
    raise TypeError("Type %s not supported" % type(m))
  return v


def GetMaxSizeFromNestedMaximumIterations(value, while_ctxt):
  """Calculate a max_size for use by stack ops inside an XLA while_loop.

  Args:
    value: The value inside the while_loop forward context.  Used for printing
      error messages.
    while_ctxt: The forward context inside which value resides.  This does not
      always match the value's immediate context, as `value` may be inside e.g.
      a cond context inside the while_loop.

  Returns:
    A tensor containing the `max_size` to feed to a Stack initializer.

  Raises:
    ValueError: If `value` is nested inside a `while_loop` that either
      lacks a `maximum_iterations` parameter, or the `maximum_iterations`
      parameter:

        - is inside a `while_loop` that is a parent of the calling context, and
        - cannot be evaluated at graph build time to a constant.
  """
  value_name = value.name
  # curr_ctxt is the context that tf.gradients was called in.
  curr_ctxt = ops.get_default_graph()._get_control_flow_context()  # pylint: disable=protected-access

  curr_ctxt_name = curr_ctxt.name if curr_ctxt is not None else ""
  max_size = constant_op.constant(1)

  # Loop through all containing while contexts between value and the
  # current context, multiplying together each context's
  # max_iterations to get the maximum stack size.
  while while_ctxt not in (None, curr_ctxt):
    max_iter = while_ctxt.maximum_iterations
    if max_iter is None:
      raise ValueError(
          "Cannot create a gradient accumulator for tensor '%s' inside "
          "XLA while_loop because maximum_iterations was not passed to "
          "the tf.while_loop call ('%s')." % (value_name, while_ctxt.name))

    # pylint: disable=protected-access
    max_iter_ctxt = max_iter.op._get_control_flow_context()
    # pylint: enable=protected-access

    # If max_iter_ctxt (non-strictly) contains curr_ctxt, then it's OK to use.
    if util.IsContainingContext(curr_ctxt, max_iter_ctxt):
      max_size *= max_iter
    else:
      # We cannot use max_iter because it's defined in a nested while
      # or cond context, so will fail if we try to use it as input to
      # any ops in curr_ctxt (e.g. max_size or the final accumulator
      # stack). Attempt to get a constant value out to use instead.
      const_max_iter = tensor_util.constant_value(max_iter)
      if const_max_iter is None:
        raise ValueError(
            "Cannot create a gradient accumulator for tensor '%s' inside XLA "
            "while_loop. maximum_iterations tensor '%s' for while_loop context "
            "'%s' must be statically known (e.g. a constant value or known "
            "shape dimension), or be defined at or outside the while loop "
            "context '%s' (currently defined in '%s')." %
            (value_name, max_iter.name, while_ctxt.name, curr_ctxt_name,
             max_iter_ctxt.name))
      max_size *= const_max_iter

    # Find the next outer WhileContext (or stop if we reach the
    # tf.gradient's context).
    while_ctxt = util.GetContainingWhileContext(
        while_ctxt.outer_context, stop_ctxt=curr_ctxt)

  return max_size


class GradLoopState(object):
  """The state used for constructing the gradient graph for a while loop.

  We create a GradLoopState for each while loop in forward and its
  corresponding while loop in backprop. This gives us access to both
  the forward and the backprop WhileContexts.

  During the construction of gradient graph, any time when we detect
  a forward value that is needed for backprop, we create a history
  accumulator and add it to `history_map`. Any time when we backprop
  a loop switch op (in _SwitchGrad), we add the grad merge op in
  `switch_map`.
  """

  def __init__(self, forward_ctxt, outer_grad_state):
    # The grad loop state for the outer while loop.
    self._outer_grad_state = None

    # The while loop context for forward.
    self._forward_context = None

    # The loop counter added by AddForwardLoopCounter. It is the value
    # of the loop counter for the next iteration.
    self._forward_index = None

    # A sync op for forward.
    self._forward_sync = None

    # The while loop context for backprop.
    self._grad_context = None

    # The loop counter added by AddBackpropLoopCounter. It is the value
    # of the loop counter for the current iteration.
    self._grad_index = None

    # A sync op for backprop.
    self._grad_sync = None

    # Information needed by backprop.
    self._history_map = {}
    self._switch_map = {}
    self._unused_exits = []
    self._deferred_exits = []
    self._forward_loop_exits = list(forward_ctxt.loop_exits)
    self._pending_exits_count = len(forward_ctxt.loop_exits)

    self._outer_grad_state = outer_grad_state
    if outer_grad_state:
      outer_forward_ctxt = outer_grad_state.forward_context
    else:
      if not hasattr(forward_ctxt, "outer_context"):
        raise ValueError("Failed to call gradients on a while loop without"
                         "properly serializing graph via MetaGraphDef")
      outer_forward_ctxt = forward_ctxt.outer_context

    # Add the forward loop counter.
    with forward_ctxt._graph.as_default():  # pylint: disable=protected-access
      if outer_forward_ctxt:
        outer_forward_ctxt.Enter()
      cnt, forward_index = forward_ctxt.AddForwardLoopCounter(outer_grad_state)
      if outer_forward_ctxt:
        outer_forward_ctxt.Exit()
    self._forward_context = forward_ctxt
    self._forward_index = forward_index

    # Add the backprop WhileContext, and the backprop loop counter.
    if outer_grad_state:
      # This is a nested loop. Remember the iteration counts for each
      # execution of this inner loop.
      outer_forward_ctxt.AddName(cnt.name)
      history_cnt = outer_grad_state.AddForwardAccumulator(cnt)

      outer_grad_ctxt = outer_grad_state.grad_context
      outer_grad_ctxt.Enter()
      self._grad_context = WhileContext(
          maximum_iterations=forward_ctxt.maximum_iterations,
          parallel_iterations=forward_ctxt.parallel_iterations,
          back_prop=forward_ctxt.back_prop,
          swap_memory=forward_ctxt.swap_memory,
          name=forward_ctxt.name,
          grad_state=self)
      real_cnt = outer_grad_state.AddBackpropAccumulatedValue(history_cnt, cnt)
      self._grad_index = self._grad_context.AddBackpropLoopCounter(
          real_cnt, outer_grad_state)
      outer_grad_ctxt.Exit()
    else:
      if outer_forward_ctxt:
        outer_forward_ctxt.Enter()
      self._grad_context = WhileContext(
          maximum_iterations=forward_ctxt.maximum_iterations,
          parallel_iterations=forward_ctxt.parallel_iterations,
          back_prop=forward_ctxt.back_prop,
          swap_memory=forward_ctxt.swap_memory,
          name=forward_ctxt.name,
          grad_state=self)
      self._grad_index = self._grad_context.AddBackpropLoopCounter(
          cnt, outer_grad_state)
      if outer_forward_ctxt:
        outer_forward_ctxt.Exit()

  @property
  def outer_grad_state(self):
    """The grad loop state for outer loop."""
    return self._outer_grad_state

  @property
  def forward_context(self):
    """The while loop context for forward."""
    return self._forward_context

  @property
  def forward_index(self):
    """The loop index of forward loop."""
    return self._forward_index

  @property
  def forward_sync(self):
    """A control trigger node for synchronization in the forward loop.

    One main use is to keep the push ops of a stack executed in the
    iteration order.
    """
    if self._forward_sync is None:
      with ops.control_dependencies(None):
        self._forward_sync = control_trigger(name="f_sync")
      self._forward_sync._set_control_flow_context(self._forward_context)
      self._forward_index.op._add_control_input(self._forward_sync)
    return self._forward_sync

  @property
  def grad_context(self):
    """The corresponding WhileContext for gradient."""
    return self._grad_context

  @property
  def grad_index(self):
    """The loop index of backprop loop."""
    return self._grad_index

  @property
  def grad_sync(self):
    """A control trigger node for synchronization in the grad loop.

    One main use is to keep the pop ops of a stack executed in the
    iteration order.
    """
    if self._grad_sync is None:
      with ops.control_dependencies(None):
        self._grad_sync = control_trigger(name="b_sync")
      self._grad_sync._set_control_flow_context(self._grad_context)
      self._grad_index.op._add_control_input(self._grad_sync)
      if self._grad_context.outer_context:
        self._grad_context.outer_context.AddInnerOp(self._grad_sync)
    return self._grad_sync

  @property
  def history_map(self):
    """The map that records all the tensors needed for backprop."""
    return self._history_map

  @property
  def switch_map(self):
    """The map that records all the Switch ops for the while loop."""
    return self._switch_map

  @property
  def unused_exits(self):
    """The list of "unused" exits."""
    return self._unused_exits

  @property
  def deferred_exits(self):
    """The list of "deferred" exits."""
    return self._deferred_exits

  @property
  def forward_loop_exits(self):
    """The list of exits of the forward loop."""
    return self._forward_loop_exits

  @property
  def pending_exits_count(self):
    """The number of exits we expect to see but haven't."""
    return self._pending_exits_count

  @pending_exits_count.setter
  def pending_exits_count(self, cnt):
    """Set the pending count to cnt."""
    self._pending_exits_count = cnt

  def AddForwardAccumulator(self, value, dead_branch=False):
    """Add an accumulator for each forward tensor that is needed in backprop.

    This is added to the forward loop at the first time when a tensor
    in the forward loop is used by backprop gradient computation loop.
    We create an accumulator that accumulates the value of tensor at each
    iteration. Called in the control flow context where gradients() is called.

    The pseudocode is:
    ```
      acc = stack();
      while (_pivot) {
        acc = stack_push(acc, value);
      }
    ```

    We make sure that the stack push op in one iteration is executed before
    next iteration. This is achieved by adding a control edge from
    `forward_index.op.inputs[0].op` to the push op, and another control
    edge from the push op to either `forward_index.op` or `forward_sync`.

    Args:
      value: The source tensor in forward that is to be accumulated.
      dead_branch: True iff the tensor is on a dead branch of a cond.

    Returns:
      The stack that contains the accumulated history of the tensor.

    Raises:
      TypeError: For internal errors involving the value condition context.
      ValueError: If `value` is inside a XLA scope and a valid max size
        for the stack can't be found.
    """
    # curr_ctxt is the context that tf.gradients was called in.
    with self._forward_index.graph.as_default():
      curr_ctxt = ops.get_default_graph()._get_control_flow_context()  # pylint: disable=protected-access
      with ops.control_dependencies(None):
        if curr_ctxt:
          curr_ctxt.Enter()
        with ops.colocate_with(value):
          # We only need to pass maximum_iterations to the stack if
          # we're inside an XLA context.
          if not util.IsInXLAContext(value.op):
            max_size = constant_op.constant(-1, dtypes.int32)
          else:
            max_size = GetMaxSizeFromNestedMaximumIterations(
                value, self.forward_context)
          acc = gen_data_flow_ops.stack_v2(
              max_size=max_size, elem_type=value.dtype.base_dtype, name="f_acc")
        if curr_ctxt:
          curr_ctxt.Exit()

        # Make acc available in the forward context.
        enter_acc = self.forward_context.AddValue(acc)

        # Add the stack_push op in the context of value.op.
        swap_enabled = self.forward_context.swap_memory
        value_ctxt = util.GetOutputContext(value.op)
        if value_ctxt == self.forward_context:
          # value is not nested in the forward context.
          self.forward_context.Enter()
          push = gen_data_flow_ops.stack_push_v2(
              enter_acc, value, swap_memory=swap_enabled)
          self.forward_context.Exit()
          # Protect stack push and order it before forward_index.
          self.forward_index.op._add_control_input(push.op)
        else:
          # value is in a cond context within the forward context.
          if not isinstance(value_ctxt, CondContext):
            raise TypeError("value_ctxt is not a CondContext: %s" % value_ctxt)
          if dead_branch:
            # The special case for creating a zero tensor for a dead
            # branch of a switch. See ControlFlowState.ZerosLike().
            value_ctxt.outer_context.Enter()
            push = gen_data_flow_ops.stack_push_v2(
                enter_acc, value, swap_memory=swap_enabled)
            value_ctxt.outer_context.Exit()
            push.op._set_control_flow_context(value_ctxt)
          else:
            value_ctxt.Enter()
            push = gen_data_flow_ops.stack_push_v2(
                enter_acc, value, swap_memory=swap_enabled)
            value_ctxt.Exit()
          # Protect stack push and order it before forward_sync.
          self.forward_sync._add_control_input(push.op)
        # Order stack push after the successor of forward_index
        add_op = self.forward_index.op.inputs[0].op
        push.op._add_control_input(add_op)
        return acc

  def AddBackpropAccumulatedValue(self, history_value, value,
                                  dead_branch=False):
    """Add the getter for an accumulated value in the grad context.

    This is added to the backprop loop. Called in the grad context to
    get the value of an accumulated value. The stack pop op must be guarded
    by the pred of the controlling cond.

    Args:
      history_value: The history (a stack) of a value.
      value: The value that is pushed onto the stack.
      dead_branch: True iff the tensor is on a dead branch of a cond.

    Returns:
      The current value (the top of the stack).
    """
    history_ctxt = history_value.op._get_control_flow_context()
    # Find the cond context that controls history_value if any.
    cond_ctxt = None
    value_ctxt = value.op._get_control_flow_context()
    while value_ctxt and value_ctxt != history_ctxt:
      if isinstance(value_ctxt, CondContext):
        cond_ctxt = value_ctxt
        break
      value_ctxt = value_ctxt.outer_context
    with ops.control_dependencies(None):
      self.grad_context.Enter()
      if cond_ctxt:
        # Guard stack pop with a switch if it is controlled by a cond.
        grad_state = self
        pred = None
        while pred is None and grad_state:
          pred = grad_state.history_map.get(cond_ctxt.pred.name)
          grad_state = grad_state.outer_grad_state
        if pred is None:
          pred = cond_ctxt.pred
        branch = (1 - cond_ctxt.branch) if dead_branch else cond_ctxt.branch
        history_value = _SwitchRefOrTensor(history_value, pred)[branch]
      pop = gen_data_flow_ops.stack_pop_v2(history_value,
                                           value.dtype.base_dtype)
      pop.set_shape(value.get_shape())
      self.grad_context.Exit()
    parallel_iterations = self.grad_context.parallel_iterations
    if parallel_iterations > 1:
      # All pops are ordered after pivot_for_body and before grad_sync.
      self.grad_sync._add_control_input(pop.op)
    return pop

  def GetRealValue(self, value):
    """Get the real value of `value`.

    If backprop "uses" a value produced by forward inference, an accumulator
    is added in the forward loop to accumulate its values.  We use the
    accumulated value. This method must be called in the grad loop context.
    `value` must be in forward and needed for backprop.

    Args:
      value: A tensor to be captured.

    Returns:
      The same tensor obtained from the saved history.
    """
    assert value.op.type not in ["Variable", "VariableV2"]
    real_value = self._history_map.get(value.name)
    if real_value is None:
      cur_value = value
      cur_grad_state = self
      while True:
        enter_op = util.GetLoopConstantEnter(cur_value)
        if enter_op:
          # Special case: cur_value comes from a constant Enter node.
          cur_value = enter_op.inputs[0]
          cur_grad_state = cur_grad_state.outer_grad_state
          if cur_grad_state is None:
            # We are now outside all nested loops for this gradient(),
            # so `value` is a loop invariant and there is no need to
            # save the history of value. Just make cur_value to enter
            # the right control flow context.
            real_value = self._grad_context.AddValue(cur_value)
            break
        elif constant_op.is_constant(cur_value):
          # If the value to be forwarded is a constant, clone the constant in
          # the gradient loop rather than using a stack.
          # TODO(phawkins): consider hoisting the constant out of the loop
          # instead.
          real_value = constant_op.constant(
              tensor_util.constant_value(cur_value), dtype=cur_value.dtype)
          break
        else:
          # Record the history of this value in forward_ctxt.
          self._grad_context.Exit()
          history_value = cur_grad_state.AddForwardAccumulator(cur_value)
          self._grad_context.Enter()
          break

      if real_value is None:
        # Add the stack pop op in the grad context.
        real_value = cur_grad_state.AddBackpropAccumulatedValue(
            history_value, cur_value)
        if cur_grad_state != self:
          real_value = self._grad_context.AddValue(real_value)
      self._history_map[value.name] = real_value
    return real_value


def _GetWhileContext(op):
  """Get the WhileContext to which this op belongs."""
  ctxt = op._get_control_flow_context()
  if ctxt:
    ctxt = ctxt.GetWhileContext()
  return ctxt


class ControlFlowState(object):
  """Maintain the mapping from the loops to their grad states."""

  def __init__(self):
    self._map = {}  # maps forward loop context to GradLoopState

  def GetGradState(self, op, before):
    """Return the grad state for this op if it's in a forward loop context."""
    if before and util.IsLoopExit(op):
      forward_ctxt = op._get_control_flow_context()
      forward_ctxt = forward_ctxt.outer_context
      if forward_ctxt:
        forward_ctxt = forward_ctxt.GetWhileContext()
    else:
      forward_ctxt = _GetWhileContext(op)
    if forward_ctxt:
      return self._map.get(forward_ctxt)
    return None

  def ProcessUnusedLoopExits(self, pending_count, to_ops_set):
    """Process all the "unused" loop exits.

    The "unused" exits of the loops are added to `unused_exits`. An exit is
    unused if its pending_count is 0. If there is an exit with real gradient,
    all these deferred exits will enter the backprop loop with zero gradient.
    Otherwise, they will enter the backprop loop with None. As an example,
    people often write:

    ```python
    v1, _ = tf.while_loop(p, b, [x1, x2])
    result = gradients(v1, x1)
    ```

    The exit node for x2 is not included by the betweenness analysis. But we
    need to backprop x2 if x2 is involved in computing v1.

    Args:
      pending_count: The number of backprop inputs for every op.
      to_ops_set: The set of ops for ys in gradients(ys, xs)

    Returns:
      The set of unused loop exits that we know at this point we need
      to backprop.
    """
    loop_exits = []
    for grad_state in self._map.values():
      for y in grad_state.forward_loop_exits:
        if pending_count[y.op] == 0:
          grad_state.pending_exits_count -= 1
          if y.op not in to_ops_set:
            grad_state.unused_exits.append(y)
          if grad_state.pending_exits_count == 0:
            loop_exits.extend(grad_state.unused_exits)
      # Need to include Enters in backprop for higher-order gradients.
      for y in grad_state.forward_context.loop_enters:
        if pending_count[y.op] == 0:
          pending_count[y.op] = 1
    return loop_exits

  def EnterGradWhileContext(self, op, before):
    """Enter the WhileContext for gradient computation."""
    grad_state = self.GetGradState(op, before)
    if grad_state:
      grad_state.grad_context.Enter()

  def ExitGradWhileContext(self, op, before):
    """Exit the WhileContext for gradient computation."""
    grad_state = self.GetGradState(op, before)
    if grad_state:
      grad_state.grad_context.Exit()

  def AddWhileContext(self, op, between_op_list, between_ops):
    """Add the grad state for the while loop that op belongs to.

    Note that op is an Exit, and this method must be called in
    the control flow context where gradients() is called.

    Note that this method modifies `between_op_list` and `between_ops`.
    """
    forward_ctxt = _GetWhileContext(op)
    grad_state = self._map.get(forward_ctxt)
    if grad_state is None:
      # This is a new while loop so create a grad state for it.
      outer_forward_ctxt = forward_ctxt.outer_context
      if outer_forward_ctxt:
        outer_forward_ctxt = outer_forward_ctxt.GetWhileContext()
      outer_grad_state = None
      if outer_forward_ctxt:
        outer_grad_state = self._map.get(outer_forward_ctxt)
      grad_state = GradLoopState(forward_ctxt, outer_grad_state)
      self._map[forward_ctxt] = grad_state

      # We need to include all exits of a loop for backprop.
      for loop_exit in grad_state.forward_loop_exits:
        if loop_exit.op not in between_ops:
          between_ops.add(loop_exit.op)
          between_op_list.append(loop_exit.op)

  def ZerosLikeForExit(self, val):
    """Create zeros_like gradient for a loop exit.

    If the result of a loop variable is not used but is involved in
    computing the result of some needed loop variable, we create a
    zero-valued tensor that is fed as gradient for the Exit node of that
    loop variable. Note that val.op is an Exit, and this method must be
    called in the control flow context where gradients() is called.

    Args:
      val: The output tensor of an Exit op.

    Returns:
      A zero tensor of the same shape of val.
    """
    val_shape = val.get_shape()
    forward_ctxt = val.op._get_control_flow_context()
    outer_forward_ctxt = forward_ctxt.outer_context
    if outer_forward_ctxt:
      outer_forward_ctxt = outer_forward_ctxt.GetWhileContext()
    outer_grad_state = None
    if outer_forward_ctxt:
      outer_grad_state = self._map.get(outer_forward_ctxt)
    if outer_grad_state:
      # This is a nested loop.
      if val_shape.is_fully_defined():
        # If the shape is known statically, just create a zero tensor
        # with the right shape in the right context.
        outer_grad_state.grad_context.Enter()
        result = array_ops.zeros(val_shape.dims, val.dtype)
        outer_grad_state.grad_context.Exit()
      else:
        # Only the shape of value is needed for backprop.
        forward_ctxt.outer_context.Enter()
        shape = array_ops.shape_internal(val, optimize=False)
        forward_ctxt.outer_context.Exit()
        # Save the shape to a stack.
        history_shape = outer_grad_state.AddForwardAccumulator(shape)
        # Get the shape back from the stack.
        outer_grad_ctxt = outer_grad_state.grad_context
        outer_grad_ctxt.Enter()
        real_shape = outer_grad_state.AddBackpropAccumulatedValue(
            history_shape, shape)
        result = array_ops.zeros(real_shape, val.dtype)
        outer_grad_ctxt.Exit()
    else:
      # This is not a nested loop.
      if val_shape.is_fully_defined():
        # If the shape is known statically, just create a zero tensor
        # with the right shape.
        result = array_ops.zeros(val_shape.dims, val.dtype)
      else:
        result = array_ops.zeros_like(val, optimize=False)
    return result

  def ZerosLike(self, op, index):
    """Create zeros_like for the specified output of an op.

    If op is in a while loop that is part of gradients(), this method
    must be called in its grad loop context.

    Args:
      op: A tensorflow operation.
      index: the index for a specific output of the op.

    Returns:
      A zero tensor of the same shape of op.outputs[index].
    """
    if util.IsLoopSwitch(op):
      return None
    if op.graph._building_function:  # pylint: disable=protected-access
      # The optimization here is tricky to apply to functions
      return array_ops.zeros_like(op.outputs[index])
    dead_branch = util.IsSwitch(op)
    forward_ctxt = _GetWhileContext(op)
    grad_state = self._map.get(forward_ctxt)
    if grad_state is None:
      # op is not in a while loop that is part of gradients().
      return ZerosLikeOutsideLoop(op, index)
    op_ctxt = op._get_control_flow_context()
    val = ops.convert_to_tensor(op.outputs[index], name="tensor")
    shape = val.get_shape()
    if shape.is_fully_defined():
      # If the shape is known statically, just create a zero tensor with
      # the right shape in the grad loop context.
      result = constant_op.constant(0, shape=shape.dims, dtype=val.dtype)
      if dead_branch:
        # op is a cond switch. Guard the zero tensor with a switch.
        pred = grad_state.history_map.get(op_ctxt.pred.name)
        branch = op_ctxt.branch
        result = _SwitchRefOrTensor(result, pred)[1 - branch]
    else:
      # Unknown shape so keep a history of the shape at runtime.
      if dead_branch:
        # Need to add a special switch to guard the value.
        pred = op_ctxt.pred
        branch = op_ctxt.branch
        op_ctxt.outer_context.Enter()
        val = _SwitchRefOrTensor(op.inputs[0], pred)[1 - branch]
        zeros_shape = array_ops.shape_internal(val, optimize=False)
        op_ctxt.outer_context.Exit()
        val.op._set_control_flow_context(op_ctxt)
        zeros_shape.op._set_control_flow_context(op_ctxt)
      else:
        op_ctxt.Enter()
        zeros_shape = array_ops.shape_internal(val, optimize=False)
        op_ctxt.Exit()

      # Add forward accumulator for shape.
      grad_state.grad_context.Exit()
      history_zeros_shape = grad_state.AddForwardAccumulator(
          zeros_shape, dead_branch=dead_branch)
      grad_state.grad_context.Enter()

      # Create a zero tensor with the right shape.
      shape = grad_state.AddBackpropAccumulatedValue(history_zeros_shape,
                                                     zeros_shape, dead_branch)
      result = array_ops.zeros(shape, val.dtype)
    return result

  def PostProcessing(self):
    """Perform postprocessing at the end of gradients().

    We have created the gradient graph at this point. So this function
    can be used to perform any postprocessing on the gradient graph.
    We currently perform the following postprocessing:
      1. Patch the gradient graph if the output of a loop variable
         doesn't depend on its input.
    """
    for _, grad_state in self._map.items():
      for _, b_merge in grad_state.switch_map.items():
        if b_merge.op.inputs[0] == b_merge.op.inputs[1]:
          # The value of this loop variable at iteration i+1 doesn't
          # depend on its value at iteration i. So use zeros as the
          # gradients for all iterations > 0.
          dtype = b_merge.op.inputs[0].dtype
          shape = b_merge.op.inputs[0].get_shape()
          # pylint: disable=protected-access
          if shape.is_fully_defined():
            grad_state.grad_context.Enter()
            # Create a zeros and use it for iterations > 0.
            grad_val = constant_op.constant(0, dtype=dtype, shape=shape)
            next_grad_val = _NextIteration(grad_val)
            grad_state.grad_context.Exit()
          else:
            # Create a zeros in the outer grad context.
            outer_grad_ctxt = grad_state.grad_context.outer_context
            if outer_grad_ctxt:
              outer_grad_ctxt.Enter()
            enter_grad_op = b_merge.op.inputs[0].op
            enter_grad = enter_grad_op.inputs[0]
            grad_shape = array_ops.shape_internal(enter_grad, optimize=False)
            grad_val = array_ops.zeros(grad_shape)
            if outer_grad_ctxt:
              outer_grad_ctxt.Exit()
            # Use the zeros for iterations > 0.
            grad_state.grad_context.Enter()
            next_grad_val = _NextIteration(grad_val)
            grad_state.grad_context.Exit()
          b_merge.op._update_input(1, next_grad_val)
          # pylint: enable=protected-access


def MaybeCreateControlFlowState(between_op_list, between_ops,
                                colocate_gradients_with_ops):
  """Create the state for all the while loops involved in one gradients().

  We create a ControlFlowState when there are while loops involved in
  gradients(). In gradients(), control flow logic is only invoked when
  the ControlFlowState is not None.

  Note that this method modifies `between_op_list` and `between_ops`.
  """
  loop_state = None
  for op in between_op_list:
    if util.IsLoopExit(op):
      if loop_state is None:
        loop_state = ControlFlowState()
      if colocate_gradients_with_ops:
        with ops.colocate_with(op):
          loop_state.AddWhileContext(op, between_op_list, between_ops)
      else:
        loop_state.AddWhileContext(op, between_op_list, between_ops)
  return loop_state


def ZerosLikeOutsideLoop(op, index):
  """Create zeros_like for the specified output of an op."""
  val = op.outputs[index]
  if not util.IsSwitch(op):
    if val.dtype == dtypes.resource:
      return array_ops.zeros(gen_resource_variable_ops.variable_shape(val))
    return array_ops.zeros_like(val, optimize=False)
  else:
    op_ctxt = op._get_control_flow_context()
    if op_ctxt:
      # We are in a cond context. Use a switch to create zeros only when needed.
      pred = op_ctxt.pred
      branch = op_ctxt.branch
      switch_val = switch(op.inputs[0], pred)[1 - branch]
      # A op is created along the branch taken as control dependencies are on
      # the whole op and not on the tensor output.
      pivot = array_ops.identity(switch_val)
      if val.dtype == dtypes.resource:
        with ops.control_dependencies([pivot]):
          return array_ops.zeros(
              gen_resource_variable_ops.variable_shape(switch_val))
      zeros_shape = array_ops.shape_internal(switch_val, optimize=False)
      # Ensure ops created within array_ops.zeros are dominated by switch in
      # cond context.
      with ops.control_dependencies([pivot]):
        return array_ops.zeros(zeros_shape, dtype=val.dtype)
    else:
      return array_ops.zeros_like(val, optimize=False)


@six.add_metaclass(abc.ABCMeta)
class ControlFlowContext(object):
  """The base class for control flow context.

  The usage pattern is a sequence of (Enter, Exit) followed by a final
  ExitResult.

  We maintain the following state for control flow contexts during graph
  construction:
   1. graph has _control_flow_context: the current context used to
      construct new nodes. Changed by ctxt.Enter() and ctxt.Exit()
   2. op has _control_flow_context: the context to which the op belongs.
      Set at the time the op is created. Immutable.
   3. A ControlFlowContext has _outer_context: the context in which this
      context is created. Set at the time a context is created. Immutable.
   4. A ControlFlowContext has _context_stack.
      Pushed and popped by ctxt.Enter() and ctxt.Exit()
  """

  def __init__(self, values_def=None, import_scope=None):
    self._nested_contexts = []
    self._outer_context = ops.get_default_graph()._get_control_flow_context()
    if self._outer_context:
      self._outer_context._nested_contexts.append(self)  # pylint: disable=protected-access
    self._context_stack = []
    if values_def:
      self._init_values_from_proto(values_def, import_scope=import_scope)
    else:
      # The names of tensors that have been already seen in this context.
      self._values = set()
      # The keys are the names of tensors referenced by but external to this
      # context. Each value is the Tensor that should be used by this context to
      # access the key value (e.g. a switch output guarding a cond input value).
      self._external_values = {}

  def _init_values_from_proto(self, values_def, import_scope=None):
    """Initializes values and external_values from `ValuesDef` protocol buffer.

    Args:
      values_def: `ValuesDef` protocol buffer.
      import_scope: Optional `string`. Name scope to add.
    """
    assert isinstance(values_def, control_flow_pb2.ValuesDef)
    self._values = set(
        ops.prepend_name_scope(value, import_scope)
        for value in values_def.values)
    g = ops.get_default_graph()
    self._external_values = {}
    for k, v in values_def.external_values.items():
      k = ops.prepend_name_scope(k, import_scope)
      self._external_values[k] = g.as_graph_element(
          ops.prepend_name_scope(v, import_scope))
    op_names = set([
        op.split(":")[0]
        for op in self._values - set(self._external_values.keys())
    ])
    for op in op_names:
      # pylint: disable=protected-access
      g.as_graph_element(op)._set_control_flow_context(self)
      # pylint: enable=protected-access

  @property
  def name(self):
    return self._name

  @property
  def outer_context(self):
    """Return the context containing this context."""
    return self._outer_context

  @property
  def grad_state(self):
    raise NotImplementedError("Abstract method")

  @property
  def back_prop(self):
    raise NotImplementedError("Abstract method")

  @abc.abstractmethod
  def to_control_flow_context_def(self, context_def, export_scope=None):
    """Serializes this into `context_def`.

    Args:
      context_def: a `ControlFlowContextDef` protocol buffer.
      export_scope: Optional `string`. Name scope to remove.
    """
    raise NotImplementedError("Abstract method")

  def _to_values_def(self, export_scope=None):
    """Converts the values to a `ValuesDef` protocol buffer.

    Args:
      export_scope: Optional `string`. Name scope to remove.

    Returns:
      A `ValuesDef` protocol buffer.
    """
    values_def = control_flow_pb2.ValuesDef()
    values_def.values.extend(
        [ops.strip_name_scope(v, export_scope) for v in sorted(self._values)])
    for k, v in self._external_values.items():
      k = ops.strip_name_scope(k, export_scope)
      values_def.external_values[k] = ops.strip_name_scope(v.name, export_scope)
    return values_def

  def AddName(self, name):
    self._values.add(name)

  # pylint: disable=protected-access
  def Enter(self):
    """Enter this control flow context."""
    graph = ops.get_default_graph()
    self._context_stack.append(graph._get_control_flow_context())
    graph._set_control_flow_context(self)

  def Exit(self):
    """Exit this control flow context."""
    graph = ops.get_default_graph()
    last_context = self._context_stack.pop()
    graph._set_control_flow_context(last_context)

  def EnterGradientColocation(self, op, gradient_uid):
    """Start building a gradient colocated with an op."""
    if self._outer_context:
      self._outer_context.EnterGradientColocation(op, gradient_uid)

  def ExitGradientColocation(self, op, gradient_uid):
    """Start building a gradient colocated with an op."""
    if self._outer_context:
      self._outer_context.ExitGradientColocation(op, gradient_uid)

  def ExitResult(self, result):
    """Make a list of tensors available in the outer context."""
    if self._outer_context:
      nest.map_structure(lambda x: self._outer_context.AddName(x.name), result)

  def GetWhileContext(self):
    """Return the while context containing this context."""
    if self._outer_context:
      return self._outer_context.GetWhileContext()
    return None

  def _IsInOuterContext(self, op):
    op_ctxt = util.GetOutputContext(op)
    outer_ctxt = self.outer_context
    while outer_ctxt != op_ctxt:
      if outer_ctxt is None:
        return False
      outer_ctxt = outer_ctxt.outer_context
    return True

  def _RemoveExternalControlEdges(self, op):
    """Remove any external control dependency on this op."""
    while_ctxt = self.GetWhileContext()
    # A control input of `op` is internal if it is in the same while
    # loop context as the enclosing while loop context of self.
    if while_ctxt is None:
      internal_control_inputs = op.control_inputs
    else:
      internal_control_inputs = []
      for x in op.control_inputs:
        ctxt = util.GetOutputContext(x)
        if ctxt is not None and ctxt.GetWhileContext() == while_ctxt:
          internal_control_inputs.append(x)
    external_control_inputs = []
    if len(internal_control_inputs) != len(op.control_inputs):
      external_control_inputs = list(
          set(op.control_inputs) - set(internal_control_inputs))
      op._remove_all_control_inputs()
      op._add_control_inputs(internal_control_inputs)
    return internal_control_inputs, external_control_inputs

  # pylint: enable=protected-access

  def AddInnerOp(self, op):
    """Notifies a scope about an operator added to an inner scope."""
    if self._outer_context:
      self._outer_context.AddInnerOp(op)

  def GetControlPivot(self):
    """Returns the pivot node for this context, or None."""
    return None

  def IsWhileContext(self):
    return False

  def IsCondContext(self):
    return False

  def IsXLAContext(self):
    return False

  def __str__(self):
    return self.name


class CondContext(ControlFlowContext):
  """The context for the conditional construct."""

  def __init__(self,
               pred=None,
               pivot=None,
               branch=None,
               name="cond_text",
               context_def=None,
               import_scope=None):
    """Creates a `CondContext`.

    Args:
      pred: The `boolean` tensor for the conditional predicate.
      pivot: The predicate tensor in this branch.
      branch: 0 or 1 representing this branch.
      name: Name of the `CondContext` python object.
      context_def: Optional `ContextDef` protocol buffer to initialize the
        `CondContext` object from.
      import_scope: Optional `string`. Name scope to add. Only used when
        initialing from protocol buffer.
    """
    self._name = ops.get_default_graph().unique_name(name)

    if context_def:
      self._init_from_proto(context_def, import_scope=import_scope)
    else:
      # Initializes the default fields.
      ControlFlowContext.__init__(self)
      self._pred = pred  # The boolean tensor for the cond predicate
      self._pivot = pivot  # The predicate tensor in this branch
      self._branch = branch  # 0 or 1 representing this branch

      # Values considered to have been already seen in this context. pred is not
      # included in this context.
      self._values.add(pred.name)
      self._external_values[pred.name] = pred
      self._values.add(pivot.name)
      pivot.op._set_control_flow_context(self)  # pylint: disable=protected-access

  def _init_from_proto(self, context_def, import_scope=None):
    """Creates a new `CondContext` from protocol buffer.

    Args:
      context_def: `CondContextDef` protocol buffer.
      import_scope: Optional `string`. Name scope to add.
    """
    assert isinstance(context_def, control_flow_pb2.CondContextDef)
    # Create from context_def.
    g = ops.get_default_graph()
    self._name = ops.prepend_name_scope(context_def.context_name, import_scope)
    self._pred = g.as_graph_element(
        ops.prepend_name_scope(context_def.pred_name, import_scope))
    self._pivot = g.as_graph_element(
        ops.prepend_name_scope(context_def.pivot_name, import_scope))
    self._branch = context_def.branch
    super(CondContext, self).__init__(
        values_def=context_def.values_def, import_scope=import_scope)

  @property
  def pred(self):
    return self._pred

  @property
  def pivot(self):
    return self._pivot

  @property
  def branch(self):
    return self._branch

  @property
  def grad_state(self):
    if self.GetWhileContext():
      return self.GetWhileContext().grad_state
    return None

  @property
  def back_prop(self):
    if self.GetWhileContext():
      self.GetWhileContext().back_prop
    return False

  def GetControlPivot(self):
    return self._pivot

  def to_proto(self, export_scope=None):
    """Converts a `CondContext` to a `CondContextDef` protocol buffer.

    Args:
      export_scope: Optional `string`. Name scope to remove.

    Returns:
      A `CondContextDef` protocol buffer.
    """
    if (export_scope is None or self.name.startswith(export_scope)):
      context_def = control_flow_pb2.CondContextDef()
      context_def.context_name = ops.strip_name_scope(self.name, export_scope)
      context_def.pred_name = ops.strip_name_scope(self._pred.name,
                                                   export_scope)
      context_def.pivot_name = ops.strip_name_scope(self._pivot.name,
                                                    export_scope)
      context_def.branch = self._branch
      context_def.values_def.MergeFrom(
          super(CondContext, self)._to_values_def(export_scope))
      for nested in self._nested_contexts:
        nested_def = context_def.nested_contexts.add()
        nested.to_control_flow_context_def(nested_def)

      return context_def
    else:
      return None

  @staticmethod
  def from_proto(context_def, import_scope=None):
    """Returns a `CondContext` object created from `context_def`."""
    ret = CondContext(context_def=context_def, import_scope=import_scope)

    ret.Enter()
    for nested_def in context_def.nested_contexts:
      from_control_flow_context_def(nested_def, import_scope=import_scope)
    ret.Exit()
    return ret

  def to_control_flow_context_def(self, context_def, export_scope=None):
    context_def.cond_ctxt.CopyFrom(self.to_proto(export_scope=export_scope))

  def AddValue(self, val):
    """Add `val` to the current context and its outer context recursively."""
    if val.name in self._values:
      # Use the real value if it comes from outer context. This is needed in
      # particular for nested conds.
      result = self._external_values.get(val.name)
      result = val if result is None else result
    else:
      result = val
      self._values.add(val.name)
      if self._outer_context:
        result = self._outer_context.AddValue(val)
        self._values.add(result.name)
        self._external_values[result.name] = result
      with ops.control_dependencies(None):
        result = _SwitchRefOrTensor(result, self._pred)[self._branch]
        if self._outer_context:
          self._outer_context.AddInnerOp(result.op)

      result.op.graph.prevent_fetching(result.op)
      # pylint: disable=protected-access
      result.op._set_control_flow_context(self)
      # pylint: enable=protected-access

      # Mark Switch output as seen by this context and any outer contexts,
      # just like what we do for normal op outputs in _AddOpInternal() below.
      ctxt = self
      while ctxt is not None:
        # pylint: disable=protected-access
        ctxt._values.add(result.name)
        ctxt = ctxt._outer_context
        # pylint: enable=protected-access

      self._external_values[val.name] = result
    return result

  def AddOp(self, op):
    self._AddOpInternal(op)

  def _AddOpInternal(self, op):
    """Add `op` to the current context."""
    if not op.inputs:
      # If we're in a while loop, remove any control inputs from outside the
      # loop.
      self._RemoveExternalControlEdges(op)

      if not any(
          util.OpInContext(input_op, self) for input_op in op.control_inputs):
        # pylint: disable=protected-access
        op._add_control_input(self._pivot.op)
        # pylint: enable=protected-access
    else:
      # Make each input to 'op' available in this CondContext. If an input is
      # already part of this context there's nothing to do, but if it's
      # external, AddValue() will handle adding the appropriate Switch node and
      # other bookkeeping.
      for index in range(len(op.inputs)):
        x = op.inputs[index]
        if op.type == "Merge" and x.op.type == "NextIteration":
          # Edge case: if we're importing a while loop inside this CondContext,
          # AddValue() will not correctly handle the NextIteration inputs to
          # Merge node. The problem is that the NextIteration should also be
          # part of this context, but if we're importing it won't have been
          # processed and added to the context yet, so AddValue() will try to
          # add a Switch which results in an invalid graph. Instead, we use the
          # NextIteration input as-is here, and it will eventually be added to
          # the context via AddOp().
          real_x = x
        else:
          real_x = self.AddValue(x)
        if real_x != x:
          # pylint: disable=protected-access
          op._update_input(index, real_x)
          # pylint: enable=protected-access
      # Remove any external control dependency on this op.
      self._RemoveExternalControlEdges(op)
      # pylint: disable=protected-access
      if op.graph._is_function(op.type) or op.type == "SymbolicGradient":
        op._add_control_input(self._pivot.op)
      # pylint: enable=protected-access

    # Mark op's outputs as seen by this context and any outer contexts.
    output_names = [x.name for x in op.outputs]
    ctxt = self
    while ctxt is not None:
      # pylint: disable=protected-access
      ctxt._values.update(output_names)
      ctxt = ctxt._outer_context
      # pylint: enable=protected-access

    if self._outer_context or not util.IsLoopExit(op):
      op.graph.prevent_fetching(op)

    if self._outer_context:
      self._outer_context.AddInnerOp(op)

  def _ProcessOutputTensor(self, val):
    """Process an output tensor of a conditional branch."""
    real_val = val
    if val.name not in self._values:
      # Handle the special case of lambda: x
      self._values.add(val.name)
      if self._outer_context:
        real_val = self._outer_context.AddValue(val)
        self._values.add(real_val.name)
        self._external_values[real_val.name] = real_val
      real_val = _SwitchRefOrTensor(real_val, self._pred)[self._branch]
      self._external_values[val.name] = real_val
    else:
      external_val = self._external_values.get(val.name)
      if external_val is not None:
        real_val = external_val
    return real_val

  def _BuildCondTensor(self, v):
    if isinstance(v, ops.Operation):
      # Use pivot as the proxy for this op.
      return with_dependencies([v], self._pivot)
    elif isinstance(v, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
      values = self._ProcessOutputTensor(v.values)
      indices = self._ProcessOutputTensor(v.indices)
      if isinstance(v, ops.IndexedSlices):
        dense_shape = v.dense_shape
        if dense_shape is not None:
          dense_shape = self._ProcessOutputTensor(dense_shape)
        return ops.IndexedSlices(values, indices, dense_shape)
      else:
        dense_shape = self._ProcessOutputTensor(v.dense_shape)
        return sparse_tensor.SparseTensor(indices, values, dense_shape)
    else:
      v = nest.map_structure(_convert_tensorarray_to_flow, v)
      return self._ProcessOutputTensor(ops.convert_to_tensor(v))

  def BuildCondBranch(self, fn):
    """Add the subgraph defined by fn() to the graph."""
    pre_summaries = ops.get_collection(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
    original_result = fn()
    post_summaries = ops.get_collection(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
    if len(post_summaries) > len(pre_summaries):
      new_summaries = post_summaries[len(pre_summaries):]
      summary_ref = ops.get_collection_ref(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
      summary_ref[:] = pre_summaries
      with ops.control_dependencies(new_summaries):
        if original_result is None:
          return no_op(), None
        else:
          original_result = nest.map_structure(array_ops.identity,
                                               original_result)
    if original_result is None:
      return None, None

    result = nest.map_structure(self._BuildCondTensor, original_result)
    if not isinstance(result, (list, _basetuple)):
      result = [result]
    return original_result, result

  def IsCondContext(self):
    return True


def _UnpackIfSingleton(res):
  if isinstance(res, (list, _basetuple)) and len(res) == 1:
    return res[0]
  else:
    return res


# pylint: disable=redefined-outer-name
# pylint: disable=g-doc-args
@tf_export(v1=["cond"])
@deprecation.deprecated_args(
    None, "fn1/fn2 are deprecated in favor of the true_fn/false_fn arguments.",
    "fn1", "fn2")
def cond(pred,
         true_fn=None,
         false_fn=None,
         strict=False,
         name=None,
         fn1=None,
         fn2=None):
  """Return `true_fn()` if the predicate `pred` is true else `false_fn()`.

  `true_fn` and `false_fn` both return lists of output tensors. `true_fn` and
  `false_fn` must have the same non-zero number and type of outputs.

  **WARNING**: Any Tensors or Operations created outside of `true_fn` and
  `false_fn` will be executed regardless of which branch is selected at runtime.

  Although this behavior is consistent with the dataflow model of TensorFlow,
  it has frequently surprised users who expected a lazier semantics.
  Consider the following simple program:

  ```python
  z = tf.multiply(a, b)
  result = tf.cond(x < y, lambda: tf.add(x, z), lambda: tf.square(y))
  ```

  If `x < y`, the `tf.add` operation will be executed and `tf.square`
  operation will not be executed. Since `z` is needed for at least one
  branch of the `cond`, the `tf.multiply` operation is always executed,
  unconditionally.

  Note that `cond` calls `true_fn` and `false_fn` *exactly once* (inside the
  call to `cond`, and not at all during `Session.run()`). `cond`
  stitches together the graph fragments created during the `true_fn` and
  `false_fn` calls with some additional graph nodes to ensure that the right
  branch gets executed depending on the value of `pred`.

  `tf.cond` supports nested structures as implemented in
  `tensorflow.python.util.nest`. Both `true_fn` and `false_fn` must return the
  same (possibly nested) value structure of lists, tuples, and/or named tuples.
  Singleton lists and tuples form the only exceptions to this: when returned by
  `true_fn` and/or `false_fn`, they are implicitly unpacked to single values.
  This behavior is disabled by passing `strict=True`.

  Args:
    pred: A scalar determining whether to return the result of `true_fn` or
      `false_fn`.
    true_fn: The callable to be performed if pred is true.
    false_fn: The callable to be performed if pred is false.
    strict: A boolean that enables/disables 'strict' mode; see above.
    name: Optional name prefix for the returned tensors.

  Returns:
    Tensors returned by the call to either `true_fn` or `false_fn`. If the
    callables return a singleton list, the element is extracted from the list.

  Raises:
    TypeError: if `true_fn` or `false_fn` is not callable.
    ValueError: if `true_fn` and `false_fn` do not return the same number of
      tensors, or return tensors of different types.

  Example:

  ```python
  x = tf.constant(2)
  y = tf.constant(5)
  def f1(): return tf.multiply(x, 17)
  def f2(): return tf.add(y, 23)
  r = tf.cond(tf.less(x, y), f1, f2)
  # r is set to f1().
  # Operations in f2 (e.g., tf.add) are not executed.
  ```

  """
  if util.ENABLE_CONTROL_FLOW_V2 and not context.executing_eagerly():
    return cond_v2.cond_v2(pred, true_fn, false_fn, name)

  # We needed to make true_fn/false_fn keyword arguments for
  # backwards-compatibility. This check exists so that we can convert back to
  # having them be positional arguments.
  # TODO(josh11b): Make `true_fn` and `false_fn` positional arguments after
  # `fn1` and `fn2` are deleted.
  if fn1 is not None:
    if true_fn is not None:
      raise TypeError("cond(): true_fn and fn1 may not be set simultaneously.")
    true_fn = fn1
  elif true_fn is None:
    raise TypeError("cond(): true_fn argument required")
  if fn2 is not None:
    if false_fn is not None:
      raise TypeError("cond(): false_fn and fn2 may not be set simultaneously.")
    false_fn = fn2
  elif false_fn is None:
    raise TypeError("cond(): false_fn argument required")

  if not callable(true_fn):
    raise TypeError("true_fn must be callable.")
  if not callable(false_fn):
    raise TypeError("false_fn must be callable.")

  with ops.name_scope(name, "cond", [pred]):
    if context.executing_eagerly():
      if pred:
        return _UnpackIfSingleton(true_fn())
      return _UnpackIfSingleton(false_fn())

    # Add the Switch to the graph.
    if isinstance(pred, bool):
      raise TypeError("pred must not be a Python bool")
    p_2, p_1 = switch(pred, pred)
    pivot_1 = array_ops.identity(p_1, name="switch_t")
    pivot_2 = array_ops.identity(p_2, name="switch_f")
    pred = array_ops.identity(pred, name="pred_id")
    # Disable the fetching of tensors that are only on one branch of cond.
    for tensor in [p_1, p_2, pivot_1, pivot_2, pred]:
      tensor.op.graph.prevent_fetching(tensor.op)

    # Build the graph for the true branch in a new context.
    context_t = CondContext(pred, pivot_1, branch=1)
    try:
      context_t.Enter()
      orig_res_t, res_t = context_t.BuildCondBranch(true_fn)
      if orig_res_t is None:
        raise ValueError("true_fn must have a return value.")
      context_t.ExitResult(res_t)
    finally:
      context_t.Exit()

    # Build the graph for the false branch in a new context.
    context_f = CondContext(pred, pivot_2, branch=0)
    try:
      context_f.Enter()
      orig_res_f, res_f = context_f.BuildCondBranch(false_fn)
      if orig_res_f is None:
        raise ValueError("false_fn must have a return value.")
      context_f.ExitResult(res_f)
    finally:
      context_f.Exit()

    if not strict:
      orig_res_t = _UnpackIfSingleton(orig_res_t)
      orig_res_f = _UnpackIfSingleton(orig_res_f)

    # Check that the return values of the two branches have the same structure.
    try:
      nest.assert_same_structure(orig_res_t, orig_res_f)
    except TypeError as e:
      raise TypeError(
          "Incompatible return types of true_fn and false_fn: {}".format(e))
    except ValueError as e:
      raise ValueError(
          "Incompatible return values of true_fn and false_fn: {}".format(e))

    # Add the final merge to the graph.
    if not res_t:
      raise ValueError("true_fn and false_fn must return at least one result.")

    res_t_flat = nest.flatten(res_t)
    res_f_flat = nest.flatten(res_f)

    for x, y in zip(res_t_flat, res_f_flat):
      assert ((isinstance(x, ops.IndexedSlices) and
               isinstance(y, ops.IndexedSlices)) or
              (isinstance(x, sparse_tensor.SparseTensor) and
               isinstance(y, sparse_tensor.SparseTensor)) or
              (isinstance(x, ops.Tensor) and isinstance(y, ops.Tensor)))
      val_x = x if isinstance(x, ops.Tensor) else x.values
      val_y = y if isinstance(y, ops.Tensor) else y.values
      if val_x.dtype.base_dtype != val_y.dtype.base_dtype:
        raise ValueError(
            "Outputs of true_fn and false_fn must have the same type: %s, %s" %
            (val_x.dtype.name, val_y.dtype.name))

    merges = [merge(pair)[0] for pair in zip(res_f_flat, res_t_flat)]
    merges = _convert_flows_to_tensorarrays(nest.flatten(orig_res_t), merges)

    # Only add non-nested conds to the collection. Any nested control flow will
    # be encapsulated in the root context.
    assert context_t.outer_context == context_f.outer_context
    if context_t.outer_context is None:
      ops.add_to_collection(ops.GraphKeys.COND_CONTEXT, context_t)
      ops.add_to_collection(ops.GraphKeys.COND_CONTEXT, context_f)

    merges = nest.pack_sequence_as(structure=orig_res_t, flat_sequence=merges)

    # Singleton lists and tuples are automatically unpacked if strict == False.
    if not strict:
      merges = _UnpackIfSingleton(merges)
    return merges


# pylint: enable=g-doc-args
# pylint: enable=redefined-outer-name


@tf_export("cond", v1=[])
def cond_for_tf_v2(pred,
                   true_fn=None,
                   false_fn=None,
                   name=None):
  """Return `true_fn()` if the predicate `pred` is true else `false_fn()`.

  `true_fn` and `false_fn` both return lists of output tensors. `true_fn` and
  `false_fn` must have the same non-zero number and type of outputs.

  **WARNING**: Any Tensors or Operations created outside of `true_fn` and
  `false_fn` will be executed regardless of which branch is selected at runtime.

  Although this behavior is consistent with the dataflow model of TensorFlow,
  it has frequently surprised users who expected a lazier semantics.
  Consider the following simple program:

  ```python
  z = tf.multiply(a, b)
  result = tf.cond(x < y, lambda: tf.add(x, z), lambda: tf.square(y))
  ```

  If `x < y`, the `tf.add` operation will be executed and `tf.square`
  operation will not be executed. Since `z` is needed for at least one
  branch of the `cond`, the `tf.multiply` operation is always executed,
  unconditionally.

  Note that `cond` calls `true_fn` and `false_fn` *exactly once* (inside the
  call to `cond`, and not at all during `Session.run()`). `cond`
  stitches together the graph fragments created during the `true_fn` and
  `false_fn` calls with some additional graph nodes to ensure that the right
  branch gets executed depending on the value of `pred`.

  `tf.cond` supports nested structures as implemented in
  `tensorflow.python.util.nest`. Both `true_fn` and `false_fn` must return the
  same (possibly nested) value structure of lists, tuples, and/or named tuples.
  Singleton lists and tuples form the only exceptions to this: when returned by
  `true_fn` and/or `false_fn`, they are implicitly unpacked to single values.

  Args:
    pred: A scalar determining whether to return the result of `true_fn` or
      `false_fn`.
    true_fn: The callable to be performed if pred is true.
    false_fn: The callable to be performed if pred is false.
    name: Optional name prefix for the returned tensors.

  Returns:
    Tensors returned by the call to either `true_fn` or `false_fn`. If the
    callables return a singleton list, the element is extracted from the list.

  Raises:
    TypeError: if `true_fn` or `false_fn` is not callable.
    ValueError: if `true_fn` and `false_fn` do not return the same number of
      tensors, or return tensors of different types.

  Example:

  ```python
  x = tf.constant(2)
  y = tf.constant(5)
  def f1(): return tf.multiply(x, 17)
  def f2(): return tf.add(y, 23)
  r = tf.cond(tf.less(x, y), f1, f2)
  # r is set to f1().
  # Operations in f2 (e.g., tf.add) are not executed.
  ```

  """
  return cond(pred, true_fn=true_fn, false_fn=false_fn, strict=True, name=name)


def _resource_safe_shape(t):
  """Returns the shape of t or the variable it points to."""
  if t.dtype == dtypes.resource:
    while t.op.inputs:
      t = t.op.inputs[0]
    return tensor_shape.TensorShape(t.op.get_attr("shape"))
  return array_ops.shape_internal(t, optimize=False)


# TODO(yuanbyu): Consider having a unified notion of context for
# not only conditionals and loops but also control dependency and
# subgraphs.
class WhileContext(ControlFlowContext):
  """The context for the loop construct."""

  def __init__(self,
               maximum_iterations=None,
               parallel_iterations=10,
               back_prop=True,
               swap_memory=False,
               name="while_context",
               grad_state=None,
               context_def=None,
               import_scope=None):
    """"Creates a `WhileContext`.

    Args:
      maximum_iterations: Optional upper bound on number of loop iterations.
      parallel_iterations: The number of iterations allowed to run in parallel.
      back_prop: Whether backprop is enabled for this while loop.
      swap_memory: Whether GPU-CPU memory swap is enabled for this loop.
      name: Optional name prefix for the returned tensors.
      grad_state: The gradient loop state.
      context_def: Optional `WhileContextDef` protocol buffer to initialize the
        `Whilecontext` python object from.
      import_scope: Optional `string`. Name scope to add. Only used when
        initialing from protocol buffer.
    """
    if context_def:
      self._init_from_proto(context_def, import_scope=import_scope)
    else:
      ControlFlowContext.__init__(self)
      self._init_from_args(maximum_iterations, parallel_iterations, back_prop,
                           swap_memory, name)
    # The gradient loop state.
    self._grad_state = grad_state

  def _init_from_args(self, maximum_iterations, parallel_iterations, back_prop,
                      swap_memory, name):
    """Creates a new `WhileContext` from arguments.

    Args:
      maximum_iterations: Optional upper bound on number of loop iterations.
      parallel_iterations: The number of iterations allowed to run in parallel.
      back_prop: Whether backprop is enabled for this while loop.
      swap_memory: Whether GPU-CPU memory swap is enabled for this loop.
      name: Optional name prefix for the returned tensors.

    Raises:
      ValueError: If `parallel_iterations` has invalid value.
    """
    if not isinstance(parallel_iterations, int) or (parallel_iterations <= 0):
      raise ValueError("`parallel_iterations` must be a positive integer: "
                       "%s" % parallel_iterations)
    self._name = ops.get_default_graph().unique_name(name)
    self._maximum_iterations = maximum_iterations
    self._parallel_iterations = parallel_iterations
    self._back_prop = back_prop
    self._swap_memory = swap_memory
    # We use this node to control constants created by the pred lambda.
    self._pivot_for_pred = None
    # We use this node to control constants created by the body lambda.
    self._pivot_for_body = None
    # The boolean tensor for loop termination condition. Used in code
    # generation for gradient computation
    self._pivot = None
    # The list of exit tensors for loop variables.
    self._loop_exits = []
    # The list of enter tensors for loop variables.
    self._loop_enters = []
    self._graph = ops.get_default_graph()

  def _init_from_proto(self, context_def, import_scope=None):
    """Creates a new `WhileContext` from protocol buffer.

    Args:
      context_def: `WhileContextDef` protocol buffer.
      import_scope: Optional `string`. Name scope to add.
    """
    assert isinstance(context_def, control_flow_pb2.WhileContextDef)
    # Create from context_def.
    g = ops.get_default_graph()
    self._name = ops.prepend_name_scope(context_def.context_name, import_scope)
    if context_def.maximum_iterations_name:
      self._maximum_iterations = g.as_graph_element(
          ops.prepend_name_scope(context_def.maximum_iterations_name,
                                 import_scope))
    else:
      self._maximum_iterations = None
    self._parallel_iterations = context_def.parallel_iterations
    self._back_prop = context_def.back_prop
    self._swap_memory = context_def.swap_memory
    self._pivot_for_pred = g.as_graph_element(
        ops.prepend_name_scope(context_def.pivot_for_pred_name, import_scope))
    # We use this node to control constants created by the body lambda.
    self._pivot_for_body = g.as_graph_element(
        ops.prepend_name_scope(context_def.pivot_for_body_name, import_scope))
    # The boolean tensor for loop termination condition. Used in code
    # generation for gradient computation.
    self._pivot = g.as_graph_element(
        ops.prepend_name_scope(context_def.pivot_name, import_scope))
    # The list of exit tensors for loop variables.
    self._loop_exits = [
        g.as_graph_element(ops.prepend_name_scope(exit_name, import_scope))
        for exit_name in context_def.loop_exit_names
    ]
    # The list of enter tensors for loop variables.
    self._loop_enters = [
        g.as_graph_element(ops.prepend_name_scope(enter_name, import_scope))
        for enter_name in context_def.loop_enter_names
    ]
    super(WhileContext, self).__init__(
        values_def=context_def.values_def, import_scope=import_scope)

    # import_scope causes self.name to be different from the original serialized
    # context's name. Rewrite "frame_name" attrs with the new name.
    if import_scope:
      for tensor_name in self._values:
        op = g.as_graph_element(tensor_name).op
        if util.IsLoopEnter(op):
          # pylint: disable=protected-access
          op._set_attr("frame_name",
                       attr_value_pb2.AttrValue(s=compat.as_bytes(self.name)))
          # pylint: enable=protected-access
    self._graph = ops.get_default_graph()

  @property
  def maximum_iterations(self):
    """The maximum number of iterations that will be executed."""
    return self._maximum_iterations

  @property
  def parallel_iterations(self):
    """The number of iterations allowed to run in parallel."""
    return self._parallel_iterations

  @property
  def back_prop(self):
    """True iff backprop is enabled for this while loop."""
    return self._back_prop

  @property
  def swap_memory(self):
    """True iff GPU-CPU memory swap is enabled for this while loop."""
    return self._swap_memory

  @property
  def pivot(self):
    """The boolean tensor representing the loop termination condition."""
    return self._pivot

  @property
  def loop_enters(self):
    """The list of enter tensors for loop variables."""
    return self._loop_enters

  @property
  def loop_exits(self):
    """The list of exit tensors for loop variables."""
    return self._loop_exits

  @property
  def grad_state(self):
    """The gradient loop state."""
    return self._grad_state

  def to_proto(self, export_scope=None):
    """Converts a `WhileContext` to a `WhileContextDef` protocol buffer.

    Args:
      export_scope: Optional `string`. Name scope to remove.

    Returns:
      A `WhileContextDef` protocol buffer.
    """
    if (export_scope is None or self.name.startswith(export_scope)):
      context_def = control_flow_pb2.WhileContextDef()
      context_def.context_name = ops.strip_name_scope(self.name, export_scope)
      context_def.parallel_iterations = self._parallel_iterations
      if self._maximum_iterations is not None:
        context_def.maximum_iterations_name = ops.strip_name_scope(
            self._maximum_iterations.name, export_scope)
      context_def.back_prop = self._back_prop
      context_def.swap_memory = self._swap_memory
      context_def.pivot_for_pred_name = ops.strip_name_scope(
          self._pivot_for_pred.name, export_scope)
      context_def.pivot_for_body_name = ops.strip_name_scope(
          self._pivot_for_body.name, export_scope)
      context_def.pivot_name = ops.strip_name_scope(self._pivot.name,
                                                    export_scope)
      context_def.loop_exit_names.extend([
          ops.strip_name_scope(l.name, export_scope) for l in self._loop_exits
      ])
      context_def.loop_enter_names.extend([
          ops.strip_name_scope(l.name, export_scope) for l in self._loop_enters
      ])
      context_def.values_def.MergeFrom(
          super(WhileContext, self)._to_values_def(export_scope=export_scope))
      for nested in self._nested_contexts:
        nested_def = context_def.nested_contexts.add()
        nested.to_control_flow_context_def(nested_def)

      return context_def
    else:
      return None

  def to_control_flow_context_def(self, context_def, export_scope=None):
    context_def.while_ctxt.CopyFrom(self.to_proto(export_scope=export_scope))

  @staticmethod
  def from_proto(context_def, import_scope=None):
    """Returns a `WhileContext` object created from `context_def`.

    Args:
      context_def: A `WhileContextDef` protocol buffer.
      import_scope: Optional `string`. Name scope to add.

    Returns:
      A `WhileContext` Python object.
    """
    ret = WhileContext(context_def=context_def, import_scope=import_scope)
    ret.Enter()
    for nested_def in context_def.nested_contexts:
      from_control_flow_context_def(nested_def, import_scope=import_scope)
    ret.Exit()
    return ret

  def GetWhileContext(self):
    return self

  def GetControlPivot(self):
    if self._pivot_for_body is not None:
      return self._pivot_for_body
    return self._pivot_for_pred

  def AddValue(self, val):
    """Add `val` to the current context and its outer context recursively."""
    result = val
    new_value = val.name not in self._values
    # Don't treat ops in this context as new values. Usually all known values
    # are in self._values, except when we're importing a while loop inside this
    # WhileContext. Since there's a cycle in this case, `val` may be part of the
    # imported while loop but not yet processed by this context and added to
    # self._values in _AddOpInternal. We only want to process external input
    # tensors to the while loop here.
    new_value &= val.op._control_flow_context is not self  # pylint: disable=protected-access
    if new_value:
      self._values.add(val.name)

      # If we are in a grad context and val is from its forward context,
      # use GetRealValue(), which adds the logic to save the history of
      # val in forward.
      grad_ctxt = ops.get_default_graph()._get_control_flow_context()
      if grad_ctxt:
        grad_ctxt = grad_ctxt.GetWhileContext()
        if grad_ctxt.grad_state:
          forward_ctxt = _GetWhileContext(val.op)
          if util.IsLoopExit(val.op):
            forward_ctxt = forward_ctxt.outer_context
            if forward_ctxt:
              forward_ctxt = forward_ctxt.GetWhileContext()
          if forward_ctxt == grad_ctxt.grad_state.forward_context:
            real_val = grad_ctxt.grad_state.GetRealValue(val)
            self._external_values[val.name] = real_val
            return real_val

      if self._outer_context is not None:
        result = self._outer_context.AddValue(val)
      # Create an Enter to make `result` known to this loop context.
      with ops.control_dependencies(None):
        enter = _Enter(
            result,
            self._name,
            is_constant=True,
            parallel_iterations=self._parallel_iterations)
        enter.graph.prevent_feeding(enter)
        if self._outer_context:
          self._outer_context.AddInnerOp(enter.op)
      # Fix the control inputs and control flow context of these enter ops.
      self._FixControlInputsAndContext([enter])

      # Add `enter` in this context.
      self._values.add(enter.name)
      self._external_values[val.name] = enter
      result = enter
    else:
      actual_val = self._external_values.get(val.name)
      if actual_val is not None:
        result = actual_val
    return result

  def AddOp(self, op):
    """Add `op` to the current context."""
    # For a reduction op, if op is in a grad context and its input is from
    # its forward context, moving op to the forward context means we would
    # store the tensor after the reduction as opposed to the tensor before
    # reduction, and therefore could significantly reduce memory consumption.
    # For now, we do this only for a few ops.
    if op.type in {"Shape", "Size", "Rank"}:
      grad_ctxt = ops.get_default_graph()._get_control_flow_context()
      if grad_ctxt:
        grad_ctxt = grad_ctxt.GetWhileContext()
        if grad_ctxt.grad_state:
          op_input_forward_ctxt = _GetWhileContext(op.inputs[0].op)
          if op_input_forward_ctxt == grad_ctxt.grad_state.forward_context:
            op_input_ctxt = op.inputs[0].op._get_control_flow_context()
            op._set_control_flow_context(op_input_ctxt)
            op_input_ctxt._AddOpInternal(op)
            return
    self._AddOpInternal(op)

  def _AddOpInternal(self, op):
    """Add `op` to the current context.

    We move any external control dependencies of the op to the loop pivot, to
    ensure they get executed.
    """
    if not op.inputs:
      # Remove any external control dependency on this op
      control_inputs, external_inputs = self._RemoveExternalControlEdges(op)
      # Add a control edge from the control pivot to this op.
      if not control_inputs:
        # pylint: disable=protected-access
        op._add_control_input(self.GetControlPivot().op)
        # pylint: enable=protected-access
      for x in op.outputs:
        self._values.add(x.name)
    else:
      for index in range(len(op.inputs)):
        x = op.inputs[index]
        real_x = self.AddValue(x)
        if real_x != x:
          op._update_input(index, real_x)  # pylint: disable=protected-access
      # Remove any external control dependency on this op.
      _, external_inputs = self._RemoveExternalControlEdges(op)
      # Add a control dependency to prevent loop invariants from
      # enabling ops that should not be executed.
      self._MaybeAddControlDependency(op)
      for x in op.outputs:
        self._values.add(x.name)
    if external_inputs:
      # Use an identity to pull control inputs as data inputs. Note that we
      # ignore ops which don't have outputs. TODO(apassos): fix that
      with ops.control_dependencies(None):
        self.Enter()
        external_inputs = [
            array_ops.identity(x.outputs[0]).op
            for x in external_inputs
            if x.outputs
        ]
        self.Exit()
      op._add_control_inputs(external_inputs)  # pylint: disable=protected-access
    if self._outer_context or not util.IsLoopExit(op):
      op.graph.prevent_fetching(op)
      for x in op.outputs:
        op.graph.prevent_feeding(x)

    if self._outer_context:
      self._outer_context.AddInnerOp(op)

  def _MaybeAddControlDependency(self, op):
    """Add a control input to the op if it only depends on loop invariants."""

    def _IsOpFree(op):
      """Determines if `op` needs a control dependency."""
      if op.control_inputs:
        return False
      # pylint: disable=protected-access
      if op.graph._is_function(op.type) or op.type == "SymbolicGradient":
        return True
      # pylint: enable=protected-access
      for x in op.inputs:
        if not util.IsLoopConstantEnter(x.op):
          return False
      return True

    if _IsOpFree(op):
      # pylint: disable=protected-access
      op._add_control_input(self.GetControlPivot().op)
      # pylint: enable=protected-access

  def AddForwardLoopCounter(self, outer_grad_state):
    """Adds a loop that counts the number of iterations.

    This is added to the forward loop at the time when we start to
    create the loop for backprop gradient computation. Called in
    the outer context of this forward context.

    The pseudocode is:
      `n = 0; while (_pivot) { n++; }`

    Note that a control dependency is added to `n` to ensure the correct
    execution order of stack push ops.

    Args:
      outer_grad_state: The outer grad state. None if not nested.

    Returns:
      The number of iterations taken by the forward loop and the loop index.
    """
    n = constant_op.constant(0, name="f_count")
    if outer_grad_state is not None:
      # Force the stack pushes of i-th execution of an inner loop to be ordered
      # before the pushes of (i+1)-th execution of the same inner loop.
      outer_add_op = outer_grad_state.forward_index.op.inputs[0].op
      n.op._add_control_input(outer_add_op)  # pylint: disable=protected-access

    self.Enter()
    self.AddName(n.name)
    enter_n = _Enter(
        n,
        self._name,
        is_constant=False,
        parallel_iterations=self._parallel_iterations,
        name="f_count")
    self.loop_enters.append(enter_n)

    merge_n = merge([enter_n, enter_n])[0]
    switch_n = switch(merge_n, self._pivot)

    index = math_ops.add(switch_n[1], 1)
    next_n = _NextIteration(index)
    merge_n.op._update_input(1, next_n)

    total_iterations = exit(switch_n[0], name="f_count")
    self.loop_exits.append(total_iterations)
    self.ExitResult([total_iterations])
    self.Exit()
    return total_iterations, next_n

  def AddBackpropLoopCounter(self, count, outer_grad_state):
    """Add the backprop loop that controls the iterations.

    This is added to the backprop loop. It is used to control the loop
    termination of the backprop loop. Called in the outer context of
    this grad context.

    The pseudocode is:
      `n = count; while (n >= 1) { n--; }`

    Note that a control dependency is added to `final_zero` to ensure the
    correct execution order of stack pop ops.

    Args:
      count: The number of iterations for backprop.
      outer_grad_state: The outer grad state. None if not nested.

    Returns:
      The loop index.
    """
    in_separate_functions = count.graph is not ops.get_default_graph()
    if in_separate_functions:
      # Brings the count into this graph
      count = array_ops.identity(count)
    else:
      # TODO(apassos) XLA expects this constant to be created outside the loop,
      # so doing that for now.
      one = constant_op.constant(1, name="b_count")

    self.Enter()
    self.AddName(count.name)
    enter_count = _Enter(
        count,
        self._name,
        is_constant=False,
        parallel_iterations=self._parallel_iterations,
        name="b_count")
    self.loop_enters.append(enter_count)

    merge_count = merge([enter_count, enter_count])[0]
    self._pivot_for_pred = merge_count

    if in_separate_functions:
      one = constant_op.constant(1, name="b_count")
    pred = math_ops.greater_equal(merge_count, one)
    self._pivot = loop_cond(pred, name="b_count")
    switch_count = switch(merge_count, self._pivot)

    index = math_ops.subtract(switch_count[1], one)
    self._pivot_for_body = index
    next_count = _NextIteration(index)
    merge_count.op._update_input(1, next_count)

    final_zero = exit(switch_count[0], name="b_count")
    self.loop_exits.append(final_zero)
    if outer_grad_state is not None:
      # Force the stack pops of i-th execution of an inner loop to be ordered
      # before the pops of (i+1)-th execution of the same inner loop.
      # pylint: disable=protected-access
      outer_grad_state.grad_sync._add_control_input(final_zero.op)
      # pylint: enable=protected-access

    self.ExitResult([final_zero])
    self.Exit()
    return next_count

  def AddBackpropAccumulator(self, op, grad):
    """Add an accumulation loop for every loop invariant.

    This is added to the backprop loop. It is used to accumulate partial
    gradients within each loop iteration. Called when in the gradient while
    context.

    The pseudocode is:
      ```
      acc = 0.0;
      while (_pivot) {
        acc += grad;
      }
      ```

    Args:
      op: The Enter op for a loop invariant.
      grad: The partial gradient of an iteration for a loop invariant.

    Returns:
      The gradient for a loop invariant.
    """
    self.Exit()
    # Create a zeros tensor with the right shape for acc. If we don't
    # know the full shape statically, we will have to get the shape
    # dynamically from the forward inference. Getting the shape right
    # for the zeros is only needed for the base case when the loop exits
    # without running any iterations.
    shape = grad.get_shape()
    if shape.is_fully_defined():
      if self.outer_context:
        self.outer_context.Enter()
      acc = constant_op.constant(0, grad.dtype, shape=shape, name="b_acc")
      if self.outer_context:
        self.outer_context.Exit()
    else:
      value = op.inputs[0]
      if (isinstance(self.outer_context, WhileContext) and
          self.outer_context.grad_state is not None):
        # We are in a nested while loop.
        forward_ctxt = self.grad_state.forward_context
        forward_ctxt.outer_context.Enter()
        zeros_shape = array_ops.shape_internal(value, optimize=False)
        forward_ctxt.outer_context.Exit()
        outer_grad_state = self.grad_state.outer_grad_state
        history_zeros_shape = outer_grad_state.AddForwardAccumulator(
            zeros_shape)
        self.outer_context.Enter()
        real_shape = outer_grad_state.AddBackpropAccumulatedValue(
            history_zeros_shape, zeros_shape)
        acc = array_ops.zeros(real_shape, grad.dtype)
        self.outer_context.Exit()
      else:
        if self.outer_context:
          self.outer_context.Enter()
        zeros_shape = array_ops.shape_internal(value, optimize=False)
        acc = array_ops.zeros(zeros_shape, grad.dtype)
        if self.outer_context:
          self.outer_context.Exit()

    self.Enter()
    self.AddName(acc.name)
    enter_acc = _Enter(
        acc,
        self._name,
        is_constant=False,
        parallel_iterations=self._parallel_iterations,
        name="b_acc")
    self.loop_enters.append(enter_acc)

    merge_acc = merge([enter_acc, enter_acc], name="b_acc")[0]
    switch_acc_false, switch_acc_true = switch(merge_acc, self._pivot)

    add_acc = math_ops.add(switch_acc_true, grad)
    next_acc = _NextIteration(add_acc)
    merge_acc.op._update_input(1, next_acc)  # pylint: disable=protected-access

    result_acc = exit(switch_acc_false, name="b_acc")
    self.loop_exits.append(result_acc)
    self.ExitResult([result_acc])
    return result_acc

  def AddBackpropIndexedSlicesAccumulator(self, op, grad):
    """This is used for accumulating gradients that are IndexedSlices.

    This is essentially the equivalent of AddBackpropAccumulator but optimized
    for things like updating embeddings from within a while loop.

    Args:
      op: The Enter op for a loop invariant.
      grad: The partial gradients represented as an IndexedSlices.

    Returns:
      The accumulated IndexedSlices gradient of the loop invariant.
    """
    values = grad.values
    indices = grad.indices
    dense_shape = grad.dense_shape

    self.Exit()
    if self.outer_context:
      self.outer_context.Enter()
    if values.get_shape().is_fully_defined():
      values_shape = tensor_shape.TensorShape([tensor_shape.Dimension(1)] +
                                              values.get_shape().dims[1:])
      if self.outer_context:
        self.outer_context.Enter()
      values_acc = constant_op.constant(
          0, values.dtype, shape=values_shape, name="b_acc")
      if self.outer_context:
        self.outer_context.Exit()
    else:
      values_shape = _resource_safe_shape(op.inputs[0])[1:]
      values_shape = array_ops.concat([[1], values_shape], 0)
      values_acc = array_ops.zeros(values_shape, dtype=values.dtype)
    indices_acc = constant_op.constant([0], indices.dtype)
    shape_acc = None
    if dense_shape is not None:
      if dense_shape.get_shape().is_fully_defined():
        if self.outer_context:
          self.outer_context.Enter()
        shape_acc = constant_op.constant(
            0, dense_shape.dtype, shape=dense_shape.get_shape())
        if self.outer_context:
          self.outer_context.Exit()
      else:
        shape_acc = array_ops.zeros_like(
            array_ops.shape_internal(
                op.inputs[0], optimize=False, out_type=dense_shape.dtype),
            optimize=False)

    if self.outer_context:
      self.outer_context.Exit()

    self.Enter()
    self.AddName(values_acc.name)
    self.AddName(indices_acc.name)
    init_acc = [indices_acc, values_acc]
    if shape_acc is not None:
      self.AddName(shape_acc.name)
      init_acc.append(shape_acc)

    # Set use_input_shape=False since the accumulator tensors will grow in
    # size. If use_input_shape=True, the _update_input call below will result in
    # incompatible shapes.
    enter_acc = [
        _Enter(
            x,
            self._name,
            is_constant=False,
            parallel_iterations=self._parallel_iterations,
            use_input_shape=False,
            name="b_acc") for x in init_acc
    ]
    # Manually set appropriate partial shapes.
    enter_acc[0].set_shape([None])
    if values_acc.shape.dims is not None:
      enter_acc[1].set_shape([None] + values_acc.shape.as_list()[1:])
    self.loop_enters.extend(enter_acc)

    merge_acc = [merge([x, x], name="b_acc")[0] for x in enter_acc]
    switch_acc = [switch(x, self._pivot) for x in merge_acc]

    # The actual accumulation.
    acc_indexed_slices = [
        array_ops.concat([xa[1], xv], 0)
        for xa, xv in zip(switch_acc[:2], [indices, values])
    ]
    if shape_acc is not None:
      # For the shape we just keep the maximum
      acc_indexed_slices.append(math_ops.maximum(dense_shape, switch_acc[2][1]))

    next_acc = [_NextIteration(x) for x in acc_indexed_slices]
    for xm, xn in zip(merge_acc, next_acc):
      xm.op._update_input(1, xn)  # pylint: disable=protected-access

    exit_acc = [exit(x[0], name="b_acc") for x in switch_acc]
    self.loop_exits.extend(exit_acc)

    self.ExitResult(exit_acc)
    return ops.IndexedSlices(
        indices=exit_acc[0],
        values=exit_acc[1],
        dense_shape=exit_acc[2] if shape_acc is not None else None)

  def _InitializeValues(self, values):
    """Makes the values known to this context."""
    self._values = set()
    for x in values:
      if isinstance(x, ops.Tensor):
        self._values.add(x.name)
      else:
        self._values.add(x.values.name)
        self._values.add(x.indices.name)
        if isinstance(x, ops.IndexedSlices):
          dense_shape = x.dense_shape
        elif isinstance(x, sparse_tensor.SparseTensor):
          dense_shape = x.dense_shape
        else:
          raise TypeError("Type %s not supported" % type(x))
        if dense_shape is not None:
          self._values.add(dense_shape.name)

  def _BuildLoop(self, pred, body, original_loop_vars, loop_vars,
                 shape_invariants):
    """Core: Add the loop termination condition and body to the graph."""
    flat_loop_vars = nest.flatten(original_loop_vars)

    # Let the context know the loop variables so the loop variables
    # would be added in the outer contexts properly.
    self._InitializeValues(loop_vars)
    real_vars = loop_vars
    if self._outer_context:
      real_vars = [self._outer_context.AddValue(x) for x in loop_vars]
    with ops.control_dependencies(None):
      enter_vars = [
          _Enter(
              x,
              self._name,
              is_constant=False,
              parallel_iterations=self._parallel_iterations,
              use_input_shape=(shape_invariants is None)) for x in real_vars
      ]
      for x in enter_vars:
        x.graph.prevent_feeding(x)
        if self._outer_context:
          self._outer_context.AddInnerOp(x.op)

    # Finds the closest enclosing non-None control pivot.
    outer_context = self._outer_context
    control_pivot = None
    while outer_context is not None and control_pivot is None:
      control_pivot = outer_context.GetControlPivot()
      # pylint: disable=protected-access
      outer_context = outer_context._outer_context
      # pylint: enable=protected-access

    if control_pivot is not None:
      for var in enter_vars:
        if util.IsLoopConstantEnter(var.op.inputs[0].op):
          # pylint: disable=protected-access
          var.op._add_control_input(control_pivot.op)
          # pylint: enable=protected-access
    _SetShapeInvariants(real_vars, enter_vars, shape_invariants)

    # Fix the control inputs and control flow context of these enter ops.
    self._FixControlInputsAndContext(enter_vars)
    self._InitializeValues(enter_vars)
    self._loop_enters = enter_vars

    merge_vars = [merge([x, x])[0] for x in enter_vars]
    self._pivot_for_pred = merge_vars[0]

    # Build the graph for pred.
    merge_vars_with_tensor_arrays = (
        _convert_flows_to_tensorarrays(flat_loop_vars, merge_vars))
    packed_vars = nest.pack_sequence_as(
        structure=original_loop_vars,
        flat_sequence=merge_vars_with_tensor_arrays)
    c = ops.convert_to_tensor(pred(*packed_vars))
    self._pivot = loop_cond(c, name="LoopCond")
    switch_vars = [_SwitchRefOrTensor(x, self._pivot) for x in merge_vars]

    # Build the graph for body.
    vars_for_body = [_Identity(x[1]) for x in switch_vars]
    self._pivot_for_body = vars_for_body[0]
    # Convert TensorArray flow variables inside the context back into
    # their associated TensorArrays for calling the body.
    vars_for_body_with_tensor_arrays = (
        _convert_flows_to_tensorarrays(flat_loop_vars, vars_for_body))
    packed_vars_for_body = nest.pack_sequence_as(
        structure=original_loop_vars,
        flat_sequence=vars_for_body_with_tensor_arrays)
    pre_summaries = ops.get_collection(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
    body_result = body(*packed_vars_for_body)
    post_summaries = ops.get_collection(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
    if not nest.is_sequence(body_result):
      body_result = [body_result]
    if len(post_summaries) > len(pre_summaries):
      new_summaries = post_summaries[len(pre_summaries):]
      summary_ref = ops.get_collection_ref(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access
      summary_ref[:] = pre_summaries
      with ops.control_dependencies(new_summaries):

        def map_fn(x):
          # TODO(apassos) figure out how to trigger with tensor arrays as well
          if isinstance(x, tensor_array_ops.TensorArray):
            return x
          return array_ops.identity(x)

        body_result = nest.map_structure(map_fn, body_result)

    # Compare the structure types of input and output of body.
    # For backwards compatibility, the first layer is forced to a list
    # during this comparison, because inputs are typically lists and
    # outputs of the body are typically tuples.
    nest.assert_same_structure(list(packed_vars_for_body), list(body_result))

    # Store body_result to keep track of TensorArrays returned by body
    original_body_result = body_result
    # Convert TensorArrays returned by body into their flow variables
    result = nest.map_structure(_convert_tensorarray_to_flow,
                                nest.flatten(body_result))
    result = ops.convert_n_to_tensor_or_indexed_slices(result)

    # Add NextIteration and the back edges to complete the loop.
    if len(merge_vars) != len(result):
      raise ValueError("Number of inputs and outputs of body must match "
                       "loop_vars: %d, %d" % (len(merge_vars), len(result)))
    next_vars = []
    for m, v in zip(merge_vars, result):
      next_vars.append(_AddNextAndBackEdge(m, v))

    # Add the exit ops.
    exit_vars = [exit(x[0]) for x in switch_vars]
    self._loop_exits = exit_vars

    # Exit the loop.
    self.ExitResult(exit_vars)

    return original_body_result, exit_vars

  def BuildLoop(self, pred, body, loop_vars, shape_invariants,
                return_same_structure):
    """Add the loop termination condition and body to the graph."""

    # Keep original_loop_vars to identify which are TensorArrays
    original_loop_vars = loop_vars
    # Convert TensorArrays to their flow variables
    loop_vars = nest.map_structure(_convert_tensorarray_to_flow,
                                   nest.flatten(loop_vars))
    loop_vars = ops.convert_n_to_tensor_or_indexed_slices(loop_vars)
    try:
      self.Enter()
      # _BuildLoop calls _update_input in several places. _mutation_lock()
      # ensures a Session.run call cannot occur between creating and mutating
      # new ops.
      with ops.get_default_graph()._mutation_lock():  # pylint: disable=protected-access
        original_body_result, exit_vars = self._BuildLoop(
            pred, body, original_loop_vars, loop_vars, shape_invariants)
    finally:
      self.Exit()

    flat_result = nest.flatten(original_body_result)
    # Convert TensorArray flow variables outside the context back into
    # their associated TensorArrays for returning to caller.
    exit_vars_with_tensor_arrays = (
        _convert_flows_to_tensorarrays(flat_result, exit_vars))
    packed_exit_vars = nest.pack_sequence_as(
        structure=original_body_result,
        flat_sequence=exit_vars_with_tensor_arrays)

    if return_same_structure:
      return packed_exit_vars
    else:
      return packed_exit_vars[0] if len(exit_vars) == 1 else packed_exit_vars

  def _FixControlInputsAndContext(self, enters):
    graph = ops.get_default_graph()
    # pylint: disable=protected-access
    for e in enters:
      if isinstance(e, ops.Tensor):
        xs = [e]
      else:
        if not isinstance(e, (ops.IndexedSlices, sparse_tensor.SparseTensor)):
          raise TypeError("Type %s not supported" % type(e))
        xs = [e.values, e.indices]
        shape = e.dense_shape
        if shape is not None:
          xs.append(shape)
      for x in xs:
        inp_op = x.op.inputs[0].op
        control_inputs = graph._control_dependencies_for_inputs([inp_op])
        outer_control_inputs = [
            op for op in control_inputs if self._IsInOuterContext(op)
        ]
        x.op._set_control_flow_context(self)
        x.op._add_control_inputs(outer_control_inputs)
        graph._record_op_seen_by_control_dependencies(x.op)
    # pylint: enable=protected-access

  def IsWhileContext(self):
    return True


# pylint: disable=redefined-outer-name
@tf_export("while_loop", v1=[])
def while_loop_v2(cond,
                  body,
                  loop_vars,
                  shape_invariants=None,
                  parallel_iterations=10,
                  back_prop=True,
                  swap_memory=False,
                  maximum_iterations=None,
                  name=None):
  """Repeat `body` while the condition `cond` is true.

  `cond` is a callable returning a boolean scalar tensor. `body` is a callable
  returning a (possibly nested) tuple, namedtuple or list of tensors of the same
  arity (length and structure) and types as `loop_vars`. `loop_vars` is a
  (possibly nested) tuple, namedtuple or list of tensors that is passed to both
  `cond` and `body`. `cond` and `body` both take as many arguments as there are
  `loop_vars`.

  In addition to regular Tensors or IndexedSlices, the body may accept and
  return TensorArray objects.  The flows of the TensorArray objects will
  be appropriately forwarded between loops and during gradient calculations.

  Note that `while_loop` calls `cond` and `body` *exactly once* (inside the
  call to `while_loop`, and not at all during `Session.run()`). `while_loop`
  stitches together the graph fragments created during the `cond` and `body`
  calls with some additional graph nodes to create the graph flow that
  repeats `body` until `cond` returns false.

  For correctness, `tf.while_loop()` strictly enforces shape invariants for
  the loop variables. A shape invariant is a (possibly partial) shape that
  is unchanged across the iterations of the loop. An error will be raised
  if the shape of a loop variable after an iteration is determined to be more
  general than or incompatible with its shape invariant. For example, a shape
  of [11, None] is more general than a shape of [11, 17], and [11, 21] is not
  compatible with [11, 17]. By default (if the argument `shape_invariants` is
  not specified), it is assumed that the initial shape of each tensor in
  `loop_vars` is the same in every iteration. The `shape_invariants` argument
  allows the caller to specify a less specific shape invariant for each loop
  variable, which is needed if the shape varies between iterations. The
  `tf.Tensor.set_shape`
  function may also be used in the `body` function to indicate that
  the output loop variable has a particular shape. The shape invariant for
  SparseTensor and IndexedSlices are treated specially as follows:

  a) If a loop variable is a SparseTensor, the shape invariant must be
  TensorShape([r]) where r is the rank of the dense tensor represented
  by the sparse tensor. It means the shapes of the three tensors of the
  SparseTensor are ([None], [None, r], [r]). NOTE: The shape invariant here
  is the shape of the SparseTensor.dense_shape property. It must be the shape of
  a vector.

  b) If a loop variable is an IndexedSlices, the shape invariant must be
  a shape invariant of the values tensor of the IndexedSlices. It means
  the shapes of the three tensors of the IndexedSlices are (shape, [shape[0]],
  [shape.ndims]).

  `while_loop` implements non-strict semantics, enabling multiple iterations
  to run in parallel. The maximum number of parallel iterations can be
  controlled by `parallel_iterations`, which gives users some control over
  memory consumption and execution order. For correct programs, `while_loop`
  should return the same result for any parallel_iterations > 0.

  For training, TensorFlow stores the tensors that are produced in the
  forward inference and are needed in back propagation. These tensors are a
  main source of memory consumption and often cause OOM errors when training
  on GPUs. When the flag swap_memory is true, we swap out these tensors from
  GPU to CPU. This for example allows us to train RNN models with very long
  sequences and large batches.

  Args:
    cond: A callable that represents the termination condition of the loop.
    body: A callable that represents the loop body.
    loop_vars: A (possibly nested) tuple, namedtuple or list of numpy array,
      `Tensor`, and `TensorArray` objects.
    shape_invariants: The shape invariants for the loop variables.
    parallel_iterations: The number of iterations allowed to run in parallel. It
      must be a positive integer.
    back_prop: Whether backprop is enabled for this while loop.
    swap_memory: Whether GPU-CPU memory swap is enabled for this loop.
    maximum_iterations: Optional maximum number of iterations of the while loop
      to run.  If provided, the `cond` output is AND-ed with an additional
      condition ensuring the number of iterations executed is no greater than
      `maximum_iterations`.
    name: Optional name prefix for the returned tensors.

  Returns:
    The output tensors for the loop variables after the loop. The return value
      has the same structure as `loop_vars`.

  Raises:
    TypeError: if `cond` or `body` is not callable.
    ValueError: if `loop_vars` is empty.

  Example:

  ```python
  i = tf.constant(0)
  c = lambda i: tf.less(i, 10)
  b = lambda i: tf.add(i, 1)
  r = tf.while_loop(c, b, [i])
  ```

  Example with nesting and a namedtuple:

  ```python
  import collections
  Pair = collections.namedtuple('Pair', 'j, k')
  ijk_0 = (tf.constant(0), Pair(tf.constant(1), tf.constant(2)))
  c = lambda i, p: i < 10
  b = lambda i, p: (i + 1, Pair((p.j + p.k), (p.j - p.k)))
  ijk_final = tf.while_loop(c, b, ijk_0)
  ```

  Example using shape_invariants:

  ```python
  i0 = tf.constant(0)
  m0 = tf.ones([2, 2])
  c = lambda i, m: i < 10
  b = lambda i, m: [i+1, tf.concat([m, m], axis=0)]
  tf.while_loop(
      c, b, loop_vars=[i0, m0],
      shape_invariants=[i0.get_shape(), tf.TensorShape([None, 2])])
  ```

  Example which demonstrates non-strict semantics: In the following
  example, the final value of the counter `i` does not depend on `x`. So
  the `while_loop` can increment the counter parallel to updates of `x`.
  However, because the loop counter at one loop iteration depends
  on the value at the previous iteration, the loop counter itself cannot
  be incremented in parallel. Hence if we just want the final value of the
  counter (which we print on the line `print(sess.run(i))`), then
  `x` will never be incremented, but the counter will be updated on a
  single thread. Conversely, if we want the value of the output (which we
  print on the line `print(sess.run(out).shape)`), then the counter may be
  incremented on its own thread, while `x` can be incremented in
  parallel on a separate thread. In the extreme case, it is conceivable
  that the thread incrementing the counter runs until completion before
  `x` is incremented even a single time. The only thing that can never
  happen is that the thread updating `x` can never get ahead of the
  counter thread because the thread incrementing `x` depends on the value
  of the counter.

  ```python
  import tensorflow as tf

  n = 10000
  x = tf.constant(list(range(n)))
  c = lambda i, x: i < n
  b = lambda i, x: (tf.Print(i + 1, [i]), tf.Print(x + 1, [i], "x:"))
  i, out = tf.while_loop(c, b, (0, x))
  with tf.Session() as sess:
      print(sess.run(i))  # prints [0] ... [9999]

      # The following line may increment the counter and x in parallel.
      # The counter thread may get ahead of the other thread, but not the
      # other way around. So you may see things like
      # [9996] x:[9987]
      # meaning that the counter thread is on iteration 9996,
      # while the other thread is on iteration 9987
      print(sess.run(out).shape)
  ```

  """
  return while_loop(
      cond=cond,
      body=body,
      loop_vars=loop_vars,
      shape_invariants=shape_invariants,
      parallel_iterations=parallel_iterations,
      back_prop=back_prop,
      swap_memory=swap_memory,
      name=name,
      maximum_iterations=maximum_iterations,
      return_same_structure=True)


# pylint: disable=redefined-outer-name
@tf_export(v1=["while_loop"])
def while_loop(cond,
               body,
               loop_vars,
               shape_invariants=None,
               parallel_iterations=10,
               back_prop=True,
               swap_memory=False,
               name=None,
               maximum_iterations=None,
               return_same_structure=False):
  """Repeat `body` while the condition `cond` is true.

  `cond` is a callable returning a boolean scalar tensor. `body` is a callable
  returning a (possibly nested) tuple, namedtuple or list of tensors of the same
  arity (length and structure) and types as `loop_vars`. `loop_vars` is a
  (possibly nested) tuple, namedtuple or list of tensors that is passed to both
  `cond` and `body`. `cond` and `body` both take as many arguments as there are
  `loop_vars`.

  In addition to regular Tensors or IndexedSlices, the body may accept and
  return TensorArray objects.  The flows of the TensorArray objects will
  be appropriately forwarded between loops and during gradient calculations.

  Note that `while_loop` calls `cond` and `body` *exactly once* (inside the
  call to `while_loop`, and not at all during `Session.run()`). `while_loop`
  stitches together the graph fragments created during the `cond` and `body`
  calls with some additional graph nodes to create the graph flow that
  repeats `body` until `cond` returns false.

  For correctness, `tf.while_loop()` strictly enforces shape invariants for
  the loop variables. A shape invariant is a (possibly partial) shape that
  is unchanged across the iterations of the loop. An error will be raised
  if the shape of a loop variable after an iteration is determined to be more
  general than or incompatible with its shape invariant. For example, a shape
  of [11, None] is more general than a shape of [11, 17], and [11, 21] is not
  compatible with [11, 17]. By default (if the argument `shape_invariants` is
  not specified), it is assumed that the initial shape of each tensor in
  `loop_vars` is the same in every iteration. The `shape_invariants` argument
  allows the caller to specify a less specific shape invariant for each loop
  variable, which is needed if the shape varies between iterations. The
  `tf.Tensor.set_shape`
  function may also be used in the `body` function to indicate that
  the output loop variable has a particular shape. The shape invariant for
  SparseTensor and IndexedSlices are treated specially as follows:

  a) If a loop variable is a SparseTensor, the shape invariant must be
  TensorShape([r]) where r is the rank of the dense tensor represented
  by the sparse tensor. It means the shapes of the three tensors of the
  SparseTensor are ([None], [None, r], [r]). NOTE: The shape invariant here
  is the shape of the SparseTensor.dense_shape property. It must be the shape of
  a vector.

  b) If a loop variable is an IndexedSlices, the shape invariant must be
  a shape invariant of the values tensor of the IndexedSlices. It means
  the shapes of the three tensors of the IndexedSlices are (shape, [shape[0]],
  [shape.ndims]).

  `while_loop` implements non-strict semantics, enabling multiple iterations
  to run in parallel. The maximum number of parallel iterations can be
  controlled by `parallel_iterations`, which gives users some control over
  memory consumption and execution order. For correct programs, `while_loop`
  should return the same result for any parallel_iterations > 0.

  For training, TensorFlow stores the tensors that are produced in the
  forward inference and are needed in back propagation. These tensors are a
  main source of memory consumption and often cause OOM errors when training
  on GPUs. When the flag swap_memory is true, we swap out these tensors from
  GPU to CPU. This for example allows us to train RNN models with very long
  sequences and large batches.

  Args:
    cond: A callable that represents the termination condition of the loop.
    body: A callable that represents the loop body.
    loop_vars: A (possibly nested) tuple, namedtuple or list of numpy array,
      `Tensor`, and `TensorArray` objects.
    shape_invariants: The shape invariants for the loop variables.
    parallel_iterations: The number of iterations allowed to run in parallel. It
      must be a positive integer.
    back_prop: Whether backprop is enabled for this while loop.
    swap_memory: Whether GPU-CPU memory swap is enabled for this loop.
    name: Optional name prefix for the returned tensors.
    maximum_iterations: Optional maximum number of iterations of the while loop
      to run.  If provided, the `cond` output is AND-ed with an additional
      condition ensuring the number of iterations executed is no greater than
      `maximum_iterations`.
    return_same_structure: If True, output has same structure as `loop_vars`. If
      eager execution is enabled, this is ignored (and always treated as True).

  Returns:
    The output tensors for the loop variables after the loop.
     If `return_same_structure` is True, the return value has the same
     structure as `loop_vars`.
     If `return_same_structure` is False, the return value is a Tensor,
     TensorArray or IndexedSlice if the length of `loop_vars` is 1, or a list
     otherwise.

  Raises:
    TypeError: if `cond` or `body` is not callable.
    ValueError: if `loop_vars` is empty.

  Example:

  ```python
  i = tf.constant(0)
  c = lambda i: tf.less(i, 10)
  b = lambda i: tf.add(i, 1)
  r = tf.while_loop(c, b, [i])
  ```

  Example with nesting and a namedtuple:

  ```python
  import collections
  Pair = collections.namedtuple('Pair', 'j, k')
  ijk_0 = (tf.constant(0), Pair(tf.constant(1), tf.constant(2)))
  c = lambda i, p: i < 10
  b = lambda i, p: (i + 1, Pair((p.j + p.k), (p.j - p.k)))
  ijk_final = tf.while_loop(c, b, ijk_0)
  ```

  Example using shape_invariants:

  ```python
  i0 = tf.constant(0)
  m0 = tf.ones([2, 2])
  c = lambda i, m: i < 10
  b = lambda i, m: [i+1, tf.concat([m, m], axis=0)]
  tf.while_loop(
      c, b, loop_vars=[i0, m0],
      shape_invariants=[i0.get_shape(), tf.TensorShape([None, 2])])
  ```

  Example which demonstrates non-strict semantics: In the following
  example, the final value of the counter `i` does not depend on `x`. So
  the `while_loop` can increment the counter parallel to updates of `x`.
  However, because the loop counter at one loop iteration depends
  on the value at the previous iteration, the loop counter itself cannot
  be incremented in parallel. Hence if we just want the final value of the
  counter (which we print on the line `print(sess.run(i))`), then
  `x` will never be incremented, but the counter will be updated on a
  single thread. Conversely, if we want the value of the output (which we
  print on the line `print(sess.run(out).shape)`), then the counter may be
  incremented on its own thread, while `x` can be incremented in
  parallel on a separate thread. In the extreme case, it is conceivable
  that the thread incrementing the counter runs until completion before
  `x` is incremented even a single time. The only thing that can never
  happen is that the thread updating `x` can never get ahead of the
  counter thread because the thread incrementing `x` depends on the value
  of the counter.

  ```python
  import tensorflow as tf

  n = 10000
  x = tf.constant(list(range(n)))
  c = lambda i, x: i < n
  b = lambda i, x: (tf.Print(i + 1, [i]), tf.Print(x + 1, [i], "x:"))
  i, out = tf.while_loop(c, b, (0, x))
  with tf.Session() as sess:
      print(sess.run(i))  # prints [0] ... [9999]

      # The following line may increment the counter and x in parallel.
      # The counter thread may get ahead of the other thread, but not the
      # other way around. So you may see things like
      # [9996] x:[9987]
      # meaning that the counter thread is on iteration 9996,
      # while the other thread is on iteration 9987
      print(sess.run(out).shape)
  ```

  """
  if util.ENABLE_CONTROL_FLOW_V2 and not context.executing_eagerly():
    return while_v2.while_loop(
        cond,
        body,
        loop_vars,
        shape_invariants=shape_invariants,
        maximum_iterations=maximum_iterations,
        name=name,
        return_same_structure=return_same_structure)

  with ops.name_scope(name, "while", loop_vars):
    if not loop_vars:
      raise ValueError("No loop variables provided")
    if not callable(cond):
      raise TypeError("cond must be callable.")
    if not callable(body):
      raise TypeError("body must be callable.")
    if parallel_iterations < 1:
      raise TypeError("parallel_iterations must be a positive integer.")

    if maximum_iterations is not None:
      maximum_iterations = ops.convert_to_tensor(
          maximum_iterations, name="maximum_iterations")
      if maximum_iterations.shape.ndims != 0:
        raise ValueError("maximum_iterations must be a scalar, saw shape: %s" %
                         maximum_iterations.shape)

      counter = constant_op.constant(
          0, dtype=maximum_iterations.dtype, name="iteration_counter")
      orig_cond = cond
      orig_body = body
      if len(loop_vars) == 1:
        loop_vars = (counter, loop_vars[0])
        cond = lambda i, lv: (  # pylint: disable=g-long-lambda
            math_ops.logical_and(i < maximum_iterations, orig_cond(lv)))
        body = lambda i, lv: (i + 1, orig_body(lv))
      else:
        loop_vars = (counter, loop_vars)
        cond = lambda i, lv: (  # pylint: disable=g-long-lambda
            math_ops.logical_and(i < maximum_iterations, orig_cond(*lv)))
        body = lambda i, lv: (i + 1, orig_body(*lv))

    if context.executing_eagerly():
      try_to_pack = len(loop_vars) == 1
      packed = False  # whether the body result was packed into a 1-item tuple

      while cond(*loop_vars):
        loop_vars = body(*loop_vars)
        if try_to_pack and not isinstance(loop_vars, (list, _basetuple)):
          packed = True
          loop_vars = (loop_vars,)
      if maximum_iterations is not None:
        return loop_vars[1]
      else:
        return loop_vars[0] if packed else loop_vars

    if shape_invariants is not None:
      if maximum_iterations is not None:
        shape_invariants = (tensor_shape.TensorShape([]), shape_invariants)
      nest.assert_same_structure(loop_vars, shape_invariants)

    loop_context = WhileContext(
        maximum_iterations=maximum_iterations,
        parallel_iterations=parallel_iterations,
        back_prop=back_prop,
        swap_memory=swap_memory)
    # Only add non-nested loops to the collection. Any nested control flow will
    # be encapsulated in the root context.
    if loop_context.outer_context is None:
      ops.add_to_collection(ops.GraphKeys.WHILE_CONTEXT, loop_context)
    result = loop_context.BuildLoop(cond, body, loop_vars, shape_invariants,
                                    return_same_structure)
    if maximum_iterations is not None:
      return result[1]
    else:
      return result


# pylint: enable=redefined-outer-name


def _AsTensorList(x, p):
  """Return x as a list of Tensors or IndexedSlices.

  For entries of `x` that are Operations, this returns an Identity of `p`
  with a dependency on the operation.

  Args:
    x: A Tensor/IndexedSlices/Operation or a list or tuple of them.
    p: A Tensor to return for entries in `x` that are Operations.

  Returns:
    A list of Tensors or IndexedSlices.
  """
  if not isinstance(x, (list, _basetuple)):
    x = [x]

  l = []
  for v in x:
    if isinstance(v, ops.Operation):
      v = with_dependencies([v], p)
    v = ops.convert_to_tensor_or_indexed_slices(v)
    if isinstance(v, ops.Tensor):
      l.append(array_ops.identity(v))
    else:
      l.append(
          ops.IndexedSlices(
              array_ops.identity(v.values), array_ops.identity(v.indices)))
  return l


def _CheckResults(a, b):
  assert len(a) == len(b), (
      "Values returned by a() and b() must have the same length.")
  for x, y in zip(a, b):
    assert x.dtype == y.dtype, (
        "Values returned by a() [%s] and b() [%s] must have "
        "the same type: %s, %s." % (x.name, y.name, x.dtype.name, y.dtype.name))


def with_dependencies(dependencies, output_tensor, name=None):
  """Produces the content of `output_tensor` only after `dependencies`.

  In some cases, a user may want the output of an operation to be
  consumed externally only after some other dependencies have run
  first. This function ensures returns `output_tensor`, but only after all
  operations in `dependencies` have run. Note that this means that there is
  no guarantee that `output_tensor` will be evaluated after any `dependencies`
  have run.

  See also `tf.tuple` and `tf.group`.

  Args:
    dependencies: Iterable of operations to run before this op finishes.
    output_tensor: A `Tensor` or `IndexedSlices` that will be returned.
    name: (Optional) A name for this operation.

  Returns:
    Same as `output_tensor`.

  Raises:
    TypeError: if `output_tensor` is not a `Tensor` or `IndexedSlices`.
  """
  if context.executing_eagerly():
    return output_tensor
  with ops.name_scope(name, "control_dependency",
                      list(dependencies) + [output_tensor]) as name:
    with ops.colocate_with(output_tensor):
      with ops.control_dependencies(dependencies):
        output_tensor = ops.convert_to_tensor_or_indexed_slices(output_tensor)
        if isinstance(output_tensor, ops.Tensor):
          return _Identity(output_tensor, name=name)
        else:
          return ops.IndexedSlices(
              _Identity(output_tensor.values, name=name), output_tensor.indices,
              output_tensor.dense_shape)


def _GroupControlDeps(dev, deps, name=None):
  with ops.control_dependencies(deps):
    if dev is None:
      return no_op(name=name)
    else:
      with ops.device(dev):
        return no_op(name=name)


# TODO(touts): Accept "inputs" as a list.
@tf_export("group")
def group(*inputs, **kwargs):
  """Create an op that groups multiple operations.

  When this op finishes, all ops in `inputs` have finished. This op has no
  output.

  See also `tf.tuple` and
  `tf.control_dependencies`.

  Args:
    *inputs: Zero or more tensors to group.
    name: A name for this operation (optional).

  Returns:
    An Operation that executes all its inputs.

  Raises:
    ValueError: If an unknown keyword argument is provided.
  """
  if context.executing_eagerly():
    return None
  name = kwargs.pop("name", None)
  if kwargs:
    raise ValueError("Unknown keyword arguments: " + ", ".join(kwargs.keys()))
  with ops.name_scope(name, "group_deps", inputs) as name:
    # Grouping no inputs means do nothing
    if not inputs:
      return no_op(name=name)

    # Sorts *inputs according to their devices.
    ops_on_device = {}  # device -> operations specified on the device.
    for inp in nest.flatten(inputs):
      if not hasattr(inp, "device"):
        raise TypeError("Expected tf.group() expected Tensor arguments not "
                        "'%s' with type '%s'" % (inp, type(inp)))
      dev = inp.device
      if dev in ops_on_device:
        ops_on_device[dev].append(inp)
      else:
        ops_on_device[dev] = [inp]
    if len(ops_on_device) == 1:
      # 1-level tree. The root node is the returned NoOp node.
      (dev, deps), = ops_on_device.items()
      return _GroupControlDeps(dev, deps, name=name)

    # 2-level tree. The root node is the returned NoOp node.
    # deps contains 1 NoOp node for each device.
    deps = []

    def device_key(dev):
      """A sort key that allows None to be compared to strings."""
      return "" if dev is None else dev

    for dev in sorted(six.iterkeys(ops_on_device), key=device_key):
      deps.append(_GroupControlDeps(dev, ops_on_device[dev]))

    with ops.control_dependencies(deps):
      return no_op(name=name)


@tf_export("tuple", v1=[])
def tuple_v2(tensors, control_inputs=None, name=None):
  """Group tensors together.

  This creates a tuple of tensors with the same values as the `tensors`
  argument, except that the value of each tensor is only returned after the
  values of all tensors have been computed.

  `control_inputs` contains additional ops that have to finish before this op
  finishes, but whose outputs are not returned.

  This can be used as a "join" mechanism for parallel computations: all the
  argument tensors can be computed in parallel, but the values of any tensor
  returned by `tuple` are only available after all the parallel computations
  are done.

  See also `tf.group` and
  `tf.control_dependencies`.

  Args:
    tensors: A list of `Tensor`s or `IndexedSlices`, some entries can be `None`.
    control_inputs: List of additional ops to finish before returning.
    name: (optional) A name to use as a `name_scope` for the operation.

  Returns:
    Same as `tensors`.

  Raises:
    ValueError: If `tensors` does not contain any `Tensor` or `IndexedSlices`.
    TypeError: If `control_inputs` is not a list of `Operation` or `Tensor`
      objects.

  """
  return tuple(tensors=tensors, name=name, control_inputs=control_inputs)  # pylint: disable=redefined-builtin


@tf_export(v1=["tuple"])
def tuple(tensors, name=None, control_inputs=None):  # pylint: disable=redefined-builtin
  """Group tensors together.

  This creates a tuple of tensors with the same values as the `tensors`
  argument, except that the value of each tensor is only returned after the
  values of all tensors have been computed.

  `control_inputs` contains additional ops that have to finish before this op
  finishes, but whose outputs are not returned.

  This can be used as a "join" mechanism for parallel computations: all the
  argument tensors can be computed in parallel, but the values of any tensor
  returned by `tuple` are only available after all the parallel computations
  are done.

  See also `tf.group` and
  `tf.control_dependencies`.

  Args:
    tensors: A list of `Tensor`s or `IndexedSlices`, some entries can be `None`.
    name: (optional) A name to use as a `name_scope` for the operation.
    control_inputs: List of additional ops to finish before returning.

  Returns:
    Same as `tensors`.

  Raises:
    ValueError: If `tensors` does not contain any `Tensor` or `IndexedSlices`.
    TypeError: If `control_inputs` is not a list of `Operation` or `Tensor`
      objects.

  """
  if context.executing_eagerly():
    return tensors
  with ops.name_scope(name, "tuple", tensors) as name:
    tensors = [
        t if (isinstance(t, ops.Operation) or tensor_util.is_tensor(t) or
              t is None) else ops.convert_to_tensor(t) for t in tensors
    ]
    gating_ops = [
        t if isinstance(t, ops.Operation) else t.op
        for t in tensors
        if t is not None
    ]
    if control_inputs:
      for c in control_inputs:
        if isinstance(c, ops.Tensor):
          c = c.op
        elif not isinstance(c, ops.Operation):
          raise TypeError("Control input must be Operation or Tensor: %s" % c)
        gating_ops.append(c)
    # Note that in order to ensure ordering in the pbtxt, we must take care to
    # ensure the order here.
    gating_ops = sorted(set(gating_ops), key=lambda op: op._id)  # Uniquify ops.
    if not gating_ops:
      raise ValueError("Must have at least one Tensor: %s" % tensors)
    gate = group(*gating_ops)
    tpl = []
    for t in tensors:
      if tensor_util.is_tensor(t):
        tpl.append(with_dependencies([gate], t))
      elif isinstance(t, ops.Operation):
        with ops.control_dependencies([gate]):
          tpl.append(group(t))
      else:
        tpl.append(None)
    return tpl


def _assert_at_most_n_true(predicates, n, msg):
  """Returns an Assert op that checks that at most n predicates are True.

  Args:
    predicates: list of bool scalar tensors.
    n: maximum number of true predicates allowed.
    msg: Error message.
  """
  preds_c = array_ops.stack(predicates, name="preds_c")
  num_true_conditions = math_ops.reduce_sum(
      math_ops.cast(preds_c, dtypes.int32), name="num_true_conds")
  condition = math_ops.less_equal(num_true_conditions,
                                  constant_op.constant(n, name="n_true_conds"))
  preds_names = ", ".join(getattr(p, "name", "?") for p in predicates)
  error_msg = [
      "%s: more than %d conditions (%s) evaluated as True:" %
      (msg, n, preds_names), preds_c
  ]
  return Assert(condition, data=error_msg, summarize=len(predicates))


def _case_create_default_action(predicates, actions):
  """Creates default action for a list of actions and their predicates.

  It uses the input actions to select an arbitrary as default and makes sure
  that corresponding predicates have valid values.

  Args:
    predicates: a list of bool scalar tensors
    actions: a list of callable objects which return tensors.

  Returns:
    a callable
  """
  k = len(predicates) - 1  # could pick any
  predicate, action = predicates[k], actions[k]
  other_predicates, other_actions = predicates[:k], actions[:k]

  def default_action():
    others_msg = ("Implementation error: "
                  "selected default action #%d was called, but some of other "
                  "predicates are True: " % k)
    default_msg = ("Input error: "
                   "None of conditions evaluated as True:",
                   array_ops.stack(predicates, name="preds_c"))
    with ops.control_dependencies([
        _assert_at_most_n_true(other_predicates, n=0, msg=others_msg),
        Assert(predicate, data=default_msg)
    ]):
      return action()

  return default_action, other_predicates, other_actions


def _case_verify_and_canonicalize_args(pred_fn_pairs, exclusive, name,
                                       allow_python_preds):
  """Verifies input arguments for the case function.

  Args:
    pred_fn_pairs: Dict or list of pairs of a boolean scalar tensor, and a
      callable which returns a list of tensors.
    exclusive: True iff at most one predicate is allowed to evaluate to `True`.
    name: A name for the case operation.
    allow_python_preds: if true, pred_fn_pairs may contain Python bools in
      addition to boolean Tensors

  Raises:
    TypeError: If `pred_fn_pairs` is not a list/dictionary.
    TypeError: If `pred_fn_pairs` is a list but does not contain 2-tuples.
    TypeError: If `fns[i]` is not callable for any i, or `default` is not
               callable.

  Returns:
    a tuple <list of scalar bool tensors, list of callables>.
  """
  if not isinstance(pred_fn_pairs, (list, _basetuple, dict)):
    raise TypeError("fns must be a list, tuple, or dict")

  if isinstance(pred_fn_pairs, collections.OrderedDict):
    pred_fn_pairs = pred_fn_pairs.items()
  elif isinstance(pred_fn_pairs, dict):
    if context.executing_eagerly():
      # No name to sort on in eager mode. Use dictionary traversal order,
      # which is nondeterministic in versions of Python < 3.6
      if not exclusive:
        raise ValueError("Unordered dictionaries are not supported for the "
                         "`pred_fn_pairs` argument when `exclusive=False` and "
                         "eager mode is enabled.")
      pred_fn_pairs = list(pred_fn_pairs.items())
    else:
      pred_fn_pairs = sorted(
          pred_fn_pairs.items(), key=lambda item: item[0].name)
      if not exclusive:
        logging.warn(
            "%s: An unordered dictionary of predicate/fn pairs was "
            "provided, but exclusive=False. The order of conditional "
            "tests is deterministic but not guaranteed.", name)
  for pred_fn_pair in pred_fn_pairs:
    if not isinstance(pred_fn_pair, _basetuple) or len(pred_fn_pair) != 2:
      raise TypeError("Each entry in pred_fn_pairs must be a 2-tuple")
    pred, fn = pred_fn_pair

    if isinstance(pred, ops.Tensor):
      if pred.dtype != dtypes.bool:
        raise TypeError("pred must be Tensor of type bool: %s" % pred.name)
    elif not allow_python_preds:
      raise TypeError("pred must be a Tensor, got: %s" % pred)
    elif not isinstance(pred, bool):
      raise TypeError("pred must be a Tensor or bool, got: %s" % pred)

    if not callable(fn):
      raise TypeError("fn for pred %s must be callable." % pred.name)

  predicates, actions = zip(*pred_fn_pairs)
  return predicates, actions


def _case_helper(cond_fn,
                 pred_fn_pairs,
                 default,
                 exclusive,
                 name,
                 allow_python_preds=False,
                 **cond_kwargs):
  """Implementation of case that allows for different cond functions.

  Args:
    cond_fn: method that has signature and semantics of `cond` above.
    pred_fn_pairs: Dict or list of pairs of a boolean scalar tensor, and a
      callable which returns a list of tensors.
    default: Optional callable that returns a list of tensors.
    exclusive: True iff at most one predicate is allowed to evaluate to `True`.
    name: A name for this operation (optional).
    allow_python_preds: if true, pred_fn_pairs may contain Python bools in
      addition to boolean Tensors
    **cond_kwargs: keyword arguments that will be passed to `cond_fn`.

  Returns:
    The tensors returned by the first pair whose predicate evaluated to True, or
    those returned by `default` if none does.

  Raises:
    TypeError: If `pred_fn_pairs` is not a list/dictionary.
    TypeError: If `pred_fn_pairs` is a list but does not contain 2-tuples.
    TypeError: If `fns[i]` is not callable for any i, or `default` is not
               callable.
  """
  predicates, actions = _case_verify_and_canonicalize_args(
      pred_fn_pairs, exclusive, name, allow_python_preds)
  with ops.name_scope(name, "case", [predicates]):
    if default is None:
      default, predicates, actions = _case_create_default_action(
          predicates, actions)
    fn = default
    # To eval conditions in direct order we create nested conditions in reverse:
    #   cond_fn(c[0], true_fn=.., false_fn=cond_fn(c[1], ...))
    for predicate, action in reversed(list(zip(predicates, actions))):
      fn = functools.partial(
          cond_fn, predicate, true_fn=action, false_fn=fn, **cond_kwargs)
    if exclusive:
      with ops.control_dependencies([
          _assert_at_most_n_true(
              predicates, n=1, msg="Input error: exclusive=True")
      ]):
        return fn()
    else:
      return fn()


@tf_export("case")
def case(pred_fn_pairs,
         default=None,
         exclusive=False,
         strict=False,
         name="case"):
  """Create a case operation.

  The `pred_fn_pairs` parameter is a dict or list of pairs of size N.
  Each pair contains a boolean scalar tensor and a python callable that
  creates the tensors to be returned if the boolean evaluates to True.
  `default` is a callable generating a list of tensors. All the callables
  in `pred_fn_pairs` as well as `default` (if provided) should return the same
  number and types of tensors.

  If `exclusive==True`, all predicates are evaluated, and an exception is
  thrown if more than one of the predicates evaluates to `True`.
  If `exclusive==False`, execution stops at the first predicate which
  evaluates to True, and the tensors generated by the corresponding function
  are returned immediately. If none of the predicates evaluate to True, this
  operation returns the tensors generated by `default`.

  `tf.case` supports nested structures as implemented in
  `tf.contrib.framework.nest`. All of the callables must return the same
  (possibly nested) value structure of lists, tuples, and/or named tuples.
  Singleton lists and tuples form the only exceptions to this: when returned by
  a callable, they are implicitly unpacked to single values. This
  behavior is disabled by passing `strict=True`.

  If an unordered dictionary is used for `pred_fn_pairs`, the order of the
  conditional tests is not guaranteed. However, the order is guaranteed to be
  deterministic, so that variables created in conditional branches are created
  in fixed order across runs.

  @compatibility{eager}
  Unordered dictionaries are not supported in eager mode when `exclusive=False`.
  Use a list of tuples instead.
  @end_compatibility


  **Example 1:**

  Pseudocode:

  ```
  if (x < y) return 17;
  else return 23;
  ```

  Expressions:

  ```python
  f1 = lambda: tf.constant(17)
  f2 = lambda: tf.constant(23)
  r = tf.case([(tf.less(x, y), f1)], default=f2)
  ```

  **Example 2:**

  Pseudocode:

  ```
  if (x < y && x > z) raise OpError("Only one predicate may evaluate to True");
  if (x < y) return 17;
  else if (x > z) return 23;
  else return -1;
  ```

  Expressions:

  ```python
  def f1(): return tf.constant(17)
  def f2(): return tf.constant(23)
  def f3(): return tf.constant(-1)
  r = tf.case({tf.less(x, y): f1, tf.greater(x, z): f2},
           default=f3, exclusive=True)
  ```

  Args:
    pred_fn_pairs: Dict or list of pairs of a boolean scalar tensor and a
      callable which returns a list of tensors.
    default: Optional callable that returns a list of tensors.
    exclusive: True iff at most one predicate is allowed to evaluate to `True`.
    strict: A boolean that enables/disables 'strict' mode; see above.
    name: A name for this operation (optional).

  Returns:
    The tensors returned by the first pair whose predicate evaluated to True, or
    those returned by `default` if none does.

  Raises:
    TypeError: If `pred_fn_pairs` is not a list/dictionary.
    TypeError: If `pred_fn_pairs` is a list but does not contain 2-tuples.
    TypeError: If `fns[i]` is not callable for any i, or `default` is not
               callable.
  """
  return _case_helper(
      cond,
      pred_fn_pairs,
      default,
      exclusive,
      name,
      allow_python_preds=False,
      strict=strict)


class XLAControlFlowContext(ControlFlowContext):
  """Base class for XLA and TPU control flow contexts."""

  def __init__(self):
    super(XLAControlFlowContext, self).__init__()
    self._name = "XLAControlFlowContext"

  def to_control_flow_context_def(self, context_def, export_scope=None):
    # pylint: disable=useless-super-delegation
    # NOTE(slebedev): the method is required by `ControlFlowContext`.
    super(XLAControlFlowContext, self).to_control_flow_context_def(
        context_def, export_scope)

  def IsXLAContext(self):
    return True

  def AddOp(self, _):
    pass

  def AddValue(self, x):
    return x


def from_control_flow_context_def(context_def, import_scope=None):
  """Deserializes `context_def` into the appropriate ControlFlowContext.

  Args:
    context_def: ControlFlowContextDef proto
    import_scope: Optional `string`. Name scope to add.

  Returns:
    A ControlFlowContext subclass
  """
  if context_def.HasField("cond_ctxt"):
    return CondContext.from_proto(
        context_def.cond_ctxt, import_scope=import_scope)
  if context_def.HasField("while_ctxt"):
    return WhileContext.from_proto(
        context_def.while_ctxt, import_scope=import_scope)
  raise NotImplementedError("Unknown ControlFlowContextDef field: %s" %
                            context_def.WhichOneof("ctxt"))


ops.register_proto_function(
    ops.GraphKeys.COND_CONTEXT,
    proto_type=control_flow_pb2.CondContextDef,
    to_proto=CondContext.to_proto,
    from_proto=CondContext.from_proto)

ops.register_proto_function(
    ops.GraphKeys.WHILE_CONTEXT,
    proto_type=control_flow_pb2.WhileContextDef,
    to_proto=WhileContext.to_proto,
    from_proto=WhileContext.from_proto)
