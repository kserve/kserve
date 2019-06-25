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
"""Tests for live_values module."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import six

from tensorflow.python.autograph.pyct import anno
from tensorflow.python.autograph.pyct import cfg
from tensorflow.python.autograph.pyct import parser
from tensorflow.python.autograph.pyct import qual_names
from tensorflow.python.autograph.pyct import transformer
from tensorflow.python.autograph.pyct.static_analysis import activity
from tensorflow.python.autograph.pyct.static_analysis import live_values
from tensorflow.python.autograph.pyct.static_analysis import reaching_definitions
from tensorflow.python.autograph.pyct.static_analysis import type_info
from tensorflow.python.framework import constant_op
from tensorflow.python.platform import test


class LiveValuesResolverTest(test.TestCase):

  def _parse_and_analyze(self,
                         test_fn,
                         namespace,
                         literals=None,
                         arg_types=None):
    literals = literals or {}
    node, source = parser.parse_entity(test_fn)
    entity_info = transformer.EntityInfo(
        source_code=source,
        source_file=None,
        namespace=namespace,
        arg_values=None,
        arg_types=arg_types,
        owner_type=None)
    node = qual_names.resolve(node)
    graphs = cfg.build(node)
    node = activity.resolve(node, entity_info)
    node = reaching_definitions.resolve(node, entity_info, graphs,
                                        reaching_definitions.Definition)
    node = live_values.resolve(node, entity_info, literals)
    node = type_info.resolve(node, entity_info)
    node = live_values.resolve(node, entity_info, literals)
    return node

  def test_literals(self):

    a = None

    def test_fn():
      return a

    node = self._parse_and_analyze(test_fn, {}, literals={'a': 'bar'})
    retval_node = node.body[0].body[0].value
    self.assertEquals('bar', anno.getanno(retval_node, 'live_val'))

  def test_primitive_values(self):

    a = None

    def test_fn():
      return a

    node = self._parse_and_analyze(test_fn, {'a': True})
    retval_node = node.body[0].body[0].value
    if six.PY2:
      self.assertEqual(
          anno.getanno(retval_node, 'fqn'), ('__builtin__', 'bool'))
    else:
      self.assertEqual(anno.getanno(retval_node, 'fqn'), ('builtins', 'bool'))

  def test_namespace(self):

    def foo():
      return 'bar'

    def test_fn():
      return foo()

    node = self._parse_and_analyze(test_fn, {'foo': foo})
    func_node = node.body[0].body[0].value.func
    self.assertEquals(foo, anno.getanno(func_node, 'live_val'))
    self.assertEquals(('foo',), anno.getanno(func_node, 'fqn'))

  def test_attribute_names(self):

    def test_fn():
      return constant_op.constant(0)

    node = self._parse_and_analyze(test_fn, {'constant_op': constant_op})
    func_node = node.body[0].body[0].value.func
    self.assertEquals(constant_op.constant, anno.getanno(func_node, 'live_val'))
    self.assertEquals((constant_op.__name__, 'constant'),
                      anno.getanno(func_node, 'fqn'))

  def test_attributes_with_type_hints(self):

    class TestClass(object):

      def member(self):
        pass

      def test_fn(self):
        return self.member()

    node = self._parse_and_analyze(
        TestClass.test_fn, {'constant_op': constant_op},
        arg_types={'self': (TestClass.__name__, TestClass)})
    func_node = node.body[0].body[0].value.func
    self.assertEquals(TestClass.member, anno.getanno(func_node, 'live_val'))
    self.assertEquals(TestClass, anno.getanno(func_node, 'parent_type'))
    self.assertEquals(('TestClass', 'member'), anno.getanno(func_node, 'fqn'))


if __name__ == '__main__':
  test.main()
