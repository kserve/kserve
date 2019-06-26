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
"""Support for ragged tensors."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import tensor_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import check_ops
from tensorflow.python.ops import gen_ragged_math_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops.ragged import ragged_functional_ops
from tensorflow.python.ops.ragged import ragged_tensor
from tensorflow.python.ops.ragged import ragged_util
from tensorflow.python.ops.ragged import segment_id_ops
from tensorflow.python.util.tf_export import tf_export


#===============================================================================
# ragged.range
#===============================================================================
# pylint: disable=redefined-builtin
@tf_export('ragged.range')
def range(starts, limits=None, deltas=1, dtype=None, name=None):
  """Returns a `RaggedTensor` containing the specified sequences of numbers.

  Each row of the returned `RaggedTensor` contains a single sequence:

  ```python
  ragged.range(starts, limits, deltas)[i] ==
      tf.range(starts[i], limits[i], deltas[i])
  ```

  If `start[i] < limits[i] and deltas[i] > 0`, then `output[i]` will be an
  empty list.  Similarly, if `start[i] > limits[i] and deltas[i] < 0`, then
  `output[i]` will be an empty list.  This behavior is consistent with the
  Python `range` function, but differs from the `tf.range` op, which returns
  an error for these cases.

  Examples:

  ```python
  >>> ragged.range([3, 5, 2]).eval().tolist()
  [[0, 1, 2], [0, 1, 2, 3, 4], [0, 1]]
  >>> ragged.range([0, 5, 8], [3, 3, 12]).eval().tolist()
  [[0, 1, 2], [], [8, 9, 10, 11]]
  >>> ragged.range([0, 5, 8], [3, 3, 12], 2).eval().tolist()
  [[0, 2], [], [8, 10]]
  ```

  The input tensors `starts`, `limits`, and `deltas` may be scalars or vectors.
  The vector inputs must all have the same size.  Scalar inputs are broadcast
  to match the size of the vector inputs.

  Args:
    starts: Vector or scalar `Tensor`.  Specifies the first entry for each range
      if `limits` is not `None`; otherwise, specifies the range limits, and the
      first entries default to `0`.
    limits: Vector or scalar `Tensor`.  Specifies the exclusive upper limits for
      each range.
    deltas: Vector or scalar `Tensor`.  Specifies the increment for each range.
      Defaults to `1`.
    dtype: The type of the elements of the resulting tensor.  If not specified,
      then a value is chosen based on the other args.
    name: A name for the operation.

  Returns:
    A `RaggedTensor` of type `dtype` with `ragged_rank=1`.
  """
  if limits is None:
    starts, limits = 0, starts

  with ops.name_scope(name, 'RaggedRange', [starts, limits, deltas]) as name:
    starts = ops.convert_to_tensor(starts, dtype=dtype, name='starts')
    limits = ops.convert_to_tensor(limits, dtype=dtype, name='limits')
    deltas = ops.convert_to_tensor(deltas, dtype=dtype, name='deltas')

    # infer dtype if not explicitly provided
    if dtype is None:
      starts, limits, deltas = _infer_matching_dtype(
          [starts, limits, deltas],
          [dtypes.int32, dtypes.int64, dtypes.float32, dtypes.float64])

    result = gen_ragged_math_ops.ragged_range(starts, limits, deltas, name=name)
    return ragged_tensor.RaggedTensor.from_row_splits(result.rt_dense_values,
                                                      result.rt_nested_splits)


def _infer_matching_dtype(tensors, dtype_hierarchy):
  """Infers a matching dtype for tensors, and casts them to that dtype."""
  assert all(t.dtype in dtype_hierarchy for t in tensors)
  inferred_dtype = max([t.dtype for t in tensors], key=dtype_hierarchy.index)
  return [math_ops.cast(t, inferred_dtype) for t in tensors]


#===============================================================================
# ragged_segment_<AGGREGATE>
#===============================================================================

# Docstring template used for the raggged_segment_<AGGREGATE> ops.
_RAGGED_SEGMENT_DOCSTRING = """\
Computes the %(combination)s along segments of a RaggedTensor.

  Returns a RaggedTensor `output` with `num_segments` rows, where the row
  `output[i]` is formed by taking the %(combination)s of all rows of `data`
  whose corresponding `segment_id` is `i`.

  The length of the row `output[i]` will be the maximum of the lengths of
  all rows of `data` whose corresponding `segment_id` is `i`.  If no `data`
  rows correspond to a given segment ID, then the output row for that segment
  ID will be empty.

  Args:
    data: A `RaggedTensor` containing the values to combine.
    segment_ids: A `Tensor` or `RaggedTensor`.  Must have type `int64` or
      `int32`.  `segment_ids.shape` must be a prefix of `data.shape`.
      Must be greater than or equal to zero, and less than `num_segments`.
      `segment_ids` is not required to be sorted.
    num_segments: An `int32` or `int64` scalar specifying the number of
      distinct segment ids.
    name: A name prefix for the returned tensor (optional).
  Returns:
    A `RaggedTensor` containing the %(combined)s values.  The returned tensor
    has the same dtype as `data`, and its shape is
    `[num_segments] + data.shape[segment_ids.rank:]`.
  Raises:
    ValueError: If `segment_ids.shape` is not a prefix of `data.shape`.
