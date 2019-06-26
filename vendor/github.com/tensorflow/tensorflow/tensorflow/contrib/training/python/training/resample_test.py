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

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections
import math

import numpy
from tensorflow.contrib.training.python.training import resample
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test


class ResampleTest(test.TestCase):
  """Tests that resampling runs and outputs are close to expected values."""

  def testRepeatRange(self):
    cases = [
        ([], []),
        ([0], []),
        ([1], [0]),
        ([1, 0], [0]),
        ([0, 1], [1]),
        ([3], [0, 0, 0]),
        ([0, 1, 2, 3], [1, 2, 2, 3, 3, 3]),
    ]
    with self.cached_session() as sess:
      for inputs, expected in cases:
        array_inputs = numpy.array(inputs, dtype=numpy.int32)
        actual = sess.run(resample._repeat_range(array_inputs))
        self.assertAllEqual(actual, expected)

  def testRoundtrip(self, rate=0.25, count=5, n=500):
    """Tests `resample(x, weights)` and resample(resample(x, rate), 1/rate)`."""

    foo = self.get_values(count)
    bar = self.get_values(count)
    weights = self.get_weights(count)

    resampled_in, rates = resample.weighted_resample(
        [foo, bar], constant_op.constant(weights), rate, seed=123)

    resampled_back_out = resample.resample_at_rate(
        resampled_in, 1.0 / rates, seed=456)

    init = control_flow_ops.group(variables.local_variables_initializer(),
                                  variables.global_variables_initializer())
    with self.cached_session() as s:
      s.run(init)  # initialize

      # outputs
      counts_resampled = collections.Counter()
      counts_reresampled = collections.Counter()
      for _ in range(n):
        resampled_vs, reresampled_vs = s.run([resampled_in, resampled_back_out])

        self.assertAllEqual(resampled_vs[0], resampled_vs[1])
        self.assertAllEqual(reresampled_vs[0], reresampled_vs[1])

        for v in resampled_vs[0]:
          counts_resampled[v] += 1
        for v in reresampled_vs[0]:
          counts_reresampled[v] += 1

      # assert that resampling worked as expected
      self.assert_expected(weights, rate, counts_resampled, n)

      # and that re-resampling gives the approx identity.
      self.assert_expected(
          [1.0 for _ in weights],
          1.0,
          counts_reresampled,
          n,
          abs_delta=0.1 * n * count)

  def testCorrectRates(self, rate=0.25, count=10, n=500, rtol=0.1):
    """Tests that the rates returned by weighted_resample are correct."""

    # The approach here is to verify that:
    #  - sum(1/rate) approximates the size of the original collection
    #  - sum(1/rate * value) approximates the sum of the original inputs,
    #  - sum(1/rate * value)/sum(1/rate) approximates the mean.
    vals = self.get_values(count)
    weights = self.get_weights(count)

    resampled, rates = resample.weighted_resample([vals],
                                                  constant_op.constant(weights),
                                                  rate)

    invrates = 1.0 / rates

    init = control_flow_ops.group(variables.local_variables_initializer(),
                                  variables.global_variables_initializer())
    expected_sum_op = math_ops.reduce_sum(vals)
    with self.cached_session() as s:
      s.run(init)
      expected_sum = n * s.run(expected_sum_op)

      weight_sum = 0.0
      weighted_value_sum = 0.0
      for _ in range(n):
        val, inv_rate = s.run([resampled[0], invrates])
        weight_sum += sum(inv_rate)
        weighted_value_sum += sum(val * inv_rate)

    # sum(inv_rate) ~= N*count:
    expected_count = count * n
    self.assertAlmostEqual(
        expected_count, weight_sum, delta=(rtol * expected_count))

    # sum(vals) * n ~= weighted_sum(resampled, 1.0/weights)
    self.assertAlmostEqual(
        expected_sum, weighted_value_sum, delta=(rtol * expected_sum))

    # Mean ~= weighted mean:
    expected_mean = expected_sum / float(n * count)
    self.assertAlmostEqual(
        expected_mean,
        weighted_value_sum / weight_sum,
        delta=(rtol * expected_mean))

  def testZeroRateUnknownShapes(self, count=10):
    """Tests that resampling runs with completely runtime shapes."""
    # Use placeholcers without shape set:
    vals = array_ops.placeholder(dtype=dtypes.int32)
    rates = array_ops.placeholder(dtype=dtypes.float32)

    resampled = resample.resample_at_rate([vals], rates)

    with self.cached_session() as s:
      rs, = s.run(resampled, {
          vals: list(range(count)),
          rates: numpy.zeros(
              shape=[count], dtype=numpy.float32)
      })
      self.assertEqual(rs.shape, (0,))

  def testDtypes(self, count=10):
    """Test that we can define the ops with float64 weights."""

    vals = self.get_values(count)
    weights = math_ops.cast(self.get_weights(count), dtypes.float64)

    # should not error:
    resample.resample_at_rate([vals], weights)
    resample.weighted_resample(
        [vals], weights, overall_rate=math_ops.cast(1.0, dtypes.float64))

  def get_weights(self, n, mean=10.0, stddev=5):
    """Returns random positive weight values."""
    assert mean > 0, 'Weights have to be positive.'
    results = []
    while len(results) < n:
      v = numpy.random.normal(mean, stddev)
      if v > 0:
        results.append(v)
    return results

  def get_values(self, n):
    return constant_op.constant(list(range(n)))

  def assert_expected(self,
                      weights,
                      overall_rate,
                      counts,
                      n,
                      tol=2.0,
                      abs_delta=0):
    # Overall, we expect sum(counts) there to be `overall_rate` * n *
    # len(weights)...  with a stddev on that expectation equivalent to
    # performing (n * len(weights)) trials each with probability of
    # overall_rate.
    expected_overall_count = len(weights) * n * overall_rate
    actual_overall_count = sum(counts.values())

    stddev = math.sqrt(len(weights) * n * overall_rate * (1 - overall_rate))

    self.assertAlmostEqual(
        expected_overall_count,
        actual_overall_count,
        delta=(stddev * tol + abs_delta))

    # And we can form a similar expectation for each item -- it should
    # appear in the results a number of time proportional to its
    # weight, which is similar to performing `expected_overall_count`
    # trials each with a probability of weight/weight_sum.
    weight_sum = sum(weights)
    fractions = [w / weight_sum for w in weights]
    expected_counts = [expected_overall_count * f for f in fractions]

    stddevs = [
        math.sqrt(expected_overall_count * f * (1 - f)) for f in fractions
    ]

    for i in range(len(expected_counts)):
      expected_count = expected_counts[i]
      actual_count = counts[i]
      self.assertAlmostEqual(
          expected_count, actual_count, delta=(stddevs[i] * tol + abs_delta))


if __name__ == '__main__':
  test.main()
