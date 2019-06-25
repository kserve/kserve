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
"""Tests for the InterleaveDataset serialization."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized
import numpy as np

from tensorflow.python.data.experimental.kernel_tests.serialization import dataset_serialization_test_base
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.ops import sparse_ops
from tensorflow.python.platform import test


class InterleaveDatasetSerializationTest(
    dataset_serialization_test_base.DatasetSerializationTestBase,
    parameterized.TestCase):

  def _build_iterator_graph(self, input_values, cycle_length, block_length,
                            num_parallel_calls):
    repeat_count = 2
    return dataset_ops.Dataset.from_tensor_slices(input_values).repeat(
        repeat_count).interleave(
            lambda x: dataset_ops.Dataset.from_tensors(x).repeat(x),
            cycle_length, block_length, num_parallel_calls)

  @parameterized.named_parameters(
      ("1", 2, 3, None),
      ("2", 2, 3, 1),
      ("3", 2, 3, 2),
      ("4", 1, 3, None),
      ("5", 1, 3, 1),
      ("6", 2, 1, None),
      ("7", 2, 1, 1),
      ("8", 2, 1, 2),
  )
  def testSerializationCore(self, cycle_length, block_length,
                            num_parallel_calls):
    input_values = np.array([4, 5, 6], dtype=np.int64)
    num_outputs = np.sum(input_values) * 2
    # pylint: disable=g-long-lambda
    self.run_core_tests(
        lambda: self._build_iterator_graph(
            input_values, cycle_length, block_length, num_parallel_calls),
        lambda: self._build_iterator_graph(
            input_values, cycle_length * 2, block_length, num_parallel_calls),
        num_outputs)
    # pylint: enable=g-long-lambda

  def testSparseCore(self):

    def _map_fn(i):
      return sparse_tensor.SparseTensorValue(
          indices=[[0, 0], [1, 1]], values=(i * [1, -1]), dense_shape=[2, 2])

    def _interleave_fn(x):
      return dataset_ops.Dataset.from_tensor_slices(
          sparse_ops.sparse_to_dense(x.indices, x.dense_shape, x.values))

    def _build_dataset():
      return dataset_ops.Dataset.range(10).map(_map_fn).interleave(
          _interleave_fn, cycle_length=1)

    self.run_core_tests(_build_dataset, None, 20)


if __name__ == "__main__":
  test.main()
