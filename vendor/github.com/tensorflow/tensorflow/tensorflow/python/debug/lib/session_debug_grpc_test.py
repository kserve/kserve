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
"""Tests for debugger functionalities in tf.Session with grpc:// URLs.

This test file focuses on the grpc:// debugging of local (non-distributed)
tf.Sessions.
"""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os
import shutil

from six.moves import xrange  # pylint: disable=redefined-builtin

from tensorflow.core.protobuf import config_pb2
from tensorflow.python.client import session
from tensorflow.python.debug.lib import debug_data
from tensorflow.python.debug.lib import debug_utils
from tensorflow.python.debug.lib import grpc_debug_test_server
from tensorflow.python.debug.lib import session_debug_testlib
from tensorflow.python.debug.wrappers import framework
from tensorflow.python.debug.wrappers import grpc_wrapper
from tensorflow.python.debug.wrappers import hooks
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import ops
from tensorflow.python.framework import test_util
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import googletest
from tensorflow.python.training import monitored_session


class GrpcDebugServerTest(test_util.TensorFlowTestCase):

  def testRepeatedRunServerRaisesException(self):
    (_, _, _, server_thread,
     server) = grpc_debug_test_server.start_server_on_separate_thread(
         poll_server=True)
    # The server is started asynchronously. It needs to be polled till its state
    # has become started.

    with self.assertRaisesRegexp(
        ValueError, "Server has already started running"):
      server.run_server()

    server.stop_server().wait()
    server_thread.join()

  def testRepeatedStopServerRaisesException(self):
    (_, _, _, server_thread,
     server) = grpc_debug_test_server.start_server_on_separate_thread(
         poll_server=True)
    server.stop_server().wait()
    server_thread.join()

    with self.assertRaisesRegexp(ValueError, "Server has already stopped"):
      server.stop_server().wait()

  def testRunServerAfterStopRaisesException(self):
    (_, _, _, server_thread,
     server) = grpc_debug_test_server.start_server_on_separate_thread(
         poll_server=True)
    server.stop_server().wait()
    server_thread.join()

    with self.assertRaisesRegexp(ValueError, "Server has already stopped"):
      server.run_server()

  def testStartServerWithoutBlocking(self):
    (_, _, _, server_thread,
     server) = grpc_debug_test_server.start_server_on_separate_thread(
         poll_server=True, blocking=False)
    # The thread that starts the server shouldn't block, so we should be able to
    # join it before stopping the server.
    server_thread.join()
    server.stop_server().wait()