"""


def _ragged_segment_aggregate(unsorted_segment_op,
                              data,
                              segment_ids,
                              num_segments,
                              name=None):
  """Aggregates along segments of a RaggedTensor using `unsorted_segment_op`.

  Returns a RaggedTensor `output` with `num_segments` rows, where the row
  `output[i]` is formed by combining all rows of `data` whose corresponding
  `segment_id` is `i`.  The values in each row are combined using
  `unsorted_segment_op`.

  The length of the row `output[i]` will be the maximum of the lengths of
  all rows of `data` whose corresponding `segment_id` is `i`.  If no `data`
  rows correspond to a given segment ID, then the output row for that segment
  ID will be empty.

  Args:
    unsorted_segment_op: The tensorflow `op` that should be used to combine
      values in each row.  Must have the same signature and basic behavior as
      `unsorted_segment_sum`, `unsorted_segment_max`, etc.
    data: A `RaggedTensor` containing the values to be combined.
    segment_ids: A `Tensor` or `RaggedTensor`.  Must have type `int64` or
      `int32`.  `segment_ids.shape` must be a prefix of `data.shape`.
      `segment_ids` is not required to be sorted.
    num_segments: An `int32` or `int64` scalar.
    name: A name prefix for the returned tensor (optional).

  Returns:
    A `RaggedTensor` containing the aggregated values.  The returned tensor
    has the same dtype as `data`, and its shape is
    `[num_segments] + data.shape[segment_ids.rank:]`.
  Raises:
    ValueError: If segment_ids.shape is not a prefix of data.shape.
  """
  if not (ragged_tensor.is_ragged(data) or
          ragged_tensor.is_ragged(segment_ids)):
    return unsorted_segment_op(data, segment_ids, num_segments, name)

  with ops.name_scope(name, 'RaggedSegment',
                      [data, segment_ids, num_segments]) as name:
    data = ragged_tensor.convert_to_tensor_or_ragged_tensor(data, name='data')
    segment_ids = ragged_tensor.convert_to_tensor_or_ragged_tensor(
        segment_ids, name='segment_ids')

    if ragged_tensor.is_ragged(segment_ids):
      if not ragged_tensor.is_ragged(data):
        raise ValueError('segment_ids.shape must be a prefix of data.shape, '
                         'but segment_ids is ragged and data is not.')
      check_splits = check_ops.assert_equal(
          segment_ids.row_splits,
          data.row_splits,
          message='segment_ids.shape must be a prefix of data.shape')
      with ops.control_dependencies([check_splits]):
        return _ragged_segment_aggregate(unsorted_segment_op, data.values,
                                         segment_ids.values, num_segments, name)

    segment_ids = math_ops.cast(segment_ids, dtypes.int64)

    # Find the length of each row in data.  (dtype=int64, shape=[data_nrows])
    data_row_lengths = data.row_splits[1:] - data.row_splits[:-1]

    # Find the length that each output row will have.  The length of the row
    # corresponding to segment `id` is `max(data_row_lengths[i])` where
    # `segment_ids[i]=id`.  (dtype=int64, shape=[output_nrows])
    output_row_lengths = math_ops.maximum(
        math_ops.unsorted_segment_max(data_row_lengths, segment_ids,
                                      num_segments), 0)
    assert output_row_lengths.dtype == dtypes.int64

    # Build the splits tensor for the output RaggedTensor.
    output_splits = array_ops.concat([
        array_ops.zeros([1], dtypes.int64),
        math_ops.cumsum(output_row_lengths)
    ],
                                     axis=0)

    # For each row in `data`, find the start & limit position where that row's
    # values will be aggregated in output.values.
    data_row_to_out_row_start = array_ops.gather(output_splits, segment_ids)
    data_row_to_out_row_limit = data_row_to_out_row_start + data_row_lengths

    # For each value in `data.values`, find the position where it will
    # aggregated in `output.values`.
    # Get the target output values index for each data values index.
    data_val_to_out_val_index = range(data_row_to_out_row_start,
                                      data_row_to_out_row_limit).values

    # Recursively aggregate the values.
    output_values = _ragged_segment_aggregate(unsorted_segment_op, data.values,
                                              data_val_to_out_val_index,
                                              output_splits[-1])
    return ragged_tensor.RaggedTensor.from_row_splits(output_values,
                                                      output_splits)


def segment_sum(data, segment_ids, num_segments, name=None):
  # For docs, see: _RAGGED_SEGMENT_DOCSTRING
  return _ragged_segment_aggregate(math_ops.unsorted_segment_sum, data,
                                   segment_ids, num_segments, name or
                                   'RaggedSegmentSum')


def segment_prod(data, segment_ids, num_segments, name=None):
  # For docs, see: _RAGGED_SEGMENT_DOCSTRING
  return _ragged_segment_aggregate(math_ops.unsorted_segment_prod, data,
                                   segment_ids, num_segments, name or
                                   'RaggedSegmentProd')


def segment_min(data, segment_ids, num_segments, name=None):
  # For docs, see: _RAGGED_SEGMENT_DOCSTRING
  return _ragged_segment_aggregate(math_ops.unsorted_segment_min, data,
                                   segment_ids, num_segments, name or
                                   'RaggedSegmentMin')


def segment_max(data, segment_ids, num_segments, name=None):
  # For docs, see: _RAGGED_SEGMENT_DOCSTRING
  return _ragged_segment_aggregate(math_ops.unsorted_segment_max, data,
                                   segment_ids, num_segments, name or
                                   'RaggedSegmentMax')


def segment_mean(data, segment_ids, num_segments, name=None):
  """For docs, see: _RAGGED_SEGMENT_DOCSTRING."""
  with ops.name_scope(name, 'RaggedSegmentMean',
                      [data, segment_ids, num_segments]):
    total = segment_sum(data, segment_ids, num_segments)
    ones = ragged_tensor.RaggedTensor.from_nested_row_splits(
        array_ops.ones_like(data.flat_values), data.nested_row_splits)
    count = segment_sum(ones, segment_ids, num_segments)
    if ragged_tensor.is_ragged(total):
      return total.with_flat_values(total.flat_values / count.flat_values)
    else:
      return total / count


def segment_sqrt_n(data, segment_ids, num_segments, name=None):
  """For docs, see: _RAGGED_SEGMENT_DOCSTRING."""
  with ops.name_scope(name, 'RaggedSegmentSqrtN',
                      [data, segment_ids, num_segments]):
    total = segment_sum(data, segment_ids, num_segments)
    ones = ragged_tensor.RaggedTensor.from_nested_row_splits(
        array_ops.ones_like(data.flat_values), data.nested_row_splits)
    count = segment_sum(ones, segment_ids, num_segments)
    if ragged_tensor.is_ragged(total):
      return total.with_flat_values(
          total.flat_values / math_ops.sqrt(count.flat_values))
    else:
      return total / math_ops.sqrt(count)


def _set_ragged_segment_docstring(func, combination, combined):
  func.__doc__ = _RAGGED_SEGMENT_DOCSTRING % dict(
      combination=combination, combined=combined)


_set_ragged_segment_docstring(segment_sum, 'sum', 'summed')
_set_ragged_segment_docstring(segment_prod, 'product', 'multiplied')
_set_ragged_segment_docstring(segment_min, 'minimum', 'minimized')
_set_ragged_segment_docstring(segment_max, 'maximum', 'maximized')
_set_ragged_segment_docstring(segment_mean, 'mean', 'averaged')
_set_ragged_segment_docstring(segment_sqrt_n, 'sum divided by sqrt(N)',
                              'summed')

#===============================================================================
# ragged_reduce_<AGGREGATE>
#===============================================================================

# Docstring template used for ragged_reduce_<AGGREGATE> ops.
_RAGGED_REDUCE_DOCSTRING = """\
Computes the %(combination)s of elements across dimensions of a `RaggedTensor`.

  Reduces `input_tensor` along the dimensions given in `axis` by taking the
  %(combination)s of values.  If a reduced dimension has no elements for
  some index, then the value for that index will be %(default)s.

  The rank of the tensor is reduced by `1` for each entry in `axis`.  If
  `axis` is not specified, then all dimensions are reduced, and a scalar
  value is returned.
  Args:
    input_tensor: A `RaggedTensor` containing the values to be %(combined)s.
    axis: The dimensions to reduce.  May be `None` (to reduce all axes), an
      `int` (to reduce a single axis), a `list` or `tuple` of `int` (to reduce
      a given set of axes), or a `Tensor` with a constant value.  Must be in
      the range `[0, input_tensor.rank]`.
    name: A name prefix for the returned tensor (optional).
  Returns:
    A `RaggedTensor` containing the %(combined)s values.  The returned tensor
    has the same dtype as `data`, and its shape is given by removing the
    dimensions specified in `axis` from `input_tensor.shape`.  The `ragged_rank`
    of the returned tensor is given by substracting any ragged dimensions
    specified in `axis` from `input_tensor.ragged_rank`.
  Raises:
    ValueError: If `axis` contains a `Tensor` whose value is not constant.
  ####Example:
    ```python%(example)s    ```
