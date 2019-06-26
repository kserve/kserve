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
"""Tests for tf.layers.convolutional."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.layers import convolutional as conv_layers
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops import variables
from tensorflow.python.platform import test


class ConvTest(test.TestCase):

  def testInvalidDataFormat(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'data_format'):
      conv_layers.conv2d(images, 32, 3, data_format='invalid')

  def testInvalidStrides(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv2d(images, 32, 3, strides=(1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv2d(images, 32, 3, strides=None)

  def testInvalidKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv2d(images, 32, (1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv2d(images, 32, None)

  @test_util.run_deprecated_v1
  def testCreateConv2D(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2D(32, [3, 3], activation=nn_ops.relu)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'conv2d/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testConv2DFloat16(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4), dtype='float16')
    output = conv_layers.conv2d(images, 32, [3, 3], activation=nn_ops.relu)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])

  def testCreateConv2DIntegerKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2D(32, 3)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testCreateConv2DChannelsFirst(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, 4, height, width))
    layer = conv_layers.Conv2D(32, [3, 3], data_format='channels_first')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height - 2, width - 2])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testUnknownInputChannels(self):
    images = array_ops.placeholder(dtypes.float32, (5, 7, 9, None))
    layer = conv_layers.Conv2D(32, [3, 3], activation=nn_ops.relu)
    with self.assertRaisesRegexp(ValueError,
                                 'The channel dimension of the inputs '
                                 'should be defined. Found `None`.'):
      _ = layer.apply(images)

    images = array_ops.placeholder(dtypes.float32, (5, None, 7, 9))
    layer = conv_layers.Conv2D(32, [3, 3], data_format='channels_first')
    with self.assertRaisesRegexp(ValueError,
                                 'The channel dimension of the inputs '
                                 'should be defined. Found `None`.'):
      _ = layer.apply(images)

  def testConv2DPaddingSame(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 32), seed=1)
    layer = conv_layers.Conv2D(64, images.get_shape()[1:3], padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(), [5, height, width, 64])

  def testCreateConvWithStrides(self):
    height, width = 6, 8
    # Test strides tuple
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    layer = conv_layers.Conv2D(32, [3, 3], strides=(2, 2), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width / 2, 32])

    # Test strides integer
    layer = conv_layers.Conv2D(32, [3, 3], strides=2, padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width / 2, 32])

    # Test unequal strides
    layer = conv_layers.Conv2D(32, [3, 3], strides=(2, 1), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width, 32])

  @test_util.run_deprecated_v1
  def testCreateConv1D(self):
    width = 7
    data = random_ops.random_uniform((5, width, 4))
    layer = conv_layers.Conv1D(32, 3, activation=nn_ops.relu)
    output = layer.apply(data)
    self.assertEqual(output.op.name, 'conv1d/Relu')
    self.assertListEqual(output.get_shape().as_list(), [5, width - 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testConv1DFloat16(self):
    width = 7
    data = random_ops.random_uniform((5, width, 4), dtype='float16')
    output = conv_layers.conv1d(data, 32, 3, activation=nn_ops.relu)
    self.assertListEqual(output.get_shape().as_list(), [5, width - 2, 32])

  @test_util.run_deprecated_v1
  def testCreateConv1DChannelsFirst(self):
    width = 7
    data = random_ops.random_uniform((5, 4, width))
    layer = conv_layers.Conv1D(32, 3, data_format='channels_first')
    output = layer.apply(data)
    self.assertListEqual(output.get_shape().as_list(), [5, 32, width - 2])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testUnknownInputChannelsConv1D(self):
    data = array_ops.placeholder(dtypes.float32, (5, 4, None))
    layer = conv_layers.Conv1D(32, 3, activation=nn_ops.relu)
    with self.assertRaisesRegexp(ValueError,
                                 'The channel dimension of the inputs '
                                 'should be defined. Found `None`.'):
      _ = layer.apply(data)

    data = array_ops.placeholder(dtypes.float32, (5, None, 4))
    layer = conv_layers.Conv1D(32, 3, data_format='channels_first')
    with self.assertRaisesRegexp(ValueError,
                                 'The channel dimension of the inputs '
                                 'should be defined. Found `None`.'):
      _ = layer.apply(data)

  @test_util.run_deprecated_v1
  def testCreateConv3D(self):
    depth, height, width = 6, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 4))
    layer = conv_layers.Conv3D(32, [3, 3, 3], activation=nn_ops.relu)
    output = layer.apply(volumes)
    self.assertEqual(output.op.name, 'conv3d/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth - 2, height - 2, width - 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testUnknownInputChannelsConv3D(self):
    volumes = array_ops.placeholder(dtypes.float32, (5, 6, 7, 9, None))
    layer = conv_layers.Conv3D(32, [3, 3, 3], activation=nn_ops.relu)
    with self.assertRaisesRegexp(ValueError,
                                 'The channel dimension of the inputs '
                                 'should be defined. Found `None`.'):
      _ = layer.apply(volumes)

  @test_util.run_deprecated_v1
  def testConv2DKernelRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv2D(32, [3, 3], kernel_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv2DBiasRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv2D(32, [3, 3], bias_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv2DNoBias(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2D(
        32, [3, 3], activation=nn_ops.relu, use_bias=False)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'conv2d/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 4, 32])
    self.assertEqual(layer.bias, None)

  def testDilatedConv2D(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2D(32, [3, 3], dilation_rate=3)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(), [5, 1, 3, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

    # Test tuple dilation rate
    layer = conv_layers.Conv2D(32, [3, 3], dilation_rate=(1, 3))
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(), [5, height - 2, 3, 32])

  @test_util.run_deprecated_v1
  def testFunctionalConv2DReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.conv2d(images, 32, [3, 3], name='conv1')
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv2d(images, 32, [3, 3], name='conv1', reuse=True)
    self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DReuseFromScope(self):
    with variable_scope.variable_scope('scope'):
      height, width = 7, 9
      images = random_ops.random_uniform((5, height, width, 3), seed=1)
      conv_layers.conv2d(images, 32, [3, 3], name='conv1')
      self.assertEqual(len(variables.trainable_variables()), 2)
    with variable_scope.variable_scope('scope', reuse=True):
      conv_layers.conv2d(images, 32, [3, 3], name='conv1')
      self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DInitializerFromScope(self):
    with self.cached_session() as sess:
      with variable_scope.variable_scope(
          'scope', initializer=init_ops.ones_initializer()):
        height, width = 7, 9
        images = random_ops.random_uniform((5, height, width, 3), seed=1)
        conv_layers.conv2d(images, 32, [3, 3], name='conv1')
        weights = variables.trainable_variables()
        # Check the names of weights in order.
        self.assertTrue('kernel' in weights[0].name)
        self.assertTrue('bias' in weights[1].name)
        self.evaluate(variables.global_variables_initializer())
        weights = self.evaluate(weights)
        # Check that the kernel weights got initialized to ones (from scope)
        self.assertAllClose(weights[0], np.ones((3, 3, 3, 32)))
        # Check that the bias still got initialized to zeros.
        self.assertAllClose(weights[1], np.zeros((32)))

  @test_util.run_deprecated_v1
  def testFunctionalConv2DNoReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.conv2d(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv2d(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 4)

  def testConstraints(self):
    # Conv1D
    k_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    conv1d = conv_layers.Conv1D(2, 3,
                                kernel_constraint=k_constraint,
                                bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 5), seed=1)
    conv1d(inputs)
    self.assertEqual(conv1d.kernel_constraint, k_constraint)
    self.assertEqual(conv1d.bias_constraint, b_constraint)

    # Conv2D
    k_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    conv2d = conv_layers.Conv2D(2, 3,
                                kernel_constraint=k_constraint,
                                bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 3, 5), seed=1)
    conv2d(inputs)
    self.assertEqual(conv2d.kernel_constraint, k_constraint)
    self.assertEqual(conv2d.bias_constraint, b_constraint)

    # Conv3D
    k_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    conv3d = conv_layers.Conv3D(2, 3,
                                kernel_constraint=k_constraint,
                                bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 3, 3, 5), seed=1)
    conv3d(inputs)
    self.assertEqual(conv3d.kernel_constraint, k_constraint)
    self.assertEqual(conv3d.bias_constraint, b_constraint)

  @test_util.run_deprecated_v1
  def testConv3DChannelsFirst(self):
    # Test case for GitHub issue 15655
    images = array_ops.placeholder(
        dtype=dtypes.float32, shape=[None, 1, 32, 32, 32])
    conv_layers.conv3d(images, 32, 9, data_format='channels_first')


class SeparableConv1DTest(test.TestCase):

  def testInvalidDataFormat(self):
    length = 9
    data = random_ops.random_uniform((5, length, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'data_format'):
      conv_layers.separable_conv1d(data, 32, 3, data_format='invalid')

  def testInvalidStrides(self):
    length = 9
    data = random_ops.random_uniform((5, length, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.separable_conv1d(data, 32, 3, strides=(1, 2))

    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.separable_conv1d(data, 32, 3, strides=None)

  def testInvalidKernelSize(self):
    length = 9
    data = random_ops.random_uniform((5, length, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.separable_conv1d(data, 32, (1, 2))

    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.separable_conv1d(data, 32, None)

  @test_util.run_deprecated_v1
  def testCreateSeparableConv1D(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    layer = conv_layers.SeparableConv1D(32, 3, activation=nn_ops.relu)
    output = layer.apply(data)
    self.assertEqual(output.op.name, 'separable_conv1d/Relu')
    self.assertEqual(output.get_shape().as_list(), [5, length - 2, 32])
    self.assertEqual(layer.depthwise_kernel.get_shape().as_list(), [3, 4, 1])
    self.assertEqual(layer.pointwise_kernel.get_shape().as_list(), [1, 4, 32])
    self.assertEqual(layer.bias.get_shape().as_list(), [32])

  def testCreateSeparableConv1DDepthMultiplier(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    layer = conv_layers.SeparableConv1D(32, 3, depth_multiplier=2)
    output = layer.apply(data)
    self.assertEqual(output.get_shape().as_list(), [5, length - 2, 32])
    self.assertEqual(layer.depthwise_kernel.get_shape().as_list(), [3, 4, 2])
    self.assertEqual(layer.pointwise_kernel.get_shape().as_list(), [1, 8, 32])
    self.assertEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testCreateSeparableConv1DChannelsFirst(self):
    length = 9
    data = random_ops.random_uniform((5, 4, length))
    layer = conv_layers.SeparableConv1D(32, 3, data_format='channels_first')
    output = layer.apply(data)
    self.assertEqual(output.get_shape().as_list(), [5, 32, length - 2])
    self.assertEqual(layer.depthwise_kernel.get_shape().as_list(), [3, 4, 1])
    self.assertEqual(layer.pointwise_kernel.get_shape().as_list(), [1, 4, 32])
    self.assertEqual(layer.bias.get_shape().as_list(), [32])

  def testSeparableConv1DPaddingSame(self):
    length = 9
    data = random_ops.random_uniform((5, length, 32), seed=1)
    layer = conv_layers.SeparableConv1D(
        64, length, padding='same')
    output = layer.apply(data)
    self.assertEqual(output.get_shape().as_list(), [5, length, 64])

  def testCreateSeparableConv1DWithStrides(self):
    length = 10
    data = random_ops.random_uniform((5, length, 3), seed=1)
    layer = conv_layers.SeparableConv1D(32, 3, strides=2, padding='same')
    output = layer.apply(data)
    self.assertEqual(output.get_shape().as_list(), [5, length // 2, 32])

  @test_util.run_deprecated_v1
  def testCreateSeparableConv1DWithStridesChannelsFirst(self):
    data_format = 'channels_first'
    length = 10
    data = random_ops.random_uniform((5, 3, length), seed=1)
    layer = conv_layers.SeparableConv1D(
        32, 3, strides=2, padding='same', data_format=data_format)
    output = layer.apply(data)
    self.assertEqual(output.get_shape().as_list(), [5, 32, length // 2])

  @test_util.run_deprecated_v1
  def testFunctionalConv1DReuse(self):
    length = 10
    data = random_ops.random_uniform((5, length, 3), seed=1)
    conv_layers.separable_conv1d(data, 32, 3, name='sepconv1')
    self.assertEqual(len(variables.trainable_variables()), 3)
    conv_layers.separable_conv1d(data, 32, 3, name='sepconv1', reuse=True)
    self.assertEqual(len(variables.trainable_variables()), 3)

  @test_util.run_deprecated_v1
  def testFunctionalConv1DReuseFromScope(self):
    with variable_scope.variable_scope('scope'):
      length = 10
      data = random_ops.random_uniform((5, length, 3), seed=1)
      conv_layers.separable_conv1d(data, 32, 3, name='sepconv1')
      self.assertEqual(len(variables.trainable_variables()), 3)
    with variable_scope.variable_scope('scope', reuse=True):
      conv_layers.separable_conv1d(data, 32, 3, name='sepconv1')
      self.assertEqual(len(variables.trainable_variables()), 3)

  @test_util.run_deprecated_v1
  def testFunctionalConv1DNoReuse(self):
    length = 10
    data = random_ops.random_uniform((5, length, 3), seed=1)
    conv_layers.separable_conv1d(data, 32, 3)
    self.assertEqual(len(variables.trainable_variables()), 3)
    conv_layers.separable_conv1d(data, 32, 3)
    self.assertEqual(len(variables.trainable_variables()), 6)

  @test_util.run_deprecated_v1
  def testSeparableConv1DDepthwiseRegularizer(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv1D(32, 3, depthwise_regularizer=reg)
    layer.apply(data)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv1DPointwiseRegularizer(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv1D(32, 3, pointwise_regularizer=reg)
    layer.apply(data)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv1DBiasRegularizer(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv1D(32, 3, bias_regularizer=reg)
    layer.apply(data)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv1DNoBias(self):
    length = 9
    data = random_ops.random_uniform((5, length, 4))
    layer = conv_layers.SeparableConv1D(
        32, 3, activation=nn_ops.relu, use_bias=False)
    output = layer.apply(data)
    self.assertEqual(output.op.name, 'separable_conv1d/Relu')
    self.assertEqual(layer.bias, None)

  def testConstraints(self):
    d_constraint = lambda x: x / math_ops.reduce_sum(x)
    p_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    layer = conv_layers.SeparableConv1D(2, 3,
                                        depthwise_constraint=d_constraint,
                                        pointwise_constraint=p_constraint,
                                        bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 5), seed=1)
    layer(inputs)
    self.assertEqual(layer.depthwise_constraint, d_constraint)
    self.assertEqual(layer.pointwise_constraint, p_constraint)
    self.assertEqual(layer.bias_constraint, b_constraint)


class SeparableConv2DTest(test.TestCase):

  def testInvalidDataFormat(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'data_format'):
      conv_layers.separable_conv2d(images, 32, 3, data_format='invalid')

  def testInvalidStrides(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.separable_conv2d(images, 32, 3, strides=(1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.separable_conv2d(images, 32, 3, strides=None)

  def testInvalidKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.separable_conv2d(images, 32, (1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.separable_conv2d(images, 32, None)

  @test_util.run_deprecated_v1
  def testCreateSeparableConv2D(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.SeparableConv2D(32, [3, 3], activation=nn_ops.relu)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'separable_conv2d/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.depthwise_kernel.get_shape().as_list(),
                         [3, 3, 4, 1])
    self.assertListEqual(layer.pointwise_kernel.get_shape().as_list(),
                         [1, 1, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testCreateSeparableConv2DDepthMultiplier(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.SeparableConv2D(32, [3, 3], depth_multiplier=2)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.depthwise_kernel.get_shape().as_list(),
                         [3, 3, 4, 2])
    self.assertListEqual(layer.pointwise_kernel.get_shape().as_list(),
                         [1, 1, 8, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testCreateSeparableConv2DIntegerKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.SeparableConv2D(32, 3)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.depthwise_kernel.get_shape().as_list(),
                         [3, 3, 4, 1])
    self.assertListEqual(layer.pointwise_kernel.get_shape().as_list(),
                         [1, 1, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  @test_util.run_deprecated_v1
  def testCreateSeparableConv2DChannelsFirst(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, 4, height, width))
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], data_format='channels_first')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height - 2, width - 2])
    self.assertListEqual(layer.depthwise_kernel.get_shape().as_list(),
                         [3, 3, 4, 1])
    self.assertListEqual(layer.pointwise_kernel.get_shape().as_list(),
                         [1, 1, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testSeparableConv2DPaddingSame(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 32), seed=1)
    layer = conv_layers.SeparableConv2D(
        64, images.get_shape()[1:3], padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(), [5, height, width, 64])

  @test_util.run_deprecated_v1
  def testCreateSeparableConvWithStrides(self):
    height, width = 6, 8
    # Test strides tuple
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], strides=(2, 2), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width / 2, 32])

    # Test strides integer
    layer = conv_layers.SeparableConv2D(32, [3, 3], strides=2, padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width / 2, 32])

    # Test unequal strides
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], strides=(2, 1), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height / 2, width, 32])

  @test_util.run_deprecated_v1
  def testCreateSeparableConvWithStridesChannelsFirst(self):
    data_format = 'channels_first'
    height, width = 6, 8
    # Test strides tuple
    images = random_ops.random_uniform((5, 3, height, width), seed=1)
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], strides=(2, 2), padding='same', data_format=data_format)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height / 2, width / 2])

    # Test strides integer
    layer = conv_layers.SeparableConv2D(32, [3, 3], strides=2, padding='same',
                                        data_format=data_format)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height / 2, width / 2])

    # Test unequal strides
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], strides=(2, 1), padding='same', data_format=data_format)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height / 2, width])

  @test_util.run_deprecated_v1
  def testFunctionalConv2DReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.separable_conv2d(images, 32, [3, 3], name='sepconv1')
    self.assertEqual(len(variables.trainable_variables()), 3)
    conv_layers.separable_conv2d(
        images, 32, [3, 3], name='sepconv1', reuse=True)
    self.assertEqual(len(variables.trainable_variables()), 3)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DReuseFromScope(self):
    with variable_scope.variable_scope('scope'):
      height, width = 7, 9
      images = random_ops.random_uniform((5, height, width, 3), seed=1)
      conv_layers.separable_conv2d(images, 32, [3, 3], name='sepconv1')
      self.assertEqual(len(variables.trainable_variables()), 3)
    with variable_scope.variable_scope('scope', reuse=True):
      conv_layers.separable_conv2d(images, 32, [3, 3], name='sepconv1')
      self.assertEqual(len(variables.trainable_variables()), 3)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DInitializerFromScope(self):
    with self.cached_session() as sess:
      with variable_scope.variable_scope(
          'scope', initializer=init_ops.ones_initializer()):
        height, width = 7, 9
        images = random_ops.random_uniform((5, height, width, 3), seed=1)
        conv_layers.separable_conv2d(images, 32, [3, 3], name='sepconv1')
        weights = variables.trainable_variables()
        # Check the names of weights in order.
        self.assertTrue('depthwise_kernel' in weights[0].name)
        self.assertTrue('pointwise_kernel' in weights[1].name)
        self.assertTrue('bias' in weights[2].name)
        self.evaluate(variables.global_variables_initializer())
        weights = self.evaluate(weights)
        # Check that the kernel weights got initialized to ones (from scope)
        self.assertAllClose(weights[0], np.ones((3, 3, 3, 1)))
        self.assertAllClose(weights[1], np.ones((1, 1, 3, 32)))
        # Check that the bias still got initialized to zeros.
        self.assertAllClose(weights[2], np.zeros((32)))

  @test_util.run_deprecated_v1
  def testFunctionalConv2DNoReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.separable_conv2d(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 3)
    conv_layers.separable_conv2d(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 6)

  @test_util.run_deprecated_v1
  def testSeparableConv2DDepthwiseRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv2D(32, [3, 3], depthwise_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv2DPointwiseRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv2D(32, [3, 3], pointwise_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv2DBiasRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.SeparableConv2D(32, [3, 3], bias_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testSeparableConv2DNoBias(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.SeparableConv2D(
        32, [3, 3], activation=nn_ops.relu, use_bias=False)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'separable_conv2d/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height - 2, width - 2, 32])
    self.assertListEqual(layer.depthwise_kernel.get_shape().as_list(),
                         [3, 3, 4, 1])
    self.assertListEqual(layer.pointwise_kernel.get_shape().as_list(),
                         [1, 1, 4, 32])
    self.assertEqual(layer.bias, None)

  def testConstraints(self):
    d_constraint = lambda x: x / math_ops.reduce_sum(x)
    p_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    layer = conv_layers.SeparableConv2D(2, 3,
                                        depthwise_constraint=d_constraint,
                                        pointwise_constraint=p_constraint,
                                        bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 3, 5), seed=1)
    layer(inputs)
    self.assertEqual(layer.depthwise_constraint, d_constraint)
    self.assertEqual(layer.pointwise_constraint, p_constraint)
    self.assertEqual(layer.bias_constraint, b_constraint)


class Conv2DTransposeTest(test.TestCase):

  def testInvalidDataFormat(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'data_format'):
      conv_layers.conv2d_transpose(images, 32, 3, data_format='invalid')

  def testInvalidStrides(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv2d_transpose(images, 32, 3, strides=(1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv2d_transpose(images, 32, 3, strides=None)

  def testInvalidKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv2d_transpose(images, 32, (1, 2, 3))

    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv2d_transpose(images, 32, None)

  @test_util.run_deprecated_v1
  def testCreateConv2DTranspose(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2DTranspose(32, [3, 3], activation=nn_ops.relu)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'conv2d_transpose/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height + 2, width + 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 32, 4])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testConv2DTransposeFloat16(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4), dtype='float16')
    output = conv_layers.conv2d_transpose(images, 32, [3, 3],
                                          activation=nn_ops.relu)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height + 2, width + 2, 32])

  def testCreateConv2DTransposeIntegerKernelSize(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2DTranspose(32, 3)
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height + 2, width + 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 32, 4])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testCreateConv2DTransposeChannelsFirst(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, 4, height, width))
    layer = conv_layers.Conv2DTranspose(
        32, [3, 3], data_format='channels_first')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 32, height + 2, width + 2])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 32, 4])
    self.assertListEqual(layer.bias.get_shape().as_list(), [32])

  def testConv2DTransposePaddingSame(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 32), seed=1)
    layer = conv_layers.Conv2DTranspose(
        64, images.get_shape()[1:3], padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(), [5, height, width, 64])

  def testCreateConv2DTransposeWithStrides(self):
    height, width = 6, 8
    # Test strides tuple
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    layer = conv_layers.Conv2DTranspose(
        32, [3, 3], strides=(2, 2), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height * 2, width * 2, 32])

    # Test strides integer
    layer = conv_layers.Conv2DTranspose(32, [3, 3], strides=2, padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height * 2, width * 2, 32])

    # Test unequal strides
    layer = conv_layers.Conv2DTranspose(
        32, [3, 3], strides=(2, 1), padding='same')
    output = layer.apply(images)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height * 2, width, 32])

  @test_util.run_deprecated_v1
  def testConv2DTransposeKernelRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv2DTranspose(32, [3, 3], kernel_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv2DTransposeBiasRegularizer(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv2DTranspose(32, [3, 3], bias_regularizer=reg)
    layer.apply(images)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv2DTransposeNoBias(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 4))
    layer = conv_layers.Conv2DTranspose(
        32, [3, 3], activation=nn_ops.relu, use_bias=False)
    output = layer.apply(images)
    self.assertEqual(output.op.name, 'conv2d_transpose/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, height + 2, width + 2, 32])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 32, 4])
    self.assertEqual(layer.bias, None)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DTransposeReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.conv2d_transpose(images, 32, [3, 3], name='deconv1')
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv2d_transpose(images, 32, [3, 3], name='deconv1', reuse=True)
    self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DTransposeReuseFromScope(self):
    with variable_scope.variable_scope('scope'):
      height, width = 7, 9
      images = random_ops.random_uniform((5, height, width, 3), seed=1)
      conv_layers.conv2d_transpose(images, 32, [3, 3], name='deconv1')
      self.assertEqual(len(variables.trainable_variables()), 2)
    with variable_scope.variable_scope('scope', reuse=True):
      conv_layers.conv2d_transpose(images, 32, [3, 3], name='deconv1')
      self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv2DTransposeInitializerFromScope(self):
    with self.cached_session() as sess:
      with variable_scope.variable_scope(
          'scope', initializer=init_ops.ones_initializer()):
        height, width = 7, 9
        images = random_ops.random_uniform((5, height, width, 3), seed=1)
        conv_layers.conv2d_transpose(images, 32, [3, 3], name='deconv1')
        weights = variables.trainable_variables()
        # Check the names of weights in order.
        self.assertTrue('kernel' in weights[0].name)
        self.assertTrue('bias' in weights[1].name)
        self.evaluate(variables.global_variables_initializer())
        weights = self.evaluate(weights)
        # Check that the kernel weights got initialized to ones (from scope)
        self.assertAllClose(weights[0], np.ones((3, 3, 32, 3)))
        # Check that the bias still got initialized to zeros.
        self.assertAllClose(weights[1], np.zeros((32)))

  @test_util.run_deprecated_v1
  def testFunctionalConv2DTransposeNoReuse(self):
    height, width = 7, 9
    images = random_ops.random_uniform((5, height, width, 3), seed=1)
    conv_layers.conv2d_transpose(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv2d_transpose(images, 32, [3, 3])
    self.assertEqual(len(variables.trainable_variables()), 4)

  def testConstraints(self):
    k_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    layer = conv_layers.Conv2DTranspose(2, 3,
                                        kernel_constraint=k_constraint,
                                        bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 3, 5), seed=1)
    layer(inputs)
    self.assertEqual(layer.kernel_constraint, k_constraint)
    self.assertEqual(layer.bias_constraint, b_constraint)


class Conv3DTransposeTest(test.TestCase):

  def testInvalidDataFormat(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    with self.assertRaisesRegexp(ValueError, 'data_format'):
      conv_layers.conv3d_transpose(volumes, 4, 3, data_format='invalid')

  def testInvalidStrides(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv3d_transpose(volumes, 4, 3, strides=(1, 2))

    with self.assertRaisesRegexp(ValueError, 'strides'):
      conv_layers.conv3d_transpose(volumes, 4, 3, strides=None)

  def testInvalidKernelSize(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv3d_transpose(volumes, 4, (1, 2))

    with self.assertRaisesRegexp(ValueError, 'kernel_size'):
      conv_layers.conv3d_transpose(volumes, 4, None)

  @test_util.run_deprecated_v1
  def testCreateConv3DTranspose(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32))
    layer = conv_layers.Conv3DTranspose(4, [3, 3, 3], activation=nn_ops.relu)
    output = layer.apply(volumes)
    self.assertEqual(output.op.name, 'conv3d_transpose/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth + 2, height + 2, width + 2, 4])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [4])

  def testCreateConv3DTransposeIntegerKernelSize(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32))
    layer = conv_layers.Conv3DTranspose(4, 3)
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth + 2, height + 2, width + 2, 4])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [4])

  @test_util.run_deprecated_v1
  def testCreateConv3DTransposeChannelsFirst(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, 32, depth, height, width))
    layer = conv_layers.Conv3DTranspose(
        4, [3, 3, 3], data_format='channels_first')
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, 4, depth + 2, height + 2, width + 2])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 3, 4, 32])
    self.assertListEqual(layer.bias.get_shape().as_list(), [4])

  def testConv3DTransposePaddingSame(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 64), seed=1)
    layer = conv_layers.Conv3DTranspose(
        32, volumes.get_shape()[1:4], padding='same')
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth, height, width, 32])

  def testCreateConv3DTransposeWithStrides(self):
    depth, height, width = 4, 6, 8
    # Test strides tuple.
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    layer = conv_layers.Conv3DTranspose(
        4, [3, 3, 3], strides=(2, 2, 2), padding='same')
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth * 2, height * 2, width * 2, 4])

    # Test strides integer.
    layer = conv_layers.Conv3DTranspose(4, [3, 3, 3], strides=2, padding='same')
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth * 2, height * 2, width * 2, 4])

    # Test unequal strides.
    layer = conv_layers.Conv3DTranspose(
        4, [3, 3, 3], strides=(2, 1, 1), padding='same')
    output = layer.apply(volumes)
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth * 2, height, width, 4])

  @test_util.run_deprecated_v1
  def testConv3DTransposeKernelRegularizer(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv3DTranspose(4, [3, 3, 3], kernel_regularizer=reg)
    layer.apply(volumes)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv3DTransposeBiasRegularizer(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32))
    reg = lambda x: 0.1 * math_ops.reduce_sum(x)
    layer = conv_layers.Conv3DTranspose(4, [3, 3, 3], bias_regularizer=reg)
    layer.apply(volumes)
    loss_keys = ops.get_collection(ops.GraphKeys.REGULARIZATION_LOSSES)
    self.assertEqual(len(loss_keys), 1)
    self.evaluate([v.initializer for v in layer.variables])
    self.assertListEqual(self.evaluate(layer.losses), self.evaluate(loss_keys))

  @test_util.run_deprecated_v1
  def testConv3DTransposeNoBias(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32))
    layer = conv_layers.Conv3DTranspose(
        4, [3, 3, 3], activation=nn_ops.relu, use_bias=False)
    output = layer.apply(volumes)
    self.assertEqual(output.op.name, 'conv3d_transpose/Relu')
    self.assertListEqual(output.get_shape().as_list(),
                         [5, depth + 2, height + 2, width + 2, 4])
    self.assertListEqual(layer.kernel.get_shape().as_list(), [3, 3, 3, 4, 32])
    self.assertEqual(layer.bias, None)

  @test_util.run_deprecated_v1
  def testFunctionalConv3DTransposeReuse(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3], name='deconv1')
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv3d_transpose(
        volumes, 4, [3, 3, 3], name='deconv1', reuse=True)
    self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv3DTransposeReuseFromScope(self):
    with variable_scope.variable_scope('scope'):
      depth, height, width = 5, 7, 9
      volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
      conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3], name='deconv1')
      self.assertEqual(len(variables.trainable_variables()), 2)
    with variable_scope.variable_scope('scope', reuse=True):
      conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3], name='deconv1')
      self.assertEqual(len(variables.trainable_variables()), 2)

  @test_util.run_deprecated_v1
  def testFunctionalConv3DTransposeInitializerFromScope(self):
    with self.cached_session() as sess:
      with variable_scope.variable_scope(
          'scope', initializer=init_ops.ones_initializer()):
        depth, height, width = 5, 7, 9
        volumes = random_ops.random_uniform(
            (5, depth, height, width, 32), seed=1)
        conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3], name='deconv1')
        weights = variables.trainable_variables()
        # Check the names of weights in order.
        self.assertTrue('kernel' in weights[0].name)
        self.assertTrue('bias' in weights[1].name)
        self.evaluate(variables.global_variables_initializer())
        weights = self.evaluate(weights)
        # Check that the kernel weights got initialized to ones (from scope)
        self.assertAllClose(weights[0], np.ones((3, 3, 3, 4, 32)))
        # Check that the bias still got initialized to zeros.
        self.assertAllClose(weights[1], np.zeros((4)))

  @test_util.run_deprecated_v1
  def testFunctionalConv3DTransposeNoReuse(self):
    depth, height, width = 5, 7, 9
    volumes = random_ops.random_uniform((5, depth, height, width, 32), seed=1)
    conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3])
    self.assertEqual(len(variables.trainable_variables()), 2)
    conv_layers.conv3d_transpose(volumes, 4, [3, 3, 3])
    self.assertEqual(len(variables.trainable_variables()), 4)

  def testConstraints(self):
    k_constraint = lambda x: x / math_ops.reduce_sum(x)
    b_constraint = lambda x: x / math_ops.reduce_max(x)
    layer = conv_layers.Conv3DTranspose(2, 3,
                                        kernel_constraint=k_constraint,
                                        bias_constraint=b_constraint)
    inputs = random_ops.random_uniform((5, 3, 3, 3, 5), seed=1)
    layer(inputs)
    self.assertEqual(layer.kernel_constraint, k_constraint)
    self.assertEqual(layer.bias_constraint, b_constraint)


if __name__ == '__main__':
  test.main()