@test_util.run_v1_only("b/120545219")
class SessionDebugGrpcTest(session_debug_testlib.SessionDebugTestBase):

  @classmethod
  def setUpClass(cls):
    session_debug_testlib.SessionDebugTestBase.setUpClass()
    (cls._server_port, cls._debug_server_url, cls._server_dump_dir,
     cls._server_thread,
     cls._server) = grpc_debug_test_server.start_server_on_separate_thread()

  @classmethod
  def tearDownClass(cls):
    # Stop the test server and join the thread.
    cls._server.stop_server().wait()
    cls._server_thread.join()

    session_debug_testlib.SessionDebugTestBase.tearDownClass()

  def setUp(self):
    # Override the dump root as the test server's dump directory.
    self._dump_root = self._server_dump_dir

  def tearDown(self):
    if os.path.isdir(self._server_dump_dir):
      shutil.rmtree(self._server_dump_dir)
    session_debug_testlib.SessionDebugTestBase.tearDown(self)

  def _debug_urls(self, run_number=None):
    return ["grpc://localhost:%d" % self._server_port]

  def _debug_dump_dir(self, run_number=None):
    if run_number is None:
      return self._dump_root
    else:
      return os.path.join(self._dump_root, "run_%d" % run_number)

  def testConstructGrpcDebugWrapperSessionWithInvalidTypeRaisesException(self):
    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    with self.assertRaisesRegexp(
        TypeError, "Expected type str or list in grpc_debug_server_addresses"):
      grpc_wrapper.GrpcDebugWrapperSession(sess, 1337)

  def testConstructGrpcDebugWrapperSessionWithInvalidTypeRaisesException2(self):
    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    with self.assertRaisesRegexp(
        TypeError, "Expected type str in list grpc_debug_server_addresses"):
      grpc_wrapper.GrpcDebugWrapperSession(sess, ["localhost:1337", 1338])

  def testUseInvalidWatchFnTypeWithGrpcDebugWrapperSessionRaisesException(self):
    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    with self.assertRaises(TypeError):
      grpc_wrapper.GrpcDebugWrapperSession(
          sess, "localhost:%d" % self._server_port, watch_fn="foo")

  def testGrpcDebugWrapperSessionWithoutWatchFnWorks(self):
    u = variables.VariableV1(2.1, name="u")
    v = variables.VariableV1(20.0, name="v")
    w = math_ops.multiply(u, v, name="w")

    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    sess.run(u.initializer)
    sess.run(v.initializer)

    sess = grpc_wrapper.GrpcDebugWrapperSession(
        sess, "localhost:%d" % self._server_port)
    w_result = sess.run(w)
    self.assertAllClose(42.0, w_result)

    dump = debug_data.DebugDumpDir(self._dump_root)
    self.assertEqual(5, dump.size)
    self.assertAllClose([2.1], dump.get_tensors("u", 0, "DebugIdentity"))
    self.assertAllClose([2.1], dump.get_tensors("u/read", 0, "DebugIdentity"))
    self.assertAllClose([20.0], dump.get_tensors("v", 0, "DebugIdentity"))
    self.assertAllClose([20.0], dump.get_tensors("v/read", 0, "DebugIdentity"))
    self.assertAllClose([42.0], dump.get_tensors("w", 0, "DebugIdentity"))

  def testGrpcDebugWrapperSessionWithWatchFnWorks(self):
    def watch_fn(feeds, fetch_keys):
      del feeds, fetch_keys
      return ["DebugIdentity", "DebugNumericSummary"], r".*/read", None

    u = variables.VariableV1(2.1, name="u")
    v = variables.VariableV1(20.0, name="v")
    w = math_ops.multiply(u, v, name="w")

    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    sess.run(u.initializer)
    sess.run(v.initializer)

    sess = grpc_wrapper.GrpcDebugWrapperSession(
        sess, "localhost:%d" % self._server_port, watch_fn=watch_fn)
    w_result = sess.run(w)
    self.assertAllClose(42.0, w_result)

    dump = debug_data.DebugDumpDir(self._dump_root)
    self.assertEqual(4, dump.size)
    self.assertAllClose([2.1], dump.get_tensors("u/read", 0, "DebugIdentity"))
    self.assertEqual(
        14, len(dump.get_tensors("u/read", 0, "DebugNumericSummary")[0]))
    self.assertAllClose([20.0], dump.get_tensors("v/read", 0, "DebugIdentity"))
    self.assertEqual(
        14, len(dump.get_tensors("v/read", 0, "DebugNumericSummary")[0]))

  def testGrpcDebugHookWithStatelessWatchFnWorks(self):
    # Perform some set up. Specifically, construct a simple TensorFlow graph and
    # create a watch function for certain ops.
    def watch_fn(feeds, fetch_keys):
      del feeds, fetch_keys
      return framework.WatchOptions(
          debug_ops=["DebugIdentity", "DebugNumericSummary"],
          node_name_regex_whitelist=r".*/read",
          op_type_regex_whitelist=None,
          tolerate_debug_op_creation_failures=True)

    u = variables.VariableV1(2.1, name="u")
    v = variables.VariableV1(20.0, name="v")
    w = math_ops.multiply(u, v, name="w")

    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    sess.run(u.initializer)
    sess.run(v.initializer)

    # Create a hook. One could use this hook with say a tflearn Estimator.
    # However, we use a HookedSession in this test to avoid depending on the
    # internal implementation of Estimators.
    grpc_debug_hook = hooks.GrpcDebugHook(
        ["localhost:%d" % self._server_port], watch_fn=watch_fn)
    sess = monitored_session._HookedSession(sess, [grpc_debug_hook])

    # Run the hooked session. This should stream tensor data to the GRPC
    # endpoints.
    w_result = sess.run(w)

    # Verify that the hook monitored the correct tensors.
    self.assertAllClose(42.0, w_result)
    dump = debug_data.DebugDumpDir(self._dump_root)
    self.assertEqual(4, dump.size)
    self.assertAllClose([2.1], dump.get_tensors("u/read", 0, "DebugIdentity"))
    self.assertEqual(
        14, len(dump.get_tensors("u/read", 0, "DebugNumericSummary")[0]))
    self.assertAllClose([20.0], dump.get_tensors("v/read", 0, "DebugIdentity"))
    self.assertEqual(
        14, len(dump.get_tensors("v/read", 0, "DebugNumericSummary")[0]))

  def testTensorBoardDebugHookWorks(self):
    u = variables.VariableV1(2.1, name="u")
    v = variables.VariableV1(20.0, name="v")
    w = math_ops.multiply(u, v, name="w")

    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    sess.run(u.initializer)
    sess.run(v.initializer)

    grpc_debug_hook = hooks.TensorBoardDebugHook(
        ["localhost:%d" % self._server_port])
    sess = monitored_session._HookedSession(sess, [grpc_debug_hook])

    # Activate watch point on a tensor before calling sess.run().
    self._server.request_watch("u/read", 0, "DebugIdentity")
    self.assertAllClose(42.0, sess.run(w))

    # self.assertAllClose(42.0, sess.run(w))
    dump = debug_data.DebugDumpDir(self._dump_root)
    self.assertAllClose([2.1], dump.get_tensors("u/read", 0, "DebugIdentity"))

    # Check that the server has received the stack trace.
    self.assertTrue(self._server.query_op_traceback("u"))
    self.assertTrue(self._server.query_op_traceback("u/read"))
    self.assertTrue(self._server.query_op_traceback("v"))
    self.assertTrue(self._server.query_op_traceback("v/read"))
    self.assertTrue(self._server.query_op_traceback("w"))

    # Check that the server has received the python file content.
    # Query an arbitrary line to make sure that is the case.
    with open(__file__, "rt") as this_source_file:
      first_line = this_source_file.readline().strip()
      self.assertEqual(
          first_line, self._server.query_source_file_line(__file__, 1))

    self._server.clear_data()
    # Call sess.run() again, and verify that this time the traceback and source
    # code is not sent, because the graph version is not newer.
    self.assertAllClose(42.0, sess.run(w))
    with self.assertRaises(ValueError):
      self._server.query_op_traceback("delta_1")
    with self.assertRaises(ValueError):
      self._server.query_source_file_line(__file__, 1)

  def testTensorBoardDebugHookDisablingTracebackSourceCodeSendingWorks(self):
    u = variables.VariableV1(2.1, name="u")
    v = variables.VariableV1(20.0, name="v")
    w = math_ops.multiply(u, v, name="w")

    sess = session.Session(
        config=session_debug_testlib.no_rewrite_session_config())
    sess.run(variables.global_variables_initializer())

    grpc_debug_hook = hooks.TensorBoardDebugHook(
        ["localhost:%d" % self._server_port],
        send_traceback_and_source_code=False)
    sess = monitored_session._HookedSession(sess, [grpc_debug_hook])

    # Activate watch point on a tensor before calling sess.run().
    self._server.request_watch("u/read", 0, "DebugIdentity")
    self.assertAllClose(42.0, sess.run(w))

    # Check that the server has _not_ received any tracebacks, as a result of
    # the disabling above.
    with self.assertRaisesRegexp(
        ValueError, r"Op .*u/read.* does not exist"):
      self.assertTrue(self._server.query_op_traceback("u/read"))
    with self.assertRaisesRegexp(
        ValueError, r".* has not received any source file"):
      self._server.query_source_file_line(__file__, 1)

  def testConstructGrpcDebugHookWithOrWithouGrpcInUrlWorks(self):
    hooks.GrpcDebugHook(["grpc://foo:42424"])
    hooks.GrpcDebugHook(["foo:42424"])