"""
_RAGGED_REDUCE_SUM_EXAMPLE = """
    >>> rt = ragged.constant([[3, 1, 4], [1, 5], [9], [2, 6]])
    >>> ragged.reduce_sum(rt, axis=0).eval().tolist()
    [15, 12, 4]  # = [3+1+9+2, 1+5+6, 4]
    >>> ragged.reduce_sum(rt, axis=1).eval().tolist()
    [8, 6, 9, 8]  # = [3+1+4, 1+5, 9, 2+6]
"""
_RAGGED_REDUCE_PROD_EXAMPLE = """
    >>> rt = ragged.constant([[3, 1, 4], [1, 5], [9], [2, 6]])
    >>> ragged.reduce_prod(rt, axis=0).eval().tolist()
    [54, 30, 4]  # = [3*1*9*2, 1*5*6, 4]
    >>> ragged.reduce_prod(rt, axis=1).eval().tolist()
    [12, 5, 9, 12]  # = [3*1*4, 1*5, 9, 2*6]
"""
_RAGGED_REDUCE_MIN_EXAMPLE = """
    >>> rt = ragged.constant([[3, 1, 4], [1, 5], [9], [2, 6]])
    >>> ragged.reduce_min(rt, axis=0).eval().tolist()
    [1, 1, 4]  # = [min(3, 1, 9, 2), min(1, 5, 6), 4]
    >>> ragged.reduce_min(rt, axis=1).eval().tolist()
    [1, 1, 9, 2]  # = [min(3, 1, 4), min(1, 5), 9, min(2, 6)]
