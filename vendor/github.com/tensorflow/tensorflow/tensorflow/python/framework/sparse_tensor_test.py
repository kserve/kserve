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

"""Tests for tensorflow.python.framework.sparse_tensor."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import test_util
from tensorflow.python.ops import sparse_ops
from tensorflow.python.platform import googletest


class SparseTensorTest(test_util.TensorFlowTestCase):

  def testPythonConstruction(self):
    indices = [[1, 2], [2, 0], [3, 4]]
    values = [b"a", b"b", b"c"]
    shape = [4, 5]
    sp_value = sparse_tensor.SparseTensorValue(indices, values, shape)
    for sp in [
        sparse_tensor.SparseTensor(indices, values, shape),
        sparse_tensor.SparseTensor.from_value(sp_value),
        sparse_tensor.SparseTensor.from_value(
            sparse_tensor.SparseTensor(indices, values, shape))]:
      self.assertEqual(sp.indices.dtype, dtypes.int64)
      self.assertEqual(sp.values.dtype, dtypes.string)
      self.assertEqual(sp.dense_shape.dtype, dtypes.int64)
      self.assertEqual(sp.get_shape(), (4, 5))

      with self.cached_session() as sess:
        value = self.evaluate(sp)
        self.assertAllEqual(indices, value.indices)
        self.assertAllEqual(values, value.values)
        self.assertAllEqual(shape, value.dense_shape)
        sess_run_value = self.evaluate(sp)
        self.assertAllEqual(sess_run_value.indices, value.indices)
        self.assertAllEqual(sess_run_value.values, value.values)
        self.assertAllEqual(sess_run_value.dense_shape, value.dense_shape)

  def testIsSparse(self):
    self.assertFalse(sparse_tensor.is_sparse(3))
    self.assertFalse(sparse_tensor.is_sparse("foo"))
    self.assertFalse(sparse_tensor.is_sparse(np.array(3)))
    self.assertTrue(
        sparse_tensor.is_sparse(sparse_tensor.SparseTensor([[0]], [0], [1])))
    self.assertTrue(
        sparse_tensor.is_sparse(
            sparse_tensor.SparseTensorValue([[0]], [0], [1])))

  @test_util.run_deprecated_v1
  def testConsumers(self):
    sp = sparse_tensor.SparseTensor([[0, 0], [1, 2]], [1.0, 3.0], [3, 4])
    w = ops.convert_to_tensor(np.ones([4, 1], np.float32))
    out = sparse_ops.sparse_tensor_dense_matmul(sp, w)
    self.assertEqual(len(sp.consumers()), 1)
    self.assertEqual(sp.consumers()[0], out.op)

    dense = sparse_ops.sparse_tensor_to_dense(sp)
    self.assertEqual(len(sp.consumers()), 2)
    self.assertTrue(dense.op in sp.consumers())
    self.assertTrue(out.op in sp.consumers())


class ConvertToTensorOrSparseTensorTest(test_util.TensorFlowTestCase):

  def test_convert_dense(self):
    with self.cached_session():
      value = [42, 43]
      from_value = sparse_tensor.convert_to_tensor_or_sparse_tensor(
          value)
      self.assertAllEqual(value, self.evaluate(from_value))

  @test_util.run_deprecated_v1
  def test_convert_sparse(self):
    with self.cached_session():
      indices = [[0, 1], [1, 0]]
      values = [42, 43]
      shape = [2, 2]
      sparse_tensor_value = sparse_tensor.SparseTensorValue(
          indices, values, shape)
      st = sparse_tensor.SparseTensor.from_value(sparse_tensor_value)
      from_value = sparse_tensor.convert_to_tensor_or_sparse_tensor(
          sparse_tensor_value).eval()
      from_tensor = sparse_tensor.convert_to_tensor_or_sparse_tensor(st).eval()
      for convertee in [from_value, from_tensor]:
        self.assertAllEqual(sparse_tensor_value.indices, convertee.indices)
        self.assertAllEqual(sparse_tensor_value.values, convertee.values)
        self.assertAllEqual(
            sparse_tensor_value.dense_shape, convertee.dense_shape)


if __name__ == "__main__":
  googletest.main()
