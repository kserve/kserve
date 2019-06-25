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
"""Tests for layer serialization utils."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python import keras
from tensorflow.python.framework import test_util as tf_test_util
from tensorflow.python.platform import test


@tf_test_util.run_all_in_graph_and_eager_modes
class LayerSerializationTest(test.TestCase):

  def test_serialize_deserialize(self):
    layer = keras.layers.Dense(
        3, activation='relu', kernel_initializer='ones', bias_regularizer='l2')
    config = keras.layers.serialize(layer)
    new_layer = keras.layers.deserialize(config)
    self.assertEqual(new_layer.activation, keras.activations.relu)
    self.assertEqual(new_layer.bias_regularizer.__class__,
                     keras.regularizers.L1L2)
    self.assertEqual(new_layer.kernel_initializer.__class__,
                     keras.initializers.Ones)
    self.assertEqual(new_layer.units, 3)

  def test_serialize_deserialize_batchnorm(self):
    layer = keras.layers.BatchNormalization(
        momentum=0.9, beta_initializer='zeros', gamma_regularizer='l2')
    config = keras.layers.serialize(layer)
    self.assertEqual(config['class_name'], 'BatchNormalization')
    new_layer = keras.layers.deserialize(config)
    self.assertEqual(new_layer.momentum, 0.9)
    self.assertEqual(new_layer.beta_initializer.__class__,
                     keras.initializers.Zeros)
    self.assertEqual(new_layer.gamma_regularizer.__class__,
                     keras.regularizers.L1L2)

if __name__ == '__main__':
  test.main()