"""
_RAGGED_REDUCE_MAX_EXAMPLE = """
    >>> rt = ragged.constant([[3, 1, 4], [1, 5], [9], [2, 6]])
    >>> ragged.reduce_max(rt, axis=0).eval().tolist()
    [9, 6, 4]  # = [max(3, 1, 9, 2), max(1, 5, 6), 4]
    >>> ragged.reduce_max(rt, axis=1).eval().tolist()
    [4, 5, 9, 6]  # = [max(3, 1, 4), max(1, 5), 9, max(2, 6)]
"""
_RAGGED_REDUCE_MEAN_EXAMPLE = """
    >>> rt = ragged.constant([[3, 1, 4], [1, 5], [9], [2, 6]])
    >>> ragged.reduce_mean(rt, axis=0).eval().tolist()
    [3.75, 4, 4]  # = [mean(3, 1, 9, 2), mean(1, 5, 6), 4]
    >>> ragged.reduce_mean(rt, axis=1).eval().tolist()
    [2.66666, 3, 9, 4]  # = [mean(3, 1, 4), mean(1, 5), 9, mean(2, 6)]
"""
_RAGGED_REDUCE_ALL_EXAMPLE = """
    >>> rt = ragged.constant([[True, True], [True, True, False, True], [False, True]])
    >>> ragged.reduce_all(rt, axis=0).eval().tolist()
    [False, True, False, True]
    >>> ragged.reduce_all(rt, axis=1).eval().tolist()
    [True, False, False]
"""
_RAGGED_REDUCE_ANY_EXAMPLE = """
    >>> rt = ragged.constant([[True, True], [True, True, False, True], [False, True]])
    >>> ragged.reduce_any(rt, axis=0).eval().tolist()
    [True, True, False, True]
    >>> ragged.reduce_any(rt, axis=1).eval().tolist()
    [True, True, True]
