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
"""Tests for RaggedTensor operator dispatch."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized
import numpy as np

from tensorflow.python.eager import context
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import clip_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import parsing_ops
from tensorflow.python.ops import string_ops
from tensorflow.python.ops.ragged import ragged_factory_ops
from tensorflow.python.ops.ragged import ragged_tensor
from tensorflow.python.ops.ragged import ragged_test_util
from tensorflow.python.platform import googletest

# Constants listing various op types to test.  Each operation
# should be included in at least one list below, or tested separately if
# necessary (e.g., because it expects additional arguments).
UNARY_FLOAT_OPS = [
    math_ops.abs,
    math_ops.acos,
    math_ops.acosh,
    math_ops.angle,
    math_ops.asin,
    math_ops.asinh,
    math_ops.atan,
    math_ops.atanh,
    math_ops.ceil,
    math_ops.conj,
    math_ops.cos,
    math_ops.cosh,
    math_ops.digamma,
    math_ops.erf,
    math_ops.erfc,
    math_ops.exp,
    math_ops.expm1,
    math_ops.floor,
    math_ops.imag,
    math_ops.is_finite,
    math_ops.is_inf,
    math_ops.is_nan,
    math_ops.lgamma,
    math_ops.log,
    math_ops.log1p,
    math_ops.log_sigmoid,
    math_ops.negative,
    math_ops.real,
    math_ops.reciprocal,
    math_ops.rint,
    math_ops.round,
    math_ops.rsqrt,
    math_ops.sign,
    math_ops.sin,
    math_ops.sinh,
    math_ops.sqrt,
    math_ops.square,
    math_ops.tan,
    array_ops.identity,
    array_ops.ones_like,
    array_ops.zeros_like,
]
UNARY_BOOL_OPS = [
    math_ops.logical_not,
]
UNARY_STRING_OPS = [
    string_ops.decode_base64,
    string_ops.encode_base64,
    string_ops.string_strip,
    parsing_ops.decode_compressed,
]
BINARY_FLOAT_OPS = [
    math_ops.add,
    math_ops.atan2,
    math_ops.complex,
    math_ops.div_no_nan,
    math_ops.divide,
    math_ops.equal,
    math_ops.floordiv,
    math_ops.floormod,
    math_ops.greater,
    math_ops.greater_equal,
    math_ops.less,
    math_ops.less_equal,
    math_ops.maximum,
    math_ops.minimum,
    math_ops.multiply,
    math_ops.not_equal,
    math_ops.pow,
    math_ops.realdiv,
    math_ops.squared_difference,
    math_ops.subtract,
    math_ops.truediv,
]
BINARY_BOOL_OPS = [
    math_ops.logical_and,
    math_ops.logical_or,
    math_ops.logical_xor,
]
UNARY_INT_OPS = [
    string_ops.unicode_script,
]
BINARY_INT_OPS = [
    math_ops.truncatediv,
    math_ops.truncatemod,
]


@test_util.run_all_in_graph_and_eager_modes
class RaggedElementwiseOpsTest(ragged_test_util.RaggedTensorTestCase,
                               parameterized.TestCase):

  def assertSameShape(self, x, y):
    """Checks that x and y have the same shape (including ragged shapes)."""
    if isinstance(x, ragged_tensor.RaggedTensor):
      self.assertIsInstance(y, ragged_tensor.RaggedTensor)
      self.assertEqual(x.ragged_rank, y.ragged_rank)
      for (x_splits, y_splits) in zip(x.nested_row_splits, y.nested_row_splits):
        self.assertAllEqual(x_splits, y_splits)
      self.assertAllEqual(
          array_ops.shape(x.flat_values), array_ops.shape(y.flat_values))
    else:
      self.assertIsInstance(y, ops.Tensor)
      self.assertAllEqual(array_ops.shape(x), array_ops.shape(y))

  @parameterized.parameters(
      #=========================================================================
      # Test different input shapes.
      #=========================================================================
      [
          # 0-dimensional input
          {'x': 12},
          # 1-dimensional input
          {'x': [1, -2, 3]},
          # 2-dimensional input
          {'x': [[-2, 3], [-3, 4]]},
          {'x': ragged_factory_ops.constant_value(
              [[-2, 3], [-3]], ragged_rank=1)},
          # 3-dimensional inputs
          {'x': [[[-2, 3], [3, 4]], [[7, 6], [5, 4]]]},
          {'x': ragged_factory_ops.constant_value(
              [[[-2, 3], [3, 4]], [[7, 6]]],
              ragged_rank=1)},
          {'x': ragged_factory_ops.constant_value(
              [[[-2, 3, 4], []], [[7, 6]], []],
              ragged_rank=2)},
          ] +
      #=========================================================================
      # Test each unary op.
      #=========================================================================
      [{'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]), 'op': op}
       for op in UNARY_FLOAT_OPS] +
      [{'x': ragged_factory_ops.constant_value([[True, False], [True]]),
        'op': op}
       for op in UNARY_BOOL_OPS] +
      [{'x': ragged_factory_ops.constant_value([[18, 512], [12412]], np.int32),
        'op': op}
       for op in UNARY_INT_OPS] +
      [{'x': ragged_factory_ops.constant_value([['abcd', 'efgh'],
                                                ['aabbccdd']]),
        'op': op}
       for op in UNARY_STRING_OPS] +
      [
          {'op': clip_ops.clip_by_value,
           'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]),
           'clip_value_min': 0.1, 'clip_value_max': 4.0},
          {'op': math_ops.cast,
           'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]),
           'dtype': dtypes.int32},
          {'op': math_ops.saturate_cast,
           'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]),
           'dtype': dtypes.int32},
          {'op': string_ops.string_to_hash_bucket,
           'x': ragged_factory_ops.constant_value(
               [['abcd', 'efgh'], ['aabbccdd']]),
           'num_buckets': 1000},
          {'op': string_ops.string_to_hash_bucket_fast,
           'x': ragged_factory_ops.constant_value(
               [['abcd', 'efgh'], ['aabbccdd']]),
           'num_buckets': 1000},
          {'op': string_ops.string_to_hash_bucket_strong,
           'x': ragged_factory_ops.constant_value(
               [['abcd', 'efgh'], ['aabbccdd']]),
           'num_buckets': 1000,
           'key': [1231, 12512]},
          {'op': string_ops.string_to_number,
           'x': ragged_factory_ops.constant_value([['-2.0', '3.0'], ['-3.0']])},
          {'op': string_ops.regex_full_match,
           'x': ragged_factory_ops.constant_value([['hello', '123'], ['1+1']]),
           'pattern': r'\w+'},
          {'op': string_ops.regex_replace,
           'x': ragged_factory_ops.constant_value([['hello', '123'], ['1+1']]),
           'pattern': r'\d',
           'rewrite': '#'},
          {'op': string_ops.substr,
           'x': ragged_factory_ops.constant_value([['hello', '123'], ['1+1']]),
           'pos': 2, 'len': 3},
          {'op': array_ops.check_numerics,
           'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]),
           'message': 'check-numerics'},
      ]
      )  # pyformat: disable
  def testUnaryElementwiseOp(self, x, op=math_ops.abs, **extra_args):
    x = ragged_tensor.convert_to_tensor_or_ragged_tensor(x)
    result = op(x, **extra_args)

    # Run the wrapped op on the dense values, for comparison.
    dense_x = x.flat_values if isinstance(x, ragged_tensor.RaggedTensor) else x
    expected_flat_values = array_ops.reshape(op(dense_x, **extra_args), [-1])

    # Check that the result has the expected shape.
    self.assertSameShape(x, result)

    # Check that the result has the expected (flattened) values.
    if isinstance(result, ragged_tensor.RaggedTensor):
      result_flat_values = array_ops.reshape(result.flat_values, [-1])
    else:
      result_flat_values = array_ops.reshape(result, [-1])
    self.assertAllEqual(expected_flat_values, result_flat_values)

  @parameterized.parameters(
      [
          #=====================================================================
          # Without broadcasting -- i.e., shapes match exactly.
          #=====================================================================
          # Shapes: x:(), y:()
          {'x': 12,
           'y': 8},
          # Shapes: x:(3,), y:(3,)
          {'x': [7, 8, 9],
           'y': [1, -2, 3]},
          # Shapes: x:(2, 2), y:(2, 2)
          {'x': [[-2, 3], [-3, -4]],
           'y': [[1, 2], [3, 4]]},
          # Shapes: x:(2, None), y:(2, None)
          {'x': ragged_factory_ops.constant_value([[-2, 3], [-3]]),
           'y': ragged_factory_ops.constant_value([[5, 6], [7]])},
          # Shapes: x:(2, 2, 2), y:(2, 2, 2)
          {'x': [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
           'y': [[[9, 3], [3, 4]], [[5, 2], [7, 6]]]},
          # Shapes: x:(2, None, None), y: (2, None, None)
          {'x': ragged_factory_ops.constant_value(
              [[[1, 2], [3], [4]], [[], [5, 7, 8]]]),
           'y': ragged_factory_ops.constant_value(
               [[[3, 8], [2], [5]], [[], [1, 9, 8]]])},
          # Shapes: x:(2, None, 2), y: (2, None, 2)
          {'x': ragged_factory_ops.constant_value(
              [[[1, 2]], [[3, 4], [5, 6], [7, 8]]],
              ragged_rank=1),
           'y': ragged_factory_ops.constant_value(
               [[[9, 3]], [[5, 2], [3, 4], [7, 6]]],
               ragged_rank=1)},

          #=====================================================================
          # With broadcasting
          #=====================================================================
          # Shapes: x:(), y:(3,)
          {'x': 12,                                 # Broadcast () -> (3,)
           'y': [1, -2, 3]},
          # Shapes: x:(1,), y:(3,)
          {'x': [12],                               # Broadcast (1,) -> (3,)
           'y': [1, -2, 3]},
          # Shapes: x:(), y:(2, 2)
          {'x': 12,                                 # Broadcast () -> (2, 2)
           'y': [[1, 2], [3, 4]]},
          # Shapes: x:(1,), y:(2, 2)
          {'x': 12,                                 # Broadcast (1,) -> (2, 2)
           'y': [[1, 2], [3, 4]]},
          # Shapes: x:(2, 1), y:(2, 2)
          {'x': [[10], [20]],                       # Broadcast (2, 1) -> (2, 2)
           'y': [[1, 2], [3, 4]]},
          # Shapes: x:(), y:(2, None)
          {'x': 10,                                 # Broadcast () -> (2, None)
           'y': ragged_factory_ops.constant_value(
               [[1, 2], [3]], dtype=np.int32)},
          # TODO(edloper): Add tests for more advanced broadcasting, once we add
          # support for it.

          #=====================================================================
          # Keyword Args
          #=====================================================================
          {'x': ragged_factory_ops.constant_value(
              [[[1, 2], [3], [4]], [[], [5, 7, 8]]]),
           'y': ragged_factory_ops.constant_value(
               [[[3, 8], [2], [5]], [[], [1, 9, 8]]]),
           'use_kwargs': ('x', 'y')},
          {'x': ragged_factory_ops.constant_value(
              [[[1, 2]], [[3, 4], [5, 6], [7, 8]]],
              ragged_rank=1),
           'y': ragged_factory_ops.constant_value(
               [[[9, 3]], [[5, 2], [3, 4], [7, 6]]],
               ragged_rank=1),
           'use_kwargs': ('x', 'y')},
          {'x': ragged_factory_ops.constant_value(
              [[[1, 2]], [[3, 4], [5, 6], [7, 8]]],
              ragged_rank=1),
           'y': ragged_factory_ops.constant_value(
               [[[9, 3]], [[5, 2], [3, 4], [7, 6]]],
               ragged_rank=1),
           'use_kwargs': ('x',)},
      ] +
      #=========================================================================
      # Test each unary op.
      #=========================================================================
      [{'x': ragged_factory_ops.constant_value([[-2.0, 3.0], [-3.0]]),
        'y': ragged_factory_ops.constant_value([[5.0, 1.0], [12.0]]),
        'op': op}
       for op in BINARY_FLOAT_OPS] +
      [{'x': ragged_factory_ops.constant_value([[-2, 3], [-3]]),
        'y': ragged_factory_ops.constant_value([[5, 1], [12]]),
        'op': op}
       for op in BINARY_INT_OPS] +
      [{'x': ragged_factory_ops.constant_value([[True, True], [False]]),
        'y': ragged_factory_ops.constant_value([[False, True], [False]]),
        'op': op}
       for op in BINARY_BOOL_OPS]
      )  # pyformat: disable
  def testBinaryElementwiseOp(self, x, y, op=math_ops.add, **extra_args):
    use_kwargs = extra_args.pop('use_kwargs', ())
    x = ragged_tensor.convert_to_tensor_or_ragged_tensor(x)
    y = ragged_tensor.convert_to_tensor_or_ragged_tensor(y)
    if 'x' in use_kwargs and 'y' in use_kwargs:
      result = op(x=x, y=y, **extra_args)
    elif 'y' in use_kwargs:
      result = op(x, y=y, **extra_args)
    else:
      result = op(x, y, **extra_args)

    # Run the wrapped op on the dense values, for comparison.
    dense_x = x.flat_values if isinstance(x, ragged_tensor.RaggedTensor) else x
    dense_y = y.flat_values if isinstance(y, ragged_tensor.RaggedTensor) else y
    expected_flat_values = array_ops.reshape(
        op(dense_x, dense_y, **extra_args), [-1])

    # Check that the result has the expected shape.
    self.assertSameShape(y, result)

    # Check that the result has the expected (flattened) values.
    if isinstance(result, ragged_tensor.RaggedTensor):
      result_flat_values = array_ops.reshape(result.flat_values, [-1])
    else:
      result_flat_values = array_ops.reshape(result, [-1])
    self.assertAllEqual(expected_flat_values, result_flat_values)

  @parameterized.parameters(
      [
          {'inputs': (12, 8, 3)},
          {'inputs': ([1, 2, 3], [7, 8, 9], [3, 6, 9])},
          {'inputs': ([[1, 2]], [[3, 4]], [[5, 6]])},
          {'inputs': (ragged_factory_ops.constant_value([[1, 3], [-3]]),
                      ragged_factory_ops.constant_value([[4, 7], [88]]),
                      ragged_factory_ops.constant_value([[2, 9], [12]]))},
          {'inputs': (ragged_factory_ops.constant_value(
              [[[1, 3], [-3]], [[1]]]),
                      ragged_factory_ops.constant_value(
                          [[[4, 7], [88]], [[2]]]),
                      ragged_factory_ops.constant_value(
                          [[[2, 9], [12]], [[8]]]))},
          {'inputs': (
              ragged_factory_ops.constant_value([[[1, 3], [3, 4]], [[1, 5]]],
                                                ragged_rank=1),
              ragged_factory_ops.constant_value([[[4, 7], [1, 2]], [[2, 2]]],
                                                ragged_rank=1),
              ragged_factory_ops.constant_value([[[2, 9], [5, 2]], [[8, 0]]],
                                                ragged_rank=1))},
          {'inputs': (
              ragged_factory_ops.constant_value([[[1, 3], [-3]], [[1]]]),
              ragged_factory_ops.constant_value([[[4, 7], [88]], [[2]]]),
              ragged_factory_ops.constant_value([[[2, 9], [12]], [[8]]])),
           'use_kwargs': True},
      ] + [
          {'op': math_ops.add_n,
           'inputs': (ragged_factory_ops.constant_value([[1, 3], [-3]]),
                      ragged_factory_ops.constant_value([[4, 7], [88]]),
                      ragged_factory_ops.constant_value([[2, 9], [12]]))},
          {'op': string_ops.string_join,
           'inputs': (
               ragged_factory_ops.constant_value([['a', 'b'], ['c']]),
               ragged_factory_ops.constant_value([['foo', 'bar'], ['baz']]),
               ragged_factory_ops.constant_value([['2', '9'], ['12']]))},
      ])  # pyformat: disable
  def testListValuedElementwiseOp(self, inputs, op=math_ops.add_n,
                                  **extra_args):
    use_kwargs = extra_args.pop('use_kwargs', False)
    inputs = [
        ragged_tensor.convert_to_tensor_or_ragged_tensor(x) for x in inputs
    ]
    if use_kwargs:
      result = op(inputs=inputs, **extra_args)
    else:
      result = op(inputs, **extra_args)

    # Run the wrapped op on the dense values, for comparison.
    dense_inputs = [
        x.flat_values if isinstance(x, ragged_tensor.RaggedTensor) else x
        for x in inputs
    ]
    expected_flat_values = array_ops.reshape(
        op(dense_inputs, **extra_args), [-1])

    # Check that the result has the expected shape.
    self.assertSameShape(inputs[0], result)

    # Check that the result has the expected (flattened) values.
    if isinstance(result, ragged_tensor.RaggedTensor):
      result_flat_values = array_ops.reshape(result.flat_values, [-1])
    else:
      result_flat_values = array_ops.reshape(result, [-1])
    self.assertAllEqual(expected_flat_values, result_flat_values)

  def testElementwiseOpUnknownRankError(self):
    if context.executing_eagerly():
      return
    x = ragged_factory_ops.constant([[1, 2], [3]])
    y = ragged_tensor.RaggedTensor.from_row_splits(
        array_ops.placeholder_with_default([1, 2, 3], shape=None), x.row_splits)
    with self.assertRaisesRegexp(ValueError,
                                 r'Unable to broadcast: unknown rank'):
      math_ops.add(x, y)

  @parameterized.parameters([
      dict(
          x=ragged_factory_ops.constant_value([[1, 2], [3]]),
          y=[[10]],
          expected=[[11, 12], [13]]),
      dict(
          x=ragged_factory_ops.constant_value([[[1, 2], [3, 4]], [[5]]],
                                              ragged_rank=2),
          y=ragged_factory_ops.constant_value([[[10], [20]], [[30]]],
                                              ragged_rank=1),
          expected=[[[11, 12], [23, 24]], [[35]]]),
      dict(
          x=ragged_factory_ops.constant_value([[[1]]]),
          y=ragged_factory_ops.constant_value([[1]]),
          expected=[[[2]]]),
  ])
  def testElementwiseOpBroadcast(self, x, y, expected):
    x = ragged_tensor.convert_to_tensor_or_ragged_tensor(x, dtype=dtypes.int32)
    y = ragged_tensor.convert_to_tensor_or_ragged_tensor(y, dtype=dtypes.int32)
    result = x + y
    self.assertRaggedEqual(result, expected)

  def testElementwiseOpShapeMismatch(self):
    x = ragged_factory_ops.constant([[1, 2, 3], [4, 5]])
    y = ragged_factory_ops.constant([[1, 2, 3], [4, 5, 6]])
    with self.assertRaises(errors.InvalidArgumentError):
      self.evaluate(math_ops.add(x, y))

  def testBinaryOpSparseAndRagged(self):
    x = ragged_factory_ops.constant([[1, 2, 3], [4, 5]])
    y = sparse_tensor.SparseTensor([[0, 0], [0, 1], [2, 0]], [1, 2, 3], [3, 2])
    with self.assertRaises((TypeError, ValueError)):
      self.evaluate(math_ops.add(x, y))

    with self.assertRaises((TypeError, ValueError)):
      self.evaluate(math_ops.add_n([x, y]))

  @parameterized.parameters([
      dict(
          op=array_ops.batch_gather,
          args=(ragged_factory_ops.constant_value([[5, 6, 7], [8, 9]]),
                ragged_factory_ops.constant_value([[2, 1, 0], [1]])),
          expected=ragged_factory_ops.constant_value([[7, 6, 5], [9]])),
      dict(
          op=array_ops.concat,
          args=([
              ragged_factory_ops.constant_value([[1, 2, 3], [4]],
                                                dtype=np.int32),
              np.array([[5, 6]], dtype=np.int32)
          ],),
          kwargs={'axis': 0},
          expected=ragged_factory_ops.constant_value([[1, 2, 3], [4], [5, 6]])),
      dict(
          op=array_ops.expand_dims,
          kwargs={
              'input': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'axis': 0
          },
          expected=ragged_factory_ops.constant_value([[[1, 2], [3]]])),
      dict(
          op=array_ops.expand_dims_v2,
          kwargs={
              'input': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'axis': -1
          },
          expected=ragged_factory_ops.constant_value([[[1], [2]], [[3]]],
                                                     ragged_rank=1),
      ),
      dict(
          op=array_ops.gather,
          kwargs={
              'params': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'indices': [1, 0, 1]
          },
          expected=ragged_factory_ops.constant_value([[3], [1, 2], [3]])),
      dict(
          op=array_ops.gather_v2,
          kwargs={
              'params': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'indices': ragged_factory_ops.constant_value([[1, 0], [1]])
          },
          expected=ragged_factory_ops.constant_value([[[3], [1, 2]], [[3]]])),
      dict(
          op=array_ops.gather_nd,
          kwargs={
              'params': ragged_factory_ops.constant_value([[7, 8], [9]]),
              'indices': [[0, 1], [1, 0], [0, 0]]
          },
          expected=ragged_factory_ops.constant_value([8, 9, 7])),
      dict(
          op=array_ops.stack,
          args=([
              ragged_factory_ops.constant_value([[1, 2, 3], [4]],
                                                dtype=np.int32),
              np.array([[5, 6]], dtype=np.int32)
          ],),
          expected=ragged_factory_ops.constant_value([[[1, 2, 3], [4]],
                                                      [[5, 6]]])),
      dict(
          op=array_ops.tile,
          args=([
              ragged_factory_ops.constant_value([[1, 2], [3]], dtype=np.int32),
              [2, 3]
          ]),
          expected=ragged_factory_ops.constant_value([[1, 2, 1, 2, 1, 2],
                                                      [3, 3, 3],
                                                      [1, 2, 1, 2, 1, 2],
                                                      [3, 3, 3]])),
      dict(
          op=array_ops.where,
          args=(ragged_factory_ops.constant_value([[True, False], [True]]),
                ragged_factory_ops.constant_value([[b'A', b'B'], [b'C']]),
                ragged_factory_ops.constant_value([[b'a', b'b'], [b'c']])),
          expected=ragged_factory_ops.constant_value([[b'A', b'b'], [b'C']])),
      dict(
          op=array_ops.where,
          args=(ragged_factory_ops.constant_value([[True, False], [True]]),),
          expected=[[0, 0], [1, 0]]),
      dict(
          op=math_ops.unsorted_segment_sum,
          kwargs={
              'data': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'segment_ids': ragged_factory_ops.constant_value([[0, 2], [0]]),
              'num_segments': 3
          },
          expected=[4, 0, 2]),
      dict(
          op=math_ops.unsorted_segment_prod,
          kwargs={
              'data': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'segment_ids': ragged_factory_ops.constant_value([[0, 2], [0]]),
              'num_segments': 3
          },
          expected=[3, 1, 2]),
      dict(
          op=math_ops.unsorted_segment_min,
          kwargs={
              'data': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'segment_ids': ragged_factory_ops.constant_value([[0, 1], [0]]),
              'num_segments': 2
          },
          expected=[1, 2]),
      dict(
          op=math_ops.unsorted_segment_max,
          kwargs={
              'data': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'segment_ids': ragged_factory_ops.constant_value([[0, 1], [0]]),
              'num_segments': 2
          },
          expected=[3, 2]),
      dict(
          op=math_ops.unsorted_segment_mean,
          kwargs={
              'data': ragged_factory_ops.constant_value([[1, 2], [3]]),
              'segment_ids': ragged_factory_ops.constant_value([[0, 1], [0]]),
              'num_segments': 2
          },
          expected=[2, 2]),
      dict(
          op=math_ops.unsorted_segment_sqrt_n,
          kwargs={
              'data':
                  ragged_factory_ops.constant_value([[1.0, 2.0],
                                                     [3.0, 4.0, 6.0]]),
              'segment_ids':
                  ragged_factory_ops.constant_value([[0, 1], [0, 0, 0]]),
              'num_segments':
                  2
          },
          expected=[7.0, 2.0]),
      dict(
          op=math_ops.reduce_sum,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[1, 2], [3, 4, 5]]),
              'axis':
                  1
          },
          expected=[3, 12]),
      dict(
          op=math_ops.reduce_prod,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[1, 2], [3, 4, 5]]),
              'axis':
                  1
          },
          expected=[2, 60]),
      dict(
          op=math_ops.reduce_min,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[1, 2], [3, 4, 5]]),
              'axis':
                  1
          },
          expected=[1, 3]),
      dict(
          op=math_ops.reduce_max,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[1, 2], [3, 4, 5]]),
              'axis':
                  1
          },
          expected=[2, 5]),
      dict(
          op=math_ops.reduce_mean,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[1, 3], [3, 4, 5]]),
              'axis':
                  1
          },
          expected=[2, 4]),
      dict(
          op=math_ops.reduce_any,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[True, False],
                                                     [True, True, True]]),
              'axis':
                  1
          },
          expected=[True, True]),
      dict(
          op=math_ops.reduce_all,
          kwargs={
              'input_tensor':
                  ragged_factory_ops.constant_value([[True, False],
                                                     [True, True, True]]),
              'axis':
                  1
          },
          expected=[False, True]),
  ])
  def testRaggedDispatch(self, op, expected, args=(), kwargs=None):
    if kwargs is None: kwargs = {}
    result = op(*args, **kwargs)
    self.assertRaggedEqual(result, expected)


if __name__ == '__main__':
  googletest.main()
