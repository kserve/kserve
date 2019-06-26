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
"""Unit tests for the quantize_graph graph rewriting API."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import functools

from tensorflow.contrib.layers.python.layers import layers
from tensorflow.contrib.quantize.python import quantize_graph
from tensorflow.python import training
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops import template
from tensorflow.python.platform import googletest


class QuantizeGraphTest(test_util.TensorFlowTestCase):
  # We have a lot of other tests that test the details of the rewrite, here we
  # just the specific features of the quantize_graph API.

  def _RunTestOverAllRewrites(self, test_fn):
    rewrite_fns = [
        quantize_graph.create_training_graph,
        quantize_graph.create_eval_graph,
        quantize_graph.experimental_create_training_graph,
        quantize_graph.experimental_create_eval_graph,
    ]
    for fn in rewrite_fns:
      test_fn(fn)

  def _RunTestOverTrainingRewrites(self, test_fn):
    rewrite_fns = [
        quantize_graph.create_training_graph,
        quantize_graph.experimental_create_training_graph,
        functools.partial(
            quantize_graph.experimental_create_training_graph, symmetric=True),
    ]
    for fn in rewrite_fns:
      test_fn(fn)

  def _RunTestOverEvalRewrites(self, test_fn):
    rewrite_fns = [
        quantize_graph.create_eval_graph,
        quantize_graph.experimental_create_eval_graph,
        functools.partial(
            quantize_graph.experimental_create_eval_graph, symmetric=True),
    ]
    for fn in rewrite_fns:
      test_fn(fn)

  def _RunTestOverExperimentalRewrites(self, test_fn):
    rewrite_fns = [
        quantize_graph.experimental_create_training_graph,
        quantize_graph.experimental_create_eval_graph,
    ]
    for fn in rewrite_fns:
      test_fn(fn)

  def _RunTestOverExperimentalRewritesWithScope(self, test_fn, scope):
    def with_absent_scope(fn):
      def fn_with_absent_scope(*args):
        fn(*args, scope=scope)
      return fn_with_absent_scope
    rewrite_fns = [
        with_absent_scope(
            quantize_graph.experimental_create_training_graph),
        with_absent_scope(
            quantize_graph.experimental_create_eval_graph),
    ]
    for fn in rewrite_fns:
      test_fn(fn)

  def testRewrite(self):
    self._RunTestOverAllRewrites(self._TestRewrite)

  def _TestRewrite(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      self._ConvLayer()

    orig_variable_names = set(
        [v.name for v in graph.get_collection(ops.GraphKeys.GLOBAL_VARIABLES)])

    rewrite_fn(graph)

    q_variables = graph.get_collection(ops.GraphKeys.GLOBAL_VARIABLES)
    # Ensure that variables were added.
    self.assertTrue(len(orig_variable_names) < len(q_variables))

  def testDefaultGraph(self):
    self._RunTestOverAllRewrites(self._TestRewrite)

  def _TestDefaultGraph(self, rewrite_fn):
    # Tests that the default graph is correctly used when no args are provided
    # to rewrite_fn.
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      orig_variable_names = set(
          [v.name for v in g.get_collection(ops.GraphKeys.GLOBAL_VARIABLES)])
      rewrite_fn()

      q_variables = g.get_collection(ops.GraphKeys.GLOBAL_VARIABLES)
      # Ensure that variables were added.
      self.assertTrue(len(orig_variable_names) < len(q_variables))

  def testWithPostActivationBypass(self):
    self._RunTestOverAllRewrites(self._TestWithPostActivationBypass)

  def _TestWithPostActivationBypass(self, rewrite_fn):
    # Tests that the default graph is correctly used when no args are provided
    # to rewrite_fn.
    with ops.Graph().as_default() as g:
      self._ConvLayer(post_activation_bypass=True, scope='scope1')
      rewrite_fn()

      op_names = [op.name for op in g.get_operations()]
      self.assertTrue(any(
          'scope1/post_activation_bypass_quant/' in name for name in op_names))

  def testQuantDelay(self):
    self._RunTestOverTrainingRewrites(self._TestQuantDelay)

  def _TestQuantDelay(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      quant_delay = 100
      rewrite_fn(quant_delay=quant_delay)

    quant_delay_found = False
    for op in g.get_operations():
      # Check to see if the quant_delay is correctly set.
      if 'activate_quant' in op.name and op.type == 'Const':
        quant_delay_found = True
        const_value = str(op.get_attr('value'))
        self.assertTrue(('int64_val: %i' % quant_delay) in const_value)
    self.assertTrue(quant_delay_found)

  def testTrainingOpsCheck(self):
    self._RunTestOverTrainingRewrites(self._TestTrainingOpsCheck)

  def _TestTrainingOpsCheck(self, rewrite_fn):
    with ops.Graph().as_default():
      output = self._ConvLayer()
      output_scalar = math_ops.reduce_sum(output)
      loss = math_ops.square(output_scalar - 1)
      opt = training.gradient_descent.GradientDescentOptimizer(0.0001)
      opt.minimize(loss)
      with self.assertRaisesRegexp(ValueError, 'Training op found in graph'):
        rewrite_fn()

  def testWeightBits(self):
    self._RunTestOverExperimentalRewrites(self._TestWeightBits)

  def _TestWeightBits(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      weight_bits = 4
      rewrite_fn(weight_bits=weight_bits)

    weights_quant_found = False
    for op in g.get_operations():
      # Check to see if FakeQuant operations for weights have the right bits
      # set.
      if 'weights_quant' in op.name and op.type == 'FakeQuantWithMinMaxVars':
        weights_quant_found = True
        self.assertEqual(op.get_attr('num_bits'), weight_bits)
    self.assertTrue(weights_quant_found)

  def testActivationBits(self):
    self._RunTestOverExperimentalRewrites(self._TestActivationBits)

  def _TestActivationBits(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      activation_bits = 4
      rewrite_fn(activation_bits=activation_bits)

    act_quant_found = False
    for op in g.get_operations():
      # Check to see if FakeQuant operations for activations have the right bits
      # set.
      act_quant_names = ['act_quant', 'conv_quant', 'add_quant']
      if any(s in op.name
             for s in act_quant_names) and op.type == 'FakeQuantWithMinMaxVars':
        act_quant_found = True
        self.assertEqual(op.get_attr('num_bits'), activation_bits)
    self.assertTrue(act_quant_found)

  def testTrainingQuantization(self):
    self._RunTestOverTrainingRewrites(self._TestTrainingQuantization)

  def _TestTrainingQuantization(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      rewrite_fn()

    # Ensure that FakeQuant and variable update nodes were found.
    quant_found = False
    assign_min_last_found = False
    assign_min_ema_found = False
    assign_max_last_found = False
    assign_max_ema_found = False
    for op in g.get_operations():
      # Check that FakeQuant operations were added.
      if op.type == 'FakeQuantWithMinMaxVars':
        quant_found = True
      # Check that update operations for the added min max variables exist in
      # the graph.
      if 'AssignMinLast' in op.name:
        assign_min_last_found = True
      elif 'AssignMinEma' in op.name:
        assign_min_ema_found = True
      elif 'AssignMaxLast' in op.name:
        assign_max_last_found = True
      elif 'AssignMaxEma' in op.name:
        assign_max_ema_found = True
    self.assertTrue(assign_min_last_found)
    self.assertTrue(assign_min_ema_found)
    self.assertTrue(assign_max_last_found)
    self.assertTrue(assign_max_ema_found)
    self.assertTrue(quant_found)

  def testEvalQuantization(self):
    self._RunTestOverEvalRewrites(self._TestEvalQuantization)

  def _TestEvalQuantization(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      rewrite_fn()

    # Ensure that FakeQuant and variable update nodes were found.
    quant_found = False
    for op in g.get_operations():
      # Check that FakeQuant operations were added.
      if op.type == 'FakeQuantWithMinMaxVars':
        quant_found = True
      # Check that update operations for the added min max variables don't
      # exist in the graph.
      update_names = [
          'AssignMinLast', 'AssignMinEma', 'AssignMaxLast', 'AssignMaxEma'
      ]
      self.assertFalse(any(s in op.name for s in update_names))
    self.assertTrue(quant_found)

  def testIdempotent(self):
    self._RunTestOverAllRewrites(self._TestIdempotent)

  def _TestIdempotent(self, rewrite_fn):
    with ops.Graph().as_default() as g:
      self._ConvLayer()
      rewrite_fn()
      graph_def_before = str(g.as_graph_def())
      # Ensuring that calling the rewrite again doesn't add more nodes.
      rewrite_fn()
      graph_def_after = str(g.as_graph_def())
      self.assertEqual(graph_def_before, graph_def_after)

  def testIdentityNode(self):
    self._RunTestOverAllRewrites(self._TestIdentityNode)

  def _TestIdentityNode(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      self._LayerWithIdentity()

    rewrite_fn(graph)
    op_names = [op.name for op in graph.get_operations()]
    self.assertTrue(any('test/Conv/weights_quant' in name for name in op_names))
    self.assertTrue(any('test/Conv/act_quant' in name for name in op_names))
    bn_out_identity = graph.get_operation_by_name('test/bn_out')
    self._AssertInputOpsAre(bn_out_identity, [
        'test/Conv/add_fold',
    ])

    conv_out_identity = graph.get_operation_by_name('test/conv_out')
    self._AssertOutputGoesToOps(conv_out_identity, graph,
                                ['test/BatchNorm/FusedBatchNorm'])

  def testActivationQuantization(self):
    self._RunTestOverAllRewrites(self._TestActivationQuantization)

  def _TestActivationQuantization(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      _ = self._LayerWithActivationProcessing()

    rewrite_fn(graph)
    # Check if outputs of multipliers and adds are quantized.

    mul_op = graph.get_operation_by_name('test/Mul')
    self._AssertOutputGoesToOps(
        mul_op, graph,
        ['test/Mul/activation_Mul_quant/FakeQuantWithMinMaxVars'])
    mul_op = graph.get_operation_by_name('test/Mul_1')
    self._AssertOutputGoesToOps(
        mul_op, graph,
        ['test/Mul_1/activation_Mul_quant/FakeQuantWithMinMaxVars'])
    add_op = graph.get_operation_by_name('test/add')
    self._AssertOutputGoesToOps(
        add_op, graph,
        ['test/add/activation_Add_quant/FakeQuantWithMinMaxVars'])

  def testRewriteWithScope(self):
    self._RunTestOverExperimentalRewritesWithScope(
        self._TestRewriteWithScope, 'scope1')

  def _TestRewriteWithScope(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      scope1_output = self._ConvLayer(scope='scope1')
      self._ConvLayer(input_tensor=scope1_output, scope='scope2')

    rewrite_fn(graph)

    op_names = [op.name for op in graph.get_operations()]
    # The weights and activation of scope1 is quantized, but not scope2.
    self.assertTrue(
        any('scope1/Conv/act_quant' in name for name in op_names))
    self.assertTrue(
        any('scope1/Conv/weights_quant' in name for name in op_names))
    self.assertFalse(
        any('scope2/Conv/act_quant' in name for name in op_names))
    self.assertFalse(
        any('scope2/Conv/weights_quant' in name for name in op_names))

  def testRewriteWithNonMatchingScope(self):
    self._RunTestOverExperimentalRewritesWithScope(
        self._TestRewriteWithNonMatchingScope, 'NonExistingScope')

  def _TestRewriteWithNonMatchingScope(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      self._ConvLayer()

    op_names_before_rewrite = set([op.name for op in graph.get_operations()])
    rewrite_fn(graph)
    op_names_after_rewrite = set([op.name for op in graph.get_operations()])

    # No ops should be inserted or removed.
    self.assertEqual(op_names_before_rewrite, op_names_after_rewrite)

  def testActivationRewriteWithScope(self):
    self._RunTestOverExperimentalRewritesWithScope(
        self._TestActivationRewriteWithScope, 'scope1')

  def _TestActivationRewriteWithScope(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      output = self._LayerWithIdentity(scope='scope1')
      with ops.name_scope('scope2'):
        output = nn_ops.relu6(output)
        scaled_output1 = math_ops.mul(2.0, output)
        scaled_output2 = math_ops.mul(3.0, output)
        output = scaled_output1 + scaled_output2
      rewrite_fn(graph)

      op_names = [op.name for op in graph.get_operations()]
      # The weights and activation of scope1 is quantized, but not scope2.
      self.assertTrue(any('scope1/Conv/act_quant' in name for name in op_names))
      self.assertTrue(
          any('scope1/Conv/weights_quant' in name for name in op_names))

      for op_name in op_names:
        if op_name.startswith('scope2'):
          self.assertTrue('FakeQuant' not in op_name)

  def testActivationRewriteWithNonMatchingScope(self):
    self._RunTestOverExperimentalRewritesWithScope(
        self._TestActivationRewriteWithNonMatchingScope, 'NonExistingScope')

  def _TestActivationRewriteWithNonMatchingScope(self, rewrite_fn):
    graph = ops.Graph()
    with graph.as_default():
      self._LayerWithActivationProcessing()

    rewrite_fn(graph)
    op_types_after_rewrite = set([op.type for op in graph.get_operations()])
    self.assertFalse(
        op_types_after_rewrite.intersection('FakeQuantWithMinMaxVars'))
    # No fake quant ops should be inserted.

  def testWithSharedWeights(self):

    self._RunTestOverAllRewrites(self._TestWithSharedWeights)
    self._RunTestOverTrainingRewrites(self._TestRewriteWithSharedWeights)

  def _TestRewriteWithSharedWeights(self, rewrite_fn, quant_delay=1):
    self._TestWithSharedWeights(rewrite_fn, quant_delay)

  def _TestWithSharedWeights(self, rewrite_fn, quant_delay=None):
    with ops.Graph().as_default() as g:
      conv = template.make_template('shared_weights_conv', self._ConvLayer)
      conv()
      conv()
      if quant_delay is None:
        rewrite_fn()
      else:
        rewrite_fn(quant_delay=quant_delay)

    conv_ops = [op for op in g.get_operations() if op.type == 'Conv2D']
    weights_quants = [
        op for op in g.get_operations()
        if 'weights_quant' in op.name and op.type == 'FakeQuantWithMinMaxVars'
    ]
    # Check that the shared weights variable is not quantized multiple times
    self.assertTrue(len(weights_quants) == 1)
    weights_quant_tensor = weights_quants[0].outputs[0]
    if quant_delay:
      delayed_weights_quants = [
          op for op in g.get_operations()
          if 'weights_quant' in op.name and op.type == 'Merge'
      ]
      self.assertTrue(len(delayed_weights_quants) == 1)
      weights_quant_tensor = delayed_weights_quants[0].outputs[0]
    # Check that the Conv2D operations get the quantized weights
    self.assertTrue(all(weights_quant_tensor in op.inputs for op in conv_ops))

  def _ConvLayer(
      self, input_tensor=None, scope='test', pre_activation_bypass=False,
      post_activation_bypass=False):
    """Add a basic convolution layer to the default graph."""
    batch_size, height, width, depth = 5, 128, 128, 3
    if input_tensor is None:
      input_tensor = array_ops.zeros((batch_size, height, width, depth))
    weight_init = init_ops.truncated_normal_initializer
    with ops.name_scope(scope):
      output = layers.conv2d(
          input_tensor,
          depth, [5, 5],
          padding='SAME',
          weights_initializer=weight_init(0.09),
          activation_fn=None)
      if pre_activation_bypass:
        output += input_tensor
      output = nn_ops.relu6(output)
      if post_activation_bypass:
        output += input_tensor
    return output

  def _LayerWithIdentity(self,
                         input_tensor=None,
                         scope='test',
                         post_activation_bypass=False):
    """Add a basic conv, identity, batch norm with skip to the default graph."""
    batch_size, height, width, depth = 5, 128, 128, 3
    if input_tensor is None:
      input_tensor = array_ops.zeros((batch_size, height, width, depth))
    weight_init = init_ops.truncated_normal_initializer
    with ops.name_scope(scope):
      output = layers.conv2d(
          input_tensor,
          depth, [5, 5],
          padding='SAME',
          weights_initializer=weight_init(0.09),
          activation_fn=None,
          normalizer_fn=None,
          biases_initializer=None)
      output = array_ops.identity(output, name='conv_out')

      output = layers.batch_norm(
          output, center=True, scale=True, decay=1.0 - 0.003, fused=True)

      output = array_ops.identity(output, name='bn_out')
      if post_activation_bypass:
        output += input_tensor
    return output

  def _LayerWithActivationProcessing(self,
                                     input_tensor=None,
                                     scope='test',
                                     post_activation_bypass=False):

    batch_size, height, width, depth = 5, 128, 128, 3
    if input_tensor is None:
      input_tensor = array_ops.zeros((batch_size, height, width, depth))
    weight_init = init_ops.truncated_normal_initializer
    with ops.name_scope(scope):
      output = layers.conv2d(
          input_tensor,
          depth, [5, 5],
          padding='SAME',
          weights_initializer=weight_init(0.09),
          activation_fn=None,
          normalizer_fn=None,
          biases_initializer=None)

      output = layers.batch_norm(
          output, center=True, scale=True, decay=1.0 - 0.003, fused=True)

      output = nn_ops.relu6(output)
      scaled_output1 = math_ops.mul(2.0, output)
      scaled_output2 = math_ops.mul(3.0, output)
      output = scaled_output1 + scaled_output2
    return output

  def _AssertInputOpsAre(self, op, in_op_names):
    """Asserts that all inputs to op come from in_op_names (disregarding order).

    Args:
      op: Operation to check inputs for.
      in_op_names: List of strings, operations where all op's inputs should come
        from.
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
