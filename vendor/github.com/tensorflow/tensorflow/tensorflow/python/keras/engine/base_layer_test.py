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
"""Tests for TensorFlow 2.0 layer behavior."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python import keras
from tensorflow.python.eager import context
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.keras import keras_parameterized
from tensorflow.python.keras import testing_utils
from tensorflow.python.keras.engine import base_layer
from tensorflow.python.keras.optimizer_v2 import rmsprop
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import test


class DynamicLayer1(base_layer.Layer):

  def call(self, inputs):
    if math_ops.reduce_sum(inputs) > 0:
      return math_ops.sqrt(inputs)
    else:
      return math_ops.square(inputs)

  def compute_output_shape(self, input_shape):
    return input_shape


class DynamicLayer2(base_layer.Layer):

  def call(self, inputs):
    samples = []
    for sample in inputs:
      samples.append(math_ops.square(sample))
    return array_ops.stack(samples, axis=0)

  def compute_output_shape(self, input_shape):
    return input_shape


class InvalidLayer(base_layer.Layer):

  def call(self, inputs):
    raise ValueError('You did something wrong!')

  def compute_output_shape(self, input_shape):
    return input_shape


class BaseLayerTest(keras_parameterized.TestCase):

  def _assert_static_graph_unfriendly_model(self, model):
    self.assertEqual(model._static_graph_friendly, False)
    if not testing_utils.should_run_eagerly():
      with self.assertRaisesRegexp(
          ValueError, 'can only be successfully run in eager execution'):
        model.compile(rmsprop.RMSprop(0.001), loss='mse',
                      run_eagerly=testing_utils.should_run_eagerly())
    else:
      model.compile(rmsprop.RMSprop(0.001), loss='mse',
                    run_eagerly=testing_utils.should_run_eagerly())
      model.train_on_batch(np.random.random((2, 3)), np.random.random((2, 3)))

  @test_util.run_v1_only
  def test_dynamic_layer_fails_in_v1(self):
    inputs = keras.Input((3,))

    if not context.executing_eagerly():
      with self.assertRaisesRegexp(
          TypeError, 'Using a `tf.Tensor` as a Python `bool` is not allowed'):
        DynamicLayer1()(inputs)
      with self.assertRaisesRegexp(
          TypeError, 'Tensor objects are only iterable when eager'):
        DynamicLayer2()(inputs)

  @keras_parameterized.run_all_keras_modes(always_skip_v1=True)
  def test_dynamic_layer(self):
    inputs = keras.Input((3,))
    outputs = DynamicLayer1()(inputs)
    model = keras.Model(inputs, outputs)
    self.assertAllClose([[0], [4], [9]], model.predict_on_batch([0, 2, -3]))
    self._assert_static_graph_unfriendly_model(model)

    inputs = keras.Input((3,))
    outputs = DynamicLayer2()(inputs)
    model = keras.Model(inputs, outputs)
    self.assertAllClose([[0], [4], [9]], model.predict_on_batch([0, 2, -3]))
    self._assert_static_graph_unfriendly_model(model)

  # TODO(b/120985967): Test fails for nested models due to _set_mask_metadata.
  @keras_parameterized.run_all_keras_modes(always_skip_v1=True)
  def nested_dynamic_layers_in_eager_mode(self):
    inputs = keras.Input((3,))
    outputs = DynamicLayer1()(inputs)
    inner_model = keras.Model(inputs, outputs)

    inputs = keras.Input((3,))
    x = DynamicLayer2()(inputs)
    outputs = inner_model(x)

    model = keras.Model(inputs, outputs)
    self.assertEqual(model._static_graph_friendly, False)
    if testing_utils.should_run_eagerly():
      model.compile(rmsprop.RMSprop(0.001), loss='mse', run_eagerly=True)
      model.train_on_batch(np.random.random((2, 3)), np.random.random((2, 3)))
    else:
      with self.assertRaisesRegexp(
          ValueError, 'only be successfully run in eager execution'):
        model.compile(rmsprop.RMSprop(0.001), loss='mse', run_eagerly=False)

  def test_invalid_forward_pass_in_graph_mode(self):
    with context.graph_mode():
      inputs = keras.Input((3,))
      with self.assertRaisesRegexp(ValueError, 'You did something wrong!'):
        _ = InvalidLayer()(inputs)

  @keras_parameterized.run_all_keras_modes(always_skip_v1=True)
  def test_invalid_forward_pass_in_eager_mode(self):
    inputs = keras.Input((3,))
    outputs = InvalidLayer()(inputs)
    model = keras.Model(inputs, outputs)
    self.assertEqual(model._static_graph_friendly, False)
    if testing_utils.should_run_eagerly():
      model.compile(rmsprop.RMSprop(0.001), loss='mse', run_eagerly=True)
      with self.assertRaisesRegexp(ValueError, 'You did something wrong!'):
        model.train_on_batch(np.random.random((2, 3)), np.random.random((2, 3)))
    else:
      with self.assertRaisesRegexp(
          ValueError, 'only be successfully run in eager execution'):
        model.compile(rmsprop.RMSprop(0.001), loss='mse', run_eagerly=False)

  def test_using_symbolic_tensors_with_tf_ops(self):
    # Single-input.
    x = keras.Input((3,))
    y = math_ops.square(x)
    self.assertEqual(y.graph, keras.backend.get_graph())

    # Multi-inputs.
    x1, x2 = keras.Input((3,)), keras.Input((3,))
    y = array_ops.concat([x1, x2], axis=1)
    self.assertEqual(y.graph, keras.backend.get_graph())

    # Mixing Keras symbolic tensors and graph tensors from the same graph works.
    with keras.backend.get_graph().as_default():
      x1 = keras.Input((3,))
    x2 = keras.Input((3,))
    y = math_ops.matmul(x1, x2)
    self.assertEqual(y.graph, keras.backend.get_graph())

    # Creating same op type (matmul) multiple times in the Keras graph works.
    x1 = keras.Input((3,))
    x2 = keras.Input((3,))
    y = math_ops.matmul(x1, x2)
    self.assertEqual(y.graph, keras.backend.get_graph())

  def test_mixing_eager_and_graph_tensors(self):
    with ops.Graph().as_default():
      x1 = array_ops.ones((3, 3))
    x2 = array_ops.ones((3, 3))
    self.assertTrue(isinstance(x2, ops.EagerTensor))
    with self.assertRaisesRegexp(TypeError,
                                 'provided list of inputs contains '
                                 'objects other than \'EagerTensor\''):
      math_ops.matmul(x1, x2)

  def test_mixing_numpy_arrays_and_graph_tensors(self):
    with ops.Graph().as_default():
      x1 = array_ops.ones((3, 3))
    x2 = np.ones((3, 3), dtype='float32')
    with self.assertRaisesRegexp(TypeError,
                                 'provided list of inputs contains '
                                 'objects other than \'EagerTensor\''):
      math_ops.matmul(x1, x2)

  @test_util.run_in_graph_and_eager_modes
  def test_mixing_keras_symbolic_tensors_and_eager_tensors(self):
    x1 = keras.Input((3,))
    x2 = array_ops.ones((3, 3))
    y = math_ops.matmul(x1, x2)
    self.assertEqual(y.graph, keras.backend.get_graph())
    fn = keras.backend.function(inputs=[x1], outputs=[y])
    x_val = np.random.random((3, 3))
    y_val = np.ones((3, 3))
    self.assertAllClose(fn([x_val])[0],
                        np.matmul(x_val, y_val),
                        atol=1e-5)

  @test_util.run_in_graph_and_eager_modes
  def test_mixing_keras_symbolic_tensors_and_numpy_arrays(self):
    x1 = keras.Input((3,))
    x2 = np.ones((3, 3), dtype='float32')
    y = math_ops.matmul(x1, x2)
    self.assertEqual(y.graph, keras.backend.get_graph())
    fn = keras.backend.function(inputs=[x1], outputs=[y])
    x_val = np.random.random((3, 3))
    y_val = np.ones((3, 3))
    self.assertAllClose(fn([x_val])[0],
                        np.matmul(x_val, y_val),
                        atol=1e-5)


@test_util.run_all_in_graph_and_eager_modes
class NestedTrackingTest(test.TestCase):

  def test_nested_layer_variable_tracking(self):
    # Test that variables from nested sublayers are
    # being tracked by subclassed layers.

    class MyLayer(keras.layers.Layer):

      def __init__(self):
        super(MyLayer, self).__init__()
        self.dense1 = keras.layers.Dense(1)
        self.dense2 = keras.layers.BatchNormalization()

      def build(self, input_shape):
        self.v1 = self.add_weight('v1', shape=input_shape[1:].as_list())
        self.v2 = variables.Variable(
            name='v2',
            initial_value=np.zeros(input_shape[1:].as_list(), dtype='float32'),
            trainable=False)

      def call(self, inputs):
        x = self.dense1(inputs) + self.dense2(inputs)
        return x + self.v1 + self.v2

    layer = MyLayer()
    inputs = keras.Input((1,))
    _ = layer(inputs)

    self.assertEqual(len(layer.weights), 8)
    self.assertEqual(len(layer.trainable_weights), 5)
    self.assertEqual(len(layer.non_trainable_weights), 3)

    layer.dense1.trainable = False
    self.assertEqual(len(layer.weights), 8)
    self.assertEqual(len(layer.trainable_weights), 3)
    self.assertEqual(len(layer.non_trainable_weights), 5)

    layer.trainable = False
    self.assertEqual(len(layer.weights), 8)
    self.assertEqual(len(layer.trainable_weights), 0)
    self.assertEqual(len(layer.non_trainable_weights), 8)

  def test_nested_layer_updates_losses_tracking(self):
    # Test that updates and losses from nested sublayers are
    # being tracked by subclassed layers.

    class UpdateAndLossLayer(keras.layers.Layer):

      def build(self, _):
        self.v1 = self.add_weight('v1', shape=())

      def call(self, inputs):
        self.add_loss(math_ops.reduce_sum(inputs))
        self.add_update(state_ops.assign_add(self.v1, 1))
        return inputs + 1

    class MyLayer(keras.layers.Layer):

      def build(self, _):
        self.v1 = self.add_weight('v1', shape=())

      def __init__(self):
        super(MyLayer, self).__init__()
        self.ul1 = UpdateAndLossLayer()
        self.ul2 = UpdateAndLossLayer()

      def call(self, inputs):
        self.add_loss(math_ops.reduce_sum(inputs))
        self.add_update(state_ops.assign_add(self.v1, 1))
        x = self.ul1(inputs)
        return self.ul2(x)

    layer = MyLayer()

    if context.executing_eagerly():
      inputs = array_ops.ones((3, 1))
      _ = layer(inputs)
      self.assertEqual(len(layer.losses), 3)
    else:
      inputs = keras.Input((1,))
      _ = layer(inputs)
      self.assertEqual(len(layer.losses), 3)
      self.assertEqual(len(layer.updates), 3)


if __name__ == '__main__':
  ops.enable_eager_execution()
  test.main()
