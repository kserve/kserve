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
"""Tests for the `MapAndFilterFusion` optimization."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized

from tensorflow.python.data.experimental.ops import optimization
from tensorflow.python.data.kernel_tests import test_base
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import test_util
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import test


def _map_and_filter_fusion_test_cases():
  """Generates test cases for the MapAndFilterFusion optimization."""

  identity = lambda x: x
  increment = lambda x: x + 1
  minus_five = lambda x: x - 5

  def increment_and_square(x):
    y = x + 1
    return y * y

  take_all = lambda x: constant_op.constant(True)
  is_zero = lambda x: math_ops.equal(x, 0)
  is_odd = lambda x: math_ops.equal(x % 2, 0)
  greater = lambda x: math_ops.greater(x + 5, 0)

  functions = [identity, increment, minus_five, increment_and_square]
  filters = [take_all, is_zero, is_odd, greater]
  tests = []

  for x, fun in enumerate(functions):
    for y, predicate in enumerate(filters):
      tests.append(("Mixed{}{}".format(x, y), fun, predicate))

  # Multi output
  tests.append(("Multi1", lambda x: (x, x),
                lambda x, y: constant_op.constant(True)))
  tests.append(
      ("Multi2", lambda x: (x, 2),
       lambda x, y: math_ops.equal(x * math_ops.cast(y, dtypes.int64), 0)))
  return tuple(tests)


@test_util.run_all_in_graph_and_eager_modes
class MapAndFilterFusionTest(test_base.DatasetTestBase, parameterized.TestCase):

  def _testMapAndFilter(self, dataset, function, predicate):
    expected_output = []
    for x in range(10):
      r = function(x)
      if isinstance(r, tuple):
        b = predicate(*r)  # Pass tuple as multiple arguments.
      else:
        b = predicate(r)
      if self.evaluate(b):
        expected_output.append(r)
    self.assertDatasetProduces(dataset, expected_output=expected_output)

  @parameterized.named_parameters(*_map_and_filter_fusion_test_cases())
  def testMapFilterFusion(self, function, predicate):
    dataset = dataset_ops.Dataset.range(10).apply(
        optimization.assert_next(
            ["Map", "FilterByLastComponent"])).map(function).filter(predicate)
    options = dataset_ops.Options()
    options.experimental_optimization.map_and_filter_fusion = True
    dataset = dataset.with_options(options)
    self._testMapAndFilter(dataset, function, predicate)

  def testCapturedInputs(self):
    a = constant_op.constant(3, dtype=dtypes.int64)
    b = constant_op.constant(4, dtype=dtypes.int64)
    some_tensor = math_ops.mul(a, b)
    function = lambda x: x * x

    def predicate(y):
      return math_ops.less(math_ops.cast(y, dtypes.int64), some_tensor)

    # We are currently not supporting functions with captured inputs.
    dataset = dataset_ops.Dataset.range(10).apply(
        optimization.assert_next(["Map",
                                  "Filter"])).map(function).filter(predicate)
    options = dataset_ops.Options()
    options.experimental_optimization.map_and_filter_fusion = True
    dataset = dataset.with_options(options)
    self._testMapAndFilter(dataset, function, predicate)


if __name__ == "__main__":
  test.main()
