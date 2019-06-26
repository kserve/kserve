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
"""Parameterized unit tests for quantizing a Tensorflow graph."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.layers.python.layers import layers
from tensorflow.contrib.quantize.python import fold_batch_norms
from tensorflow.contrib.quantize.python import quantize
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.platform import googletest

batch_norm = layers.batch_norm
conv2d = layers.conv2d
fully_connected = layers.fully_connected
separable_conv2d = layers.separable_conv2d


class QuantizeTest(test_util.TensorFlowTestCase):

  def _RunWithoutBatchNormTestOverParameters(self, test_fn):
    # TODO(suharshs): Use parameterized test once OSS TF supports it.
    parameters_list = [
        # (activation, activation_op_name, with_bypass, delay)
        (nn_ops.relu6, 'Relu6', False, None),
        (nn_ops.relu, 'Relu', False, None),
        (array_ops.identity, 'Identity', False, None),
        (nn_ops.relu6, 'Relu6', False, 5000),
        (nn_ops.relu, 'Relu', False, 5000),
        (array_ops.identity, 'Identity', False, 5000),
        (nn_ops.relu6, 'Relu6', True, None),
        (nn_ops.relu, 'Relu', True, None),
        (array_ops.identity, 'Identity', True, None),
        (nn_ops.relu6, 'Relu6', True, 5000),
        (nn_ops.relu, 'Relu', True, 5000),
        (array_ops.identity, 'Identity', True, 5000),
    ]
    for params in parameters_list:
      # Test everything with resource variables and normal variables.
      test_fn(params[0], params[1], params[2], params[3], False, None)
      test_fn(params[0], params[1], params[2], params[3], True, None)
      # Test with both empty scope and an example scope
      test_fn(params[0], params[1], params[2], params[3], False, 'test')
      test_fn(params[0], params[1], params[2], params[3], True, 'test')

  def _AssertCorrectQuantizedGraphWithoutBatchNorm(
      self, graph, scope, layer, activation_op_name, with_bypass, delay,
      use_resource):
    quantization_node_name = 'FakeQuantWithMinMaxVars'
    conv_scope = self._GetConvScope(scope, with_bypass)
    delim = '/' if conv_scope else ''

    if scope:
      scope = scope + '/'
    weights_quant = graph.get_operation_by_name(
        conv_scope + delim + 'weights_quant/' + quantization_node_name)
    self.assertEqual(weights_quant.type, quantization_node_name)

    # Assemble the expected inputs.
    if use_resource:
      expected_inputs = [
          conv_scope + delim +
          'weights_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
          conv_scope + delim +
          'weights_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
      ]
      if layer == 'DepthwiseConv2dNative':
        expected_inputs.append(conv_scope + delim + 'depthwise/ReadVariableOp')
      else:
        expected_inputs.append(conv_scope + delim + layer + '/ReadVariableOp')
    else:
      expected_inputs = [
          conv_scope + delim + 'weights_quant/AssignMinLast',
          conv_scope + delim + 'weights_quant/AssignMaxLast',
      ]
      if layer == 'DepthwiseConv2dNative':
        expected_inputs.append(conv_scope + delim + 'depthwise_weights/read')
      else:
        expected_inputs.append(conv_scope + delim + 'weights/read')

    self._AssertInputOpsAre(weights_quant, expected_inputs)
    if delay and delay > 0:
      output_op_name = (
          conv_scope + delim + 'weights_quant/delayed_quant/Switch_1')
    else:
      if layer == 'DepthwiseConv2dNative':
        output_op_name = conv_scope + delim + 'depthwise'
      else:
        output_op_name = conv_scope + delim + layer

    self._AssertOutputGoesToOps(weights_quant, graph, [output_op_name])

    if with_bypass:
      conv_quant = graph.get_operation_by_name(
          conv_scope + delim + 'conv_quant/' + quantization_node_name)
      self.assertEqual(conv_quant.type, quantization_node_name)
      if use_resource:
        expected_inputs = [
            conv_scope + delim +
            'conv_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
            conv_scope + delim +
            'conv_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
            conv_scope + delim + 'BiasAdd',
        ]
      else:
        expected_inputs = [
            conv_scope + delim + 'conv_quant/AssignMinEma',
            conv_scope + delim + 'conv_quant/AssignMaxEma',
            conv_scope + delim + 'BiasAdd'
        ]
      self._AssertInputOpsAre(conv_quant, expected_inputs)

      output_op_name = (
          conv_scope + delim + 'conv_quant/delayed_quant/Switch_1'
          if delay else scope + 'Add')
      self._AssertOutputGoesToOps(conv_quant, graph, [output_op_name])

    act_quant = graph.get_operation_by_name(scope + 'act_quant/' +
                                            quantization_node_name)
    self.assertEqual(act_quant.type, quantization_node_name)
    if use_resource:
      expected_inputs = [
          scope + 'act_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
          scope + 'act_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
          scope + activation_op_name,
      ]
    else:
      expected_inputs = [
          scope + 'act_quant/AssignMinEma', scope + 'act_quant/AssignMaxEma',
          scope + activation_op_name
      ]
    self._AssertInputOpsAre(act_quant, expected_inputs)
    output_op_name = (
        scope + 'act_quant/delayed_quant/Switch_1'
        if delay else 'control_dependency')
    self._AssertOutputGoesToOps(act_quant, graph, [output_op_name])
    self._AssertIdempotent(graph)

  def testQuantize_Conv2dWithoutBatchNorm(self):
    self._RunWithoutBatchNormTestOverParameters(
        self._TestQuantize_Conv2dWithoutBatchNorm)

  def _TestQuantize_Conv2dWithoutBatchNorm(self, activation, activation_op_name,
                                           with_bypass, delay, use_resource,
                                           scope):
    """Tests quantization: inputs -> Conv2d no batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      stride = 1 if with_bypass else 2
      out_depth = 3 if with_bypass else 32
      activation_fn = None if with_bypass else activation
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''
      node = conv2d(
          inputs,
          out_depth, [5, 5],
          stride=stride,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=activation_fn,
          scope=conv_scope)
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')
        node = activation(node, name=scope + delim + activation_op_name)
      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')

      quantize.Quantize(graph, True, quant_delay=delay)

    if conv_scope is None:
      conv_scope = ''

    self._AssertCorrectQuantizedGraphWithoutBatchNorm(
        graph, scope, 'Conv2D', activation_op_name, with_bypass, delay,
        use_resource)

  def testQuantize_FCWithoutBatchNorm(self):
    self._RunWithoutBatchNormTestOverParameters(
        self._TestQuantize_FCWithoutBatchNorm)

  def _TestQuantize_FCWithoutBatchNorm(self, activation, activation_op_name,
                                       with_bypass, delay, use_resource, scope):
    """Tests quantization: inputs -> FC no batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, depth = 5, 256
      inputs = array_ops.zeros((batch_size, depth))
      out_depth = 256 if with_bypass else 128
      activation_fn = None if with_bypass else activation
      fc_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''
      node = fully_connected(
          inputs,
          out_depth,
          weights_initializer=self._WeightInit(0.03),
          activation_fn=activation_fn,
          scope=fc_scope)
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')
        node = activation(node, name=scope + delim + activation_op_name)
      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')
      quantize.Quantize(graph, True, quant_delay=delay)

    self._AssertCorrectQuantizedGraphWithoutBatchNorm(
        graph, scope, 'MatMul', activation_op_name, with_bypass, delay,
        use_resource)

  def testQuantize_DepthwiseConv2dWithoutBatchNorm(self):
    self._RunWithoutBatchNormTestOverParameters(
        self._TestQuantize_DepthwiseConv2dWithoutBatchNorm)

  def _TestQuantize_DepthwiseConv2dWithoutBatchNorm(
      self, activation, activation_op_name, with_bypass, delay, use_resource,
      scope):
    """Tests quantization: inputs -> DWConv2d no batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      stride = 1 if with_bypass else 2
      activation_fn = None if with_bypass else activation
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''

      node = separable_conv2d(
          inputs,
          None, [5, 5],
          stride=stride,
          depth_multiplier=1.0,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=activation_fn,
          scope=conv_scope)
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')
        node = activation(node, name=scope + delim + activation_op_name)
      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')
      quantize.Quantize(graph, True, quant_delay=delay)

    self._AssertCorrectQuantizedGraphWithoutBatchNorm(
        graph, scope, 'DepthwiseConv2dNative', activation_op_name, with_bypass,
        delay, use_resource)

  def testQuantize_AtrousConvWithoutBatchNorm(self):
    self._RunWithoutBatchNormTestOverParameters(
        self._TestQuantize_AtrousConvWithoutBatchNorm)

  def _TestQuantize_AtrousConvWithoutBatchNorm(self, activation,
                                               activation_op_name, with_bypass,
                                               delay, use_resource, scope):
    """Tests quantization: inputs -> atrous conv no batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      dilation_rate = 2
      activation_fn = None if with_bypass else activation
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''

      node = separable_conv2d(
          inputs,
          None, [3, 3],
          rate=dilation_rate,
          depth_multiplier=1.0,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=activation_fn,
          scope=conv_scope)
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')
        node = activation(node, name=scope + delim + activation_op_name)
      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')
      quantize.Quantize(graph, True, quant_delay=delay)

    self._AssertCorrectQuantizedGraphWithoutBatchNorm(
        graph, scope, 'DepthwiseConv2dNative', activation_op_name, with_bypass,
        delay, use_resource)

  def _RunBatchNormTestOverParameters(self, test_fn):
    # TODO(suharshs): Use parameterized test once OSS TF supports it.
    parameters_list = [
        # (activation, activation_op_name, with_bypass, delay, fused_batch_norm)
        (nn_ops.relu6, 'Relu6', False, None, False),
        (nn_ops.relu, 'Relu', False, None, False),
        (array_ops.identity, 'Identity', False, None, False),
        (nn_ops.relu6, 'Relu6', False, 5000, False),
        (nn_ops.relu, 'Relu', False, 5000, False),
        (array_ops.identity, 'Identity', False, 5000, False),
        (nn_ops.relu6, 'Relu6', True, None, False),
        (nn_ops.relu, 'Relu', True, None, False),
        (array_ops.identity, 'Identity', True, None, False),
        (nn_ops.relu6, 'Relu6', True, 5000, False),
        (nn_ops.relu, 'Relu', True, 5000, False),
        (array_ops.identity, 'Identity', True, 5000, False),
        (nn_ops.relu6, 'Relu6', False, None, True),
        (nn_ops.relu, 'Relu', False, None, True),
        (array_ops.identity, 'Identity', False, None, True),
        (nn_ops.relu6, 'Relu6', False, 5000, True),
        (nn_ops.relu, 'Relu', False, 5000, True),
        (array_ops.identity, 'Identity', False, 5000, True),
        (nn_ops.relu6, 'Relu6', True, None, True),
        (nn_ops.relu, 'Relu', True, None, True),
        (array_ops.identity, 'Identity', True, None, True),
        (nn_ops.relu6, 'Relu6', True, 5000, True),
        (nn_ops.relu, 'Relu', True, 5000, True),
        (array_ops.identity, 'Identity', True, 5000, True)
    ]
    for params in parameters_list:
      # Test everything with resource variables and normal variables.
      test_fn(params[0], params[1], params[2], params[3], params[4], False,
              None)
      test_fn(params[0], params[1], params[2], params[3], params[4], True, None)
      test_fn(params[0], params[1], params[2], params[3], params[4], False,
              'test')
      test_fn(params[0], params[1], params[2], params[3], params[4], True,
              'test')

  def _AssertCorrectQuantizedGraphWithBatchNorm(self, graph, scope, layer,
                                                activation_op_name, with_bypass,
                                                delay, use_resource):
    quantization_node_name = 'FakeQuantWithMinMaxVars'
    conv_scope = self._GetConvScope(scope, with_bypass)
    delim = '/' if conv_scope else ''

    if scope:
      scope = scope + '/'

    weights_quant = graph.get_operation_by_name(
        conv_scope + delim + 'weights_quant/' + quantization_node_name)

    self.assertEqual(weights_quant.type, quantization_node_name)
    if use_resource:
      expected_inputs = [
          conv_scope + delim +
          'weights_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
          conv_scope + delim +
          'weights_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
      ]
    else:
      expected_inputs = [
          conv_scope + delim + 'weights_quant/' + 'AssignMinLast',
          conv_scope + delim + 'weights_quant/' + 'AssignMaxLast'
      ]
    expected_inputs.append(conv_scope + delim + 'mul_fold')

    self._AssertInputOpsAre(weights_quant, expected_inputs)
    if layer == 'DepthwiseConv2dNative':
      output_op_name = conv_scope + delim + (
          'weights_quant/delayed_quant/Switch_1' if delay else 'depthwise_Fold')
    else:
      output_op_name = conv_scope + delim + (
          'weights_quant/delayed_quant/Switch_1' if delay else layer + '_Fold')
    self._AssertOutputGoesToOps(weights_quant, graph, [output_op_name])

    if with_bypass:
      conv_quant = graph.get_operation_by_name(
          conv_scope + delim + 'conv_quant/' + quantization_node_name)
      self.assertEqual(conv_quant.type, quantization_node_name)

      if use_resource:
        expected_inputs = [
            conv_scope + delim +
            'conv_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
            conv_scope + delim +
            'conv_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
        ]
      else:
        expected_inputs = [
            conv_scope + delim + 'conv_quant/AssignMinEma',
            conv_scope + delim + 'conv_quant/AssignMaxEma',
        ]
      expected_inputs.append(conv_scope + delim + 'add_fold')

      self._AssertInputOpsAre(conv_quant, expected_inputs)
      output_op_name = (
          conv_scope + delim + 'conv_quant/delayed_quant/Switch_1'
          if delay else scope + 'Add')
      self._AssertOutputGoesToOps(conv_quant, graph, [output_op_name])

    act_quant = graph.get_operation_by_name(scope + 'act_quant/' +
                                            quantization_node_name)
    self.assertEqual(act_quant.type, quantization_node_name)

    if use_resource:
      expected_inputs = [
          scope + 'act_quant/FakeQuantWithMinMaxVars/ReadVariableOp',
          scope + 'act_quant/FakeQuantWithMinMaxVars/ReadVariableOp_1',
      ]
    else:
      expected_inputs = [
          scope + 'act_quant/AssignMinEma',
          scope + 'act_quant/AssignMaxEma',
      ]
    expected_inputs.append(scope + activation_op_name)

    self._AssertInputOpsAre(act_quant, expected_inputs)
    output_op_name = (
        scope + 'act_quant/delayed_quant/Switch_1'
        if delay else 'control_dependency')
    self._AssertOutputGoesToOps(act_quant, graph, [output_op_name])
    self._AssertIdempotent(graph)

  def testQuantize_Conv2dWithBatchNorm(self):
    self._RunBatchNormTestOverParameters(self._TestQuantize_Conv2dWithBatchNorm)

  def _TestQuantize_Conv2dWithBatchNorm(self, activation, activation_op_name,
                                        with_bypass, delay, fused_batch_norm,
                                        use_resource, scope):
    """Tests quantization: inputs -> Conv2d with batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      fused_batch_norm: Bool, when true use FusedBatchNorm.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      stride = 1 if with_bypass else 2
      out_depth = 3 if with_bypass else 32
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''
      node = conv2d(
          inputs,
          out_depth, [5, 5],
          stride=stride,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=None,
          normalizer_fn=batch_norm,
          normalizer_params=self._BatchNormParams(fused_batch_norm),
          scope=conv_scope)

      # Manually add a bypass (optional) and an activation.
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')

      node = activation(node, name=scope + delim + activation_op_name)

      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')

      fold_batch_norms.FoldBatchNorms(graph, is_training=True)
      quantize.Quantize(graph, True, quant_delay=delay)

      self._AssertCorrectQuantizedGraphWithBatchNorm(
          graph, scope, 'Conv2D', activation_op_name, with_bypass, delay,
          use_resource)

  def testQuantize_FCWithBatchNorm(self):
    self._RunBatchNormTestOverParameters(self._TestQuantize_FCWithBatchNorm)

  def _TestQuantize_FCWithBatchNorm(self, activation, activation_op_name,
                                    with_bypass, delay, fused_batch_norm,
                                    use_resource, scope):
    """Tests quantization: inputs -> FC with batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      fused_batch_norm: Bool, when true use FusedBatchNorm.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, depth = 5, 256
      inputs = array_ops.zeros((batch_size, depth))
      out_depth = 256 if with_bypass else 128
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''
      node = fully_connected(
          inputs,
          out_depth,
          weights_initializer=self._WeightInit(0.03),
          activation_fn=None,
          normalizer_fn=batch_norm,
          normalizer_params=self._BatchNormParams(fused_batch_norm),
          scope=conv_scope)

      # Manually add a bypass (optional) and an activation.
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')

      node = activation(node, name=scope + delim + activation_op_name)

      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')

      fold_batch_norms.FoldBatchNorms(graph, is_training=True)

      quantize.Quantize(graph, True, quant_delay=delay)

    self._AssertCorrectQuantizedGraphWithBatchNorm(
        graph, scope, 'MatMul', activation_op_name, with_bypass, delay,
        use_resource)

  def testQuantize_DepthwiseConv2dWithBatchNorm(self):
    self._RunBatchNormTestOverParameters(
        self._TestQuantize_DepthwiseConv2dWithBatchNorm)

  def _TestQuantize_DepthwiseConv2dWithBatchNorm(
      self, activation, activation_op_name, with_bypass, delay,
      fused_batch_norm, use_resource, scope):
    """Tests quantization: inputs -> DWConv2d with batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      fused_batch_norm: Bool, when true use FusedBatchNorm.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      stride = 1 if with_bypass else 2
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''
      node = separable_conv2d(
          inputs,
          None, [5, 5],
          stride=stride,
          depth_multiplier=1.0,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=None,
          normalizer_fn=batch_norm,
          normalizer_params=self._BatchNormParams(fused_batch_norm),
          scope=conv_scope)

      # Manually add a bypass (optional) and an activation.
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')

      node = activation(node, name=scope + delim + activation_op_name)

      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')

      fold_batch_norms.FoldBatchNorms(graph, is_training=True)
      quantize.Quantize(graph, True, quant_delay=delay)

      self._AssertCorrectQuantizedGraphWithBatchNorm(
          graph, scope, 'DepthwiseConv2dNative', activation_op_name,
          with_bypass, delay, use_resource)

  def testQuantize_AtrousConvWithBatchNorm(self):
    self._RunBatchNormTestOverParameters(
        self._TestQuantize_AtrousConvWithBatchNorm)

  def _TestQuantize_AtrousConvWithBatchNorm(
      self, activation, activation_op_name, with_bypass, delay,
      fused_batch_norm, use_resource, scope):
    """Tests quantization: inputs -> atrous conv with batch norm -> Activation.

    Args:
      activation: Callable that returns an Operation, a factory method for the
        Activation.
      activation_op_name: String, name of the Activation operation.
      with_bypass: Bool, when true there is an extra connection added from
        inputs to just before Activation.
      delay: Int (optional), delay in number of steps until quantization starts.
      fused_batch_norm: Bool, when true use FusedBatchNorm.
      use_resource: Bool, when true uses resource variables.
      scope: String, specifies top level scope for the graph
    """
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      inputs = array_ops.zeros((batch_size, height, width, depth))
      dilation_rate = 2
      conv_scope = self._GetConvScope(scope, with_bypass)
      scope = '' if scope is None else scope
      delim = '/' if scope else ''

      node = separable_conv2d(
          inputs,
          None, [3, 3],
          rate=dilation_rate,
          depth_multiplier=1.0,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=None,
          normalizer_fn=batch_norm,
          normalizer_params=self._BatchNormParams(fused_batch_norm),
          scope=conv_scope)

      # Manually add a bypass (optional) and an activation.
      if with_bypass:
        node = math_ops.add(inputs, node, name=scope + delim + 'Add')

      node = activation(node, name=scope + delim + activation_op_name)

      update_barrier = control_flow_ops.no_op(name='update_barrier')
      with ops.control_dependencies([update_barrier]):
        array_ops.identity(node, name='control_dependency')

      fold_batch_norms.FoldBatchNorms(graph, is_training=True)
      quantize.Quantize(graph, True, quant_delay=delay)

      self._AssertCorrectQuantizedGraphWithBatchNorm(
          graph, scope, 'DepthwiseConv2dNative', activation_op_name,
          with_bypass, delay, use_resource)

  def _AssertIdempotent(self, graph):
    # Ensure that calling the rewrite again doesn't change the graph.
    graph_def_before = str(graph.as_graph_def())
    with graph.as_default():
      # Ensuring that calling the rewrite again doesn't add more nodes.
      fold_batch_norms.FoldBatchNorms(graph, is_training=True)
      quantize.Quantize(graph, True)
    graph_def_after = str(graph.as_graph_def())
    self.assertEqual(graph_def_before, graph_def_after)

  def testBatchNormForcedUpdates(self):
    parameter_list = [
        # (activation, activation_op_name, fused_batch_norm)
        (nn_ops.relu6, 'Relu6', False),
        (nn_ops.relu, 'Relu', False),
        (array_ops.identity, 'Identity', False),
        (nn_ops.relu6, 'Relu6', True),
        (nn_ops.relu, 'Relu', True),
        (array_ops.identity, 'Identity', True),
    ]
    for params in parameter_list:
      self._TestBatchNormForcedUpdates(params[0], params[1], params[2], False)
      self._TestBatchNormForcedUpdates(params[0], params[1], params[2], True)

  def _TestBatchNormForcedUpdates(self, activation, activation_op_name,
                                  fused_batch_norm, use_resource):
    """post_activation bypass quantization should happen with forced updates."""
    graph = ops.Graph()
    with graph.as_default():
      variable_scope.get_variable_scope().set_use_resource(use_resource)
      batch_size, height, width, depth = 5, 128, 128, 3
      input1 = array_ops.zeros((batch_size, height, width, depth))
      input2 = array_ops.zeros((batch_size, height / 2, width / 2, 32))
      # Setting updates_collections to None forces updates adding an extra
      # identity operation following batch norms.
      bn_params = self._BatchNormParams(
          fused=fused_batch_norm, force_updates=True)
      conv = conv2d(
          input1,
          32, [5, 5],
          stride=2,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          activation_fn=activation,
          normalizer_fn=batch_norm,
          normalizer_params=bn_params,
          scope='test/test')
      bypass_tensor = math_ops.add(conv, input2, name='test/add')
      # The output of the post_activation bypass will be another layer.
      _ = conv2d(
          bypass_tensor,
          32, [5, 5],
          stride=2,
          padding='SAME',
          weights_initializer=self._WeightInit(0.09),
          normalizer_fn=batch_norm,
          normalizer_params=bn_params,
          activation_fn=activation,
          scope='test/unused')

      fold_batch_norms.FoldBatchNorms(graph, is_training=True)
      quantize.Quantize(graph, is_training=True)

      # Ensure that the bypass node is preceded by and followed by a
      # FakeQuantWithMinMaxVar operation, since the output of the Add isn't an
      # activation.
      self.assertTrue('FakeQuantWithMinMaxVars' in
                      [c.type for c in bypass_tensor.consumers()])
      self.assertTrue('FakeQuantWithMinMaxVars' in
                      [i.op.type for i in bypass_tensor.op.inputs])

    with open('/tmp/bn_quant_test.pbtxt', 'w') as f:
      f.write(str(graph.as_graph_def()))

  def _GetConvScope(self, scope, with_bypass):
    if scope is None:
      scope = ''
    delim = '/' if scope else ''

    if with_bypass:
      conv_scope = scope + delim + 'test2'
    else:
      conv_scope = scope

    return conv_scope

  def _BatchNormParams(self, fused=False, force_updates=False):
    params = {
        'center': True,
        'scale': True,
        'decay': 1.0 - 0.003,
        'fused': fused
    }
    if force_updates:
      params['updates_collections'] = None
    return params

  def _WeightInit(self, stddev):
    """Returns truncated normal variable initializer.

    Function is defined purely to shorten the name so that it stops wrapping.

    Args:
      stddev: Standard deviation of normal variable.

    Returns:
      An initialized that initializes with a truncated normal variable.
    """
    return init_ops.truncated_normal_initializer(stddev=stddev)

  def _AssertInputOpsAre(self, op, in_op_names):
    """Asserts that all inputs to op come from in_op_names (disregarding order).

    Args:
      op: Operation to check inputs for.
      in_op_names: List of strings, operations where all op's inputs should
        come from.
    """
    expected_inputs = [in_op_name + ':0' for in_op_name in in_op_names]
    self.assertItemsEqual([t.name for t in op.inputs], expected_inputs)

  def _AssertOutputGoesToOps(self, op, graph, out_op_names):
    """Asserts that outputs from op go to out_op_names (and perhaps others).

    Args:
      op: Operation to check outputs for.
      graph: Graph where output operations are located.
      out_op_names: List of strings, operations where op's outputs should go.
    """
    for out_op_name in out_op_names:
      out_op = graph.get_operation_by_name(out_op_name)
      self.assertIn(op.outputs[0].name, [str(t.name) for t in out_op.inputs])


if __name__ == '__main__':
  googletest.main()
