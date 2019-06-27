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
"""Tests specific to `Sequential` model."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from absl.testing import parameterized
import numpy as np

from tensorflow.python import keras
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.eager import context
from tensorflow.python.eager import function
from tensorflow.python.framework import test_util as tf_test_util
from tensorflow.python.keras import keras_parameterized
from tensorflow.python.keras import testing_utils
from tensorflow.python.ops import array_ops
from tensorflow.python.platform import test
from tensorflow.python.training import rmsprop


class TestSequential(keras_parameterized.TestCase):
  """Most Sequential model API tests are covered in `training_test.py`.
  """

  @keras_parameterized.run_all_keras_modes
  def test_basic_methods(self):
    model = keras.models.Sequential()
    model.add(keras.layers.Dense(1, input_dim=2))
    model.add(keras.layers.Dropout(0.3, name='dp'))
    model.add(keras.layers.Dense(2, kernel_regularizer='l2',
                                 kernel_constraint='max_norm'))
    self.assertEqual(len(model.layers), 3)
    self.assertEqual(len(model.weights), 2 * 2)
    self.assertEqual(model.get_layer(name='dp').name, 'dp')

  @keras_parameterized.run_all_keras_modes
  def test_sequential_pop(self):
    num_hidden = 5
    input_dim = 3
    batch_size = 5
    num_classes = 2

    model = testing_utils.get_small_sequential_mlp(
        num_hidden, num_classes, input_dim)
    model.compile(loss='mse', optimizer=rmsprop.RMSPropOptimizer(1e-3),
                  run_eagerly=testing_utils.should_run_eagerly())
    x = np.random.random((batch_size, input_dim))
    y = np.random.random((batch_size, num_classes))
    model.fit(x, y, epochs=1)
    model.pop()
    self.assertEqual(len(model.layers), 1)
    self.assertEqual(model.output_shape, (None, num_hidden))
    model.compile(loss='mse', optimizer=rmsprop.RMSPropOptimizer(1e-3),
                  run_eagerly=testing_utils.should_run_eagerly())
    y = np.random.random((batch_size, num_hidden))
    model.fit(x, y, epochs=1)

    # Test popping single-layer model
    model = keras.models.Sequential()
    model.add(keras.layers.Dense(num_hidden, input_dim=input_dim))
    model.pop()
    self.assertEqual(model.layers, [])
    self.assertEqual(model.outputs, None)

    # Invalid use case
    model = keras.models.Sequential()
    with self.assertRaises(TypeError):
      model.pop()

  @keras_parameterized.run_all_keras_modes
  def test_sequential_deferred_build_with_np_arrays(self):
    num_hidden = 5
    input_dim = 3
    batch_size = 5
    num_classes = 2

    model = testing_utils.get_small_sequential_mlp(num_hidden, num_classes)
    model.compile(
        loss='mse',
        optimizer=rmsprop.RMSPropOptimizer(1e-3),
        metrics=[keras.metrics.CategoricalAccuracy()],
        run_eagerly=testing_utils.should_run_eagerly())
    self.assertEqual(len(model.layers), 2)
    self.assertEqual(len(model.weights), 0)
    self.assertFalse(model.built)

    x = np.random.random((batch_size, input_dim))
    y = np.random.random((batch_size, num_classes))
    model.fit(x, y, epochs=1)
    self.assertTrue(model.built)
    self.assertFalse(model._is_graph_network)
    self.assertEqual(len(model.weights), 2 * 2)

  @keras_parameterized.run_all_keras_modes
  def test_sequential_deferred_build_with_dataset_iterators(self):
    num_hidden = 5
    input_dim = 3
    num_classes = 2
    num_samples = 50
    steps_per_epoch = 10

    model = testing_utils.get_small_sequential_mlp(num_hidden, num_classes)
    model.compile(
        loss='mse',
        optimizer=rmsprop.RMSPropOptimizer(1e-3),
        metrics=[keras.metrics.CategoricalAccuracy()],
        run_eagerly=testing_utils.should_run_eagerly())
    self.assertEqual(len(model.layers), 2)
    self.assertEqual(len(model.weights), 0)
    self.assertFalse(model.built)

    x = array_ops.ones((num_samples, input_dim))
    y = array_ops.zeros((num_samples, num_classes))
    dataset = dataset_ops.Dataset.from_tensor_slices((x, y))
    dataset = dataset.repeat(100)
    dataset = dataset.batch(10)
    iterator = dataset_ops.make_one_shot_iterator(dataset)

    model.fit(iterator, epochs=1, steps_per_epoch=steps_per_epoch)
    self.assertTrue(model.built)
    self.assertEqual(len(model.weights), 2 * 2)
    self.assertFalse(model._is_graph_network)

  # TODO(kaftan) This test fails w/ run_with_all_keras_modes. File ticket
  @parameterized.parameters((True,), (False,))
  @tf_test_util.run_deprecated_v1
  def test_training_and_eval_methods_on_symbolic_tensors(self, deferred):
    with self.cached_session():

      def get_model():
        if deferred:
          model = testing_utils.get_small_sequential_mlp(10, 4)
        else:
          model = testing_utils.get_small_sequential_mlp(10, 4, input_dim=3)
        model.compile(
            optimizer=rmsprop.RMSPropOptimizer(1e-3),
            loss='categorical_crossentropy',
            metrics=['accuracy'])
        return model

      inputs = keras.backend.zeros(shape=(10, 3))
      targets = keras.backend.zeros(shape=(10, 4))

      model = get_model()
      model.fit(inputs, targets, epochs=10, steps_per_epoch=30)

      model = get_model()
      model.evaluate(inputs, targets, steps=2, verbose=0)

      model = get_model()
      model.predict(inputs, steps=2)

      model = get_model()
      model.train_on_batch(inputs, targets)

      model = get_model()
      model.test_on_batch(inputs, targets)

      model = get_model()
      model.fit(
          inputs,
          targets,
          epochs=1,
          steps_per_epoch=2,
          verbose=0,
          validation_data=(inputs, targets),
          validation_steps=2)

  @keras_parameterized.run_all_keras_modes
  def test_invalid_use_cases(self):
    # Added objects must be layer instances
    with self.assertRaises(TypeError):
      model = keras.models.Sequential()
      model.add(None)

    # Added layers cannot have multiple outputs
    class MyLayer(keras.layers.Layer):

      def call(self, inputs):
        return [3 * inputs, 2 * inputs]

      def compute_output_shape(self, input_shape):
        return [input_shape, input_shape]

    with self.assertRaises(ValueError):
      model = keras.models.Sequential()
      model.add(MyLayer(input_shape=(3,)))
    with self.assertRaises(TypeError):
      model = keras.models.Sequential()
      model.add(keras.layers.Dense(1, input_dim=1))
      model.add(MyLayer())

  @keras_parameterized.run_all_keras_modes
  def test_nested_sequential_trainability(self):
    input_dim = 20
    num_units = 10
    num_classes = 2

    inner_model = keras.models.Sequential()
    inner_model.add(keras.layers.Dense(num_units, input_shape=(input_dim,)))

    model = keras.models.Sequential()
    model.add(inner_model)
    model.add(keras.layers.Dense(num_classes))

    self.assertEqual(len(model.layers), 2)

    self.assertEqual(len(model.trainable_weights), 4)
    inner_model.trainable = False
    self.assertEqual(len(model.trainable_weights), 2)
    inner_model.trainable = True
    self.assertEqual(len(model.trainable_weights), 4)

  def test_sequential_update_disabling(self):
    val_a = np.random.random((10, 4))
    val_out = np.random.random((10, 4))

    with self.cached_session():
      model = keras.models.Sequential()
      model.add(keras.layers.BatchNormalization(input_shape=(4,)))
      assert model.updates

      model.trainable = False
      assert not model.updates

      model.compile('sgd', 'mse')
      assert not model.updates

      x1 = model.predict(val_a)
      model.train_on_batch(val_a, val_out)
      x2 = model.predict(val_a)
      self.assertAllClose(x1, x2, atol=1e-7)

      model.trainable = True
      model.compile('sgd', 'mse')
      assert model.updates

      model.train_on_batch(val_a, val_out)
      x2 = model.predict(val_a)
      assert np.abs(np.sum(x1 - x2)) > 1e-5

  @keras_parameterized.run_all_keras_modes
  def test_sequential_deferred_build_serialization(self):
    num_hidden = 5
    input_dim = 3
    batch_size = 5
    num_classes = 2

    model = testing_utils.get_small_sequential_mlp(num_hidden, num_classes)
    model.compile(
        loss='mse',
        optimizer=rmsprop.RMSPropOptimizer(1e-3),
        metrics=[keras.metrics.CategoricalAccuracy()],
        run_eagerly=testing_utils.should_run_eagerly())
    self.assertFalse(model.built)

    x = np.random.random((batch_size, input_dim))
    y = np.random.random((batch_size, num_classes))
    model.train_on_batch(x, y)
    self.assertTrue(model.built)

    config = model.get_config()
    self.assertIn('build_input_shape', config)

    new_model = keras.models.Sequential.from_config(config)
    self.assertEqual(len(new_model.layers), 2)
    self.assertEqual(len(new_model.weights), 4)

  @keras_parameterized.run_all_keras_modes
  def test_sequential_shape_inference_deferred(self):
    model = testing_utils.get_small_sequential_mlp(4, 5)
    output_shape = model.compute_output_shape((None, 7))
    self.assertEqual(tuple(output_shape.as_list()), (None, 5))

  @keras_parameterized.run_all_keras_modes
  def test_sequential_build_deferred(self):
    model = testing_utils.get_small_sequential_mlp(4, 5)

    model.build((None, 10))
    self.assertTrue(model.built)
    self.assertEqual(len(model.weights), 4)

    # Test with nested model
    model = testing_utils.get_small_sequential_mlp(4, 3)
    inner_model = testing_utils.get_small_sequential_mlp(4, 5)
    model.add(inner_model)

    model.build((None, 10))
    self.assertTrue(model.built)
    self.assertEqual(len(model.weights), 8)

  @keras_parameterized.run_all_keras_modes
  def test_sequential_nesting(self):
    model = testing_utils.get_small_sequential_mlp(4, 3)
    inner_model = testing_utils.get_small_sequential_mlp(4, 5)
    model.add(inner_model)

    model.compile(loss='mse', optimizer=rmsprop.RMSPropOptimizer(1e-3),
                  run_eagerly=testing_utils.should_run_eagerly())
    x = np.random.random((2, 6))
    y = np.random.random((2, 5))
    model.fit(x, y, epochs=1)

  @keras_parameterized.run_all_keras_modes
  def test_variable_names(self):
    model = keras.models.Sequential([keras.layers.Dense(3)])
    model.add(keras.layers.Dense(2))
    model(array_ops.ones([2, 4]))
    self.assertEqual(
        ['sequential/dense/kernel:0', 'sequential/dense/bias:0',
         'sequential/dense_1/kernel:0', 'sequential/dense_1/bias:0'],
        [v.name for v in model.variables])

  @keras_parameterized.run_all_keras_modes
  def test_input_assumptions_propagation(self):
    model = keras.models.Sequential()
    model.add(keras.layers.Dense(1))
    if context.executing_eagerly():
      with self.assertRaisesRegexp(ValueError,
                                   'expected min_ndim=2, found ndim=0'):
        model(1.0)


class TestSequentialEagerIntegration(keras_parameterized.TestCase):

  @keras_parameterized.run_all_keras_modes
  def test_defun_on_call(self):
    # Check that one can subclass Sequential and place the `call` in a `defun`.

    class MySequential(keras.Sequential):

      def __init__(self, name=None):
        super(MySequential, self).__init__(name=name)
        self.call = function.defun(self.call)

    model = MySequential()
    model.add(keras.layers.Dense(4, activation='relu'))
    model.add(keras.layers.Dense(5, activation='softmax'))

    model.compile(loss='mse', optimizer=rmsprop.RMSPropOptimizer(1e-3),
                  run_eagerly=testing_utils.should_run_eagerly())

    x = np.random.random((2, 6))
    y = np.random.random((2, 5))
    model.fit(x, y, epochs=1)

  @keras_parameterized.run_all_keras_modes
  def test_build_before_fit(self):
    # Fix for b/112433577
    model = testing_utils.get_small_sequential_mlp(4, 5)
    model.compile(loss='mse', optimizer=rmsprop.RMSPropOptimizer(1e-3),
                  run_eagerly=testing_utils.should_run_eagerly())

    model.build((None, 6))

    x = np.random.random((2, 6))
    y = np.random.random((2, 5))
    model.fit(x, y, epochs=1)

  @keras_parameterized.run_all_keras_modes
  def test_sequential_model_fails_with_dict_inputs(self):
    num_classes = 5
    model = testing_utils.get_small_sequential_mlp(
        num_hidden=10, num_classes=num_classes)
    model.compile(
        rmsprop.RMSPropOptimizer(learning_rate=0.001),
        metrics=['acc'],
        weighted_metrics=['mae'],
        loss='categorical_crossentropy',
        run_eagerly=testing_utils.should_run_eagerly())

    x = {'dense_input': np.random.random((10, 1))}
    y = np.random.randint(num_classes, size=(10, 1))

    with self.assertRaisesRegexp(
        ValueError, 'Passing a dictionary input to a Sequential Model which '
        'doesn\'t have FeatureLayer as the first layer is an error'):
      model.fit(x, y, batch_size=5, epochs=1)


if __name__ == '__main__':
  test.main()
