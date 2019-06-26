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
"""Tests for tf upgrader."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function
import tensorflow as tf
from tensorflow.python.framework import test_util
from tensorflow.python.platform import test as test_lib


class TestUpgrade(test_util.TensorFlowTestCase):
  """Test various APIs that have been changed in 2.0."""

  def setUp(self):
    tf.enable_eager_execution()

  @test_util.run_v1_only("b/120545219")
  def testRenames(self):
    with self.cached_session():
      self.assertAllClose(1.04719755, tf.acos(0.5))
      self.assertAllClose(0.5, tf.rsqrt(4.0))

  @test_util.run_v1_only("b/120545219")
  def testSerializeSparseTensor(self):
    sp_input = tf.SparseTensor(
        indices=tf.constant([[1]], dtype=tf.int64),
        values=tf.constant([2], dtype=tf.int64),
        dense_shape=[2])

    with self.cached_session():
      serialized_sp = tf.serialize_sparse(sp_input, 'serialize_name', tf.string)
      self.assertEqual((3,), serialized_sp.shape)
      self.assertTrue(serialized_sp[0].numpy())  # check non-empty

  @test_util.run_v1_only("b/120545219")
  def testSerializeManySparse(self):
    sp_input = tf.SparseTensor(
        indices=tf.constant([[0, 1]], dtype=tf.int64),
        values=tf.constant([2], dtype=tf.int64),
        dense_shape=[1, 2])

    with self.cached_session():
      serialized_sp = tf.serialize_many_sparse(
          sp_input, 'serialize_name', tf.string)
      self.assertEqual((1, 3), serialized_sp.shape)

  @test_util.run_v1_only("b/120545219")
  def testArgMaxMin(self):
    self.assertAllClose(
        [1],
        tf.argmax([[1, 3, 2]], name='abc', dimension=1))
    self.assertAllClose(
        [0, 0, 0],
        tf.argmax([[1, 3, 2]], dimension=0))
    self.assertAllClose(
        [0],
        tf.argmin([[1, 3, 2]], name='abc', dimension=1))


if __name__ == "__main__":
  test_lib.main()
