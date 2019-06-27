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
"""critical section tests."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.framework.python.ops import critical_section_ops
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.eager import context
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.platform import test
from tensorflow.python.platform import tf_logging as logging
# TODO(ebrevdo): Re-enable once CriticalSection is in core.
# from tensorflow.python.training import saver as saver_lib


class CriticalSectionTest(test.TestCase):

  @test_util.run_in_graph_and_eager_modes
  def testCreateCriticalSection(self):
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    v = resource_variable_ops.ResourceVariable(0.0, name="v")

    def fn(a, b):
      c = v.value()
      with ops.control_dependencies([c]):
        nv = v.assign_add(a * b)
        with ops.control_dependencies([nv]):
          return array_ops.identity(c)

    num_concurrent = 100
    r = [cs.execute(fn, 1.0, 2.0) for _ in range(num_concurrent)]
    self.evaluate(v.initializer)
    r_value = self.evaluate(r)
    self.assertAllClose([2.0 * i for i in range(num_concurrent)],
                        sorted(r_value))

  @test_util.run_in_graph_and_eager_modes
  def testCriticalSectionWithControlFlow(self):
    for outer_cond in [False, True]:
      for inner_cond in [False, True]:
        cs = critical_section_ops.CriticalSection(shared_name="cs")
        v = resource_variable_ops.ResourceVariable(0.0, name="v")
        num_concurrent = 100

        # pylint: disable=cell-var-from-loop
        def fn(a, b):
          c = v.read_value()
          def true_fn():
            with ops.control_dependencies([c]):
              nv = v.assign_add(a * b)
              with ops.control_dependencies([nv]):
                return array_ops.identity(c)
          return control_flow_ops.cond(
              array_ops.identity(inner_cond), true_fn, lambda: c)

        def execute():
          return cs.execute(fn, 1.0, 2.0)

        r = [
            control_flow_ops.cond(array_ops.identity(outer_cond),
                                  execute,
                                  v.read_value)
            for _ in range(num_concurrent)
        ]
        # pylint: enable=cell-var-from-loop

        self.evaluate(v.initializer)
        r_value = self.evaluate(r)
        if inner_cond and outer_cond:
          self.assertAllClose([2.0 * i for i in range(num_concurrent)],
                              sorted(r_value))
        else:
          self.assertAllClose([0] * num_concurrent, r_value)

  def testCriticalSectionInParallelDoesntDeadlockOnError(self):
    # No eager mode execution of this test because eager does not
    # run fn() in parallel, which is where the deadlock could
    # potentially occur (in graph mode).
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    v = resource_variable_ops.ResourceVariable(0.0, name="v")

    def fn(i):
      error = control_flow_ops.Assert((i % 2) == 1, ["Error"])
      with ops.control_dependencies([error]):
        return v.read_value()
    num_concurrent = 2
    r = [cs.execute(fn, i) for i in range(num_concurrent)]
    self.evaluate(v.initializer)
    for _ in range(100):
      with self.assertRaisesOpError("Error"):
        self.evaluate(r)

  @test_util.run_in_graph_and_eager_modes
  def testCreateCriticalSectionFnReturnsOp(self):
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    v = resource_variable_ops.ResourceVariable(0.0, name="v")

    def fn_return_op(a, b):
      c = v.read_value()
      with ops.control_dependencies([c]):
        nv = v.assign_add(a * b)
        with ops.control_dependencies([nv]):
          return control_flow_ops.no_op()

    num_concurrent = 100
    r = [cs.execute(fn_return_op, 1.0, 2.0) for _ in range(num_concurrent)]
    self.evaluate(v.initializer)
    self.evaluate(r)
    final_v = self.evaluate(v)
    self.assertAllClose(2.0 * num_concurrent, final_v)

  def testCollection(self):
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    self.assertIn(
        cs, ops.get_collection(critical_section_ops.CRITICAL_SECTIONS))
    execute = cs.execute(lambda x: x + 1, 1.0, name="my_execute")
    execute_op = [
        x for x in execute.graph.get_operations()
        if "my_execute" in x.name and "MutexLock" in x.type
    ][0]
    self.assertIn(
        execute_op,
        [signature.op for signature in
         ops.get_collection(critical_section_ops.CRITICAL_SECTION_EXECUTIONS)])

  def testRecursiveCriticalSectionAccessIsIllegal(self):
    # This does not work properly in eager mode.  Eager users will
    # just hit a deadlock if they do this.  But at least it'll be easier
    # to debug.
    cs = critical_section_ops.CriticalSection()
    def fn(x):
      return cs.execute(lambda y: y + 1, x)
    with self.assertRaisesRegexp(
        ValueError,
        r"attempts to directly access the CriticalSection in which it "
        r"would be running"):
      cs.execute(fn, 1.0)

  def testRecursiveCriticalSectionAccessViaCapturedTensorIsProtected(self):
    # This one is subtle; and we're being overly cautious here.  The
    # deadlock we are ensuring we catch is:
    #
    # to_capture = CS[lambda x: x + 1](1.0)
    # deadlocked = CS[lambda x: x + to_capture](1.0)
    #
    # This would have caused a deadlock because executing `deadlocked` will
    # lock the mutex on CS; but then due to dependencies, will attempt
    # to compute `to_capture`.  This computation requires locking CS,
    # but that is not possible now because CS is already locked by
    # `deadlocked`.
    #
    # We check that CriticalSection.execute properly inserts new
    # control dependencies to its lock to ensure all captured
    # operations are finished before anything runs within the critical section.
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    fn = array_ops.identity
    to_capture = cs.execute(fn, 1.0)
    fn_captures = lambda x: x + to_capture
    to_capture_too = array_ops.identity(to_capture)

    ex_0 = cs.execute(fn_captures, 1.0)

    with ops.control_dependencies([to_capture]):
      # This is OK because to_capture will execute before this next call
      ex_1 = cs.execute(fn_captures, 1.0)

    dependency = array_ops.identity(to_capture)

    fn_captures_dependency = lambda x: x + dependency

    ex_2 = cs.execute(fn_captures_dependency, 1.0)

    with ops.control_dependencies([to_capture_too]):
      ex_3 = cs.execute(fn_captures_dependency, 1.0)

    # Ensure there's no actual deadlock on to_execute.
    self.assertEquals(2.0, self.evaluate(ex_0))
    self.assertEquals(2.0, self.evaluate(ex_1))
    self.assertEquals(2.0, self.evaluate(ex_2))
    self.assertEquals(2.0, self.evaluate(ex_3))

  def testRecursiveCriticalSectionAccessWithinLoopIsProtected(self):
    cs = critical_section_ops.CriticalSection(shared_name="cs")

    def body_implicit_capture(i, j):
      # This would have caused a deadlock if not for logic in execute
      # that inserts additional control dependencies onto the lock op:
      #   * Loop body argument j is captured by fn()
      #   * i is running in parallel to move forward the execution
      #   * j is not being checked by the predicate function
      #   * output of cs.execute() is returned as next j.
      fn = lambda: j + 1
      return (i + 1, cs.execute(fn))

    (i_n, j_n) = control_flow_ops.while_loop(
        lambda i, _: i < 1000,
        body_implicit_capture,
        [0, 0],
        parallel_iterations=25)
    logging.warn(
        "\n==============\nRunning "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_implicit_capture'\n"
        "==============\n")
    self.assertEquals((1000, 1000), self.evaluate((i_n, j_n)))
    logging.warn(
        "\n==============\nSuccessfully finished running "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_implicit_capture'\n"
        "==============\n")

    def body_implicit_capture_protected(i, j):
      # This version is ok because we manually add a control
      # dependency on j, which is an argument to the while_loop body
      # and captured by fn.
      fn = lambda: j + 1
      with ops.control_dependencies([j]):
        return (i + 1, cs.execute(fn))

    (i_n, j_n) = control_flow_ops.while_loop(
        lambda i, _: i < 1000,
        body_implicit_capture_protected,
        [0, 0],
        parallel_iterations=25)
    logging.warn(
        "\n==============\nRunning "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_implicit_capture_protected'\n"
        "==============\n")
    self.assertEquals((1000, 1000), self.evaluate((i_n, j_n)))
    logging.warn(
        "\n==============\nSuccessfully finished running "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_implicit_capture_protected'\n"
        "==============\n")

    def body_args_capture(i, j):
      # This version is ok because j is an argument to fn and we can
      # ensure there's a control dependency on j.
      fn = lambda x: x + 1
      return (i + 1, cs.execute(fn, j))

    (i_n, j_n) = control_flow_ops.while_loop(
        lambda i, _: i < 1000,
        body_args_capture,
        [0, 0],
        parallel_iterations=25)
    logging.warn(
        "\n==============\nRunning "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_args_capture'\n"
        "==============\n")
    self.assertEquals((1000, 1000), self.evaluate((i_n, j_n)))
    logging.warn(
        "\n==============\nSuccessfully finished running "
        "'testRecursiveCriticalSectionAccessWithinLoopDoesNotDeadlock "
        "body_args_capture'\n"
        "==============\n")

  def testRecursiveCriticalSectionAccessIsIllegalSameSharedName(self):
    # This does not work properly in eager mode.  Eager users will
    # just hit a deadlock if they do this.  But at least it'll be easier
    # to debug.
    cs = critical_section_ops.CriticalSection(shared_name="cs")
    cs_same = critical_section_ops.CriticalSection(shared_name="cs")
    def fn(x):
      return cs_same.execute(lambda x: x+1, x)
    with self.assertRaisesRegexp(
        ValueError,
        r"attempts to directly access the CriticalSection in which it "
        r"would be running"):
      cs.execute(fn, 1.0)

  def testMultipleCSExecutionsRequestSameResource(self):
    cs0 = critical_section_ops.CriticalSection()
    cs1 = critical_section_ops.CriticalSection()
    v = resource_variable_ops.ResourceVariable(0.0, name="v")
    cs0.execute(lambda: v + 1)
    # It's OK for the same CriticalSection to access this resource.
    cs0.execute(lambda: v - 1)
    # It's *not* OK for a different CriticalSection to access it by
    # default.
    with self.assertRaisesRegexp(
        ValueError, "requested exclusive resource access"):
      cs1.execute(lambda: v + 1)
    # It's not even OK if the second call doesn't request exclusive access.
    with self.assertRaisesRegexp(
        ValueError, "requested exclusive resource access"):
      cs1.execute(lambda: v + 1, exclusive_resource_access=False)

    v2 = resource_variable_ops.ResourceVariable(0.0, name="v2")
    cs0.execute(lambda: v2 + 1, exclusive_resource_access=False)
    # It's OK if neither requests exclusive resource access.
    cs1.execute(lambda: v2 + 1, exclusive_resource_access=False)

    # It's not OK if the second request requires exlusive resource
    # access.
    with self.assertRaisesRegexp(
        ValueError, "requested exclusive resource access"):
      cs1.execute(lambda: v2 + 1)

  def testControlDependencyFromOutsideWhileLoopMixedWithInsideLoop(self):
    cs = critical_section_ops.CriticalSection()
    v = resource_variable_ops.ResourceVariable(0, name="v")
    # Make sure that the control dependencies on v do not cause issues
    # in the lock_op's automatic control dependency adder.
    #
    # Note, here v must be a resource variable (or something similar),
    # otherwise it gets hoisted into the while_loop by the time we add
    # control dependencies to the lock_op.
    out = control_flow_ops.while_loop(
        lambda i: i < 10, lambda i: cs.execute(lambda j: v + j + 1, i), [0])
    self.evaluate(v.initializer)
    self.assertEqual(10, self.evaluate(out))

  @test_util.run_in_graph_and_eager_modes
  def testInsideFunction(self):
    cs = critical_section_ops.CriticalSection()
    v = resource_variable_ops.ResourceVariable(1)
    def fn():
      return v.read_value()

    # map() creates a TensorFlow function.
    ds = dataset_ops.Dataset.range(1).map(lambda _: cs.execute(fn))

    def get_first():
      if context.executing_eagerly():
        return self.evaluate(ds.make_one_shot_iterator().get_next())
      itr = ds.make_initializable_iterator()
      self.evaluate([v.initializer, itr.initializer])
      return self.evaluate(itr.get_next())

    self.assertEqual(1, get_first())

  # TODO(ebrevdo): Re-enable once CriticalSection is in core.
  #
  # def testCriticalSectionAndExecuteOpSaverRoundTrip(self):
  #   cs = critical_section_ops.CriticalSection()
  #   r = cs.execute(lambda x: x + 1, 1.0)
  #   graph = ops.get_default_graph()
  #   meta_graph = saver_lib.export_meta_graph(
  #       graph=graph, collection_list=graph.get_all_collection_keys())
  #   graph_copy = ops.Graph()
  #   with graph_copy.as_default():
  #     _ = saver_lib.import_meta_graph(meta_graph, import_scope="imported")
  #     restored_cs = ops.get_collection(critical_section_ops.CRITICAL_SECTIONS)
  #     restored_exec = ops.get_collection(
  #         critical_section_ops.CRITICAL_SECTION_EXECUTIONS)
  #     self.assertEqual(1, len(restored_cs))
  #     self.assertEqual(1, len(restored_exec))
  #     self.assertEqual(restored_cs[0].name, "imported/%s" % cs.name)
  #     self.assertEqual(restored_exec[0].op.name, "imported/%s" % r.op.name)

  # def testToProto(self):
  #   cs = critical_section_ops.CriticalSection(shared_name="cs")
  #   proto = cs.to_proto()
  #   self.assertEqual(proto.critical_section_name, cs._handle.name)
  #   cs_copy = critical_section_ops.CriticalSection.from_proto(proto)
  #   self.assertEqual(cs_copy._handle, cs._handle)


if __name__ == "__main__":
  test.main()
