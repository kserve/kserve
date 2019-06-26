# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
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
"""Tests for operations in eager execution."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import threading
import weakref

import numpy as np

from tensorflow.core.protobuf import config_pb2
from tensorflow.python.eager import context
from tensorflow.python.eager import execute
from tensorflow.python.eager import test
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import test_util
from tensorflow.python.layers import core
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.ops import sparse_ops


class OpsTest(test_util.TensorFlowTestCase):

  def testExecuteBasic(self):
    three = constant_op.constant(3)
    five = constant_op.constant(5)
    product = three * five
    self.assertAllEqual(15, product)

  def testMatMulGPU(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    three = constant_op.constant([[3.]]).gpu()
    five = constant_op.constant([[5.]]).gpu()
    product = math_ops.matmul(three, five)
    self.assertEqual([[15.0]], product.numpy())

  def testExecuteStringAttr(self):
    three = constant_op.constant(3.0)
    checked_three = array_ops.check_numerics(three,
                                             message='just checking')
    self.assertEqual([[3]], checked_three.numpy())

  def testExecuteFloatAttr(self):
    three = constant_op.constant(3.0)
    almost_three = constant_op.constant(2.8)
    almost_equal = math_ops.approximate_equal(
        three, almost_three, tolerance=0.3)
    self.assertTrue(almost_equal)

  def testExecuteIntAttr(self):
    three = constant_op.constant(3)
    four = constant_op.constant(4)
    total = math_ops.add_n([three, four])
    self.assertAllEqual(7, total)

  def testExecuteBoolAttr(self):
    three = constant_op.constant([[3]])
    five = constant_op.constant([[5]])
    product = math_ops.matmul(three, five, transpose_a=True)
    self.assertAllEqual([[15]], product)

  def testExecuteOneListOutput(self):
    split_dim = constant_op.constant(1)
    value = constant_op.constant([[0, 1, 2], [3, 4, 5]])
    x1, x2, x3 = array_ops.split(value, 3, axis=split_dim)
    self.assertAllEqual([[0], [3]], x1)
    self.assertAllEqual([[1], [4]], x2)
    self.assertAllEqual([[2], [5]], x3)

  def testGraphMode(self):
    graph = ops.Graph()
    with graph.as_default(), context.graph_mode():
      array_ops.placeholder(dtypes.int32)
    self.assertEqual(1, len(graph.get_operations()))

  # See comments on handling of int32 tensors on GPU in
  # EagerTensor.__init__.
  def testInt32CPUDefault(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    with context.device('/gpu:0'):
      r = constant_op.constant(1) + constant_op.constant(2)
    self.assertAllEqual(r, 3)

  def testExecuteListOutputLen1(self):
    split_dim = constant_op.constant(1)
    value = constant_op.constant([[0, 1, 2], [3, 4, 5]])
    result = array_ops.split(value, 1, axis=split_dim)
    self.assertTrue(isinstance(result, list))
    self.assertEqual(1, len(result))
    self.assertAllEqual([[0, 1, 2], [3, 4, 5]], result[0])

  def testExecuteListOutputLen0(self):
    empty = constant_op.constant([], dtype=dtypes.int32)
    result = array_ops.unstack(empty, 0)
    self.assertTrue(isinstance(result, list))
    self.assertEqual(0, len(result))

  def testExecuteMultipleNonListOutput(self):
    x = constant_op.constant([1, 2, 3, 4, 5, 6])
    y = constant_op.constant([1, 3, 5])
    result = array_ops.listdiff(x, y)
    out, idx = result
    self.assertTrue(out is result.out)
    self.assertTrue(idx is result.idx)
    self.assertAllEqual([2, 4, 6], out)
    self.assertAllEqual([1, 3, 5], idx)

  def testExecuteMultipleListOutput(self):
    split_dim = constant_op.constant(1, dtype=dtypes.int64)
    indices = constant_op.constant([[0, 2], [0, 4], [0, 5], [1, 0], [1, 1]],
                                   dtype=dtypes.int64)
    values = constant_op.constant([2, 3, 5, 7, 11])
    shape = constant_op.constant([2, 7], dtype=dtypes.int64)
    result = sparse_ops.gen_sparse_ops.sparse_split(
        split_dim,
        indices,
        values,
        shape,
        num_split=2)
    output_indices, output_values, output_shape = result
    self.assertEqual(2, len(output_indices))
    self.assertEqual(2, len(output_values))
    self.assertEqual(2, len(output_shape))
    self.assertEqual(output_indices, result.output_indices)
    self.assertEqual(output_values, result.output_values)
    self.assertEqual(output_shape, result.output_shape)
    self.assertAllEqual([[0, 2], [1, 0], [1, 1]], output_indices[0])
    self.assertAllEqual([[0, 0], [0, 1]], output_indices[1])
    self.assertAllEqual([2, 7, 11], output_values[0])
    self.assertAllEqual([3, 5], output_values[1])
    self.assertAllEqual([2, 4], output_shape[0])
    self.assertAllEqual([2, 3], output_shape[1])

  # TODO(josh11b): Test an op that has multiple outputs, some but not
  # all of which are lists. Examples: barrier_take_many (currently
  # unsupported since it uses a type list) or sdca_optimizer (I don't
  # have an example of legal inputs & outputs).

  def testComposition(self):
    x = constant_op.constant(1, dtype=dtypes.int32)
    three_x = x + x + x
    self.assertEquals(dtypes.int32, three_x.dtype)
    self.assertAllEqual(3, three_x)

  def testOperatorOverrides(self):
    # TODO(henrytan): test with negative number.
    a = constant_op.constant([1])
    b = constant_op.constant([2])

    self.assertAllEqual((-a), [-1])
    self.assertAllEqual(abs(b), [2])

    self.assertAllEqual((a + b), [3])
    self.assertAllEqual((a - b), [-1])
    self.assertAllEqual((a * b), [2])
    self.assertAllEqual((a * a), [1])

    self.assertAllEqual((a**b), [1])
    self.assertAllEqual((a / b), [1 / 2])
    self.assertAllEqual((a / a), [1])
    self.assertAllEqual((a % b), [1])

    self.assertAllEqual((a < b), [True])
    self.assertAllEqual((a <= b), [True])
    self.assertAllEqual((a > b), [False])
    self.assertAllEqual((a >= b), [False])
    self.assertAllEqual((a == b), False)
    self.assertAllEqual((a != b), True)

    self.assertAllEqual(1, a[constant_op.constant(0)])

  def test_basic_slice(self):
    npt = np.arange(1, 19, dtype=np.float32).reshape(3, 2, 3)
    t = constant_op.constant(npt)

    self.assertAllEqual(npt[:, :, :], t[:, :, :])
    self.assertAllEqual(npt[::, ::, ::], t[::, ::, ::])
    self.assertAllEqual(npt[::1, ::1, ::1], t[::1, ::1, ::1])
    self.assertAllEqual(npt[::1, ::5, ::2], t[::1, ::5, ::2])
    self.assertAllEqual(npt[::-1, :, :], t[::-1, :, :])
    self.assertAllEqual(npt[:, ::-1, :], t[:, ::-1, :])
    self.assertAllEqual(npt[:, :, ::-1], t[:, :, ::-1])
    self.assertAllEqual(npt[-2::-1, :, ::1], t[-2::-1, :, ::1])
    self.assertAllEqual(npt[-2::-1, :, ::2], t[-2::-1, :, ::2])

  def testDegenerateSlices(self):
    npt = np.arange(1, 19, dtype=np.float32).reshape(3, 2, 3)
    t = constant_op.constant(npt)
    # degenerate by offering a forward interval with a negative stride
    self.assertAllEqual(npt[0:-1:-1, :, :], t[0:-1:-1, :, :])
    # degenerate with a reverse interval with a positive stride
    self.assertAllEqual(npt[-1:0, :, :], t[-1:0, :, :])
    # empty interval in every dimension
    self.assertAllEqual(npt[-1:0, 2:2, 2:3:-1], t[-1:0, 2:2, 2:3:-1])

  def testEllipsis(self):
    npt = np.array(
        [[[[[1, 2], [3, 4], [5, 6]]], [[[7, 8], [9, 10], [11, 12]]]]])
    t = constant_op.constant(npt)

    self.assertAllEqual(npt[0:], t[0:])
    # implicit ellipsis
    self.assertAllEqual(npt[0:, ...], t[0:, ...])
    # ellipsis alone
    self.assertAllEqual(npt[...], t[...])
    # ellipsis at end
    self.assertAllEqual(npt[0:1, ...], t[0:1, ...])
    # ellipsis at begin
    self.assertAllEqual(npt[..., 0:1], t[..., 0:1])
    # ellipsis at middle
    self.assertAllEqual(npt[0:1, ..., 0:1], t[0:1, ..., 0:1])

  def testShrink(self):
    npt = np.array([[[[[1, 2, 4, 5], [5, 6, 7, 8], [9, 10, 11, 12]]],
                     [[[13, 14, 15, 16], [17, 18, 19, 20], [21, 22, 23, 24]]]]])
    t = constant_op.constant(npt)
    self.assertAllEqual(npt[:, :, :, :, 3], t[:, :, :, :, 3])
    self.assertAllEqual(npt[..., 3], t[..., 3])
    self.assertAllEqual(npt[:, 0], t[:, 0])
    self.assertAllEqual(npt[:, :, 0], t[:, :, 0])

  def testOpWithInputsOnDifferentDevices(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')

    # The GPU kernel for the Reshape op requires that the
    # shape input be on CPU.
    value = constant_op.constant([1., 2.]).gpu()
    shape = constant_op.constant([2, 1])
    reshaped = array_ops.reshape(value, shape)
    self.assertAllEqual([[1], [2]], reshaped.cpu())

  def testInt64(self):
    # Fill requires the first input to be an int32 tensor.
    self.assertAllEqual(
        [1.0, 1.0],
        array_ops.fill(constant_op.constant([2], dtype=dtypes.int64),
                       constant_op.constant(1)))

  def testOutputOnHostMemory(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    # The Shape op kernel on GPU places the output in host memory.
    value = constant_op.constant([1.]).gpu()
    shape = array_ops.shape(value)
    self.assertEqual([1], shape.numpy())

  def testSilentCopy(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    # Temporarily replace the context
    # pylint: disable=protected-access
    del context._context
    try:
      context._context = context.Context(
          device_policy=context.DEVICE_PLACEMENT_SILENT)
      cpu_tensor = constant_op.constant(1.0)
      gpu_tensor = cpu_tensor.gpu()
      self.assertAllEqual(cpu_tensor + gpu_tensor, 2.0)
    finally:
      del context._context
      context._context = context.Context()
    # pylint: enable=protected-access

  def testSoftPlacement(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    # Temporarily replace the context
    # pylint: disable=protected-access
    del context._context
    try:
      context._context = context.Context(
          device_policy=context.DEVICE_PLACEMENT_SILENT,
          config=config_pb2.ConfigProto(allow_soft_placement=True))
      cpu_tensor = constant_op.constant(1.0)
      result = cpu_tensor + cpu_tensor
      self.assertEqual(result.device,
                       '/job:localhost/replica:0/task:0/device:GPU:0')
    finally:
      del context._context
      context._context = context.Context()
    # pylint: enable=protected-access

  def testRandomUniform(self):
    scalar_shape = constant_op.constant([], dtype=dtypes.int32)

    x = random_ops.random_uniform(scalar_shape)
    self.assertEquals(0, x.shape.ndims)
    self.assertEquals(dtypes.float32, x.dtype)

    x = random_ops.random_uniform(
        scalar_shape, minval=constant_op.constant(5.),
        maxval=constant_op.constant(6.))
    self.assertLess(x, 6)
    self.assertGreaterEqual(x, 5)

  def testArgsToMatchingEagerDefault(self):
    # Uses default
    ctx = context.context()
    t, r = execute.args_to_matching_eager([[3, 4]], ctx, dtypes.int32)
    self.assertEquals(t, dtypes.int32)
    self.assertEquals(r[0].dtype, dtypes.int32)
    t, r = execute.args_to_matching_eager([[3, 4]], ctx, dtypes.int64)
    self.assertEquals(t, dtypes.int64)
    self.assertEquals(r[0].dtype, dtypes.int64)
    # Doesn't use default
    t, r = execute.args_to_matching_eager(
        [['string', 'arg']], ctx, dtypes.int32)
    self.assertEquals(t, dtypes.string)
    self.assertEquals(r[0].dtype, dtypes.string)

  def testFlattenLayer(self):
    flatten_layer = core.Flatten()
    x = constant_op.constant([[[-10, -20], [-30, -40]], [[10, 20], [30, 40]]])
    y = flatten_layer(x)
    self.assertAllEqual([[-10, -20, -30, -40], [10, 20, 30, 40]], y)

  def testIdentity(self):
    self.assertAllEqual(2, array_ops.identity(2))

  def testIdentityOnVariable(self):
    if not context.context().num_gpus():
      self.skipTest('No GPUs found')
    with context.device('/gpu:0'):
      v = resource_variable_ops.ResourceVariable(True)
    self.assertAllEqual(True, array_ops.identity(v))

  def testIncompatibleSetShape(self):
    x = constant_op.constant(1)
    with self.assertRaises(ValueError):
      x.set_shape((1, 2))

  def testCompatibleSetShape(self):
    x = constant_op.constant([[1, 2]])
    x.set_shape(tensor_shape.TensorShape([None, 2]))
    self.assertEqual(x.get_shape(), (1, 2))

  def testCastScalarToPrimitiveTypes(self):
    x = constant_op.constant(1.3)
    self.assertIsInstance(int(x), int)
    self.assertEqual(int(x), 1)
    self.assertIsInstance(float(x), float)
    self.assertAllClose(float(x), 1.3)

  def testCastNonScalarToPrimitiveTypesFails(self):
    x = constant_op.constant([1.3, 2])
    with self.assertRaises(TypeError):
      int(x)
    with self.assertRaises(TypeError):
      float(x)

  def testRange(self):
    x = constant_op.constant(2)
    self.assertEqual([0, 1], list(range(x)))

  def testFormatString(self):
    x = constant_op.constant(3.1415)
    self.assertEqual('3.14', '{:.2f}'.format(x))

  def testNoOpIsNone(self):
    self.assertTrue(control_flow_ops.no_op() is None)

  def testEagerContextPreservedAcrossThreads(self):
    def init_fn():
      self.assertTrue(context.executing_eagerly())
      with ops.init_scope():
        self.assertTrue(context.executing_eagerly())
        context_switches = context.context().context_switches
        self.assertEqual(len(context_switches.stack), 1)
        self.assertFalse(context_switches.stack[0].is_building_function)
        self.assertEqual(context_switches.stack[0].enter_context_fn,
                         context.eager_mode)

    self.assertTrue(context.executing_eagerly())
    t1 = threading.Thread(target=init_fn)
    t1.start()
    t1.join()

  def testWeakrefEagerTensor(self):
    x = constant_op.constant([[1.]])
    x.at1 = constant_op.constant([[2.]])
    x.at2 = 3.
    weak_x = weakref.ref(x)
    weak_xat1 = weakref.ref(x.at1)
    del x
    self.assertIs(weak_x(), None)
    self.assertIs(weak_xat1(), None)

  def testWeakKeyDictionaryTensor(self):
    weak_key_dict = weakref.WeakKeyDictionary()
    strong_x = constant_op.constant([[1.]])
    strong_y = constant_op.constant([[2.]])
    weak_key_dict[strong_x] = constant_op.constant([[3.]])
    weak_key_dict[strong_y] = constant_op.constant([[4.]])
    strong_y.a = constant_op.constant([[5.]])
    weak_x = weakref.ref(strong_x)
    del strong_x
    self.assertIs(weak_x(), None)
    self.assertEqual([strong_y], list(weak_key_dict))
    self.assertEqual(1, len(list(weak_key_dict)))
    self.assertEqual(1, len(weak_key_dict))
    del strong_y
    self.assertEqual([], list(weak_key_dict))


if __name__ == '__main__':
  test.main()
