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
"""Tests for tensorflow.ops.math_ops."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.eager import context
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import gradients
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import googletest


exp = np.exp
log = np.log


class ReduceTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testReduceAllDims(self):
    x = np.array([[1, 2, 3], [4, 5, 6]], dtype=np.int32)
    with test_util.device(use_gpu=True):
      y_tf = self.evaluate(math_ops.reduce_sum(x))
      self.assertEqual(y_tf, 21)

  @test_util.run_in_graph_and_eager_modes
  def testReduceExplicitAxes(self):
    x = np.array([[1, 2, 3], [4, 5, 6]], dtype=np.int32)
    with test_util.device(use_gpu=True):
      for axis in (0, -2, (0, 0), (0, -2)):
        self.assertAllEqual(self.evaluate(math_ops.reduce_sum(x, axis=axis)),
                            [5, 7, 9])
      for axis in (1, -1, (1, 1), (1, -1)):
        self.assertAllEqual(self.evaluate(math_ops.reduce_sum(x, axis=axis)),
                            [6, 15])
      for axis in (None, (0, 1), (-1, -2), (-2, -1, 0, 1)):
        self.assertEqual(self.evaluate(math_ops.reduce_sum(x, axis=axis)), 21)

  @test_util.run_in_graph_and_eager_modes
  def testReduceInvalidAxis(self):
    if context.executing_eagerly():
      # The shape check is in run a graph construction time. In eager mode,
      # it misses the check, magically return result given wrong shape.
      return
    x = np.array([[1, 2, 3], [4, 5, 6]], dtype=np.int32)
    axis = np.array([[0], [1]])
    with self.assertRaisesRegexp(ValueError, "must be at most rank 1"):
      math_ops.reduce_sum(x, axis)

  @test_util.run_in_graph_and_eager_modes
  def testReduceVar(self):
    x = np.array([[0, 0, 0], [0, 0, 0]], "float32")
    self.assertAllClose(self.evaluate(math_ops.reduce_variance(x)), 0)
    self.assertAllClose(
        self.evaluate(math_ops.reduce_variance(x, axis=0)), [0, 0, 0])

    x = np.array([[0, 2, 1, 1], [1, 2, 0, 1]], "float32")
    self.assertAllClose(self.evaluate(math_ops.reduce_variance(x)), 0.5)

  @test_util.run_in_graph_and_eager_modes
  def testReduceStd(self):
    x = np.array([[0, 0, 0], [0, 0, 0]], "float32")
    self.assertAllClose(self.evaluate(math_ops.reduce_std(x)), 0)
    self.assertAllClose(
        self.evaluate(math_ops.reduce_std(x, axis=0)), [0, 0, 0])

    x = np.array([[1, 2, 1, 1], [1, 1, 0, 1]], "float32")
    self.assertAllClose(self.evaluate(math_ops.reduce_std(x)), 0.5)


class LogSumExpTest(test_util.TensorFlowTestCase):

  @test_util.run_deprecated_v1
  def testReduceLogSumExp(self):
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.random.rand(5, 5).astype(dtype)
      with self.cached_session(use_gpu=True):
        y_tf_np = math_ops.reduce_logsumexp(x_np).eval()
        y_np = log(np.sum(exp(x_np)))
        self.assertAllClose(y_tf_np, y_np)

  def testReductionIndices(self):
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.random.rand(5, 5).astype(dtype)
      with self.cached_session(use_gpu=True):
        y_tf = math_ops.reduce_logsumexp(x_np, axis=[0])
        y_np = log(np.sum(exp(x_np), axis=0))
        self.assertShapeEqual(y_np, y_tf)
        y_tf_np = self.evaluate(y_tf)
        self.assertAllClose(y_tf_np, y_np)

  def testReductionIndices2(self):
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.random.rand(5, 5).astype(dtype)
      with self.cached_session(use_gpu=True):
        y_tf = math_ops.reduce_logsumexp(x_np, axis=0)
        y_np = log(np.sum(exp(x_np), axis=0))
        self.assertShapeEqual(y_np, y_tf)
        y_tf_np = self.evaluate(y_tf)
        self.assertAllClose(y_tf_np, y_np)

  @test_util.run_deprecated_v1
  def testKeepDims(self):
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.random.rand(5, 5).astype(dtype)
      with self.cached_session(use_gpu=True):
        y_tf_np = math_ops.reduce_logsumexp(x_np, keepdims=True).eval()
        self.assertEqual(y_tf_np.ndim, x_np.ndim)
        y_np = log(np.sum(exp(x_np), keepdims=True))
        self.assertAllClose(y_tf_np, y_np)

  @test_util.run_deprecated_v1
  def testOverflow(self):
    x = [1000, 1001, 1002, 1003]
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.array(x, dtype=dtype)
      max_np = np.max(x_np)
      with self.assertRaisesRegexp(RuntimeWarning,
                                   "overflow encountered in exp"):
        out = log(np.sum(exp(x_np)))
        if out == np.inf:
          raise RuntimeWarning("overflow encountered in exp")

      with self.cached_session(use_gpu=True):
        x_tf = constant_op.constant(x_np, shape=x_np.shape)
        y_tf_np = math_ops.reduce_logsumexp(x_tf).eval()
        y_np = log(np.sum(exp(x_np - max_np))) + max_np
        self.assertAllClose(y_tf_np, y_np)

  @test_util.run_deprecated_v1
  def testUnderflow(self):
    x = [-1000, -1001, -1002, -1003]
    for dtype in [np.float16, np.float32, np.double]:
      x_np = np.array(x, dtype=dtype)
      max_np = np.max(x_np)
      with self.assertRaisesRegexp(RuntimeWarning,
                                   "divide by zero encountered in log"):
        out = log(np.sum(exp(x_np)))
        if out == -np.inf:
          raise RuntimeWarning("divide by zero encountered in log")

      with self.cached_session(use_gpu=True):
        x_tf = constant_op.constant(x_np, shape=x_np.shape)
        y_tf_np = math_ops.reduce_logsumexp(x_tf).eval()
        y_np = log(np.sum(exp(x_np - max_np))) + max_np
        self.assertAllClose(y_tf_np, y_np)

  @test_util.run_deprecated_v1
  def testInfinity(self):
    with self.session(use_gpu=True):
      res = math_ops.reduce_logsumexp(-np.inf).eval()
      self.assertEqual(-np.inf, res)


class RoundTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testRounding(self):
    x = np.arange(-5.0, 5.0, .25)
    for dtype in [np.float32, np.double, np.int32]:
      x_np = np.array(x, dtype=dtype)
      with test_util.device(use_gpu=True):
        x_tf = constant_op.constant(x_np, shape=x_np.shape)
        y_tf = math_ops.round(x_tf)
        y_tf_np = self.evaluate(y_tf)
        y_np = np.round(x_np)
        self.assertAllClose(y_tf_np, y_np, atol=1e-2)


class ModTest(test_util.TensorFlowTestCase):

  @test_util.run_deprecated_v1
  def testFloat(self):
    x = [0.5, 0.7, 0.3]
    for dtype in [np.float32, np.double]:
      # Test scalar and vector versions.
      for denom in [x[0], [x[0]] * 3]:
        x_np = np.array(x, dtype=dtype)
        with self.cached_session(use_gpu=True):
          x_tf = constant_op.constant(x_np, shape=x_np.shape)
          y_tf = math_ops.mod(x_tf, denom)
          y_tf_np = self.evaluate(y_tf)
          y_np = np.fmod(x_np, denom)
        self.assertAllClose(y_tf_np, y_np, atol=1e-2)

  def testFixed(self):
    x = [5, 10, 23]
    for dtype in [np.int32, np.int64]:
      # Test scalar and vector versions.
      for denom in [x[0], x]:
        x_np = np.array(x, dtype=dtype)
        with self.cached_session(use_gpu=True):
          x_tf = constant_op.constant(x_np, shape=x_np.shape)
          y_tf = math_ops.mod(x_tf, denom)
          y_tf_np = self.evaluate(y_tf)
          y_np = np.mod(x_np, denom)
        self.assertAllClose(y_tf_np, y_np)


class SquaredDifferenceTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testSquaredDifference(self):
    for dtype in [np.float16, np.float32, np.float64, np.int32, np.int64]:
      x = np.array([[1, 2, 3], [4, 5, 6]], dtype=dtype)
      y = np.array([-3, -2, -1], dtype=dtype)
      z = (x - y) * (x - y)
      with test_util.device(use_gpu=True):
        z_tf = self.evaluate(math_ops.squared_difference(x, y))
        self.assertAllClose(z, z_tf)

  @test_util.run_in_graph_and_eager_modes()
  def testComplexSquaredDifference(self):
    for dtype in [np.complex64, np.complex128]:
      x = np.array([[1 + 3j, 2 + 2j, 3 + 1j], [4 - 1j, 5 - 2j, 6 - 3j]],
                   dtype=dtype)
      y = np.array([-3 + 1j, -2 + 2j, -1 + 3j], dtype=dtype)
      z = np.conj(x - y) * (x - y)
      with test_util.device(use_gpu=False):
        z_tf = self.evaluate(math_ops.squared_difference(x, y))
        self.assertAllClose(z, z_tf)


class ApproximateEqualTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testApproximateEqual(self):
    for dtype in [np.float32, np.double]:
      x = dtype(1)
      y = dtype(1.00009)
      z = False
      with test_util.device(use_gpu=True):
        # Default tolerance is 0.00001
        z_tf = self.evaluate(math_ops.approximate_equal(x, y))
        self.assertAllEqual(z, z_tf)

    for dtype in [np.float32, np.double]:
      x = dtype(1)
      y = dtype(1.000009)
      z = True
      with test_util.device(use_gpu=True):
        # Default tolerance is 0.00001
        z_tf = self.evaluate(math_ops.approximate_equal(x, y))
        self.assertAllEqual(z, z_tf)

    for dtype in [np.float32, np.double]:
      x = np.array([[[[-1, 2.00009999], [-3, 4.01]]]], dtype=dtype)
      y = np.array([[[[-1.001, 2], [-3.00009, 4]]]], dtype=dtype)
      z = np.array([[[[False, True], [True, False]]]], dtype=np.bool)
      with test_util.device(use_gpu=True):
        z_tf = self.evaluate(math_ops.approximate_equal(x, y, tolerance=0.0001))
        self.assertAllEqual(z, z_tf)

  @test_util.run_deprecated_v1
  def testApproximateEqualShape(self):
    for dtype in [np.float32, np.double]:
      x = np.array([1, 2], dtype=dtype)
      y = np.array([[1, 2]], dtype=dtype)
      # The inputs 'x' and 'y' must have the same shape.
      with self.assertRaisesRegexp(
          ValueError, "Shapes must be equal rank, but are 1 and 2"):
        math_ops.approximate_equal(x, y)


class ScalarMulTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testAcceptsRefs(self):
    if context.executing_eagerly():
      var = resource_variable_ops.ResourceVariable(10, name="var")
    else:
      var = variables.Variable(10)
    result = math_ops.scalar_mul(3, var)
    init = variables.global_variables_initializer()
    with test_util.device(use_gpu=True):
      self.evaluate(init)
      self.assertEqual(30, self.evaluate(result))

  @test_util.run_in_graph_and_eager_modes
  def testAcceptsConstant(self):
    const = constant_op.constant(10)
    result = math_ops.scalar_mul(3, const)
    with test_util.device(use_gpu=True):
      self.assertEqual(30, self.evaluate(result))

  @test_util.run_in_graph_and_eager_modes
  def testAcceptsTensor(self):
    tensor = array_ops.ones([10, 10])
    result = math_ops.scalar_mul(3, tensor)
    expected = array_ops.ones([10, 10]) * 3

    with test_util.device(use_gpu=True):
      self.assertAllEqual(self.evaluate(expected), self.evaluate(result))

  @test_util.run_in_graph_and_eager_modes
  def testAcceptsIndexedSlices(self):
    values = constant_op.constant([2, 3, 5, 7, 0, -1], shape=[3, 2])
    indices = constant_op.constant([0, 2, 5])
    x = math_ops.scalar_mul(-3, ops.IndexedSlices(values, indices))
    with test_util.device(use_gpu=True):
      self.assertAllEqual(self.evaluate(x.values),
                          [[-6, -9], [-15, -21], [0, 3]])
      self.assertAllEqual(self.evaluate(x.indices), [0, 2, 5])


class AccumulateNTest(test_util.TensorFlowTestCase):

  @test_util.run_deprecated_v1
  def testFloat(self):
    np.random.seed(12345)
    x = [np.random.random((1, 2, 3, 4, 5)) - 0.5 for _ in range(5)]
    tf_x = ops.convert_n_to_tensor(x)
    with self.session(use_gpu=True):
      self.assertAllClose(sum(x), math_ops.accumulate_n(tf_x).eval())
      self.assertAllClose(x[0] * 5, math_ops.accumulate_n([tf_x[0]] * 5).eval())

  @test_util.run_deprecated_v1
  def testInt(self):
    np.random.seed(54321)
    x = [np.random.randint(-128, 128, (5, 4, 3, 2, 1)) for _ in range(6)]
    tf_x = ops.convert_n_to_tensor(x)
    with self.session(use_gpu=True):
      self.assertAllEqual(sum(x), math_ops.accumulate_n(tf_x).eval())
      self.assertAllEqual(x[0] * 6, math_ops.accumulate_n([tf_x[0]] * 6).eval())


class AddNTest(test_util.TensorFlowTestCase):

  @test_util.run_deprecated_v1
  def testPartials(self):
    """Test that previously revealed a bug in buffer forwarding for AddN."""
    partials = []
    for _ in range(98):
      partials.append(math_ops.add_n([constant_op.constant(1)]))
    partials.append(
        math_ops.add_n([constant_op.constant(1),
                        constant_op.constant(1)]))

    res = math_ops.add_n(partials) + constant_op.constant(0)
    with self.session(use_gpu=True):
      self.assertAllEqual(res.eval(), 100)

  @test_util.run_deprecated_v1
  def testFloat(self):
    np.random.seed(12345)
    for num_inputs in range(1, 10):
      x = [np.random.random((1, 2, 3, 4, 5)) - 0.5 for _ in range(num_inputs)]
      tf_x = ops.convert_n_to_tensor(x)
      with self.cached_session(use_gpu=True):
        self.assertAllClose(sum(x), math_ops.add_n(tf_x).eval())
        self.assertAllClose(x[0] * num_inputs,
                            math_ops.add_n([tf_x[0]] * num_inputs).eval())

  @test_util.run_deprecated_v1
  def testInt(self):
    np.random.seed(54321)
    for num_inputs in range(1, 10):
      x = [
          np.random.randint(-128, 128, (5, 4, 3, 2, 1))
          for _ in range(num_inputs)
      ]
      tf_x = ops.convert_n_to_tensor(x)
      with self.cached_session(use_gpu=True):
        self.assertAllEqual(sum(x), math_ops.add_n(tf_x).eval())
        self.assertAllEqual(x[0] * num_inputs,
                            math_ops.add_n([tf_x[0]] * num_inputs).eval())

  @test_util.run_deprecated_v1
  def testGrad(self):
    np.random.seed(42)
    for num_inputs in range(1, 10):
      with self.cached_session(use_gpu=True) as sess:
        input_vars = [
            variables.Variable(10.0 * np.random.random())
            for i in range(0, num_inputs)
        ]
        addn = math_ops.add_n(input_vars)
        self.evaluate(variables.global_variables_initializer())
        add_n_grad = gradients.gradients(addn, input_vars)
        self.assertAllEqual(np.repeat(1.0, num_inputs), # d/dx (x + y + ...) = 1
                            [g.eval() for g in add_n_grad])


  @test_util.run_deprecated_v1
  def testIndexedSlices(self):
    slc = ops.IndexedSlices(
        array_ops.constant([1, 2], shape=[1, 2]), array_ops.constant([1]),
        array_ops.constant([2, 2]))
    slc_as_dense = np.array([[0, 0], [1, 2]])
    with self.test_session(use_gpu=True):
      # add_n currently always converts IndexedSlices to dense
      self.assertAllEqual(slc_as_dense, math_ops.add_n([slc]).eval())
      self.assertAllEqual(2 * slc_as_dense, math_ops.add_n([slc, slc]).eval())


class DivAndModTest(test_util.TensorFlowTestCase):
  # TODO(aselle): Test more types before exposing new division operators.

  def intTestData(self):
    nums = np.arange(-10, 10, 1).reshape(20, 1)
    divs = np.arange(-3, 4, 2).reshape(1, 4)
    return nums, divs

  def floatTestData(self):
    nums = np.arange(-10, 10, .25).reshape(80, 1)
    divs = np.arange(-3, 0, .25).reshape(1, 12)
    return nums, divs

  @test_util.run_deprecated_v1
  def testFloorModInt(self):
    nums, divs = self.intTestData()
    with self.cached_session():
      # TODO(aselle): Change test to use % after switch
      # tf_result = math_ops.floor_mod(nums, divs).eval()
      tf_result = math_ops.floormod(nums, divs).eval()
      np_result = nums % divs
      self.assertAllEqual(tf_result, np_result)

  @test_util.run_deprecated_v1
  def testFloorModFloat(self):
    nums, divs = self.floatTestData()
    with self.cached_session():
      tf_result = math_ops.floormod(nums, divs).eval()
      np_result = nums % divs
      self.assertAllEqual(tf_result, np_result)
      # TODO(aselle): put this test in once % switched to floormod
      # tf2_result = (array_ops.constant(nums)
      #               % array_ops.constant(divs)).eval()
      # self.assertAllEqual(tf2_result, tf_result)

  @test_util.run_deprecated_v1
  def testTruncateModInt(self):
    nums, divs = self.intTestData()
    with self.cached_session():
      tf_result = math_ops.truncatemod(nums, divs).eval()
      np_result = np.fmod(nums, divs)
      self.assertAllEqual(tf_result, np_result)

  @test_util.run_deprecated_v1
  def testTruncateModFloat(self):
    nums, divs = self.floatTestData()
    with self.cached_session():
      tf_result = math_ops.truncatemod(nums, divs).eval()
      np_result = np.fmod(nums, divs)
      self.assertAllEqual(tf_result, np_result)

  @test_util.run_deprecated_v1
  def testDivideInt(self):
    nums, divs = self.intTestData()
    with self.cached_session():
      tf_result = math_ops.floor_div(nums, divs).eval()
      np_result = nums // divs
      self.assertAllEqual(tf_result, np_result)
      # TODO(aselle): Put this test in once // is switched to floordiv
      # tf2_result = (array_ops.constant(nums)
      #               // array_ops.constant(divs)).eval()
      # self.assertAllEqual(tf2_result, tf_result)

  @test_util.run_deprecated_v1
  def testDivideName(self):
    with self.cached_session():
      op = math_ops.divide(
          array_ops.constant(3), array_ops.constant(4), name="my_cool_divide")
      self.assertEqual(op.name, "my_cool_divide:0")

  @test_util.run_deprecated_v1
  def testRealDiv(self):
    nums, divs = self.floatTestData()
    with self.cached_session():
      tf_result = math_ops.realdiv(nums, divs).eval()
      np_result = np.divide(nums, divs)
      self.assertAllEqual(tf_result, np_result)

  @test_util.run_deprecated_v1
  def testComplexDiv(self):
    foo = array_ops.constant([1. + 3.j])
    with self.cached_session():
      _ = math_ops.divide(foo, 1.).eval()
      _ = math_ops.div(foo, 2.).eval()

  @test_util.run_deprecated_v1
  def testFloorDivGrad(self):
    with self.cached_session():
      a = variables.Variable(2.)
      b = variables.Variable(4.)
      with self.cached_session() as sess:
        self.evaluate(variables.global_variables_initializer())
        c_grad = gradients.gradients(math_ops.divide(a, b), [a, b])
        self.assertAllEqual([x.eval() for x in c_grad], [.25, -.125])
        c_grad = gradients.gradients(math_ops.div(a, b), [a, b])
        self.assertAllEqual([x.eval() for x in c_grad], [.25, -.125])
        c_grad = gradients.gradients(math_ops.floordiv(a, b), [a, b])
        self.assertAllEqual(
            [None if x is None else self.evaluate(x) for x in c_grad],
            [None, None])

  @test_util.run_deprecated_v1
  def testConsistent(self):
    nums, divs = self.intTestData()
    with self.cached_session():
      tf_result = (math_ops.floor_div(nums, divs) * divs + math_ops.floormod(
          nums, divs)).eval()
      tf_nums = array_ops.constant(nums)
      tf_divs = array_ops.constant(divs)
      tf2_result = (tf_nums // tf_divs * tf_divs + tf_nums % tf_divs).eval()
      np_result = (nums // divs) * divs + (nums % divs)
      # Consistent with numpy
      self.assertAllEqual(tf_result, np_result)
      # Consistent with two forms of divide
      self.assertAllEqual(tf_result, tf2_result)
      # consistency for truncation form
      tf3_result = (math_ops.truncatediv(nums, divs) * divs +
                    math_ops.truncatemod(nums, divs)).eval()
      expanded_nums = np.reshape(
          np.tile(nums, divs.shape[1]), (nums.shape[0], divs.shape[1]))
      # Consistent with desire to get numerator
      self.assertAllEqual(tf3_result, expanded_nums)
      # Consistent with desire to get numerator
      self.assertAllEqual(tf_result, expanded_nums)


class DivNoNanTest(test_util.TensorFlowTestCase):

  @test_util.run_deprecated_v1
  def testBasic(self):
    for dtype in [np.float32, np.float64]:
      nums = np.arange(-10, 10, .25, dtype=dtype).reshape(80, 1)
      divs = np.arange(-3, 3, .25, dtype=dtype).reshape(1, 24)

      np_result = np.true_divide(nums, divs)
      np_result[:, divs[0] == 0] = 0

      with self.cached_session(use_gpu=True):
        tf_result = math_ops.div_no_nan(nums, divs).eval()
        self.assertAllEqual(tf_result, np_result)


class XlogyTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testXlogyNoZero(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant([[0.1, 0.2, 3.5], [-2., -5., 30.]], dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [3.1, 4., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xlogy = self.evaluate(math_ops.xlogy(x, y))
        xtimeslogy = self.evaluate(x * math_ops.log(y))
        self.assertAllClose(xlogy, xtimeslogy)

  @test_util.run_in_graph_and_eager_modes
  def testXlogyWithZero(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant(np.zeros((2, 3)), dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [0., 1., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xlogy_tf_np = self.evaluate(math_ops.xlogy(x, y))
        zeros_np = self.evaluate(array_ops.zeros_like(y))
        self.assertAllClose(xlogy_tf_np, zeros_np)

  @test_util.run_in_graph_and_eager_modes
  def testXlogyWithZeroBroadcast(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant([[0.], [1.]], dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [0., 1., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xlogy_tf_np = self.evaluate(math_ops.xlogy(x, y))
        zeros_np = self.evaluate(array_ops.zeros_like(y[0]))
        xtimes_logy = self.evaluate(math_ops.log(y[1]))
        self.assertAllClose(zeros_np, xlogy_tf_np[0])
        self.assertAllClose(xtimes_logy, xlogy_tf_np[1])


class XdivyTest(test_util.TensorFlowTestCase):

  @test_util.run_in_graph_and_eager_modes
  def testXdivyNoZero(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant([[0.1, 0.2, 3.5], [-2., -5., 30.]], dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [3.1, 4., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xdivy = self.evaluate(math_ops.xdivy(x, y))
        x_over_y = self.evaluate(x / y)
        self.assertAllClose(xdivy, x_over_y)

  @test_util.run_in_graph_and_eager_modes
  def testXdivyWithZero(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant(np.zeros((2, 3)), dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [0., 1., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xdivy_tf_np = self.evaluate(math_ops.xdivy(x, y))
        zeros_np = self.evaluate(array_ops.zeros_like(y))
        self.assertAllClose(xdivy_tf_np, zeros_np)

  @test_util.run_in_graph_and_eager_modes
  def testXdivyWithZeroBroadcast(self):
    for dtype in [dtypes.float16, dtypes.float32, dtypes.float64]:
      x = constant_op.constant([[0.], [1.]], dtype=dtype)
      y = constant_op.constant([[0.1, 0.2, 3.5], [0., 1., 2.]], dtype=dtype)
      with self.cached_session(use_gpu=True):
        xdivy_tf_np = self.evaluate(math_ops.xdivy(x, y))
        zeros_np = self.evaluate(array_ops.zeros_like(y[0]))
        x_over_y = self.evaluate(1 / y[1])
        self.assertAllClose(zeros_np, xdivy_tf_np[0])
        self.assertAllClose(x_over_y, xdivy_tf_np[1])


if __name__ == "__main__":
  googletest.main()
