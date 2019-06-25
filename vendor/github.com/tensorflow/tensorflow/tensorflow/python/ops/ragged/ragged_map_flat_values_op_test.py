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
"""Tests for ragged_functional_ops.map_flat_values."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops.ragged import ragged_factory_ops
from tensorflow.python.ops.ragged import ragged_functional_ops
from tensorflow.python.ops.ragged import ragged_tensor
from tensorflow.python.ops.ragged import ragged_test_util
from tensorflow.python.platform import googletest


@test_util.run_all_in_graph_and_eager_modes
class RaggedMapInnerValuesOpTest(ragged_test_util.RaggedTensorTestCase):

  def assertRaggedMapInnerValuesReturns(self,
                                        op,
                                        expected,
                                        args=(),
                                        kwargs=None):
    kwargs = kwargs or {}
    result = ragged_functional_ops.map_flat_values(op, *args, **kwargs)
    self.assertRaggedEqual(result, expected)

  def testDocStringExamples(self):
    """Test the examples in apply_op_to_ragged_values.__doc__."""
    rt = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5], [6]])
    v1 = ragged_functional_ops.map_flat_values(array_ops.ones_like, rt)
    v2 = ragged_functional_ops.map_flat_values(math_ops.multiply, rt, rt)
    v3 = ragged_functional_ops.map_flat_values(math_ops.add, rt, 5)
    self.assertRaggedEqual(v1, [[1, 1, 1], [], [1, 1], [1]])
    self.assertRaggedEqual(v2, [[1, 4, 9], [], [16, 25], [36]])
    self.assertRaggedEqual(v3, [[6, 7, 8], [], [9, 10], [11]])

  def testOpWithSingleRaggedTensorArg(self):
    tensor = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    self.assertRaggedMapInnerValuesReturns(
        op=array_ops.zeros_like,
        args=(tensor,),
        expected=[[0, 0, 0], [], [0, 0]])

  def testOpWithTwoRaggedTensorArgs(self):
    x = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5]])
    y = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply, args=(x, y), expected=[[3, 2, 12], [], [4, 25]])

  def testOpWithRaggedTensorAndScalarArgs(self):
    y = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply, args=(5, y), expected=[[5, 10, 15], [], [20, 25]])

  def testOpWithThreeRaggedTensorArgs(self):
    condition = ragged_factory_ops.constant(
        [[True, True, False], [], [True, False]])  # pyformat: disable
    x = ragged_factory_ops.constant([['a', 'b', 'c'], [], ['d', 'e']])
    y = ragged_factory_ops.constant([['A', 'B', 'C'], [], ['D', 'E']])
    self.assertRaggedMapInnerValuesReturns(
        op=array_ops.where,
        args=(condition, x, y),
        expected=[[b'a', b'b', b'C'], [], [b'd', b'E']])

  def testOpWithRaggedTensorListArg(self):
    x = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    y = ragged_factory_ops.constant([[10, 20, 30], [], [40, 50]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.add_n,
        args=([x, y, x],),
        expected=[[12, 24, 36], [], [48, 60]])

  def testOpWithKeywordArgs(self):
    x = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5]])
    y = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        kwargs=dict(x=x, y=y),
        expected=[[3, 2, 12], [], [4, 25]])

  def testOpWithMixedPositionalAndKeywordArgs(self):
    x = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5]])
    y = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        args=(x,),
        kwargs=dict(y=y),
        expected=[[3, 2, 12], [], [4, 25]])

  def testNonElementWiseOp(self):
    x = ragged_factory_ops.constant(
        [[[3, 1, 4], [1, 5, 9], [2, 6, 5]], [], [[3, 5, 8], [9, 7, 9]]],
        ragged_rank=1)
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.reduce_sum,
        kwargs={
            'input_tensor': x,
            'axis': 1,
        },
        expected=[[8, 15, 13], [], [16, 25]])

  def testOpWithRaggedRankGreaterThanOne(self):
    # ragged_rank=0
    x0 = [3, 1, 4, 1, 5, 9, 2, 6, 5]
    y0 = [1, 2, 3, 4, 5, 6, 7, 8, 9]
    self.assertRaggedEqual(
        math_ops.multiply(x0, y0), [3, 2, 12, 4, 25, 54, 14, 48, 45])

    # ragged_rank=1
    x1 = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5], [9, 2], [6, 5]])
    y1 = ragged_factory_ops.constant([[1, 2, 3], [], [4, 5], [6, 7], [8, 9]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        args=(x1, y1),
        expected=[[3, 2, 12], [], [4, 25], [54, 14], [48, 45]])

    # ragged_rank=2
    x2 = ragged_factory_ops.constant([[[3, 1, 4]], [], [[], [1, 5]],
                                      [[9, 2], [6, 5]]])
    y2 = ragged_factory_ops.constant([[[1, 2, 3]], [], [[], [4, 5]],
                                      [[6, 7], [8, 9]]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        args=(x2, y2),
        expected=[[[3, 2, 12]],          # row 0
                  [],                    # row 1
                  [[], [4, 25]],         # row 2
                  [[54, 14], [48, 45]]   # row 3
                 ])  # pyformat: disable

    # ragged_rank=3
    x3 = ragged_factory_ops.constant([[[[3, 1, 4]], []], [], [[[], [1, 5]]],
                                      [[[9, 2], [6, 5]]]])
    y3 = ragged_factory_ops.constant([[[[1, 2, 3]], []], [], [[[], [4, 5]]],
                                      [[[6, 7], [8, 9]]]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        args=(x3, y3),
        expected=[
            [[[3, 2, 12]], []],       # row 0
            [],                       # row 1
            [[[], [4, 25]]],          # row 2
            [[[54, 14], [48, 45]]]    # row 3
        ])  # pyformat: disable

  def testOpWithRaggedRankThree(self):
    x = ragged_factory_ops.constant([[[3, 1, 4]], [], [[], [1, 5]]])
    y = ragged_factory_ops.constant([[[1, 2, 3]], [], [[], [4, 5]]])
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply,
        args=(x, y),
        expected=[[[3, 2, 12]], [], [[], [4, 25]]])

  def testOpWithInnerValuesOnly(self):
    x = constant_op.constant([[1, 2], [3, 4], [5, 6]])
    y = constant_op.constant(2)
    self.assertRaggedMapInnerValuesReturns(
        op=math_ops.multiply, args=(x, y), expected=[[2, 4], [6, 8], [10, 12]])

  def testRaggedTensorSplitsRaggedRankMismatchError(self):
    x = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5]])
    y = ragged_factory_ops.constant([[[3, 1, 4], []], [], [[1, 5]]])
    self.assertRaisesRegexp(
        ValueError, r'Inputs must have identical ragged splits.*',
        ragged_functional_ops.map_flat_values, math_ops.add, x, y)

  def testRaggedTensorSplitsValueMismatchError(self):
    x = ragged_factory_ops.constant([[3, 1, 4], [], [1, 5]])
    y = ragged_factory_ops.constant([[1], [2, 3], [4, 5]])
    self.assertRaisesRegexp(errors.InvalidArgumentError,
                            r'Inputs must have identical ragged splits.*',
                            ragged_functional_ops.map_flat_values, math_ops.add,
                            x, y)

  def testRaggedTensorSplitsMismatchErrorAtRuntime(self):
    splits1 = array_ops.placeholder_with_default(
        constant_op.constant([0, 3, 3, 5], dtypes.int64), None)
    splits2 = array_ops.placeholder_with_default(
        constant_op.constant([0, 1, 3, 5], dtypes.int64), None)
    x = ragged_tensor.RaggedTensor.from_row_splits([3, 1, 4, 1, 5], splits1)
    y = ragged_tensor.RaggedTensor.from_row_splits([1, 2, 3, 4, 5], splits2)
    with self.assertRaisesRegexp(errors.InvalidArgumentError,
                                 r'.*Inputs must have identical ragged splits'):
      self.evaluate(ragged_functional_ops.map_flat_values(math_ops.add, x, y))


if __name__ == '__main__':
  googletest.main()
