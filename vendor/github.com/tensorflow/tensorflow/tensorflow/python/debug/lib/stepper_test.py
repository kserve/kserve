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
"""Unit tests of the tfdbg Stepper."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.core.protobuf import config_pb2
from tensorflow.core.protobuf import rewriter_config_pb2
from tensorflow.python.client import session
from tensorflow.python.debug.lib.stepper import NodeStepper
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import googletest
from tensorflow.python.training import gradient_descent


@test_util.run_v1_only("b/120545219")
class StepperTest(test_util.TensorFlowTestCase):

  def setUp(self):
    self.a = variables.VariableV1(2.0, name="a")
    self.b = variables.VariableV1(3.0, name="b")

    self.c = math_ops.multiply(self.a, self.b, name="c")  # Should be 6.0.
    self.d = math_ops.multiply(self.a, self.a, name="d")  # Should be 4.0.

    self.e = math_ops.multiply(self.d, self.c, name="e")  # Should be 24.0.

    self.f_y = constant_op.constant(0.30, name="f_y")
    self.f = math_ops.div(self.b, self.f_y, name="f")  # Should be 10.0.

    # The there nodes x, y and z form a graph with "cross-links" in. I.e., x
    # and y are both direct inputs to z, but x is also a direct input to y.
    self.x = variables.VariableV1(2.0, name="x")  # Should be 2.0
    self.y = math_ops.negative(self.x, name="y")  # Should be -2.0.

    self.z = math_ops.multiply(self.x, self.y, name="z")  # Should be -4.0.

    rewriter_config = rewriter_config_pb2.RewriterConfig(
        disable_model_pruning=True,
        arithmetic_optimization=rewriter_config_pb2.RewriterConfig.OFF,
        constant_folding=rewriter_config_pb2.RewriterConfig.OFF)
    graph_options = config_pb2.GraphOptions(rewrite_options=rewriter_config)
    config = config_pb2.ConfigProto(graph_options=graph_options)
    self.sess = session.Session(config=config)
    self.sess.run(variables.global_variables_initializer())

  def tearDown(self):
    ops.reset_default_graph()

  def testContToFetchNotInTransitiveClosureShouldError(self):
    with NodeStepper(self.sess, "e:0") as stepper:
      sorted_nodes = stepper.sorted_nodes()
      self.assertEqual(7, len(sorted_nodes))
      self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("a/read"))
      self.assertLess(sorted_nodes.index("b"), sorted_nodes.index("b/read"))
      self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("c"))
      self.assertLess(sorted_nodes.index("b"), sorted_nodes.index("c"))
      self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("d"))
      self.assertLess(sorted_nodes.index("d"), sorted_nodes.index("e"))
      self.assertLess(sorted_nodes.index("c"), sorted_nodes.index("e"))

      self.assertSetEqual(
          {"e:0", "d:0", "c:0", "a/read:0", "b/read:0", "b:0", "a:0"},
          set(stepper.closure_elements()))

      with self.assertRaisesRegexp(
          ValueError,
          "Target \"f:0\" is not in the transitive closure for the fetch of "
          "the stepper"):
        stepper.cont("f:0")

  def testContToNodeNameShouldReturnTensorValue(self):
    with NodeStepper(self.sess, "e:0") as stepper:
      self.assertAllClose(6.0, stepper.cont("c"))

  def testUsingNamesNotUsingIntermediateTensors(self):
    with NodeStepper(self.sess, "e:0") as stepper:
      # The first cont() call should have used no feeds.
      result = stepper.cont("c:0")
      self.assertAllClose(6.0, result)
      self.assertItemsEqual(["a/read:0", "b/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(2.0, stepper.get_tensor_value("a/read:0"))
      self.assertAllClose(3.0, stepper.get_tensor_value("b/read:0"))
      self.assertEqual({}, stepper.last_feed_types())

      # The second cont() call should have used the tensor handle from the
      # previous cont() call.
      result = stepper.cont("e:0")
      self.assertAllClose(24.0, result)
      self.assertItemsEqual(["a/read:0", "b/read:0", "d:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(2.0, stepper.get_tensor_value("a/read:0"))
      self.assertAllClose(3.0, stepper.get_tensor_value("b/read:0"))
      self.assertAllClose(4.0, stepper.get_tensor_value("d:0"))
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_HANDLE,
          "a/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

  def testUsingNodesNotUsingIntermediateTensors(self):
    with NodeStepper(self.sess, self.e) as stepper:
      # There should be no handles before any cont() calls.
      self.assertEqual([], stepper.handle_names())
      self.assertSetEqual(set(), stepper.handle_node_names())

      # Before the cont() call, the stepper should not have access to the value
      # of c:0.
      with self.assertRaisesRegexp(
          ValueError,
          "This stepper instance does not have access to the value of tensor "
          "\"c:0\""):
        stepper.get_tensor_value("c:0")

      # Using the node/tensor itself, instead of the name str, should work on
      # cont().
      result = stepper.cont(self.c)
      self.assertItemsEqual(["a/read:0", "b/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(6.0, result)
      self.assertEqual({}, stepper.last_feed_types())

      self.assertEqual(["c:0"], stepper.handle_names())
      self.assertEqual({"c"}, stepper.handle_node_names())

      # After the cont() call, the stepper should have access to the value of
      # c:0 via a tensor handle.
      self.assertAllClose(6.0, stepper.get_tensor_value("c:0"))

      result = stepper.cont(self.e)
      self.assertAllClose(24.0, result)
      self.assertItemsEqual(["a/read:0", "b/read:0", "d:0"],
                            stepper.intermediate_tensor_names())
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_HANDLE,
          "a/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

  def testContToTensorWithIntermediateDumpShouldUseDump(self):
    with NodeStepper(self.sess, ["e:0", "f:0"]) as stepper:
      stepper.cont("c:0")
      self.assertItemsEqual(["a/read:0", "b/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(2.0, stepper.get_tensor_value("a/read:0"))
      self.assertAllClose(3.0, stepper.get_tensor_value("b/read:0"))

      self.assertAllClose(2.0, stepper.cont("a/read:0"))
      self.assertEqual({
          "a/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE
      }, stepper.last_feed_types())

      self.assertAllClose(10.0, stepper.cont("f:0"))
      self.assertEqual({
          "b/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE
      }, stepper.last_feed_types())

  def testDisablingUseDumpedIntermediatesWorks(self):
    with NodeStepper(self.sess, ["e:0", "f:0"]) as stepper:
      stepper.cont("c:0")
      self.assertItemsEqual(["a/read:0", "b/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(2.0, stepper.get_tensor_value("a/read:0"))
      self.assertAllClose(3.0, stepper.get_tensor_value("b/read:0"))

      self.assertAllClose(10.0,
                          stepper.cont("f:0", use_dumped_intermediates=False))
      self.assertEqual({}, stepper.last_feed_types())

  def testIsFeedableShouldGiveCorrectAnswers(self):
    with NodeStepper(self.sess, self.e) as stepper:
      self.assertTrue(stepper.is_feedable("a/read:0"))
      self.assertTrue(stepper.is_feedable("b/read:0"))
      self.assertTrue(stepper.is_feedable("c:0"))
      self.assertTrue(stepper.is_feedable("d:0"))

  def testOverrideValue(self):
    with NodeStepper(self.sess, self.e) as stepper:
      result = stepper.cont(self.c)
      self.assertAllClose(6.0, result)
      self.assertEqual({}, stepper.last_feed_types())

      # There should be no overrides before any cont() calls.
      self.assertEqual([], stepper.override_names())

      # Calling cont() on c again should lead to use of the handle.
      result = stepper.cont(self.c)
      self.assertAllClose(6.0, result)
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_HANDLE
      }, stepper.last_feed_types())

      # Override c:0.
      stepper.override_tensor("c:0", 7.0)

      # After the overriding, calling get_tensor_value() on c:0 should yield the
      # overriding value.
      self.assertEqual(7.0, stepper.get_tensor_value("c:0"))

      # Now c:0 should have only an override value, but no cached handle,
      # because the handle should have been invalidated.
      self.assertEqual([], stepper.handle_names())
      self.assertSetEqual(set(), stepper.handle_node_names())
      self.assertEqual(["c:0"], stepper.override_names())

      # Run a downstream tensor after the value override.
      result = stepper.cont(self.e)
      self.assertAllClose(28.0, result)  # Should reflect the overriding value.

      # Should use override, instead of the handle.
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_OVERRIDE,
          "a/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

  def testOverrideValueTwice(self):
    with NodeStepper(self.sess, self.e) as stepper:
      # Override once.
      stepper.override_tensor("c:0", 7.0)
      self.assertAllClose(28.0, stepper.cont(self.e))
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_OVERRIDE
      }, stepper.last_feed_types())

      self.assertEqual(["e:0"], stepper.handle_names())
      self.assertSetEqual({"e"}, stepper.handle_node_names())
      self.assertEqual(["c:0"], stepper.override_names())

      # Calling cont(self.e) again. This time the cached tensor handle of e
      # should be used.
      self.assertEqual(28.0, stepper.cont(self.e))
      self.assertEqual({
          "e:0": NodeStepper.FEED_TYPE_HANDLE
      }, stepper.last_feed_types())

      # Override c again. This should have invalidated the cache for e.
      stepper.override_tensor("c:0", 8.0)

      self.assertEqual([], stepper.handle_names())
      self.assertEqual(set(), stepper.handle_node_names())
      self.assertEqual(["c:0"], stepper.override_names())

      self.assertAllClose(32.0, stepper.cont(self.e))
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_OVERRIDE,
          "d:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

  def testRemoveOverrideValue(self):
    with NodeStepper(self.sess, self.e) as stepper:
      result = stepper.cont(self.c)
      self.assertAllClose(6.0, result)
      self.assertEqual({}, stepper.last_feed_types())

      # The previous cont() step should have generated a cached tensor handle.
      self.assertEqual(["c:0"], stepper.handle_names())
      self.assertSetEqual({"c"}, stepper.handle_node_names())

      # Override c:0.
      stepper.override_tensor("c:0", 7.0)

      # The overriding should have invalidated the tensor handle.
      self.assertEqual([], stepper.handle_names())
      self.assertSetEqual(set(), stepper.handle_node_names())
      self.assertEqual(["c:0"], stepper.override_names())

      result = stepper.cont(self.e)
      self.assertAllClose(28.0, result)  # Should reflect the overriding value.
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_OVERRIDE,
          "a/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

      # The handle to tensor e:0 should have been cached, even though its
      # transitive closure contains an override.
      self.assertIn("e:0", stepper.handle_names())
      self.assertSetEqual({"e"}, stepper.handle_node_names())

      # Remove the override.
      stepper.remove_override("c:0")
      # c:0 should not be in the overrides anymore.
      self.assertEqual([], stepper.override_names())

      # Removing the override should have invalidated the tensor handle for c.
      self.assertNotIn("e:0", stepper.handle_names())
      self.assertNotIn("e", stepper.handle_node_names())

      # Should reflect the non-overriding value.
      self.assertAllClose(24.0, stepper.cont(self.e))

      # This time, the handle to tensor e:0 should have been cached again, even
      # thought its transitive closure contains an override.
      self.assertIn("e:0", stepper.handle_names())
      self.assertIn("e", stepper.handle_node_names())

      # Calling cont(self.e) again should have used the tensor handle to e:0.
      self.assertAllClose(24.0, stepper.cont(self.e))
      self.assertEqual({
          "e:0": NodeStepper.FEED_TYPE_HANDLE,
      }, stepper.last_feed_types())

  def testOverrideAndContToSameTensor(self):
    with NodeStepper(self.sess, self.e) as stepper:
      result = stepper.cont(self.c)
      self.assertAllClose(6.0, result)
      self.assertEqual({}, stepper.last_feed_types())
      self.assertEqual(["c:0"], stepper.handle_names())
      self.assertSetEqual({"c"}, stepper.handle_node_names())

      self.assertAllClose(6.0, stepper.cont(self.c))

      # The last cont() call should use the tensor handle directly.
      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_HANDLE
      }, stepper.last_feed_types())

      # Override c:0.
      stepper.override_tensor("c:0", 7.0)

      # As a result of the override, the tensor handle should have been
      # invalidated.
      self.assertEqual([], stepper.handle_names())
      self.assertSetEqual(set(), stepper.handle_node_names())

      result = stepper.cont(self.c)
      self.assertAllClose(7.0, result)

      self.assertEqual({
          "c:0": NodeStepper.FEED_TYPE_OVERRIDE
      }, stepper.last_feed_types())

  def testFinalizeWithPreviousOverrides(self):
    with NodeStepper(self.sess, self.e) as stepper:
      stepper.override_tensor("a/read:0", 20.0)
      self.assertEqual(["a/read:0"], stepper.override_names())

      # Should reflect the overriding value.
      self.assertAllClose(24000.0, stepper.cont("e:0"))
      self.assertEqual({
          "a/read:0": NodeStepper.FEED_TYPE_OVERRIDE
      }, stepper.last_feed_types())

      # Finalize call should have ignored the overriding value.
      self.assertAllClose(24.0, stepper.finalize())

  def testRemoveNonexistentOverrideValue(self):
    with NodeStepper(self.sess, self.e) as stepper:
      self.assertEqual([], stepper.override_names())
      with self.assertRaisesRegexp(
          ValueError, "No overriding value exists for tensor \"c:0\""):
        stepper.remove_override("c:0")

  def testAttemptToOverrideInvalidTensor(self):
    stepper = NodeStepper(self.sess, self.e)

    with self.assertRaisesRegexp(ValueError, "Cannot override tensor \"f:0\""):
      stepper.override_tensor("f:0", 42.0)

  def testInvalidOverrideArgumentType(self):
    with NodeStepper(self.sess, self.e) as stepper:
      with self.assertRaisesRegexp(TypeError, "Expected type str; got type"):
        stepper.override_tensor(self.a, 42.0)

  def testTransitiveClosureWithCrossLinksShouldHaveCorrectOrder(self):
    with NodeStepper(self.sess, "z:0") as stepper:
      sorted_nodes = stepper.sorted_nodes()
      self.assertEqual(4, len(sorted_nodes))
      self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("x/read"))
      self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("y"))
      self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("z"))
      self.assertLess(sorted_nodes.index("y"), sorted_nodes.index("z"))

  def testNodeStepperConstructorShouldAllowListOrTupleOrDictOfFetches(self):
    for i in range(6):
      if i == 0:
        fetches = [self.e, [self.f, self.z]]
      elif i == 1:
        fetches = (self.e, (self.f, self.z))
      elif i == 2:
        fetches = {"e": self.e, "fz": {"f": self.f, "z": self.z}}
      elif i == 3:
        fetches = ["e:0", ["f:0", "z:0"]]
      elif i == 4:
        fetches = ("e:0", ("f:0", "z:0"))
      elif i == 5:
        fetches = {"e": "e:0", "fz": {"f": "f:0", "z": "z:0"}}

      with NodeStepper(self.sess, fetches) as stepper:
        sorted_nodes = stepper.sorted_nodes()
        self.assertEqual(13, len(sorted_nodes))

        # Check the topological order of the sorted nodes.
        self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("x/read"))
        self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("y"))
        self.assertLess(sorted_nodes.index("x"), sorted_nodes.index("z"))
        self.assertLess(sorted_nodes.index("y"), sorted_nodes.index("z"))

        self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("a/read"))
        self.assertLess(sorted_nodes.index("b"), sorted_nodes.index("b/read"))
        self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("c"))
        self.assertLess(sorted_nodes.index("b"), sorted_nodes.index("c"))
        self.assertLess(sorted_nodes.index("a"), sorted_nodes.index("d"))
        self.assertLess(sorted_nodes.index("d"), sorted_nodes.index("e"))
        self.assertLess(sorted_nodes.index("c"), sorted_nodes.index("e"))
        self.assertLess(sorted_nodes.index("b"), sorted_nodes.index("f"))
        self.assertLess(sorted_nodes.index("f_y"), sorted_nodes.index("f"))

        closure_elements = stepper.closure_elements()
        self.assertIn("x/read:0", closure_elements)
        self.assertIn("e:0", closure_elements)
        self.assertIn("f:0", closure_elements)

        self.assertEqual([0], stepper.output_slots_in_closure("x/read"))
        self.assertEqual([0], stepper.output_slots_in_closure("e"))
        self.assertEqual([0], stepper.output_slots_in_closure("f"))

        result = stepper.finalize()
        if i == 0 or i == 1 or i == 3 or i == 4:
          self.assertAllClose(24.0, result[0])
          self.assertAllClose(10.0, result[1][0])
          self.assertAllClose(-4.0, result[1][1])
        elif i == 2 or i == 5:
          self.assertAllClose(24.0, result["e"])
          self.assertAllClose(10.0, result["fz"]["f"])
          self.assertAllClose(-4.0, result["fz"]["z"])


@test_util.run_v1_only("b/120545219")
class StepperTestWithPlaceHolders(test_util.TensorFlowTestCase):

  def setUp(self):
    self.ph0 = array_ops.placeholder(dtypes.float32, shape=(2, 2), name="ph0")
    self.ph1 = array_ops.placeholder(dtypes.float32, shape=(2, 1), name="ph1")

    self.x = math_ops.matmul(self.ph0, self.ph1, name="x")
    self.y = math_ops.add(self.x, self.ph1, name="y")

    self.sess = session.Session()

  def tearDown(self):
    ops.reset_default_graph()

  def testGetTensorValueWorksOnPlaceholder(self):
    with NodeStepper(
        self.sess,
        self.y,
        feed_dict={
            self.ph0: [[1.0, 2.0], [-3.0, 5.0]],
            self.ph1: [[-1.0], [0.5]]
        }) as stepper:
      self.assertAllClose([[1.0, 2.0], [-3.0, 5.0]],
                          stepper.get_tensor_value("ph0"))
      self.assertAllClose([[1.0, 2.0], [-3.0, 5.0]],
                          stepper.get_tensor_value("ph0:0"))
      with self.assertRaisesRegexp(
          KeyError,
          r"The name 'ph0:1' refers to a Tensor which does not exist"):
        stepper.get_tensor_value("ph0:1")

  def testIsPlaceholdersShouldGiveCorrectAnswers(self):
    with NodeStepper(self.sess, self.y) as stepper:
      self.assertTrue(stepper.is_placeholder(self.ph0.name))
      self.assertTrue(stepper.is_placeholder(self.ph1.name))

      self.assertFalse(stepper.is_placeholder(self.x.name))
      self.assertFalse(stepper.is_placeholder(self.y.name))

      with self.assertRaisesRegexp(ValueError,
                                   "A is not in the transitive closure"):
        self.assertFalse(stepper.is_placeholder("A"))

  def testPlaceholdersShouldGiveCorrectAnswers(self):
    with NodeStepper(self.sess, self.y) as stepper:
      self.assertSetEqual({"ph0", "ph1"}, set(stepper.placeholders()))

  def testContWithPlaceholders(self):
    with NodeStepper(
        self.sess,
        self.y,
        feed_dict={
            self.ph0: [[1.0, 2.0], [-3.0, 5.0]],
            self.ph1: [[-1.0], [0.5]]
        }) as stepper:
      self.assertEqual(4, len(stepper.sorted_nodes()))
      self.assertSetEqual({"ph0:0", "ph1:0", "x:0", "y:0"},
                          set(stepper.closure_elements()))

      result = stepper.cont(self.x)
      self.assertAllClose([[0.0], [5.5]], result)
      self.assertEqual({
          "ph0:0": NodeStepper.FEED_TYPE_CLIENT,
          "ph1:0": NodeStepper.FEED_TYPE_CLIENT,
      }, stepper.last_feed_types())

      self.assertEqual(["x:0"], stepper.handle_names())
      self.assertSetEqual({"x"}, stepper.handle_node_names())

      result = stepper.cont(self.y)
      self.assertAllClose([[-1.0], [6.0]], result)
      self.assertEqual({
          "x:0": NodeStepper.FEED_TYPE_HANDLE,
          "ph1:0": NodeStepper.FEED_TYPE_CLIENT,
      }, stepper.last_feed_types())

  def testAttemptToContToPlaceholderWithTensorFeedKeysShouldWork(self):
    """Continuing to a placeholder should be allowed, using client feed."""

    ph0_feed = [[1.0, 2.0], [-3.0, 5.0]]
    ph1_feed = [[-1.0], [0.5]]
    with NodeStepper(
        self.sess, self.y, feed_dict={
            self.ph0: ph0_feed,
            self.ph1: ph1_feed,
        }) as stepper:
      self.assertAllClose(ph0_feed, stepper.cont(self.ph0))
      self.assertEqual({
          self.ph0.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      self.assertAllClose(ph1_feed, stepper.cont(self.ph1))
      self.assertEqual({
          self.ph1.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      ph0_node = self.sess.graph.as_graph_element("ph0")
      self.assertAllClose(ph0_feed, stepper.cont(ph0_node))
      self.assertEqual({
          self.ph0.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      self.assertAllClose([[-1.0], [6.0]], stepper.finalize())

  def testAttemptToContToPlaceholderWithTensorNameFeedKeysShouldWork(self):

    ph0_feed = [[1.0, 2.0], [-3.0, 5.0]]
    ph1_feed = [[-1.0], [0.5]]
    with NodeStepper(
        self.sess,
        self.y,
        feed_dict={
            self.ph0.name: ph0_feed,
            self.ph1.name: ph1_feed,
        }) as stepper:
      self.assertAllClose(ph0_feed, stepper.cont(self.ph0))
      self.assertEqual({
          self.ph0.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      self.assertAllClose(ph1_feed, stepper.cont(self.ph1))
      self.assertEqual({
          self.ph1.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      ph0_node = self.sess.graph.as_graph_element("ph0")
      self.assertAllClose(ph0_feed, stepper.cont(ph0_node))
      self.assertEqual({
          self.ph0.name: NodeStepper.FEED_TYPE_CLIENT
      }, stepper.last_feed_types())

      self.assertAllClose([[-1.0], [6.0]], stepper.finalize())


@test_util.run_v1_only("b/120545219")
class StepperAssignAddTest(test_util.TensorFlowTestCase):

  def setUp(self):
    self.v = variables.VariableV1(10.0, name="v")
    self.p = math_ops.add(self.v, self.v, name="p")
    self.q = math_ops.multiply(self.p, self.p, name="q")
    self.delta = constant_op.constant(2.0, name="delta")
    self.v_add = state_ops.assign_add(self.v, self.delta, name="v_add")
    self.v_add_plus_one = math_ops.add(self.v_add,
                                       1.0,
                                       name="v_add_plus_one")

    rewriter_config = rewriter_config_pb2.RewriterConfig(
        disable_model_pruning=True,
        arithmetic_optimization=rewriter_config_pb2.RewriterConfig.OFF,
        constant_folding=rewriter_config_pb2.RewriterConfig.OFF)
    graph_options = config_pb2.GraphOptions(rewrite_options=rewriter_config)
    config = config_pb2.ConfigProto(graph_options=graph_options)
    self.sess = session.Session(config=config)
    self.sess.run(self.v.initializer)

  def tearDown(self):
    ops.reset_default_graph()

  def testLastUpdatedVariablesReturnsNoneBeforeAnyContCalls(self):
    with NodeStepper(self.sess, [self.q, self.v_add]) as stepper:
      self.assertIsNone(stepper.last_updated())

  def testContToUpdateInvalidatesDumpedIntermediates(self):
    with NodeStepper(self.sess, [self.q, self.v_add]) as stepper:
      self.assertAllClose(400.0, stepper.cont("q:0"))
      self.assertItemsEqual(["v/read:0", "p:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(10.0, stepper.get_tensor_value("v/read:0"))
      self.assertAllClose(20.0, stepper.get_tensor_value("p:0"))

      self.assertAllClose(
          12.0, stepper.cont(
              self.v_add, invalidate_from_updated_variables=True))
      self.assertAllClose(12.0, self.sess.run(self.v))
      self.assertSetEqual({self.v.name}, stepper.last_updated())
      self.assertItemsEqual(["v:0"], stepper.dirty_variables())
      # Updating the value of v by calling v_add should have invalidated the
      # dumped intermediate tensors for v/read:0 and p:0.
      self.assertItemsEqual(["delta:0"], stepper.intermediate_tensor_names())
      with self.assertRaisesRegexp(
          ValueError,
          r"This stepper instance does not have access to the value of tensor "
          r"\"p:0\""):
        stepper.get_tensor_value("p:0")

      # The next cont to q should not have used any dumped intermediate tensors
      # and its result should reflect the updated value.
      self.assertAllClose(576.0, stepper.cont("q:0"))
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual({}, stepper.last_feed_types())

  def testOverridingUpstreamTensorInvalidatesDumpedIntermediates(self):
    with NodeStepper(self.sess, self.q) as stepper:
      self.assertAllClose(400.0, stepper.cont("q:0"))
      self.assertItemsEqual(["v/read:0", "p:0"],
                            stepper.intermediate_tensor_names())
      self.assertAllClose(10.0, stepper.get_tensor_value("v/read:0"))
      self.assertAllClose(20.0, stepper.get_tensor_value("p:0"))

      stepper.override_tensor("v/read:0", 11.0)
      self.assertItemsEqual(["v/read:0"], stepper.override_names())
      # Overriding the upstream v/read:0 should have invalidated the dumped
      # intermediate tensor for the downstream p:0.
      self.assertItemsEqual([], stepper.intermediate_tensor_names())

      # The next cont to q should not have used any dumped intermediate tensors
      # and its result should reflect the overriding value.
      self.assertAllClose(484.0, stepper.cont("q:0"))
      self.assertEqual({
          "v/read:0": NodeStepper.FEED_TYPE_OVERRIDE
      }, stepper.last_feed_types())

  def testRemovingOverrideToUpstreamTensorInvalidatesDumpedIntermediates(self):
    with NodeStepper(self.sess, self.q) as stepper:
      stepper.override_tensor("v/read:0", 9.0)
      self.assertItemsEqual(["v/read:0"], stepper.override_names())

      self.assertAllClose(324.0, stepper.cont(self.q))
      self.assertItemsEqual(["p:0"], stepper.intermediate_tensor_names())

      stepper.remove_override("v/read:0")
      self.assertItemsEqual([], stepper.override_names())
      # Removing the pre-existing override to v/read:0 should have invalidated
      # the dumped intermediate tensor.
      self.assertItemsEqual([], stepper.intermediate_tensor_names())

  def testRepeatedCallsToAssignAddDoesNotUpdateVariableAgain(self):
    with NodeStepper(self.sess, self.v_add) as stepper:
      stepper.cont(self.v_add)
      self.assertSetEqual({self.v.name}, stepper.last_updated())
      self.assertAllClose(12.0, stepper.cont(self.v))
      stepper.cont(self.v_add)
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual({"v_add:0": NodeStepper.FEED_TYPE_HANDLE},
                       stepper.last_feed_types())
      self.assertAllClose(12.0, stepper.cont(self.v))

  def testRepeatedCallsToAssignAddDownStreamDoesNotUpdateVariableAgain(self):
    with NodeStepper(self.sess, self.v_add_plus_one) as stepper:
      stepper.cont(self.v_add_plus_one)
      self.assertSetEqual({self.v.name}, stepper.last_updated())
      self.assertAllClose(12.0, stepper.cont(self.v))
      stepper.cont(self.v_add_plus_one)
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual({"v_add_plus_one:0": NodeStepper.FEED_TYPE_HANDLE},
                       stepper.last_feed_types())
      self.assertAllClose(12.0, stepper.cont(self.v))


@test_util.run_v1_only("b/120545219")
class StepperBackwardRunTest(test_util.TensorFlowTestCase):

  def setUp(self):
    """Test setup.

    Structure of the forward graph:
              f
             | |
        -----   -----
        |           |
        d           e
       | |         | |
    ---   ---------  ---
    |         |        |
    a         b        c

    Construct a backward graph using the GradientDescentOptimizer.
    """

    self.a = variables.VariableV1(1.0, name="a")
    self.b = variables.VariableV1(2.0, name="b")
    self.c = variables.VariableV1(4.0, name="c")
    self.d = math_ops.multiply(self.a, self.b, name="d")
    self.e = math_ops.multiply(self.b, self.c, name="e")
    self.f = math_ops.multiply(self.d, self.e, name="f")

    # Gradient descent optimizer that minimizes g.
    gradient_descent.GradientDescentOptimizer(0.01).minimize(
        self.f, name="optim")

    rewriter_config = rewriter_config_pb2.RewriterConfig(
        disable_model_pruning=True,
        arithmetic_optimization=rewriter_config_pb2.RewriterConfig.OFF,
        constant_folding=rewriter_config_pb2.RewriterConfig.OFF)
    graph_options = config_pb2.GraphOptions(rewrite_options=rewriter_config)
    config = config_pb2.ConfigProto(graph_options=graph_options)
    self.sess = session.Session(config=config)
    self.sess.run(variables.global_variables_initializer())

  def tearDown(self):
    ops.reset_default_graph()

  def testContToUpdateA(self):
    with NodeStepper(self.sess, "optim") as stepper:
      result = stepper.cont("a:0")
      self.assertAllClose(1.0, result)
      self.assertEqual({}, stepper.last_feed_types())

      result = stepper.cont("optim/learning_rate:0")
      self.assertAllClose(0.01, result)
      self.assertEqual({}, stepper.last_feed_types())

      # Before any cont calls on ApplyGradientDescent, there should be no
      # "dirty" variables.
      self.assertEqual(set(), stepper.dirty_variables())

      # First, all the two control inputs to optim.
      result = stepper.cont("optim/update_a/ApplyGradientDescent",
                            invalidate_from_updated_variables=True)

      # Now variable a should have been marked as dirty due to the update
      # by optim/update_a/ApplyGradientDescent.
      self.assertSetEqual({"a:0"}, stepper.last_updated())
      self.assertEqual({"a:0"}, stepper.dirty_variables())
      self.assertIsNone(result)
      self.assertEqual({
          "optim/learning_rate:0": NodeStepper.FEED_TYPE_HANDLE
      }, stepper.last_feed_types())

      # Check that Variable "a" has been updated properly, but "b", "c" and "d"
      # remain the same.
      # For backprop on Variable a:
      #   Because f = a * b * b * c, df / da = b * b * c.
      #   1.0 - learning_rate * b * b * c
      #     = 1.0 -  0.01 * 2.0 * 2.0 * 4.0 = 0.84.
      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(2.0, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

  def testContToUpdateB(self):
    with NodeStepper(self.sess, "optim") as stepper:
      result = stepper.cont("optim/update_b/ApplyGradientDescent",
                            invalidate_from_updated_variables=True)
      self.assertIsNone(result)
      self.assertSetEqual({"b:0"}, stepper.last_updated())
      self.assertEqual(set(["b:0"]), stepper.dirty_variables())

      # For backprop on Variable b:
      #   Because f = a * b * b * c, df / da = 2 * a * b * c.
      #   2.0 - learning_rate * 2 * a * b * c
      #     = 2.0 - 0.01 * 2 * 1.0 * 2.0 * 4.0 = 1.84
      self.assertAllClose(1.0, self.sess.run(self.a))
      self.assertAllClose(1.84, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

  def testContAfterUpdateWithoutRestoringVariableValue(self):
    with NodeStepper(self.sess, "optim") as stepper:
      # First, update Variable a from 1.0 to 0.84.
      result = stepper.cont(
          "optim/update_a/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertIsNone(result)
      self.assertSetEqual({"a:0"}, stepper.last_updated())
      self.assertEqual(set(["a:0"]), stepper.dirty_variables())
      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(2.0, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))
      # Tracking of the updated variables should have invalidated all
      # intermediate tensors downstream to a:0.
      self.assertNotIn("a/read:0", stepper.intermediate_tensor_names())
      self.assertNotIn("d:0", stepper.intermediate_tensor_names())

      # Second, update Variable b without the default restore_variable_values.
      result = stepper.cont(
          "optim/update_b/ApplyGradientDescent", restore_variable_values=False)
      self.assertIsNone(result)
      # For the backprop on Variable b under the updated value of a:
      #   2.0 - learning_rate * 2 * a' * b * c
      #     = 2.0 - 0.01 * 2 * 0.84 * 2.0 * 4.0 = 1.8656
      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(1.8656, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

  def testContNotInvalidatingFromVariableUpdatesWorksForNextUpdate(self):
    with NodeStepper(self.sess, "optim") as stepper:
      self.assertIsNone(stepper.cont(
          "optim/update_a/ApplyGradientDescent",
          invalidate_from_updated_variables=False))
      # Even though invalidate_from_updated_variables is set to False, dirty
      # variables should still have been tracked.
      self.assertSetEqual({"a:0"}, stepper.last_updated())
      self.assertEqual({"a:0"}, stepper.dirty_variables())
      self.assertIn("a/read:0", stepper.intermediate_tensor_names())
      self.assertIn("b/read:0", stepper.intermediate_tensor_names())
      self.assertIn("c/read:0", stepper.intermediate_tensor_names())
      self.assertIn("d:0", stepper.intermediate_tensor_names())
      self.assertIn("e:0", stepper.intermediate_tensor_names())
      self.assertIn("optim/learning_rate:0",
                    stepper.intermediate_tensor_names())
      self.assertNotIn("a:0", stepper.intermediate_tensor_names())
      self.assertNotIn("b:0", stepper.intermediate_tensor_names())
      self.assertNotIn("c:0", stepper.intermediate_tensor_names())

      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(2.0, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

      # For the backprop on Variable b, the result should reflect the original
      # value of Variable a, even though Variable a has actually been updated.
      #   2.0 - learning_rate * 2 * a * b * c
      #     = 2.0 - 0.01 * 2 * 1.0 * 2.0 * 4.0 = 1.84
      self.assertIsNone(stepper.cont(
          "optim/update_b/ApplyGradientDescent",
          invalidate_from_updated_variables=False,
          restore_variable_values=False))
      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(1.84, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

  def testUpdateTwiceRestoreVariable(self):
    with NodeStepper(self.sess, "optim") as stepper:
      result = stepper.cont(
          "optim/update_a/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertIsNone(result)
      self.assertSetEqual({"a:0"}, stepper.last_updated())
      self.assertEqual({"a:0"}, stepper.dirty_variables())

      result = stepper.cont(
          "optim/update_b/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertIsNone(result)
      # Variables a and c should have been restored and hence no longer dirty.
      # Variable b should have been marked as dirty.
      self.assertSetEqual({"b:0"}, stepper.last_updated())
      self.assertEqual({"b:0"}, stepper.dirty_variables())

    # The result of the update should be identitcal to as if only update_b is
    # run.
    self.assertAllClose(1.0, self.sess.run(self.a))
    self.assertAllClose(1.84, self.sess.run(self.b))
    self.assertAllClose(4.0, self.sess.run(self.c))

  def testSelectiveHandleUsageDependingOnTransitiveCleanliness(self):
    """Test tensor handlers are using only during clean transitive closure.

    "clean" means no Variables have been updated by preceding cont() calls.
    """

    with NodeStepper(self.sess, "optim") as stepper:
      # First, call cont() on the two tensors on the intermediate level: e and
      # f.
      result = stepper.cont("d:0")
      self.assertAllClose(2.0, result)
      self.assertEqual({}, stepper.last_feed_types())
      self.assertItemsEqual(["a/read:0", "b/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertItemsEqual(["d:0"], stepper.handle_names())
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual(set(), stepper.dirty_variables())

      result = stepper.cont("e:0")
      self.assertAllClose(8.0, result)
      self.assertEqual({
          "b/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE
      }, stepper.last_feed_types())
      self.assertItemsEqual(["d:0", "e:0"], stepper.handle_names())
      self.assertItemsEqual(["a/read:0", "b/read:0", "c/read:0"],
                            stepper.intermediate_tensor_names())
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual(set(), stepper.dirty_variables())

      # Now run update_a, so as to let Variable a be dirty.
      result = stepper.cont(
          "optim/update_a/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertIsNone(result)
      # Due to the update to the value of a:0, the dumped intermediate a/read:0
      # should have been invalidated.
      self.assertNotIn("a/read:0", stepper.intermediate_tensor_names())
      self.assertSetEqual({"a:0"}, stepper.last_updated())
      self.assertEqual({"a:0"}, stepper.dirty_variables())

      # Now, run update_b.
      result = stepper.cont(
          "optim/update_b/ApplyGradientDescent", restore_variable_values=True)
      self.assertIsNone(result)

      # The last cont() run should have use the handle of tensor e, but not the
      # handle of tensor d, because the transitive closure of e is clean,
      # whereas that of d is dirty due to the update to a in the previous cont()
      # call.
      last_feed_types = stepper.last_feed_types()
      self.assertNotIn("d:0", last_feed_types)
      self.assertEqual(NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
                       last_feed_types["b/read:0"])
      self.assertEqual(NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
                       last_feed_types["c/read:0"])

      # The result of the update_b should be identical to as if no other
      # update_* cont() calls have occurred before.
      self.assertAllClose(1.0, self.sess.run(self.a))
      self.assertAllClose(1.84, self.sess.run(self.b))
      self.assertAllClose(4.0, self.sess.run(self.c))

  def testRestoreVariableValues(self):
    """Test restore_variable_values() restores the old values of variables."""

    with NodeStepper(self.sess, "optim") as stepper:
      stepper.cont(
          "optim/update_b/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertAllClose(1.84, self.sess.run(self.b))

      stepper.restore_variable_values()
      self.assertAllClose(2.0, self.sess.run(self.b))

  def testFinalize(self):
    """Test finalize() to restore variables and run the original fetch."""

    with NodeStepper(self.sess, "optim") as stepper:
      # Invoke update_b before calling finalize.
      stepper.cont(
          "optim/update_b/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)

      result = stepper.finalize()
      self.assertIsNone(result)

      # The results of the Variable updates should be the same as if no cont()
      # call has occurred on update_b.
      self.assertAllClose(0.84, self.sess.run(self.a))
      self.assertAllClose(1.84, self.sess.run(self.b))
      self.assertAllClose(3.96, self.sess.run(self.c))

  def testOverrideThenContToUpdateThenRemoveOverrideThenUpdateAgain(self):
    """Test cont() to update nodes after overriding tensor values."""

    with NodeStepper(self.sess, "optim") as stepper:
      result = stepper.cont("d:0")
      self.assertAllClose(2.0, result)
      self.assertEqual({}, stepper.last_feed_types())
      self.assertSetEqual(set(), stepper.last_updated())
      self.assertEqual(set(), stepper.dirty_variables())
      self.assertEqual(["d:0"], stepper.handle_names())
      self.assertSetEqual({"d"}, stepper.handle_node_names())

      # Override the value from 1.0 to 10.0.
      stepper.override_tensor("a/read:0", 10.0)

      self.assertEqual(["a/read:0"], stepper.override_names())

      result = stepper.cont(
          "optim/update_c/ApplyGradientDescent",
          invalidate_from_updated_variables=True,
          restore_variable_values=True)
      self.assertIsNone(result)

      # The last cont() call should have not used the tensor handle to d:0,
      # because the transitive closure of d:0 contains an override tensor.
      self.assertEqual({
          "a/read:0": NodeStepper.FEED_TYPE_OVERRIDE,
          "b/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

      # The tensor handle to d:0 should have been removed due to the dirty
      # transitive closure.
      self.assertEqual([], stepper.handle_names())
      self.assertSetEqual(set(), stepper.handle_node_names())

      # For this backprop on c, the overriding value of a/read:0 should have
      # been used:
      #   4.0 - learning_rate * a * b * b
      #     = 4.0 - 0.01 * 10.0 * 2.0 * 2.0 = 3.6.
      self.assertAllClose(3.6, self.sess.run(self.c))

      # Now remove the overriding value of a/read:0.
      stepper.remove_override("a/read:0")
      self.assertEqual([], stepper.override_names())

      # Obtain the tensor handle to d:0 again.
      result = stepper.cont("d:0")
      self.assertAllClose(2.0, result)
      self.assertEqual(["d:0"], stepper.handle_names())
      self.assertSetEqual({"d"}, stepper.handle_node_names())
      self.assertNotIn("a/read:0", stepper.last_feed_types())

      # Then call update_c again, without restoring c.
      result = stepper.cont("optim/update_c/ApplyGradientDescent",
                            restore_variable_values=False)
      self.assertIsNone(result)
      self.assertNotIn("a/read:0", stepper.last_feed_types())

      # This time, the d:0 tensor handle should have been used, because its
      # transitive closure is clean.
      self.assertEqual({
          "b/read:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
          "d:0": NodeStepper.FEED_TYPE_HANDLE,
          "optim/learning_rate:0": NodeStepper.FEED_TYPE_DUMPED_INTERMEDIATE,
      }, stepper.last_feed_types())

      # For this backprop on c, the overriding value of a/read:0 should have
      # been used:
      #   3.6 - learning_rate * a * b * b
      #     = 3.6 - 0.01 * 1.0 * 2.0 * 2.0 = 3.56.
      self.assertAllClose(3.56, self.sess.run(self.c))

  def testContToNodeWithOutputTensors(self):
    """cont() to an op should cache its output tensors if appropriate."""

    with NodeStepper(self.sess, "optim") as stepper:
      # In the transitive closure of the stepper, look for an op of which the
      # output tensor also is in the transitive closure.
      # Do not assume a specific op, e.g., ""gradients/e_grad/Reshape_1",
      # because it may vary between builds.
      closure_elements = stepper.closure_elements()
      op_with_output_in_closure = None
      for element_name in closure_elements:
        if element_name + ":0" in closure_elements:
          op_with_output_in_closure = str(element_name)
          break

      self.assertEqual(
          [0], stepper.output_slots_in_closure(op_with_output_in_closure))

      self.assertIsNotNone(op_with_output_in_closure)
      output_tensor = op_with_output_in_closure + ":0"

      # The op "gradients/?_grad/Reshape_1" is in the transitive closure of the
      # stepper, because it is the control input to another o. However, its
      # output tensor "gradients/?_grad/Reshape_1:0" is also in the transitive
      # closure, because it is the (non-control) input of certain ops. Calling
      # cont() on the op should lead to the caching of the tensor handle for
      # the output tensor.
      stepper.cont(op_with_output_in_closure)

      self.assertEqual([output_tensor], stepper.handle_names())
      self.assertSetEqual({op_with_output_in_closure},
                          stepper.handle_node_names())

      # Do a cont() call that uses the cached tensor of
      # "gradients/?_grad/Reshape_1:0".
      stepper.cont(output_tensor)
      self.assertEqual({
          output_tensor: NodeStepper.FEED_TYPE_HANDLE
      }, stepper.last_feed_types())


if __name__ == "__main__":
  googletest.main()
