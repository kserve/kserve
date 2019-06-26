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
"""Tests for Python module traversal."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.platform import googletest
from tensorflow.tools.common import test_module1
from tensorflow.tools.common import test_module2
from tensorflow.tools.common import traverse


class TestVisitor(object):

  def __init__(self):
    self.call_log = []

  def __call__(self, path, parent, children):
    self.call_log += [(path, parent, children)]


class TraverseTest(googletest.TestCase):

  def test_cycle(self):

    class Cyclist(object):
      pass
    Cyclist.cycle = Cyclist

    visitor = TestVisitor()
    traverse.traverse(Cyclist, visitor)
    # We simply want to make sure we terminate.

  def test_module(self):
    visitor = TestVisitor()
    traverse.traverse(test_module1, visitor)

    called = [parent for _, parent, _ in visitor.call_log]

    self.assertIn(test_module1.ModuleClass1, called)
    self.assertIn(test_module2.ModuleClass2, called)

  def test_class(self):
    visitor = TestVisitor()
    traverse.traverse(TestVisitor, visitor)
    self.assertEqual(TestVisitor,
                     visitor.call_log[0][1])
    # There are a bunch of other members, but make sure that the ones we know
    # about are there.
    self.assertIn('__init__', [name for name, _ in visitor.call_log[0][2]])
    self.assertIn('__call__', [name for name, _ in visitor.call_log[0][2]])

    # There are more classes descended into, at least __class__ and
    # __class__.__base__, neither of which are interesting to us, and which may
    # change as part of Python version etc., so we don't test for them.

  def test_non_class(self):
    integer = 5
    visitor = TestVisitor()
    traverse.traverse(integer, visitor)
    self.assertEqual([], visitor.call_log)


if __name__ == '__main__':
  googletest.main()
