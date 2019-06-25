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
"""Tests for class OneDeviceStrategy."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.distribute.python import one_device_strategy
from tensorflow.contrib.distribute.python import strategy_test_lib
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.eager import test
from tensorflow.python.framework import test_util


class OneDeviceStrategyTest(strategy_test_lib.DistributionTestBase):

  def _get_distribution_strategy(self):
    return one_device_strategy.OneDeviceStrategy("/device:CPU:0")

  def testMinimizeLossEager(self):
    self._test_minimize_loss_eager(self._get_distribution_strategy())

  def testMinimizeLossGraph(self):
    self._test_minimize_loss_graph(self._get_distribution_strategy())

  def testReplicaId(self):
    self._test_replica_id(self._get_distribution_strategy())

  @test_util.run_in_graph_and_eager_modes
  def testCallAndMergeExceptions(self):
    self._test_call_and_merge_exceptions(self._get_distribution_strategy())

  @test_util.run_in_graph_and_eager_modes
  def testMakeInputFnIterator(self):
    d = one_device_strategy.OneDeviceStrategy("/device:CPU:0")
    dataset_fn = lambda: dataset_ops.Dataset.range(10)
    expected_values = [[i] for i in range(10)]
    input_fn = self._input_fn_to_test_input_context(
        dataset_fn,
        expected_num_replicas_in_sync=1,
        expected_num_input_pipelines=1,
        expected_input_pipeline_id=0)
    iterator = d.make_input_fn_iterator(input_fn)
    self._test_input_fn_iterator(
        iterator, d.extended.worker_devices, expected_values)


if __name__ == "__main__":
  test.main()