"""


def _ragged_reduce_aggregate(reduce_op,
                             unsorted_segment_op,
                             rt_input,
                             axis,
                             keepdims,
                             name=None):
  """Aggregates across axes of a RaggedTensor using the given `Tensor` ops.

  Reduces `rt_input` along the dimensions given in `axis`.  The rank of the
  tensor is reduced by 1 for each entry in `axis`.  If `axis` is not specified,
  then all dimensions are reduced, and a scalar value is returned.

  This op assumes that `reduce_op` and `unsorted_segment_op` are associative;
  if not, then reducing multiple axes will return incorrect results.  (In
  particular, reducing multiple axes is currently implemented by reducing the
  axes one at a time.)

  Args:
    reduce_op: The tensorflow `op` that should be used to reduce values in
      uniform dimensions.  Must have the same signature and basic behavior as
      `reduce_sum`, `reduce_max`, etc.
    unsorted_segment_op: The tensorflow `op` that should be used to combine
      values in ragged dimensions.  Must have the same signature and basic
      behavior as `unsorted_segment_sum`, `unsorted_segment_max`, etc.
    rt_input: A `Tensor` or `RaggedTensor` containing the values to be reduced.
    axis: The axis or axes to reduce.  May be `None` (to reduce all axes), an
      `int` (to reduce a single axis), a `list` or `tuple` of `int` (to reduce a
      given set of axes), or a `Tensor` with a constant value.  Must be in the
      range `[0, rt_input.rank)`.
    keepdims: If true, retains reduced dimensions with length 1.
    name: A name prefix for the returned tensor (optional).

  Returns:
    A `RaggedTensor` containing the reduced values.  The returned tensor
    has the same dtype as `data`, and its shape is given by removing the
    dimensions specified in `axis` from `rt_input.shape`.  The `ragged_rank`
    of the returned tensor is given by substracting any ragged dimensions
    specified in `axis` from `rt_input.ragged_rank`.
  Raises:
    ValueError: If `axis` contains a `Tensor` whose value is not constant.
  """
  if not ragged_tensor.is_ragged(rt_input):
    return reduce_op(rt_input, axis, name=name)

  if keepdims:
    raise ValueError('keepdims=True is not supported for RaggedTensors.')

  if isinstance(axis, ops.Tensor):
    axis = tensor_util.constant_value(axis)
    if axis is None:
      raise ValueError('axis must be known at graph construction time.')
    if isinstance(axis, np.ndarray):
      axis = axis.tolist()

  # When reducing all axes, just ignore splits & reduce the inner values.
  if axis is None:
    return reduce_op(rt_input.flat_values, None, name=name)

  with ops.name_scope(name, 'RaggedReduce', [rt_input, axis]):
    if isinstance(axis, (tuple, list)):
      if not axis:
        return rt_input
      elif len(axis) == 1:
        axis = axis[0]
      else:
        # When reducing multiple axes, just reduce one at a time.  This is less
        # efficient, and only works for associative ops.  (In particular, it
        # does not work for reduce_mean.)  However, reducing multiple axes at
        # once will probably require a nontrivial c++ op.
        axis = sorted(axis)
        inner_reduced = _ragged_reduce_aggregate(reduce_op, unsorted_segment_op,
                                                 rt_input, axis[-1], keepdims)
        return _ragged_reduce_aggregate(reduce_op, unsorted_segment_op,
                                        inner_reduced, axis[:-1], keepdims)

    rt_input = ragged_tensor.convert_to_tensor_or_ragged_tensor(
        rt_input, name='rt_input')

    axis = ragged_util.get_positive_axis(axis, rt_input.shape.ndims)

    if axis == 0:
      # out[i_1, i_2, ..., i_N] = sum_{j} rt_input[j, i_1, i_2, ..., i_N]
      row_lengths = rt_input.row_splits[1:] - rt_input.row_splits[:-1]
      num_segments = math_ops.maximum(math_ops.reduce_max(row_lengths), 0)
      segment_ids = range(row_lengths).values
      return _ragged_segment_aggregate(unsorted_segment_op, rt_input.values,
                                       segment_ids, num_segments)
    elif axis == 1:
      # out[i_0, i_1, i_2, ..., i_N] = sum_{j} rt_input[i_0, j, i_2, ..., i_N]
      num_segments = array_ops.shape(rt_input.row_splits)[0] - 1
      segment_ids = segment_id_ops.row_splits_to_segment_ids(
          rt_input.row_splits)
      return _ragged_segment_aggregate(unsorted_segment_op, rt_input.values,
                                       segment_ids, num_segments)
    else:
      # out[i_0, ..., i_[axis-1], i_axis+1], ..., i_N] =
      #     sum_{j} rt_input [i_0, ..., i_[axis-1], j, i_axis+1], ..., i_N]
      return rt_input.with_values(
          _ragged_reduce_aggregate(reduce_op, unsorted_segment_op,
                                   rt_input.values, axis - 1, keepdims))


def reduce_sum(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  return _ragged_reduce_aggregate(math_ops.reduce_sum,
                                  math_ops.unsorted_segment_sum, input_tensor,
                                  axis, keepdims, name or 'RaggedReduceSum')


def reduce_prod(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  return _ragged_reduce_aggregate(math_ops.reduce_prod,
                                  math_ops.unsorted_segment_prod, input_tensor,
                                  axis, keepdims, name or 'RaggedReduceProd')


def reduce_min(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  return _ragged_reduce_aggregate(math_ops.reduce_min,
                                  math_ops.unsorted_segment_min, input_tensor,
                                  axis, keepdims, name or 'RaggedReduceMin')


def reduce_max(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  return _ragged_reduce_aggregate(math_ops.reduce_max,
                                  math_ops.unsorted_segment_max, input_tensor,
                                  axis, keepdims, name or 'RaggedReduceMax')


def reduce_mean(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  with ops.name_scope(name, 'RaggedReduceMean', [input_tensor, axis]):
    total = reduce_sum(input_tensor, axis, keepdims)
    if ragged_tensor.is_ragged(input_tensor):
      ones = ragged_tensor.RaggedTensor.from_nested_row_splits(
          array_ops.ones_like(input_tensor.flat_values),
          input_tensor.nested_row_splits)
    else:
      ones = array_ops.ones_like(input_tensor)
    count = reduce_sum(ones, axis, keepdims)
    if ragged_tensor.is_ragged(total):
      return ragged_tensor.RaggedTensor.from_nested_row_splits(
          total.flat_values / count.flat_values, total.nested_row_splits)
    else:
      return total / count


def _cast(input_tensor, dtype):
  return ragged_functional_ops.map_flat_values(math_ops.cast, input_tensor,
                                               dtype)


def reduce_all(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  with ops.name_scope(name, 'RaggedReduceAll', [input_tensor, axis]):
    return _cast(
        reduce_prod(_cast(input_tensor, dtypes.int32), axis, keepdims),
        dtypes.bool)


def reduce_any(input_tensor, axis=None, keepdims=None, name=None):
  """For docs, see: _RAGGED_REDUCE_DOCSTRING."""
  with ops.name_scope(name, 'RaggedReduceAny', [input_tensor, axis]):
    return _cast(
        reduce_sum(_cast(input_tensor, dtypes.int32), axis, keepdims),
        dtypes.bool)


def _set_ragged_reduce_docstring(func, combination, combined, default, example):
  func.__doc__ = _RAGGED_REDUCE_DOCSTRING % dict(
      combination=combination,
      combined=combined,
      default=default,
      example=example)


_set_ragged_reduce_docstring(reduce_sum, 'sum', 'summed', '0',
                             _RAGGED_REDUCE_SUM_EXAMPLE)
_set_ragged_reduce_docstring(reduce_prod, 'product', 'multiplied', '1',
                             _RAGGED_REDUCE_PROD_EXAMPLE)
_set_ragged_reduce_docstring(reduce_min, 'minimum', 'minimized',
                             '`input_tensor.dtype.min`',
                             _RAGGED_REDUCE_MIN_EXAMPLE)
_set_ragged_reduce_docstring(reduce_max, 'maximum', 'maximized',
                             '`input_tensor.dtype.max`',
                             _RAGGED_REDUCE_MAX_EXAMPLE)
_set_ragged_reduce_docstring(reduce_mean, 'mean', 'averaged', 'NaN',
                             _RAGGED_REDUCE_MEAN_EXAMPLE)

_set_ragged_reduce_docstring(reduce_all, 'logical and', 'and-ed', 'True',
                             _RAGGED_REDUCE_ALL_EXAMPLE)
_set_ragged_reduce_docstring(reduce_any, 'logical or', 'or-ed', 'False',
                             _RAGGED_REDUCE_ANY_EXAMPLE)
