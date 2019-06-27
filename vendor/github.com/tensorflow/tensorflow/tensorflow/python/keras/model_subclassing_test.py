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
"""Tests for Model subclassing."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os

import numpy as np

from tensorflow.python import keras
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.eager import context
from tensorflow.python.framework import ops
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import embedding_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.platform import test
from tensorflow.python.training.checkpointable import data_structures
from tensorflow.python.training.rmsprop import RMSPropOptimizer

try:
  import h5py  # pylint:disable=g-import-not-at-top
except ImportError:
  h5py = None


# pylint: disable=not-callable
class SimpleTestModel(keras.Model):

  def __init__(self, use_bn=False, use_dp=False, num_classes=10):
    super(SimpleTestModel, self).__init__(name='test_model')
    self.use_bn = use_bn
    self.use_dp = use_dp
    self.num_classes = num_classes

    self.dense1 = keras.layers.Dense(32, activation='relu')
    self.dense2 = keras.layers.Dense(num_classes, activation='softmax')
    if self.use_dp:
      self.dp = keras.layers.Dropout(0.5)
    if self.use_bn:
      self.bn = keras.layers.BatchNormalization(axis=-1)

  def call(self, x):
    x = self.dense1(x)
    if self.use_dp:
      x = self.dp(x)
    if self.use_bn:
      x = self.bn(x)
    return self.dense2(x)


class SimpleConvTestModel(keras.Model):

  def __init__(self, num_classes=10):
    super(SimpleConvTestModel, self).__init__(name='test_model')
    self.num_classes = num_classes

    self.conv1 = keras.layers.Conv2D(32, (3, 3), activation='relu')
    self.flatten = keras.layers.Flatten()
    self.dense1 = keras.layers.Dense(num_classes, activation='softmax')

  def call(self, x):
    x = self.conv1(x)
    x = self.flatten(x)
    return self.dense1(x)


class MultiIOTestModel(keras.Model):

  def __init__(self, use_bn=False, use_dp=False, num_classes=(2, 3)):
    super(MultiIOTestModel, self).__init__(name='test_model')
    self.use_bn = use_bn
    self.use_dp = use_dp
    self.num_classes = num_classes

    self.dense1 = keras.layers.Dense(32, activation='relu')
    self.dense2 = keras.layers.Dense(num_classes[0], activation='softmax')
    self.dense3 = keras.layers.Dense(num_classes[1], activation='softmax')
    if use_dp:
      self.dp = keras.layers.Dropout(0.5)
    if use_bn:
      self.bn = keras.layers.BatchNormalization()

  def call(self, inputs):
    x1, x2 = inputs
    x1 = self.dense1(x1)
    x2 = self.dense1(x2)
    if self.use_dp:
      x1 = self.dp(x1)
    if self.use_bn:
      x2 = self.bn(x2)
    return [self.dense2(x1), self.dense3(x2)]


class NestedTestModel1(keras.Model):
  """A model subclass nested inside a model subclass.
  """

  def __init__(self, num_classes=2):
    super(NestedTestModel1, self).__init__(name='nested_model_1')
    self.num_classes = num_classes
    self.dense1 = keras.layers.Dense(32, activation='relu')
    self.dense2 = keras.layers.Dense(num_classes, activation='relu')
    self.bn = keras.layers.BatchNormalization()
    self.test_net = SimpleTestModel(num_classes=4,
                                    use_bn=True,
                                    use_dp=True)

  def call(self, inputs):
    x = self.dense1(inputs)
    x = self.bn(x)
    x = self.test_net(x)
    return self.dense2(x)


def get_functional_graph_model(input_dim, num_classes):
  # A simple functional-API model (a.k.a. graph network)
  inputs = keras.Input(shape=(input_dim,))
  x = keras.layers.Dense(32, activation='relu')(inputs)
  x = keras.layers.BatchNormalization()(x)
  outputs = keras.layers.Dense(num_classes)(x)
  return keras.Model(inputs, outputs)


class NestedTestModel2(keras.Model):
  """A model subclass with a functional-API graph network inside.
  """

  def __init__(self, num_classes=2):
    super(NestedTestModel2, self).__init__(name='nested_model_2')
    self.num_classes = num_classes
    self.dense1 = keras.layers.Dense(32, activation='relu')
    self.dense2 = keras.layers.Dense(num_classes, activation='relu')
    self.bn = self.bn = keras.layers.BatchNormalization()
    self.test_net = get_functional_graph_model(32, 4)

  def call(self, inputs):
    x = self.dense1(inputs)
    x = self.bn(x)
    x = self.test_net(x)
    return self.dense2(x)


def get_nested_model_3(input_dim, num_classes):
  # A functional-API model with a subclassed model inside.
  # NOTE: this requires the inner subclass to implement `compute_output_shape`.

  inputs = keras.Input(shape=(input_dim,))
  x = keras.layers.Dense(32, activation='relu')(inputs)
  x = keras.layers.BatchNormalization()(x)

  class Inner(keras.Model):

    def __init__(self):
      super(Inner, self).__init__()
      self.dense1 = keras.layers.Dense(32, activation='relu')
      self.dense2 = keras.layers.Dense(5, activation='relu')
      self.bn = keras.layers.BatchNormalization()

    def call(self, inputs):
      x = self.dense1(inputs)
      x = self.dense2(x)
      return self.bn(x)

  test_model = Inner()
  x = test_model(x)
  outputs = keras.layers.Dense(num_classes)(x)
  return keras.Model(inputs, outputs, name='nested_model_3')


@test_util.run_all_in_graph_and_eager_modes
class ModelSubclassingTest(test.TestCase):

  def test_custom_build(self):
    class DummyModel(keras.Model):

      def __init__(self):
        super(DummyModel, self).__init__()
        self.dense1 = keras.layers.Dense(32, activation='relu')
        self.uses_custom_build = False

      def call(self, inputs):
        return self.dense1(inputs)

      def build(self, input_shape):
        self.uses_custom_build = True

    test_model = DummyModel()
    dummy_data = array_ops.ones((32, 50))
    test_model(dummy_data)
    self.assertTrue(test_model.uses_custom_build, 'Model should use user '
                                                  'defined build when called.')

  def test_invalid_input_shape_build(self):
    num_classes = 2
    input_dim = 50

    model = SimpleTestModel(num_classes=num_classes,
                            use_dp=True,
                            use_bn=True)

    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    with self.assertRaisesRegexp(
        ValueError, 'input shape is not one of the valid types'):
      model.build(input_shape=tensor_shape.Dimension(input_dim))

  def test_embed_dtype_with_subclass_build(self):
    class Embedding(keras.layers.Layer):
      """An Embedding layer."""

      def __init__(self, vocab_size, embedding_dim, **kwargs):
        super(Embedding, self).__init__(**kwargs)
        self.vocab_size = vocab_size
        self.embedding_dim = embedding_dim

      def build(self, _):
        self.embedding = self.add_variable(
            'embedding_kernel',
            shape=[self.vocab_size, self.embedding_dim],
            dtype=np.float32,
            initializer=init_ops.random_uniform_initializer(-0.1, 0.1),
            trainable=True)

      def call(self, x):
        return embedding_ops.embedding_lookup(self.embedding, x)

    class EmbedModel(keras.Model):

      def __init__(self, vocab_size, embed_size):
        super(EmbedModel, self).__init__()
        self.embed1 = Embedding(vocab_size, embed_size)

      def call(self, inputs):
        return self.embed1(inputs)

    model = EmbedModel(100, 20)
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    with self.assertRaisesRegexp(
        ValueError, 'if your layers do not support float type inputs'):
      model.build(input_shape=(35, 20))

  def test_single_time_step_rnn_build(self):
    dim = 4
    timesteps = 1
    batch_input_shape = (None, timesteps, dim)
    units = 3

    class SimpleRNNModel(keras.Model):

      def __init__(self):
        super(SimpleRNNModel, self).__init__()
        self.lstm = keras.layers.LSTM(units)

      def call(self, inputs):
        return self.lstm(inputs)

    model = SimpleRNNModel()
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build(batch_input_shape)
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')
    model(array_ops.ones((32, timesteps, dim)))

  def test_single_io_subclass_build(self):
    num_classes = 2
    input_dim = 50
    batch_size = None

    model = SimpleTestModel(num_classes=num_classes,
                            use_dp=True,
                            use_bn=True)

    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build(input_shape=(batch_size, input_dim))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')
    model(array_ops.ones((32, input_dim)))

  def test_single_io_dimension_subclass_build(self):
    num_classes = 2
    input_dim = tensor_shape.Dimension(50)
    batch_size = tensor_shape.Dimension(None)

    model = SimpleTestModel(num_classes=num_classes,
                            use_dp=True,
                            use_bn=True)

    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build(input_shape=(batch_size, input_dim))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')
    model(array_ops.ones((32, input_dim)))

  def test_multidim_io_subclass_build(self):
    num_classes = 10
    # Input size, e.g. image
    batch_size = 32
    input_shape = (32, 32, 3)

    model = SimpleConvTestModel(num_classes)
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    batch_input_shape = (batch_size,) + input_shape
    model.build(input_shape=batch_input_shape)
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')

    model(array_ops.ones(batch_input_shape))

  def test_tensorshape_io_subclass_build(self):
    num_classes = 10
    # Input size, e.g. image
    batch_size = None
    input_shape = (32, 32, 3)

    model = SimpleConvTestModel(num_classes)
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build(
        input_shape=tensor_shape.TensorShape((batch_size,) + input_shape))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')

    model(array_ops.ones((32,) + input_shape))

  def test_subclass_save_model(self):
    num_classes = 10
    # Input size, e.g. image
    batch_size = None
    input_shape = (32, 32, 3)

    model = SimpleConvTestModel(num_classes)
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build(
        input_shape=tensor_shape.TensorShape((batch_size,) + input_shape))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')
    weights = model.get_weights()

    tf_format_name = os.path.join(self.get_temp_dir(), 'ckpt')
    model.save_weights(tf_format_name)
    if h5py is not None:
      hdf5_format_name = os.path.join(self.get_temp_dir(), 'weights.h5')
      model.save_weights(hdf5_format_name)

    model = SimpleConvTestModel(num_classes)
    model.build(
        input_shape=tensor_shape.TensorShape((batch_size,) + input_shape))
    if h5py is not None:
      model.load_weights(hdf5_format_name)
      self.assertAllClose(weights, model.get_weights())
    model.load_weights(tf_format_name)
    self.assertAllClose(weights, model.get_weights())

  def test_multi_io_subclass_build(self):
    batch_size = None
    num_samples = 1000
    input_dim = 50
    model = MultiIOTestModel()
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    batch_input_shape = tensor_shape.TensorShape((batch_size, input_dim))
    model.build(
        input_shape=[batch_input_shape, batch_input_shape])
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')
    x1 = array_ops.ones((num_samples, input_dim))
    x2 = array_ops.ones((num_samples, input_dim))
    model([x1, x2])

  def test_single_io_workflow_with_np_arrays(self):
    num_classes = 2
    num_samples = 100
    input_dim = 50

    model = SimpleTestModel(num_classes=num_classes,
                            use_dp=True,
                            use_bn=True)
    model.compile(
        loss='mse',
        optimizer=RMSPropOptimizer(learning_rate=0.001),
        metrics=['acc', keras.metrics.CategoricalAccuracy()])

    x = np.ones((num_samples, input_dim))
    y = np.zeros((num_samples, num_classes))

    model.fit(x, y, epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate(x, y, verbose=0)

  def test_multi_io_workflow_with_np_arrays(self):
    num_classes = (2, 3)
    num_samples = 1000
    input_dim = 50

    model = MultiIOTestModel(num_classes=num_classes,
                             use_dp=True,
                             use_bn=True)
    model.compile(loss='mse',
                  optimizer=RMSPropOptimizer(learning_rate=0.001),
                  metrics=['acc'])

    x1 = np.ones((num_samples, input_dim))
    x2 = np.ones((num_samples, input_dim))
    y1 = np.zeros((num_samples, num_classes[0]))
    y2 = np.zeros((num_samples, num_classes[1]))

    model.fit([x1, x2], [y1, y2], epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate([x1, x2], [y1, y2], verbose=0)

  def test_single_io_workflow_with_dataset_iterators(self):
    num_classes = 2
    num_samples = 10
    input_dim = 50

    with self.cached_session():
      model = SimpleTestModel(num_classes=num_classes, use_dp=True, use_bn=True)
      model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))

      x = np.ones((num_samples, input_dim), dtype=np.float32)
      y = np.zeros((num_samples, num_classes), dtype=np.float32)
      dataset = dataset_ops.Dataset.from_tensor_slices((x, y))
      dataset = dataset.repeat(100)
      dataset = dataset.batch(10)
      iterator = dataset_ops.make_one_shot_iterator(dataset)

      model.fit(iterator, epochs=2, steps_per_epoch=10, verbose=0)
      _ = model.evaluate(iterator, steps=10, verbose=0)

  def test_attributes(self):
    # layers, weights, trainable_weights, non_trainable_weights, inputs, outputs

    num_classes = (2, 3)
    num_samples = 100
    input_dim = 50

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)

    x1 = np.ones((num_samples, input_dim))
    x2 = np.ones((num_samples, input_dim))
    y1 = np.zeros((num_samples, num_classes[0]))
    y2 = np.zeros((num_samples, num_classes[1]))

    self.assertEqual(model.name, 'test_model')
    self.assertEqual(model.built, False)
    self.assertEqual(len(model.weights), 0)

    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    model.train_on_batch([x1, x2], [y1, y2])

    self.assertEqual(model.built, True)
    self.assertEqual(len(model.layers), 4)
    self.assertEqual(len(model.weights), 10)
    self.assertEqual(len(model.trainable_weights), 8)
    self.assertEqual(len(model.non_trainable_weights), 2)
    self.assertEqual(len(model.inputs), 2)
    self.assertEqual(len(model.outputs), 2)

  def test_updates(self):
    # test that updates get run during training
    num_samples = 100
    input_dim = 50

    class BNNet(keras.Model):

      def __init__(self):
        super(BNNet, self).__init__()
        self.bn = keras.layers.BatchNormalization(beta_initializer='ones',
                                                  gamma_initializer='ones')

      def call(self, inputs):
        return self.bn(inputs)

    x = np.ones((num_samples, input_dim))
    y = np.ones((num_samples, input_dim))

    model = BNNet()
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    y_ref = model.predict(x)

    model.train_on_batch(x, y)
    y_new = model.predict(x)
    self.assertGreater(np.sum(np.abs(y_ref - y_new)), 0.1)

  def test_training_and_inference_behavior(self):
    # test that dropout is applied in training and not inference

    num_samples = 100
    input_dim = 50

    class DPNet(keras.Model):

      def __init__(self):
        super(DPNet, self).__init__()
        self.dp = keras.layers.Dropout(0.5)
        self.dense = keras.layers.Dense(1,
                                        use_bias=False,
                                        kernel_initializer='ones')

      def call(self, inputs):
        x = self.dp(inputs)
        return self.dense(x)

    model = DPNet()
    x = np.ones((num_samples, input_dim))
    y = model.predict(x)
    self.assertEqual(np.sum(y), np.sum(x))
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    loss = model.train_on_batch(x, y)
    self.assertGreater(loss, 0.1)

  def test_training_methods(self):
    # test fit, train_on_batch
    # on different input types: list, dict

    num_classes = (2, 3)
    num_samples = 100
    input_dim = 50

    x1 = np.ones((num_samples, input_dim))
    x2 = np.ones((num_samples, input_dim))
    y1 = np.zeros((num_samples, num_classes[0]))
    y2 = np.zeros((num_samples, num_classes[1]))

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    model.fit([x1, x2], [y1, y2], epochs=2, batch_size=32, verbose=0)
    model.fit({'input_1': x1, 'input_2': x2},
              {'output_1': y1, 'output_2': y2},
              epochs=2, batch_size=32)
    model.fit([x1, x2], [y1, y2], epochs=2, batch_size=32, verbose=0,
              validation_data=([x1, x2], [y1, y2]))

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    model.train_on_batch([x1, x2], [y1, y2])
    model.train_on_batch({'input_1': x1, 'input_2': x2},
                         {'output_1': y1, 'output_2': y2})

  def test_inference_methods(self):
    # test predict, evaluate, test_on_batch, predict_on_batch
    # on different input types: list, dict
    num_classes = (2, 3)
    num_samples = 100
    input_dim = 50

    x1 = np.ones((num_samples, input_dim))
    x2 = np.ones((num_samples, input_dim))
    y1 = np.zeros((num_samples, num_classes[0]))
    y2 = np.zeros((num_samples, num_classes[1]))

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    model.evaluate([x1, x2], [y1, y2])
    model.test_on_batch([x1, x2], [y1, y2])

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.predict([x1, x2])

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.predict_on_batch([x1, x2])

  def test_saving(self):

    num_classes = (2, 3)
    num_samples = 100
    input_dim = 50

    x1 = np.ones((num_samples, input_dim))
    x2 = np.ones((num_samples, input_dim))
    y1 = np.zeros((num_samples, num_classes[0]))
    y2 = np.zeros((num_samples, num_classes[1]))

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    model.fit([x1, x2], [y1, y2], epochs=2, batch_size=32, verbose=0)
    y_ref_1, y_ref_2 = model.predict([x1, x2])

    tf_format_name = os.path.join(self.get_temp_dir(), 'ckpt')
    model.save_weights(tf_format_name)
    if h5py is not None:
      hdf5_format_name = os.path.join(self.get_temp_dir(), 'weights.h5')
      model.save_weights(hdf5_format_name)

    model = MultiIOTestModel(num_classes=num_classes, use_bn=True)

    if h5py is not None:
      with self.assertRaises(ValueError):
        model.load_weights(hdf5_format_name)

    model.load_weights(tf_format_name)

    y1, y2 = model.predict([x1, x2])
    self.assertAllClose(y_ref_1, y1, atol=1e-5)
    self.assertAllClose(y_ref_2, y2, atol=1e-5)

    if h5py is not None:
      model.load_weights(hdf5_format_name)

      y1, y2 = model.predict([x1, x2])
      self.assertAllClose(y_ref_1, y1, atol=1e-5)
      self.assertAllClose(y_ref_2, y2, atol=1e-5)

  def test_summary(self):

    class ToString(object):

      def __init__(self):
        self.contents = ''

      def __call__(self, msg):
        self.contents += msg + '\n'

    # Single-io
    model = SimpleTestModel(num_classes=4, use_bn=True, use_dp=True)
    model._set_inputs(np.ones((3, 4)))  # need to build model first
    print_fn = ToString()
    model.summary(print_fn=print_fn)
    self.assertTrue('Trainable params: 356' in print_fn.contents)

    # Multi-io
    model = MultiIOTestModel(num_classes=(5, 6), use_bn=True, use_dp=True)
    model._set_inputs([np.ones((3, 4)),
                       np.ones((3, 4))])  # need to build model first
    print_fn = ToString()
    model.summary(print_fn=print_fn)
    self.assertTrue('Trainable params: 587' in print_fn.contents)

  def test_subclass_nested_in_subclass(self):
    num_classes = 2
    num_samples = 100
    input_dim = 50

    model = NestedTestModel1(num_classes=num_classes)
    model.compile(loss='mse',
                  optimizer=RMSPropOptimizer(learning_rate=0.001),
                  metrics=['acc'])

    x = np.ones((num_samples, input_dim))
    y = np.zeros((num_samples, num_classes))

    model.fit(x, y, epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate(x, y, verbose=0)

    self.assertEqual(len(model.weights), 8 + len(model.test_net.weights))
    self.assertEqual(len(model.non_trainable_weights),
                     2 + len(model.test_net.non_trainable_weights))
    self.assertEqual(len(model.trainable_weights),
                     6 + len(model.test_net.trainable_weights))

  def test_graph_nested_in_subclass(self):
    num_classes = 2
    num_samples = 100
    input_dim = 50

    model = NestedTestModel2(num_classes=num_classes)
    model.compile(loss='mse',
                  optimizer=RMSPropOptimizer(learning_rate=0.001),
                  metrics=['acc'])

    x = np.ones((num_samples, input_dim))
    y = np.zeros((num_samples, num_classes))

    model.fit(x, y, epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate(x, y, verbose=0)

    self.assertEqual(len(model.weights), 8 + len(model.test_net.weights))
    self.assertEqual(len(model.non_trainable_weights),
                     2 + len(model.test_net.non_trainable_weights))
    self.assertEqual(len(model.trainable_weights),
                     6 + len(model.test_net.trainable_weights))

  def test_subclass_nested_in_graph(self):
    num_classes = 2
    num_samples = 100
    input_dim = 50

    model = get_nested_model_3(input_dim=input_dim, num_classes=num_classes)
    model.compile(loss='mse',
                  optimizer=RMSPropOptimizer(learning_rate=0.001),
                  metrics=['acc'])

    x = np.ones((num_samples, input_dim))
    y = np.zeros((num_samples, num_classes))

    model.fit(x, y, epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate(x, y, verbose=0)

    self.assertEqual(len(model.weights), 16)
    self.assertEqual(len(model.non_trainable_weights), 4)
    self.assertEqual(len(model.trainable_weights), 12)

  def test_subclass_nested_in_sequential(self):
    num_classes = 2
    num_samples = 100
    input_dim = 50

    class Inner(keras.Model):

      def __init__(self):
        super(Inner, self).__init__()
        self.dense1 = keras.layers.Dense(32, activation='relu')
        self.dense2 = keras.layers.Dense(num_classes, activation='relu')
        self.bn = keras.layers.BatchNormalization()

      def call(self, inputs):
        x = self.dense1(inputs)
        x = self.dense2(x)
        return self.bn(x)

    model = keras.Sequential([Inner()])
    model.compile(loss='mse',
                  optimizer=RMSPropOptimizer(learning_rate=0.001),
                  metrics=['acc'])

    x = np.ones((num_samples, input_dim))
    y = np.zeros((num_samples, num_classes))
    model.fit(x, y, epochs=2, batch_size=32, verbose=0)
    _ = model.evaluate(x, y, verbose=0)

    self.assertEqual(len(model.weights), 8)
    self.assertEqual(len(model.non_trainable_weights), 2)
    self.assertEqual(len(model.trainable_weights), 6)

  def test_support_for_manual_training_arg(self):
    # In most cases, the `training` argument is left unspecified, in which
    # case it defaults to value corresponding to the Model method being used
    # (fit -> True, predict -> False, etc).
    # If the user writes their model `call` method to take
    # an explicit `training` argument, we must check that the correct value
    # is being passed to the model for each method call.

    class DPNet(keras.Model):

      def __init__(self):
        super(DPNet, self).__init__()
        self.dp = keras.layers.Dropout(0.5)
        self.dense = keras.layers.Dense(1,
                                        use_bias=False,
                                        kernel_initializer='ones')

      def call(self, inputs, training=False):
        x = self.dp(inputs, training=training)
        return self.dense(x)

    model = DPNet()
    x = np.ones((10, 10))
    y = model.predict(x)
    self.assertEqual(np.sum(y), np.sum(x))
    model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))
    loss = model.train_on_batch(x, y)
    self.assertGreater(loss, 0.1)

  def test_no_dependency(self):
    class Foo(keras.Model):

      def __init__(self):
        super(Foo, self).__init__()
        self.isdep = keras.layers.Dense(1)
        self.notdep = data_structures.NoDependency(keras.layers.Dense(2))
        self.notdep_var = data_structures.NoDependency(
            resource_variable_ops.ResourceVariable(1., name='notdep_var'))

    m = Foo()
    self.assertEqual([m.isdep, m.notdep], m.layers)
    self.assertEqual(1, len(m._checkpoint_dependencies))
    self.assertIs(m.isdep, m._checkpoint_dependencies[0].ref)
    self.assertEqual('notdep_var:0', m.notdep_var.name)

  def test_extra_variable(self):

    class ExtraVar(keras.Model):

      def __init__(self):
        super(ExtraVar, self).__init__()
        self.dense = keras.layers.Dense(1)
        self.var = resource_variable_ops.ResourceVariable(1.)
        self.not_trainable_var = resource_variable_ops.ResourceVariable(
            2., trainable=False)

      def call(self, inputs):
        return self.dense(inputs + self.var)

    m = ExtraVar()
    self.assertTrue(m.trainable)
    self.assertEqual([m.dense], m.layers)
    self.assertEqual([m.var, m.not_trainable_var], m.variables)
    self.assertEqual([m.var], m.trainable_variables)
    self.assertEqual([m.not_trainable_var], m.non_trainable_variables)
    m.trainable = False
    self.assertEqual([m.var, m.not_trainable_var], m.variables)
    self.assertEqual([], m.trainable_variables)
    self.assertEqual([m.var, m.not_trainable_var], m.non_trainable_variables)
    m.trainable = True

    m(array_ops.ones([1, 1]))

    self.assertEqual([m.dense.kernel, m.dense.bias], m.dense.variables)
    self.assertEqual([m.dense.kernel, m.dense.bias], m.dense.weights)

    self.assertEqual([m.dense.kernel, m.dense.bias, m.var, m.not_trainable_var],
                     m.variables)
    self.assertEqual([m.dense.kernel, m.dense.bias, m.var],
                     m.trainable_variables)
    self.assertEqual([m.not_trainable_var], m.non_trainable_variables)

    m.dense.trainable = False
    self.assertEqual(
        [m.var, m.dense.kernel, m.dense.bias, m.not_trainable_var],
        m.variables)
    self.assertEqual([m.var], m.trainable_variables)
    self.assertEqual([m.dense.kernel, m.dense.bias, m.not_trainable_var],
                     m.non_trainable_variables)

  @test_util.run_in_graph_and_eager_modes
  def test_add_weight_in_model(self):

    class MyModel(keras.Model):

      def __init__(self):
        super(MyModel, self).__init__()
        self.b = self.add_weight('bias', (10,))
        self.c = self.add_weight('bias2', (10,), trainable=False)

      def call(self, inputs):
        return inputs + self.b + self.c

    x = ops.convert_to_tensor(np.ones((10, 10), 'float32'))
    model = MyModel()
    model(x)
    self.assertEqual(1, len(model.trainable_weights))
    self.assertEqual(1, len(model.non_trainable_weights))
    self.assertEqual(2, len(model.weights))

    class MyModelCustomBuild(keras.Model):

      def build(self, input_shape):
        self.b = self.add_weight('bias', (10,))
        self.c = self.add_weight('bias2', (10,), trainable=False)

      def call(self, inputs):
        return inputs + self.b + self.c

    x = ops.convert_to_tensor(np.ones((10, 10), 'float32'))
    model = MyModelCustomBuild()
    model(x)
    self.assertEqual(1, len(model.trainable_weights))
    self.assertEqual(1, len(model.non_trainable_weights))
    self.assertEqual(2, len(model.weights))

  def test_add_update_in_model(self):

    class MyModel(keras.Model):

      def __init__(self):
        super(MyModel, self).__init__()
        self.b = self.add_weight('bias', (10,))
        self.c = self.add_weight('bias2', (10,))

      def call(self, inputs):
        # Unconditional
        self.add_update(self.b.assign(self.b * 2))
        # Conditional
        self.add_update(self.c.assign(inputs[1, :]), inputs)
        return inputs + self.b + self.c

    x = ops.convert_to_tensor(np.ones((10, 10), 'float32'))
    model = MyModel()
    model(x)

    if context.executing_eagerly():
      self.assertEqual(0, len(model.updates))
    else:
      self.assertEqual(2, len(model.updates))
      self.assertEqual(1, len(model.get_updates_for(None)))
      self.assertEqual(1, len(model.get_updates_for(x)))


class GraphSpecificModelSubclassingTests(test.TestCase):

  @test_util.run_deprecated_v1
  def test_single_io_workflow_with_tensors(self):
    num_classes = 2
    num_samples = 10
    input_dim = 50

    with self.cached_session():
      model = SimpleTestModel(num_classes=num_classes,
                              use_dp=True,
                              use_bn=True)
      model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))

      x = array_ops.ones((num_samples, input_dim))
      y = array_ops.zeros((num_samples, num_classes))

      model.fit(x, y, epochs=2, steps_per_epoch=10, verbose=0)
      _ = model.evaluate(steps=10, verbose=0)

  @test_util.run_deprecated_v1
  def test_multi_io_workflow_with_tensors(self):
    num_classes = (2, 3)
    num_samples = 10
    input_dim = 50

    with self.cached_session():
      model = MultiIOTestModel(num_classes=num_classes,
                               use_dp=True,
                               use_bn=True)
      model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))

      x1 = array_ops.ones((num_samples, input_dim))
      x2 = array_ops.ones((num_samples, input_dim))
      y1 = array_ops.zeros((num_samples, num_classes[0]))
      y2 = array_ops.zeros((num_samples, num_classes[1]))

      model.fit([x1, x2], [y1, y2], epochs=2, steps_per_epoch=10, verbose=0)
      _ = model.evaluate(steps=10, verbose=0)

  @test_util.run_deprecated_v1
  def test_updates_and_losses_for_nested_models_in_subclassed_model(self):

    # Case 1: deferred-build sequential nested in subclass.
    class TestModel1(keras.Model):

      def __init__(self):
        super(TestModel1, self).__init__()
        self.fc = keras.layers.Dense(10, input_shape=(784,),
                                     activity_regularizer='l1')
        self.bn = keras.Sequential([keras.layers.BatchNormalization(axis=1)])

      def call(self, x):
        return self.bn(self.fc(x))

    with self.cached_session():
      model = TestModel1()

      x = array_ops.ones(shape=[100, 784], dtype='float32')
      model(x)
      self.assertEqual(len(model.get_updates_for(x)), 2)
      self.assertEqual(len(model.get_losses_for(x)), 1)

    # Case 2: placeholder-sequential nested in subclass.
    class TestModel2(keras.Model):

      def __init__(self):
        super(TestModel2, self).__init__()
        self.fc = keras.layers.Dense(10, input_shape=(784,),
                                     activity_regularizer='l1')
        self.bn = keras.Sequential(
            [keras.layers.BatchNormalization(axis=1, input_shape=(10,))])

      def call(self, x):
        return self.bn(self.fc(x))

    with self.cached_session():
      model = TestModel2()

      x = array_ops.ones(shape=[100, 784], dtype='float32')
      model(x)
      self.assertEqual(len(model.get_updates_for(x)), 2)
      self.assertEqual(len(model.get_losses_for(x)), 1)

    # Case 3: functional-API model nested in subclass.
    inputs = keras.Input((10,))
    outputs = keras.layers.BatchNormalization(axis=1)(inputs)
    bn = keras.Model(inputs, outputs)

    class TestModel3(keras.Model):

      def __init__(self):
        super(TestModel3, self).__init__()
        self.fc = keras.layers.Dense(10, input_shape=(784,),
                                     activity_regularizer='l1')
        self.bn = bn

      def call(self, x):
        return self.bn(self.fc(x))

    with self.cached_session():
      model = TestModel3()

      x = array_ops.ones(shape=[100, 784], dtype='float32')
      model(x)
      self.assertEqual(len(model.get_updates_for(x)), 2)
      self.assertEqual(len(model.get_losses_for(x)), 1)

  @test_util.run_deprecated_v1
  def test_multi_io_workflow_with_numpy_arrays_and_custom_placeholders(self):
    num_classes = (2, 3)
    num_samples = 1000
    input_dim = 50

    with self.cached_session():
      model = MultiIOTestModel(num_classes=num_classes,
                               use_dp=True,
                               use_bn=True)
      model.compile(loss='mse', optimizer=RMSPropOptimizer(learning_rate=0.001))

      x1 = np.ones((num_samples, input_dim))
      x2 = np.ones((num_samples, input_dim))
      y1 = np.zeros((num_samples, num_classes[0]))
      y2 = np.zeros((num_samples, num_classes[1]))

      x2_placeholder = array_ops.placeholder(
          dtype='float32', shape=(None, input_dim))
      model._set_inputs([x1, x2_placeholder])

      model.fit([x1, x2], [y1, y2], epochs=2, batch_size=32, verbose=0)
      _ = model.evaluate([x1, x2], [y1, y2], verbose=0)


class CustomCallModel(keras.Model):

  def __init__(self):
    super(CustomCallModel, self).__init__()
    self.dense1 = keras.layers.Dense(1, activation='relu')
    self.dense2 = keras.layers.Dense(1, activation='softmax')

  def call(self, first, second, fiddle_with_output='no', training=True):
    combined = self.dense1(first) + self.dense2(second)
    if fiddle_with_output == 'yes':
      return 10. * combined
    else:
      return combined


class TrainingNoDefaultModel(keras.Model):

  def __init__(self):
    super(TrainingNoDefaultModel, self).__init__()
    self.dense1 = keras.layers.Dense(1)

  def call(self, x, training):
    return self.dense1(x)


class TrainingMaskingModel(keras.Model):

  def __init__(self):
    super(TrainingMaskingModel, self).__init__()
    self.dense1 = keras.layers.Dense(1)

  def call(self, x, training=False, mask=None):
    return self.dense1(x)


class CustomCallSignatureTests(test.TestCase):

  @test_util.run_in_graph_and_eager_modes
  def test_no_inputs_in_signature(self):
    model = CustomCallModel()
    first = array_ops.ones([2, 3])
    second = array_ops.ones([2, 5])
    output = model(first, second)
    self.evaluate([v.initializer for v in model.variables])
    expected_output = self.evaluate(model.dense1(first) + model.dense2(second))
    self.assertAllClose(expected_output, self.evaluate(output))
    output = model(first, second, fiddle_with_output='yes')
    self.assertAllClose(10. * expected_output, self.evaluate(output))
    output = model(first, second=second, training=False)
    self.assertAllClose(expected_output, self.evaluate(output))

  @test_util.run_in_graph_and_eager_modes
  def test_training_args_call_build(self):
    input_dim = 2

    model = TrainingNoDefaultModel()
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build((None, input_dim))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')

  @test_util.run_in_graph_and_eager_modes
  def test_training_and_mask_args_call_build(self):
    input_dim = 2

    model = TrainingMaskingModel()
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    model.build((None, input_dim))
    self.assertTrue(model.weights, ('Model should have weights now that it '
                                    'has been properly built.'))
    self.assertTrue(model.built, 'Model should be built after calling `build`.')

  @test_util.run_in_graph_and_eager_modes
  def test_custom_call_kwargs_and_build(self):
    first_input_shape = (2, 3)
    second_input_shape = (2, 5)

    model = CustomCallModel()
    self.assertFalse(model.built, 'Model should not have been built')
    self.assertFalse(model.weights, ('Model should have no weights since it '
                                     'has not been built.'))
    with self.assertRaisesRegexp(
        ValueError, 'cannot build your model if it has positional'):
      model.build(input_shape=[first_input_shape, second_input_shape])

  @test_util.run_in_graph_and_eager_modes
  def test_inputs_in_signature(self):

    class HasInputsAndOtherPositional(keras.Model):

      def call(self, inputs, some_other_arg, training=False):
        return inputs

      def compute_output_shape(self, input_shape):
        return input_shape

    model = HasInputsAndOtherPositional()
    with self.assertRaisesRegexp(
        TypeError, 'everything else as a keyword argument'):
      x1, x2 = keras.Input((1, 1)), keras.Input((1, 1))
      model(x1, x2)

  @test_util.run_in_graph_and_eager_modes
  def test_kwargs_in_signature(self):

    class HasKwargs(keras.Model):

      def call(self, x, y=3, **kwargs):
        return x

    model = HasKwargs()
    arg = array_ops.ones([])
    model(arg, a=3)
    if not context.executing_eagerly():
      self.assertEqual(len(model.inputs), 1)

  @test_util.run_in_graph_and_eager_modes
  def test_args_in_signature(self):

    class HasArgs(keras.Model):

      def call(self, x, *args, **kwargs):
        return [x] + list(args)

      def compute_output_shape(self, input_shape):
        return input_shape

    model = HasArgs()
    x1, x2, x3 = keras.Input((1, 1)), keras.Input((1, 1)), keras.Input((1, 1))
    model(x1, x2, x3, a=3)
    self.assertEqual(len(model.inputs), 3)

  def test_args_and_keywords_in_signature(self):

    class HasArgs(keras.Model):

      def call(self, x, training=True, *args, **kwargs):
        return x

    with context.graph_mode():
      model = HasArgs()
      x1, x2, x3 = keras.Input((1, 1)), keras.Input((1, 1)), keras.Input((1, 1))
      with self.assertRaisesRegexp(
          TypeError, 'may not accept both positional arguments and '):
        model(x1, x2, x3, a=3)

  def test_training_no_default(self):

    with context.graph_mode():
      model = TrainingNoDefaultModel()
      arg = array_ops.ones([1, 1])
      model(arg, True)
      self.assertEqual(len(model.inputs), 1)

  def test_training_no_default_with_positional(self):

    class TrainingNoDefaultWithPositional(keras.Model):

      def call(self, x, training, positional):
        return x

    with context.graph_mode():
      model = TrainingNoDefaultWithPositional()
      x1, x2, x3 = keras.Input((1, 1)), keras.Input((1, 1)), keras.Input((1, 1))
      with self.assertRaisesRegexp(TypeError, 'after a non-input'):
        model(x1, x2, x3)

if __name__ == '__main__':
  test.main()
