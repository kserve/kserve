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
"""Tests for conversion module."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import gast

from tensorflow.python.autograph import utils
from tensorflow.python.autograph.core import config
from tensorflow.python.autograph.core import converter
from tensorflow.python.autograph.impl import api
from tensorflow.python.autograph.pyct import compiler
from tensorflow.python.autograph.impl import conversion
from tensorflow.python.framework import constant_op
from tensorflow.python.keras.engine import training
from tensorflow.python.platform import test


class ConversionTest(test.TestCase):

  def _simple_program_ctx(self):
    return converter.ProgramContext(
        options=converter.ConversionOptions(recursive=True),
        partial_types=(),
        autograph_module=api,
        uncompiled_modules=config.DEFAULT_UNCOMPILED_MODULES)

  def test_is_whitelisted_for_graph(self):

    def test_fn():
      return constant_op.constant(1)

    self.assertFalse(conversion.is_whitelisted_for_graph(test_fn))
    self.assertTrue(conversion.is_whitelisted_for_graph(utils))
    self.assertTrue(conversion.is_whitelisted_for_graph(constant_op.constant))

  def test_entity_to_graph_unsupported_types(self):
    with self.assertRaises(NotImplementedError):
      program_ctx = self._simple_program_ctx()
      conversion.entity_to_graph('dummy', program_ctx, None, None)

  def test_entity_to_graph_callable(self):
    b = 2
    def f(a):
      return a + b

    program_ctx = self._simple_program_ctx()
    nodes, name, ns = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.FunctionDef)
    self.assertEqual('tf__f', name)
    self.assertIs(ns['b'], b)

  def test_entity_to_graph_function_with_defaults(self):
    b = 2
    c = 1
    def f(a, d=c + 1):
      return a + b + d

    program_ctx = self._simple_program_ctx()
    nodes, name, _ = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.FunctionDef)
    self.assertEqual('tf__f', name)
    self.assertEqual(
        compiler.ast_to_source(fn_node.args.defaults[0]).strip(), 'None')

  def test_entity_to_graph_call_tree(self):

    def g(a):
      return a

    def f(a):
      return g(a)

    program_ctx = self._simple_program_ctx()
    conversion.entity_to_graph(f, program_ctx, None, None)

    self.assertTrue(f in program_ctx.dependency_cache)
    self.assertTrue(g in program_ctx.dependency_cache)
    f_node = program_ctx.dependency_cache[f][0]
    g_node = program_ctx.dependency_cache[g][0]
    self.assertEqual('tf__f', f_node.name)
    self.assertEqual('tf__g', f_node.body[0].body[0].body[0].value.func.id)
    self.assertEqual('tf__g', g_node.name)

  def test_entity_to_graph_class_hierarchy(self):

    class TestBase(object):

      def __init__(self, x='base'):
        self.x = x

      def foo(self):
        return self.x

      def bar(self):
        return self.x

    class TestSubclass(TestBase):

      def __init__(self, y):
        super(TestSubclass, self).__init__('sub')
        self.y = y

      def foo(self):
        return self.y

      def baz(self):
        return self.y

    program_ctx = self._simple_program_ctx()
    conversion.entity_to_graph(TestSubclass, program_ctx, None, None)

    self.assertTrue(TestBase in program_ctx.dependency_cache)
    self.assertTrue(TestSubclass in program_ctx.dependency_cache)
    # The returned nodes will include:
    # <import nodes>, <class node>, <assignment node>
    self.assertEqual('TfTestBase',
                     program_ctx.dependency_cache[TestBase][-2].name)
    self.assertEqual('TfTestSubclass',
                     program_ctx.dependency_cache[TestSubclass][-2].name)

  def test_entity_to_graph_class_hierarchy_whitelisted(self):

    class TestSubclass(training.Model):

      def __init__(self, y):
        super(TestSubclass, self).__init__()
        self.built = False

      def call(self, x):
        return 3 * x

    program_ctx = self._simple_program_ctx()
    conversion.entity_to_graph(TestSubclass, program_ctx, None, None)

    self.assertTrue(TestSubclass in program_ctx.dependency_cache)
    self.assertFalse(training.Model in program_ctx.dependency_cache)
    self.assertEqual(
        'Model', program_ctx.dependency_cache[TestSubclass][0].names[0].name)
    # The returned nodes will include:
    # <import nodes>, <class node>, <assignment node>
    self.assertEqual('TfTestSubclass',
                     program_ctx.dependency_cache[TestSubclass][-2].name)

  def test_entity_to_graph_lambda(self):
    b = 2
    f = lambda x: b * x if x > 0 else -x

    program_ctx = self._simple_program_ctx()
    nodes, name, ns = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.Assign)
    self.assertIsInstance(fn_node.value, gast.Lambda)
    self.assertEqual('tf__lambda', name)
    self.assertIs(ns['b'], b)

  def test_entity_to_graph_multiple_lambdas(self):
    a, b = 1, 2
    f, _ = (lambda x: a * x, lambda y: b * y)

    program_ctx = self._simple_program_ctx()
    nodes, name, ns = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.Assign)
    self.assertIsInstance(fn_node.value, gast.Lambda)
    self.assertEqual('tf__lambda', name)
    self.assertIs(ns['a'], a)

  def test_entity_to_graph_multiple_lambdas_ambiguous_definitions(self):
    a, b = 1, 2
    f, _ = (lambda x: a * x, lambda x: b * x)

    program_ctx = self._simple_program_ctx()
    with self.assertRaises(ValueError):
      conversion.entity_to_graph(f, program_ctx, None, None)

  def test_entity_to_graph_lambda_code_with_garbage(self):
    # pylint:disable=g-long-lambda
    f = (  # intentional wrap
        lambda x: (x  # intentional wrap
                   + 1),)[0]
    # pylint:enable=g-long-lambda

    program_ctx = self._simple_program_ctx()
    nodes, name, _ = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.Assign)
    self.assertIsInstance(fn_node.value, gast.Lambda)
    self.assertEqual('tf__lambda', name)

  def test_entity_to_graph_nested_functions(self):
    b = 2

    def f(x):
      def g(x):
        return b * x
      return g(x)

    program_ctx = self._simple_program_ctx()
    nodes, name, ns = conversion.entity_to_graph(f, program_ctx, None, None)
    fn_node, _ = nodes
    self.assertIsInstance(fn_node, gast.FunctionDef)
    self.assertEqual(fn_node.name, 'tf__f')
    self.assertEqual('tf__f', name)
    self.assertIs(ns['b'], b)

  def test_ag_module_cached(self):
    def callee():
      return range(3)

    def caller(a):
      return a()

    program_ctx = self._simple_program_ctx()
    _, _, callee_ns = conversion.entity_to_graph(callee, program_ctx, None,
                                                 None)
    _, _, caller_ns = conversion.entity_to_graph(caller, program_ctx, None,
                                                 None)

    self.assertTrue(callee_ns['ag__'] is caller_ns['ag__'])


if __name__ == '__main__':
  test.main()