class SessionDebugConcurrentTest(
    session_debug_testlib.DebugConcurrentRunCallsTest):

  @classmethod
  def setUpClass(cls):
    session_debug_testlib.SessionDebugTestBase.setUpClass()
    (cls._server_port, cls._debug_server_url, cls._server_dump_dir,
     cls._server_thread,
     cls._server) = grpc_debug_test_server.start_server_on_separate_thread()

  @classmethod
  def tearDownClass(cls):
    # Stop the test server and join the thread.
    cls._server.stop_server().wait()
    cls._server_thread.join()
    session_debug_testlib.SessionDebugTestBase.tearDownClass()

  def setUp(self):
    self._num_concurrent_runs = 3
    self._dump_roots = []
    for i in range(self._num_concurrent_runs):
      self._dump_roots.append(
          os.path.join(self._server_dump_dir, "thread%d" % i))

  def tearDown(self):
    ops.reset_default_graph()
    if os.path.isdir(self._server_dump_dir):
      shutil.rmtree(self._server_dump_dir)

  def _get_concurrent_debug_urls(self):
    urls = []
    for i in range(self._num_concurrent_runs):
      urls.append(self._debug_server_url + "/thread%d" % i)
    return urls


@test_util.run_v1_only("b/120545219")
class SessionDebugGrpcGatingTest(test_util.TensorFlowTestCase):
  """Test server gating of debug ops."""

  @classmethod
  def setUpClass(cls):
    (cls._server_port_1, cls._debug_server_url_1, _, cls._server_thread_1,
     cls._server_1) = grpc_debug_test_server.start_server_on_separate_thread(
         dump_to_filesystem=False)
    (cls._server_port_2, cls._debug_server_url_2, _, cls._server_thread_2,
     cls._server_2) = grpc_debug_test_server.start_server_on_separate_thread(
         dump_to_filesystem=False)
    cls._servers_and_threads = [(cls._server_1, cls._server_thread_1),
                                (cls._server_2, cls._server_thread_2)]

  @classmethod
  def tearDownClass(cls):
    for server, thread in cls._servers_and_threads:
      server.stop_server().wait()
      thread.join()

  def tearDown(self):
    ops.reset_default_graph()
    self._server_1.clear_data()
    self._server_2.clear_data()

  def testToggleEnableTwoDebugWatchesNoCrosstalkBetweenDebugNodes(self):
    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v_1 = variables.VariableV1(50.0, name="v_1")
      v_2 = variables.VariableV1(-50.0, name="v_1")
      delta_1 = constant_op.constant(5.0, name="delta_1")
      delta_2 = constant_op.constant(-5.0, name="delta_2")
      inc_v_1 = state_ops.assign_add(v_1, delta_1, name="inc_v_1")
      inc_v_2 = state_ops.assign_add(v_2, delta_2, name="inc_v_2")

      sess.run([v_1.initializer, v_2.initializer])

      run_metadata = config_pb2.RunMetadata()
      run_options = config_pb2.RunOptions(output_partition_graphs=True)
      debug_utils.watch_graph(
          run_options,
          sess.graph,
          debug_ops=["DebugIdentity(gated_grpc=true)",
                     "DebugNumericSummary(gated_grpc=true)"],
          debug_urls=[self._debug_server_url_1])

      for i in xrange(4):
        self._server_1.clear_data()

        if i % 2 == 0:
          self._server_1.request_watch("delta_1", 0, "DebugIdentity")
          self._server_1.request_watch("delta_2", 0, "DebugIdentity")
          self._server_1.request_unwatch("delta_1", 0, "DebugNumericSummary")
          self._server_1.request_unwatch("delta_2", 0, "DebugNumericSummary")
        else:
          self._server_1.request_unwatch("delta_1", 0, "DebugIdentity")
          self._server_1.request_unwatch("delta_2", 0, "DebugIdentity")
          self._server_1.request_watch("delta_1", 0, "DebugNumericSummary")
          self._server_1.request_watch("delta_2", 0, "DebugNumericSummary")

        sess.run([inc_v_1, inc_v_2],
                 options=run_options, run_metadata=run_metadata)

        # Watched debug tensors are:
        #   Run 0: delta_[1,2]:0:DebugIdentity
        #   Run 1: delta_[1,2]:0:DebugNumericSummary
        #   Run 2: delta_[1,2]:0:DebugIdentity
        #   Run 3: delta_[1,2]:0:DebugNumericSummary
        self.assertEqual(2, len(self._server_1.debug_tensor_values))
        if i % 2 == 0:
          self.assertAllClose(
              [5.0],
              self._server_1.debug_tensor_values["delta_1:0:DebugIdentity"])
          self.assertAllClose(
              [-5.0],
              self._server_1.debug_tensor_values["delta_2:0:DebugIdentity"])
        else:
          self.assertAllClose(
              [[1.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 5.0, 5.0, 5.0,
                0.0, 1.0, 0.0]],
              self._server_1.debug_tensor_values[
                  "delta_1:0:DebugNumericSummary"])
          self.assertAllClose(
              [[1.0, 1.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, -5.0, -5.0, -5.0,
                0.0, 1.0, 0.0]],
              self._server_1.debug_tensor_values[
                  "delta_2:0:DebugNumericSummary"])

  def testToggleWatchesOnCoreMetadata(self):
    (_, debug_server_url, _, server_thread,
     server) = grpc_debug_test_server.start_server_on_separate_thread(
         dump_to_filesystem=False,
         toggle_watch_on_core_metadata=[("toggled_1", 0, "DebugIdentity"),
                                        ("toggled_2", 0, "DebugIdentity")])
    self._servers_and_threads.append((server, server_thread))

    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v_1 = variables.VariableV1(50.0, name="v_1")
      v_2 = variables.VariableV1(-50.0, name="v_1")
      # These two nodes have names that match those in the
      # toggle_watch_on_core_metadata argument used when calling
      # start_server_on_separate_thread().
      toggled_1 = constant_op.constant(5.0, name="toggled_1")
      toggled_2 = constant_op.constant(-5.0, name="toggled_2")
      inc_v_1 = state_ops.assign_add(v_1, toggled_1, name="inc_v_1")
      inc_v_2 = state_ops.assign_add(v_2, toggled_2, name="inc_v_2")

      sess.run([v_1.initializer, v_2.initializer])

      run_metadata = config_pb2.RunMetadata()
      run_options = config_pb2.RunOptions(output_partition_graphs=True)
      debug_utils.watch_graph(
          run_options,
          sess.graph,
          debug_ops=["DebugIdentity(gated_grpc=true)"],
          debug_urls=[debug_server_url])

      for i in xrange(4):
        server.clear_data()

        sess.run([inc_v_1, inc_v_2],
                 options=run_options, run_metadata=run_metadata)

        if i % 2 == 0:
          self.assertEqual(2, len(server.debug_tensor_values))
          self.assertAllClose(
              [5.0],
              server.debug_tensor_values["toggled_1:0:DebugIdentity"])
          self.assertAllClose(
              [-5.0],
              server.debug_tensor_values["toggled_2:0:DebugIdentity"])
        else:
          self.assertEqual(0, len(server.debug_tensor_values))

  def testToggleEnableTwoDebugWatchesNoCrosstalkBetweenServers(self):
    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v = variables.VariableV1(50.0, name="v")
      delta = constant_op.constant(5.0, name="delta")
      inc_v = state_ops.assign_add(v, delta, name="inc_v")

      sess.run(v.initializer)

      run_metadata = config_pb2.RunMetadata()
      run_options = config_pb2.RunOptions(output_partition_graphs=True)
      debug_utils.watch_graph(
          run_options,
          sess.graph,
          debug_ops=["DebugIdentity(gated_grpc=true)"],
          debug_urls=[self._debug_server_url_1, self._debug_server_url_2])

      for i in xrange(4):
        self._server_1.clear_data()
        self._server_2.clear_data()

        if i % 2 == 0:
          self._server_1.request_watch("delta", 0, "DebugIdentity")
          self._server_2.request_watch("v", 0, "DebugIdentity")
        else:
          self._server_1.request_unwatch("delta", 0, "DebugIdentity")
          self._server_2.request_unwatch("v", 0, "DebugIdentity")

        sess.run(inc_v, options=run_options, run_metadata=run_metadata)

        if i % 2 == 0:
          self.assertEqual(1, len(self._server_1.debug_tensor_values))
          self.assertEqual(1, len(self._server_2.debug_tensor_values))
          self.assertAllClose(
              [5.0],
              self._server_1.debug_tensor_values["delta:0:DebugIdentity"])
          self.assertAllClose(
              [50 + 5.0 * i],
              self._server_2.debug_tensor_values["v:0:DebugIdentity"])
        else:
          self.assertEqual(0, len(self._server_1.debug_tensor_values))
          self.assertEqual(0, len(self._server_2.debug_tensor_values))

  def testToggleBreakpointsWorks(self):
    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v_1 = variables.VariableV1(50.0, name="v_1")
      v_2 = variables.VariableV1(-50.0, name="v_2")
      delta_1 = constant_op.constant(5.0, name="delta_1")
      delta_2 = constant_op.constant(-5.0, name="delta_2")
      inc_v_1 = state_ops.assign_add(v_1, delta_1, name="inc_v_1")
      inc_v_2 = state_ops.assign_add(v_2, delta_2, name="inc_v_2")

      sess.run([v_1.initializer, v_2.initializer])

      run_metadata = config_pb2.RunMetadata()
      run_options = config_pb2.RunOptions(output_partition_graphs=True)
      debug_utils.watch_graph(
          run_options,
          sess.graph,
          debug_ops=["DebugIdentity(gated_grpc=true)"],
          debug_urls=[self._debug_server_url_1])

      for i in xrange(4):
        self._server_1.clear_data()

        if i in (0, 2):
          # Enable breakpoint at delta_[1,2]:0:DebugIdentity in runs 0 and 2.
          self._server_1.request_watch(
              "delta_1", 0, "DebugIdentity", breakpoint=True)
          self._server_1.request_watch(
              "delta_2", 0, "DebugIdentity", breakpoint=True)
        else:
          # Disable the breakpoint in runs 1 and 3.
          self._server_1.request_unwatch("delta_1", 0, "DebugIdentity")
          self._server_1.request_unwatch("delta_2", 0, "DebugIdentity")

        output = sess.run([inc_v_1, inc_v_2],
                          options=run_options, run_metadata=run_metadata)
        self.assertAllClose([50.0 + 5.0 * (i + 1), -50 - 5.0 * (i + 1)], output)

        if i in (0, 2):
          # During runs 0 and 2, the server should have received the published
          # debug tensor delta:0:DebugIdentity. The breakpoint should have been
          # unblocked by EventReply reponses from the server.
          self.assertAllClose(
              [5.0],
              self._server_1.debug_tensor_values["delta_1:0:DebugIdentity"])
          self.assertAllClose(
              [-5.0],
              self._server_1.debug_tensor_values["delta_2:0:DebugIdentity"])
          # After the runs, the server should have properly registered the
          # breakpoints due to the request_unwatch calls.
          self.assertSetEqual({("delta_1", 0, "DebugIdentity"),
                               ("delta_2", 0, "DebugIdentity")},
                              self._server_1.breakpoints)
        else:
          # After the end of runs 1 and 3, the server has received the requests
          # to disable the breakpoint at delta:0:DebugIdentity.
          self.assertSetEqual(set(), self._server_1.breakpoints)

  def testTensorBoardDebuggerWrapperToggleBreakpointsWorks(self):
    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v_1 = variables.VariableV1(50.0, name="v_1")
      v_2 = variables.VariableV1(-50.0, name="v_2")
      delta_1 = constant_op.constant(5.0, name="delta_1")
      delta_2 = constant_op.constant(-5.0, name="delta_2")
      inc_v_1 = state_ops.assign_add(v_1, delta_1, name="inc_v_1")
      inc_v_2 = state_ops.assign_add(v_2, delta_2, name="inc_v_2")

      sess.run([v_1.initializer, v_2.initializer])

      # The TensorBoardDebugWrapperSession should add a DebugIdentity debug op
      # with attribute gated_grpc=True for every tensor in the graph.
      sess = grpc_wrapper.TensorBoardDebugWrapperSession(
          sess, self._debug_server_url_1)

      for i in xrange(4):
        self._server_1.clear_data()

        if i in (0, 2):
          # Enable breakpoint at delta_[1,2]:0:DebugIdentity in runs 0 and 2.
          self._server_1.request_watch(
              "delta_1", 0, "DebugIdentity", breakpoint=True)
          self._server_1.request_watch(
              "delta_2", 0, "DebugIdentity", breakpoint=True)
        else:
          # Disable the breakpoint in runs 1 and 3.
          self._server_1.request_unwatch("delta_1", 0, "DebugIdentity")
          self._server_1.request_unwatch("delta_2", 0, "DebugIdentity")

        output = sess.run([inc_v_1, inc_v_2])
        self.assertAllClose([50.0 + 5.0 * (i + 1), -50 - 5.0 * (i + 1)], output)

        if i in (0, 2):
          # During runs 0 and 2, the server should have received the published
          # debug tensor delta:0:DebugIdentity. The breakpoint should have been
          # unblocked by EventReply reponses from the server.
          self.assertAllClose(
              [5.0],
              self._server_1.debug_tensor_values["delta_1:0:DebugIdentity"])
          self.assertAllClose(
              [-5.0],
              self._server_1.debug_tensor_values["delta_2:0:DebugIdentity"])
          # After the runs, the server should have properly registered the
          # breakpoints.
        else:
          # After the end of runs 1 and 3, the server has received the requests
          # to disable the breakpoint at delta:0:DebugIdentity.
          self.assertSetEqual(set(), self._server_1.breakpoints)

        if i == 0:
          # Check that the server has received the stack trace.
          self.assertTrue(self._server_1.query_op_traceback("delta_1"))
          self.assertTrue(self._server_1.query_op_traceback("delta_2"))
          self.assertTrue(self._server_1.query_op_traceback("inc_v_1"))
          self.assertTrue(self._server_1.query_op_traceback("inc_v_2"))
          # Check that the server has received the python file content.
          # Query an arbitrary line to make sure that is the case.
          with open(__file__, "rt") as this_source_file:
            first_line = this_source_file.readline().strip()
          self.assertEqual(
              first_line, self._server_1.query_source_file_line(__file__, 1))
        else:
          # In later Session.run() calls, the traceback shouldn't have been sent
          # because it is already sent in the 1st call. So calling
          # query_op_traceback() should lead to an exception, because the test
          # debug server clears the data at the beginning of every iteration.
          with self.assertRaises(ValueError):
            self._server_1.query_op_traceback("delta_1")
          with self.assertRaises(ValueError):
            self._server_1.query_source_file_line(__file__, 1)

  def testTensorBoardDebuggerWrapperDisablingTracebackSourceSendingWorks(self):
    with session.Session(
        config=session_debug_testlib.no_rewrite_session_config()) as sess:
      v_1 = variables.VariableV1(50.0, name="v_1")
      v_2 = variables.VariableV1(-50.0, name="v_2")
      delta_1 = constant_op.constant(5.0, name="delta_1")
      delta_2 = constant_op.constant(-5.0, name="delta_2")
      inc_v_1 = state_ops.assign_add(v_1, delta_1, name="inc_v_1")
      inc_v_2 = state_ops.assign_add(v_2, delta_2, name="inc_v_2")

      sess.run(variables.global_variables_initializer())

      # Disable the sending of traceback and source code.
      sess = grpc_wrapper.TensorBoardDebugWrapperSession(
          sess, self._debug_server_url_1, send_traceback_and_source_code=False)

      for i in xrange(4):
        self._server_1.clear_data()

        if i == 0:
          self._server_1.request_watch(
              "delta_1", 0, "DebugIdentity", breakpoint=True)

        output = sess.run([inc_v_1, inc_v_2])
        self.assertAllClose([50.0 + 5.0 * (i + 1), -50 - 5.0 * (i + 1)], output)

        # No op traceback or source code should have been received by the debug
        # server due to the disabling above.
        with self.assertRaisesRegexp(
            ValueError, r"Op .*delta_1.* does not exist"):
          self.assertTrue(self._server_1.query_op_traceback("delta_1"))
        with self.assertRaisesRegexp(
            ValueError, r".* has not received any source file"):
          self._server_1.query_source_file_line(__file__, 1)

  def testGetGrpcDebugWatchesReturnsCorrectAnswer(self):
    with session.Session() as sess:
      v = variables.VariableV1(50.0, name="v")
      delta = constant_op.constant(5.0, name="delta")
      inc_v = state_ops.assign_add(v, delta, name="inc_v")

      sess.run(v.initializer)

      # Before any debugged runs, the server should be aware of no debug
      # watches.
      self.assertEqual([], self._server_1.gated_grpc_debug_watches())

      run_metadata = config_pb2.RunMetadata()
      run_options = config_pb2.RunOptions(output_partition_graphs=True)
      debug_utils.add_debug_tensor_watch(
          run_options, "delta", output_slot=0,
          debug_ops=["DebugNumericSummary(gated_grpc=true)"],
          debug_urls=[self._debug_server_url_1])
      debug_utils.add_debug_tensor_watch(
          run_options, "v", output_slot=0,
          debug_ops=["DebugIdentity"],
          debug_urls=[self._debug_server_url_1])
      sess.run(inc_v, options=run_options, run_metadata=run_metadata)

      # After the first run, the server should have noted the debug watches
      # for which gated_grpc == True, but not the ones with gated_grpc == False.
      self.assertEqual(1, len(self._server_1.gated_grpc_debug_watches()))
      debug_watch = self._server_1.gated_grpc_debug_watches()[0]
      self.assertEqual("delta", debug_watch.node_name)
      self.assertEqual(0, debug_watch.output_slot)
      self.assertEqual("DebugNumericSummary", debug_watch.debug_op)


@test_util.run_v1_only("b/120545219")
class DelayedDebugServerTest(test_util.TensorFlowTestCase):

  def testDebuggedSessionRunWorksWithDelayedDebugServerStartup(self):
    """Test debugged Session.run() tolerates delayed debug server startup."""
    ops.reset_default_graph()

    # Start a debug server asynchronously, with a certain amount of delay.
    (debug_server_port, _, _, server_thread,
     debug_server) = grpc_debug_test_server.start_server_on_separate_thread(
         server_start_delay_sec=2.0, dump_to_filesystem=False)

    with self.cached_session() as sess:
      a_init = constant_op.constant(42.0, name="a_init")
      a = variables.VariableV1(a_init, name="a")

      def watch_fn(fetches, feeds):
        del fetches, feeds
        return framework.WatchOptions(debug_ops=["DebugIdentity"])

      sess = grpc_wrapper.GrpcDebugWrapperSession(
          sess, "localhost:%d" % debug_server_port, watch_fn=watch_fn)
      sess.run(a.initializer)
      self.assertAllClose(
          [42.0], debug_server.debug_tensor_values["a_init:0:DebugIdentity"])

    debug_server.stop_server().wait()
    server_thread.join()


if __name__ == "__main__":
  googletest.main()
