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
"""Tests for Keras activation functions."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python import keras
from tensorflow.python.framework import test_util
from tensorflow.python.platform import test


def _ref_softmax(values):
  m = np.max(values)
  e = np.exp(values - m)
  return e / np.sum(e)


@test_util.run_all_in_graph_and_eager_modes
class KerasActivationsTest(test.TestCase):

  def test_serialization(self):
    all_activations = ['softmax', 'relu', 'elu', 'tanh',
                       'sigmoid', 'hard_sigmoid', 'linear',
                       'softplus', 'softsign', 'selu']
    for name in all_activations:
      fn = keras.activations.get(name)
      ref_fn = getattr(keras.activations, name)
      assert fn == ref_fn
      config = keras.activations.serialize(fn)
      fn = keras.activations.deserialize(config)
      assert fn == ref_fn

  def test_softmax(self):
    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.softmax(x)])
    test_values = np.random.random((2, 5))

    result = f([test_values])[0]
    expected = _ref_softmax(test_values[0])
    self.assertAllClose(result[0], expected, rtol=1e-05)

    with self.assertRaises(ValueError):
      x = keras.backend.placeholder(ndim=1)
      keras.activations.softmax(x)

  def test_temporal_softmax(self):
    x = keras.backend.placeholder(shape=(2, 2, 3))
    f = keras.backend.function([x], [keras.activations.softmax(x)])
    test_values = np.random.random((2, 2, 3)) * 10
    result = f([test_values])[0]
    expected = _ref_softmax(test_values[0, 0])
    self.assertAllClose(result[0, 0], expected, rtol=1e-05)

  def test_selu(self):
    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.selu(x)])
    alpha = 1.6732632423543772848170429916717
    scale = 1.0507009873554804934193349852946

    positive_values = np.array([[1, 2]], dtype=keras.backend.floatx())
    result = f([positive_values])[0]
    self.assertAllClose(result, positive_values * scale, rtol=1e-05)

    negative_values = np.array([[-1, -2]], dtype=keras.backend.floatx())
    result = f([negative_values])[0]
    true_result = (np.exp(negative_values) - 1) * scale * alpha
    self.assertAllClose(result, true_result)

  def test_softplus(self):
    def softplus(x):
      return np.log(np.ones_like(x) + np.exp(x))

    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.softplus(x)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    expected = softplus(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_softsign(self):
    def softsign(x):
      return np.divide(x, np.ones_like(x) + np.absolute(x))

    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.softsign(x)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    expected = softsign(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_sigmoid(self):
    def ref_sigmoid(x):
      if x >= 0:
        return 1 / (1 + np.exp(-x))
      else:
        z = np.exp(x)
        return z / (1 + z)
    sigmoid = np.vectorize(ref_sigmoid)

    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.sigmoid(x)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    expected = sigmoid(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_hard_sigmoid(self):
    def ref_hard_sigmoid(x):
      x = (x * 0.2) + 0.5
      z = 0.0 if x <= 0 else (1.0 if x >= 1 else x)
      return z
    hard_sigmoid = np.vectorize(ref_hard_sigmoid)
    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.hard_sigmoid(x)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    expected = hard_sigmoid(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_relu(self):
    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.relu(x)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    # No negative values in test values...
    self.assertAllClose(result, test_values, rtol=1e-05)

  def test_elu(self):
    x = keras.backend.placeholder(ndim=2)
    f = keras.backend.function([x], [keras.activations.elu(x, 0.5)])
    test_values = np.random.random((2, 5))
    result = f([test_values])[0]
    self.assertAllClose(result, test_values, rtol=1e-05)
    negative_values = np.array([[-1, -2]], dtype=keras.backend.floatx())
    result = f([negative_values])[0]
    true_result = (np.exp(negative_values) - 1) / 2
    self.assertAllClose(result, true_result)

  def test_tanh(self):
    test_values = np.random.random((2, 5))
    x = keras.backend.placeholder(ndim=2)
    exp = keras.activations.tanh(x)
    f = keras.backend.function([x], [exp])
    result = f([test_values])[0]
    expected = np.tanh(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_exponential(self):
    test_values = np.random.random((2, 5))
    x = keras.backend.placeholder(ndim=2)
    exp = keras.activations.exponential(x)
    f = keras.backend.function([x], [exp])
    result = f([test_values])[0]
    expected = np.exp(test_values)
    self.assertAllClose(result, expected, rtol=1e-05)

  def test_linear(self):
    x = np.random.random((10, 5))
    self.assertAllClose(x, keras.activations.linear(x))

  def test_invalid_usage(self):
    with self.assertRaises(ValueError):
      keras.activations.get('unknown')

    # The following should be possible but should raise a warning:
    keras.activations.get(keras.layers.LeakyReLU())

if __name__ == '__main__':
  test.main()
