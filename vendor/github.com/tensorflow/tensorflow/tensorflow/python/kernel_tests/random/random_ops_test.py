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
"""Tests for tensorflow.ops.random_ops."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np
from six.moves import xrange  # pylint: disable=redefined-builtin

from tensorflow.python.eager import context
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import random_seed
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test


class RandomOpTestCommon(test.TestCase):

  # Checks that executing the same rng_func multiple times rarely produces the
  # same result.
  def _testSingleSessionNotConstant(self,
                                    rng_func,
                                    num,
                                    dtype,
                                    min_or_mean,
                                    max_or_stddev,
                                    use_gpu,
                                    op_seed=None,
                                    graph_seed=None):
    with self.session(use_gpu=use_gpu, graph=ops.Graph()) as sess:
      if graph_seed is not None:
        random_seed.set_random_seed(graph_seed)
      x = rng_func([num], min_or_mean, max_or_stddev, dtype=dtype, seed=op_seed)

      y = self.evaluate(x)
      z = self.evaluate(x)
      w = self.evaluate(x)

      # We use exact equality here. If the random-number generator is producing
      # the same output, all three outputs will be bitwise identical.
      self.assertTrue((not np.array_equal(y, z)) or
                      (not np.array_equal(z, w)) or (not np.array_equal(y, w)))


class RandomNormalTest(RandomOpTestCommon):

  def _Sampler(self, num, mu, sigma, dtype, use_gpu, seed=None):

    def func():
      with self.session(use_gpu=use_gpu, graph=ops.Graph()) as sess:
        rng = random_ops.random_normal(
            [num], mean=mu, stddev=sigma, dtype=dtype, seed=seed)
        ret = np.empty([10, num])
        for i in xrange(10):
          ret[i, :] = self.evaluate(rng)
      return ret

    return func

  # Asserts that different trials (1000 samples per trial) is unlikely
  # to see the same sequence of values. Will catch buggy
  # implementations which uses the same random number seed.
  def testDistinct(self):
    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      sampler = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=True)
      x = sampler()
      y = sampler()
      # Number of different samples.
      count = (x == y).sum()
      if count >= 10:
        print("x = ", x)
        print("y = ", y)
        print("count = ", count)
      self.assertTrue(count < 10)

  # Checks that the CPU and GPU implementation returns the same results,
  # given the same random seed
  @test_util.run_deprecated_v1
  def testCPUGPUMatch(self):
    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      results = {}
      for use_gpu in [False, True]:
        sampler = self._Sampler(
            1000000, 0.0, 1.0, dt, use_gpu=use_gpu, seed=12345)
        results[use_gpu] = sampler()
      if dt == dtypes.float16:
        self.assertAllClose(results[False], results[True], rtol=1e-3, atol=1e-3)
      else:
        self.assertAllClose(results[False], results[True], rtol=1e-6, atol=1e-6)

  @test_util.run_deprecated_v1
  def testSeed(self):
    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      sx = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=True, seed=345)
      sy = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=True, seed=345)
      self.assertAllEqual(sx(), sy())

  @test_util.run_deprecated_v1
  def testNoCSE(self):
    for use_gpu in [False, True]:
      with self.session(use_gpu=use_gpu):
        shape = [2, 3, 4]
        rnd1 = random_ops.random_normal(shape, 0.0, 1.0, dtypes.float32)
        rnd2 = random_ops.random_normal(shape, 0.0, 1.0, dtypes.float32)
        diff = rnd2 - rnd1
        self.assertTrue(np.linalg.norm(diff.eval()) > 0.1)

  @test_util.run_deprecated_v1
  def testSingleSessionNotConstant(self):
    for use_gpu in [False, True]:
      for dt in dtypes.float16, dtypes.float32, dtypes.float64:
        self._testSingleSessionNotConstant(
            random_ops.random_normal, 100, dt, 0.0, 1.0, use_gpu=use_gpu)

  @test_util.run_deprecated_v1
  def testSingleSessionOpSeedNotConstant(self):
    for use_gpu in [False, True]:
      for dt in dtypes.float16, dtypes.float32, dtypes.float64:
        self._testSingleSessionNotConstant(
            random_ops.random_normal,
            100,
            dt,
            0.0,
            1.0,
            use_gpu=use_gpu,
            op_seed=1345)

  @test_util.run_deprecated_v1
  def testSingleSessionGraphSeedNotConstant(self):
    for use_gpu in [False, True]:
      for dt in dtypes.float16, dtypes.float32, dtypes.float64:
        self._testSingleSessionNotConstant(
            random_ops.random_normal,
            100,
            dt,
            0.0,
            1.0,
            use_gpu=use_gpu,
            graph_seed=965)


class TruncatedNormalTest(test.TestCase):

  def _Sampler(self, num, mu, sigma, dtype, use_gpu, seed=None):

    def func():
      with self.session(use_gpu=use_gpu, graph=ops.Graph()) as sess:
        rng = random_ops.truncated_normal(
            [num], mean=mu, stddev=sigma, dtype=dtype, seed=seed)
        ret = np.empty([10, num])
        for i in xrange(10):
          ret[i, :] = self.evaluate(rng)
      return ret

    return func

  # Asserts that different trials (1000 samples per trial) is unlikely
  # to see the same sequence of values. Will catch buggy
  # implementations which uses the same random number seed.
  def testDistinct(self):
    # NOTE: TruncatedNormal on GPU is not supported.
    if not test.is_gpu_available():
      for dt in dtypes.float16, dtypes.float32, dtypes.float64:
        sampler = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=False)
        x = sampler()
        y = sampler()
        # Number of different samples.
        count = (x == y).sum()
        if count >= 10:
          print("x = ", x)
          print("y = ", y)
          print("count = ", count)
        self.assertTrue(count < 10)

  # Checks that the CPU and GPU implementation returns the same results,
  # given the same random seed
  @test_util.run_deprecated_v1
  def testCPUGPUMatch(self):
    # Skip the test if there is no GPU.
    if not test.is_gpu_available():
      return

    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      results = {}
      for use_gpu in [False, True]:
        # We need a particular larger number of samples to test multiple rounds
        # on GPU
        sampler = self._Sampler(
            1000000, 0.0, 1.0, dt, use_gpu=use_gpu, seed=12345)
        results[use_gpu] = sampler()
      if dt == dtypes.float16:
        self.assertAllClose(results[False], results[True], rtol=1e-3, atol=1e-3)
      else:
        self.assertAllClose(results[False], results[True], rtol=1e-6, atol=1e-6)

  @test_util.run_deprecated_v1
  def testSeed(self):
    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      sx = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=True, seed=345)
      sy = self._Sampler(1000, 0.0, 1.0, dt, use_gpu=True, seed=345)
      self.assertAllEqual(sx(), sy())

  # The effective standard deviation of truncated normal is 85% of the
  # requested one.
  def testStdDev(self):
    for dt in dtypes.float16, dtypes.float32, dtypes.float64:
      stddev = 3.0
      sampler = self._Sampler(100000, 0.0, stddev, dt, use_gpu=True)
      x = sampler()
      print("std(x)", np.std(x), abs(np.std(x) / stddev - 0.85))
      self.assertTrue(abs(np.std(x) / stddev - 0.85) < 0.04)

  @test_util.run_deprecated_v1
  def testLargeShape(self):
    with self.session(use_gpu=True):
      v = variables.Variable(
          array_ops.zeros(dtype=dtypes.float32, shape=[2**33, 1]))
      n = random_ops.truncated_normal(v.shape)
      self.assertEqual([8589934592, 1], n.shape.as_list())

  @test_util.run_deprecated_v1
  def testNoCSE(self):
    with self.session(use_gpu=True):
      shape = [2, 3, 4]
      rnd1 = random_ops.truncated_normal(shape, 0.0, 1.0, dtypes.float32)
      rnd2 = random_ops.truncated_normal(shape, 0.0, 1.0, dtypes.float32)
      diff = rnd2 - rnd1
      self.assertTrue(np.linalg.norm(diff.eval()) > 0.1)

  def testEagerSeed(self):
    with context.eager_mode():
      # Ensure a context has been created
      random_ops.random_normal([])
      # Set the same seed twice and check that the values match
      context.set_global_seed(42)
      rnd1 = random_ops.random_normal([])
      context.set_global_seed(42)
      rnd2 = random_ops.random_normal([])
      self.assertAllEqual(rnd1, rnd2)


class RandomUniformTest(RandomOpTestCommon):

  def _Sampler(self, num, minv, maxv, dtype, use_gpu, seed=None):

    def func():
      with self.session(use_gpu=use_gpu, graph=ops.Graph()) as sess:
        rng = random_ops.random_uniform(
            [num], minval=minv, maxval=maxv, dtype=dtype, seed=seed)
        ret = np.empty([10, num])
        for i in xrange(10):
          ret[i, :] = self.evaluate(rng)
      return ret

    return func

  def testRange(self):
    for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
               dtypes.int64):
      sampler = self._Sampler(1000, minv=-2, maxv=8, dtype=dt, use_gpu=True)
      x = sampler()
      self.assertTrue(-2 <= np.min(x))
      self.assertTrue(np.max(x) < 8)

  # Asserts that different trials (1000 samples per trial) is unlikely
  # to see the same sequence of values. Will catch buggy
  # implementations which uses the same random number seed.
  def testDistinct(self):
    for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
               dtypes.int64):
      maxv = 1.0 if dt.is_floating else 1 << 30
      sampler = self._Sampler(1000, minv=0, maxv=maxv, dtype=dt, use_gpu=True)
      x = sampler()
      y = sampler()
      count = (x == y).sum()
      count_limit = 50 if dt == dtypes.float16 else 10
      if count >= count_limit:
        print("x = ", x)
        print("y = ", y)
        print("count = ", count)
      self.assertTrue(count < count_limit)

  @test_util.run_deprecated_v1
  def testUniformIntsWithInvalidShape(self):
    for dtype in dtypes.int32, dtypes.int64:
      with self.assertRaisesRegexp(
          ValueError, "Shape must be rank 0 but is rank 1"):
        random_ops.random_uniform(
            [1000], minval=[1, 2], maxval=3, dtype=dtype)
      with self.assertRaisesRegexp(
          ValueError, "Shape must be rank 0 but is rank 1"):
        random_ops.random_uniform(
            [1000], minval=1, maxval=[2, 3], dtype=dtype)

  # Check that uniform ints actually follow a uniform distribution.
  @test_util.run_deprecated_v1
  def testUniformInts(self):
    minv = -2
    maxv = 15
    n = 100000
    p = 1 / (maxv - minv)
    # The counts should follow an (n, p) binomial distribution.
    mean = p * n
    std = np.sqrt(n * p * (1 - p))
    for dt in dtypes.int32, dtypes.int64:
      # Use a fixed seed here to make the test deterministic.
      # Without the fixed seed, the 5 * std bound will (very rarely) fail.
      sampler = self._Sampler(
          n // 10, minv=minv, maxv=maxv, dtype=dt, use_gpu=True, seed=17)
      x = sampler().ravel()
      self.assertEqual(x.shape, (n,))
      counts, _ = np.histogram(x, bins=maxv - minv)
      self.assertEqual(counts.shape, (maxv - minv,))
      self.assertEqual(counts.sum(), n)
      error = np.abs(counts - mean)
      self.assertLess(error.max(), 5 * std)

  # Check that minval = maxval is fine iff we're producing no numbers
  def testUniformIntsDegenerate(self):
    for dt in dtypes.int32, dtypes.int64:
      def sample(n):
        return self._Sampler(n, minv=0, maxv=0, dtype=dt, use_gpu=True)()
      self.assertEqual(sample(0).shape, (10, 0))
      with self.assertRaisesOpError('Need minval < maxval, got 0 >= 0'):
        sample(1)

  # Checks that the CPU and GPU implementation returns the same results,
  # given the same random seed
  @test_util.run_deprecated_v1
  def testCPUGPUMatch(self):
    for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
               dtypes.int64):
      maxv = 1.0 if dt.is_floating else 17
      results = {}
      for use_gpu in False, True:
        sampler = self._Sampler(
            1000000, minv=0, maxv=maxv, dtype=dt, use_gpu=use_gpu, seed=12345)
        results[use_gpu] = sampler()
      self.assertAllEqual(results[False], results[True])

  @test_util.run_deprecated_v1
  def testSeed(self):
    for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
               dtypes.int64):
      for seed in [345, 2**100, -2**100]:
        sx = self._Sampler(1000, 0, 17, dtype=dt, use_gpu=True, seed=seed)
        sy = self._Sampler(1000, 0, 17, dtype=dt, use_gpu=True, seed=seed)
        self.assertAllEqual(sx(), sy())

  @test_util.run_deprecated_v1
  def testNoCSE(self):
    shape = [2, 3, 4]
    for dtype in dtypes.float16, dtypes.float32, dtypes.int32:
      with self.session(use_gpu=True):
        rnd1 = random_ops.random_uniform(shape, 0, 17, dtype=dtype)
        rnd2 = random_ops.random_uniform(shape, 0, 17, dtype=dtype)
        diff = (rnd2 - rnd1).eval()
        self.assertTrue(np.linalg.norm(diff) > 0.1)

  @test_util.run_deprecated_v1
  def testSingleSessionNotConstant(self):
    for use_gpu in [False, True]:
      for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
                 dtypes.int64):
        self._testSingleSessionNotConstant(
            random_ops.random_uniform, 100, dt, 0, 17, use_gpu=use_gpu)

  @test_util.run_deprecated_v1
  def testSingleSessionOpSeedNotConstant(self):
    for use_gpu in [False, True]:
      for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
                 dtypes.int64):
        self._testSingleSessionNotConstant(
            random_ops.random_uniform,
            100,
            dt,
            10,
            20,
            use_gpu=use_gpu,
            op_seed=1345)

  @test_util.run_deprecated_v1
  def testSingleSessionGraphSeedNotConstant(self):
    for use_gpu in [False, True]:
      for dt in (dtypes.float16, dtypes.float32, dtypes.float64, dtypes.int32,
                 dtypes.int64):
        self._testSingleSessionNotConstant(
            random_ops.random_uniform,
            100,
            dt,
            20,
            200,
            use_gpu=use_gpu,
            graph_seed=965)


class RandomShapeTest(test.TestCase):

  @test_util.run_deprecated_v1
  def testTruncatedNormal(self):
    # Fully known shape.
    rnd1 = random_ops.truncated_normal([1, 2, 3])
    self.assertEqual([1, 2, 3], rnd1.get_shape())
    # Partially known shape.
    rnd2 = random_ops.truncated_normal(
        array_ops.placeholder(dtypes.int32, shape=(3,)))
    self.assertEqual([None, None, None], rnd2.get_shape().as_list())
    # Unknown shape.
    rnd3 = random_ops.truncated_normal(array_ops.placeholder(dtypes.int32))
    self.assertIs(None, rnd3.get_shape().ndims)

  @test_util.run_deprecated_v1
  def testRandomNormal(self):
    # Fully known shape.
    rnd1 = random_ops.random_normal([1, 2, 3])
    self.assertEqual([1, 2, 3], rnd1.get_shape())
    # Partially known shape.
    rnd2 = random_ops.random_normal(
        array_ops.placeholder(dtypes.int32, shape=(3,)))
    self.assertEqual([None, None, None], rnd2.get_shape().as_list())
    # Unknown shape.
    rnd3 = random_ops.random_normal(array_ops.placeholder(dtypes.int32))
    self.assertIs(None, rnd3.get_shape().ndims)

  @test_util.run_deprecated_v1
  def testRandomUniform(self):
    # Fully known shape.
    rnd1 = random_ops.random_uniform([1, 2, 3])
    self.assertEqual([1, 2, 3], rnd1.get_shape())
    # Partially known shape.
    rnd2 = random_ops.random_uniform(
        array_ops.placeholder(dtypes.int32, shape=(3,)))
    self.assertEqual([None, None, None], rnd2.get_shape().as_list())
    # Unknown shape.
    rnd3 = random_ops.random_uniform(array_ops.placeholder(dtypes.int32))
    self.assertIs(None, rnd3.get_shape().ndims)


if __name__ == "__main__":
  test.main()
