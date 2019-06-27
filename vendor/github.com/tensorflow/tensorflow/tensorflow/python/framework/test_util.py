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

# pylint: disable=invalid-name
"""Test utils for tensorflow."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections
from collections import OrderedDict
import contextlib
import gc
import itertools
import math
import os
import random
import re
import tempfile
import threading
import unittest

import numpy as np
import six

_portpicker_import_error = None
try:
  import portpicker  # pylint: disable=g-import-not-at-top
except ImportError as _error:
  _portpicker_import_error = _error
  portpicker = None

# pylint: disable=g-import-not-at-top
from google.protobuf import descriptor_pool
from google.protobuf import text_format

from tensorflow.core.framework import graph_pb2
from tensorflow.core.protobuf import config_pb2
from tensorflow.core.protobuf import rewriter_config_pb2
from tensorflow.python import pywrap_tensorflow
from tensorflow.python import tf2
from tensorflow.python.client import device_lib
from tensorflow.python.client import session
from tensorflow.python.eager import context
from tensorflow.python.eager import def_function
from tensorflow.python.eager import tape
from tensorflow.python.framework import device as pydev
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import errors_impl
from tensorflow.python.framework import importer
from tensorflow.python.framework import ops
from tensorflow.python.framework import random_seed
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import versions
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_util
from tensorflow.python.ops import script_ops
from tensorflow.python.ops import variables
from tensorflow.python.platform import googletest
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.training import server_lib
from tensorflow.python.util import compat
from tensorflow.python.util import deprecation
from tensorflow.python.util import memory
from tensorflow.python.util import nest
from tensorflow.python.util import tf_decorator
from tensorflow.python.util import tf_inspect
from tensorflow.python.util.protobuf import compare
from tensorflow.python.util.tf_export import tf_export


@tf_export("test.gpu_device_name")
def gpu_device_name():
  """Returns the name of a GPU device if available or the empty string."""
  for x in device_lib.list_local_devices():
    if x.device_type == "GPU" or x.device_type == "SYCL":
      return compat.as_str(x.name)
  return ""


def assert_ops_in_graph(expected_ops, graph):
  """Assert all expected operations are found.

  Args:
    expected_ops: `dict<string, string>` of op name to op type.
    graph: Graph to check.
  Returns:
    `dict<string, node>` of node name to node.

  Raises:
    ValueError: If the expected ops are not present in the graph.
  """
  actual_ops = {}
  gd = graph.as_graph_def()
  for node in gd.node:
    if node.name in expected_ops:
      if expected_ops[node.name] != node.op:
        raise ValueError("Expected op for node %s is different. %s vs %s" %
                         (node.name, expected_ops[node.name], node.op))
      actual_ops[node.name] = node
  if set(expected_ops.keys()) != set(actual_ops.keys()):
    raise ValueError("Not all expected ops are present. Expected %s, found %s" %
                     (expected_ops.keys(), actual_ops.keys()))
  return actual_ops


@tf_export("test.assert_equal_graph_def", v1=[])
def assert_equal_graph_def_v2(actual, expected):
  """Asserts that two `GraphDef`s are (mostly) the same.

  Compares two `GraphDef` protos for equality, ignoring versions and ordering of
  nodes, attrs, and control inputs.  Node names are used to match up nodes
  between the graphs, so the naming of nodes must be consistent. This function
  ignores randomized attribute values that may appear in V2 checkpoints.

  Args:
    actual: The `GraphDef` we have.
    expected: The `GraphDef` we expected.

  Raises:
    AssertionError: If the `GraphDef`s do not match.
    TypeError: If either argument is not a `GraphDef`.
  """
  assert_equal_graph_def(actual, expected, checkpoint_v2=True)


@tf_export(v1=["test.assert_equal_graph_def"])
def assert_equal_graph_def_v1(actual, expected, checkpoint_v2=False):
  """Asserts that two `GraphDef`s are (mostly) the same.

  Compares two `GraphDef` protos for equality, ignoring versions and ordering of
  nodes, attrs, and control inputs.  Node names are used to match up nodes
  between the graphs, so the naming of nodes must be consistent.

  Args:
    actual: The `GraphDef` we have.
    expected: The `GraphDef` we expected.
    checkpoint_v2: boolean determining whether to ignore randomized attribute
        values that appear in V2 checkpoints.

  Raises:
    AssertionError: If the `GraphDef`s do not match.
    TypeError: If either argument is not a `GraphDef`.
  """
  assert_equal_graph_def(actual, expected, checkpoint_v2)


def assert_equal_graph_def(actual, expected, checkpoint_v2=False):
  if not isinstance(actual, graph_pb2.GraphDef):
    raise TypeError(
        "Expected tf.GraphDef for actual, got %s" % type(actual).__name__)
  if not isinstance(expected, graph_pb2.GraphDef):
    raise TypeError(
        "Expected tf.GraphDef for expected, got %s" % type(expected).__name__)

  if checkpoint_v2:
    _strip_checkpoint_v2_randomized(actual)
    _strip_checkpoint_v2_randomized(expected)

  diff = pywrap_tensorflow.EqualGraphDefWrapper(actual.SerializeToString(),
                                                expected.SerializeToString())
  if diff:
    raise AssertionError(compat.as_str(diff))


def assert_meta_graph_protos_equal(tester, a, b):
  """Compares MetaGraphDefs `a` and `b` in unit test class `tester`."""
  # Carefully check the collection_defs
  tester.assertEqual(set(a.collection_def), set(b.collection_def))
  collection_keys = a.collection_def.keys()
  for k in collection_keys:
    a_value = a.collection_def[k]
    b_value = b.collection_def[k]
    proto_type = ops.get_collection_proto_type(k)
    if proto_type:
      a_proto = proto_type()
      b_proto = proto_type()
      # Number of entries in the collections is the same
      tester.assertEqual(
          len(a_value.bytes_list.value), len(b_value.bytes_list.value))
      for (a_value_item, b_value_item) in zip(a_value.bytes_list.value,
                                              b_value.bytes_list.value):
        a_proto.ParseFromString(a_value_item)
        b_proto.ParseFromString(b_value_item)
        tester.assertProtoEquals(a_proto, b_proto)
    else:
      tester.assertEquals(a_value, b_value)
  # Compared the fields directly, remove their raw values from the
  # proto comparison below.
  a.ClearField("collection_def")
  b.ClearField("collection_def")

  # Check the graph_defs.
  assert_equal_graph_def(a.graph_def, b.graph_def, checkpoint_v2=True)
  # Check graph_def versions (ignored by assert_equal_graph_def).
  tester.assertProtoEquals(a.graph_def.versions, b.graph_def.versions)
  # Compared the fields directly, remove their raw values from the
  # proto comparison below.
  a.ClearField("graph_def")
  b.ClearField("graph_def")

  tester.assertProtoEquals(a, b)


# Matches attributes named via _SHARDED_SUFFIX in
# tensorflow/python/training/saver.py
_SHARDED_SAVE_OP_PATTERN = "_temp_[0-9a-z]{32}/part"


def _strip_checkpoint_v2_randomized(graph_def):
  for node in graph_def.node:
    delete_keys = []
    for attr_key in node.attr:
      attr_tensor_value = node.attr[attr_key].tensor
      if attr_tensor_value and len(attr_tensor_value.string_val) == 1:
        attr_tensor_string_value = attr_tensor_value.string_val[0]
        if (attr_tensor_string_value and
            re.match(_SHARDED_SAVE_OP_PATTERN, str(attr_tensor_string_value))):
          delete_keys.append(attr_key)
    for attr_key in delete_keys:
      del node.attr[attr_key]


def IsGoogleCudaEnabled():
  return pywrap_tensorflow.IsGoogleCudaEnabled()


def CudaSupportsHalfMatMulAndConv():
  return pywrap_tensorflow.CudaSupportsHalfMatMulAndConv()


def IsMklEnabled():
  return pywrap_tensorflow.IsMklEnabled()


def InstallStackTraceHandler():
  pywrap_tensorflow.InstallStacktraceHandler()


def NHWCToNCHW(input_tensor):
  """Converts the input from the NHWC format to NCHW.

  Args:
    input_tensor: a 4- or 5-D tensor, or an array representing shape

  Returns:
    converted tensor or shape array
  """
  # tensor dim -> new axis order
  new_axes = {4: [0, 3, 1, 2], 5: [0, 4, 1, 2, 3]}
  if isinstance(input_tensor, ops.Tensor):
    ndims = input_tensor.shape.ndims
    return array_ops.transpose(input_tensor, new_axes[ndims])
  else:
    ndims = len(input_tensor)
    return [input_tensor[a] for a in new_axes[ndims]]


def NHWCToNCHW_VECT_C(input_shape_or_tensor):
  """Transforms the input from the NHWC layout to NCHW_VECT_C layout.

  Note: Does not include quantization or type conversion steps, which should
  be applied afterwards.

  Args:
    input_shape_or_tensor: a 4- or 5-D tensor, or an array representing shape

  Returns:
    tensor or shape array transformed into NCHW_VECT_C

  Raises:
    ValueError: if last dimension of `input_shape_or_tensor` is not evenly
        divisible by 4.
  """
  permutations = {5: [0, 3, 1, 2, 4], 6: [0, 4, 1, 2, 3, 5]}
  is_tensor = isinstance(input_shape_or_tensor, ops.Tensor)
  temp_shape = (
      input_shape_or_tensor.shape.as_list()
      if is_tensor else input_shape_or_tensor)
  if temp_shape[-1] % 4 != 0:
    raise ValueError(
        "Last dimension of input must be evenly divisible by 4 to convert to "
        "NCHW_VECT_C.")
  temp_shape[-1] //= 4
  temp_shape.append(4)
  permutation = permutations[len(temp_shape)]
  if is_tensor:
    t = array_ops.reshape(input_shape_or_tensor, temp_shape)
    return array_ops.transpose(t, permutation)
  else:
    return [temp_shape[a] for a in permutation]


def NCHW_VECT_CToNHWC(input_shape_or_tensor):
  """Transforms the input from the NCHW_VECT_C layout to NHWC layout.

  Note: Does not include de-quantization or type conversion steps, which should
  be applied beforehand.

  Args:
    input_shape_or_tensor: a 5- or 6-D tensor, or an array representing shape

  Returns:
    tensor or shape array transformed into NHWC

  Raises:
    ValueError: if last dimension of `input_shape_or_tensor` is not 4.
  """
  permutations = {5: [0, 2, 3, 1, 4], 6: [0, 2, 3, 4, 1, 5]}
  is_tensor = isinstance(input_shape_or_tensor, ops.Tensor)
  input_shape = (
      input_shape_or_tensor.shape.as_list()
      if is_tensor else input_shape_or_tensor)
  if input_shape[-1] != 4:
    raise ValueError("Last dimension of NCHW_VECT_C must be 4.")
  permutation = permutations[len(input_shape)]
  nhwc_shape = [input_shape[a] for a in permutation[:-1]]
  nhwc_shape[-1] *= input_shape[-1]
  if is_tensor:
    t = array_ops.transpose(input_shape_or_tensor, permutation)
    return array_ops.reshape(t, nhwc_shape)
  else:
    return nhwc_shape


def NCHWToNHWC(input_tensor):
  """Converts the input from the NCHW format to NHWC.

  Args:
    input_tensor: a 4- or 5-D tensor, or an array representing shape

  Returns:
    converted tensor or shape array
  """
  # tensor dim -> new axis order
  new_axes = {4: [0, 2, 3, 1], 5: [0, 2, 3, 4, 1]}
  if isinstance(input_tensor, ops.Tensor):
    ndims = input_tensor.shape.ndims
    return array_ops.transpose(input_tensor, new_axes[ndims])
  else:
    ndims = len(input_tensor)
    return [input_tensor[a] for a in new_axes[ndims]]


def skip_if(condition):
  """Skips the decorated function if condition is or evaluates to True.

  Args:
    condition: Either an expression that can be used in "if not condition"
               statement, or a callable whose result should be a boolean.
  Returns:
    The wrapped function
  """

  def real_skip_if(fn):

    def wrapper(*args, **kwargs):
      if callable(condition):
        skip = condition()
      else:
        skip = condition
      if not skip:
        fn(*args, **kwargs)

    return wrapper

  return real_skip_if


def enable_c_shapes(fn):
  """No-op. TODO(b/74620627): Remove this."""
  return fn


def with_c_shapes(cls):
  """No-op. TODO(b/74620627): Remove this."""
  return cls


def enable_control_flow_v2(fn):
  """Decorator for enabling CondV2 and WhileV2 on a test.

  Note this enables using CondV2 and WhileV2 after running the test class's
  setup/teardown methods.

  In addition to this, callers must import the while_v2 module in order to set
  the _while_v2 module in control_flow_ops.

  Args:
    fn: the function to be wrapped

  Returns:
    The wrapped function
  """

  def wrapper(*args, **kwargs):
    enable_control_flow_v2_old = control_flow_util.ENABLE_CONTROL_FLOW_V2
    control_flow_util.ENABLE_CONTROL_FLOW_V2 = True
    try:
      fn(*args, **kwargs)
    finally:
      control_flow_util.ENABLE_CONTROL_FLOW_V2 = enable_control_flow_v2_old

  return wrapper


def with_control_flow_v2(cls):
  """Adds methods that call original methods with WhileV2 and CondV2 enabled.

  Note this enables CondV2 and WhileV2 in new methods after running the test
  class's setup method.

  In addition to this, callers must import the while_v2 module in order to set
  the _while_v2 module in control_flow_ops.

  If a test function has _disable_control_flow_v2 attr set to True (using the
  @disable_control_flow_v2 decorator), the v2 function is not generated for it.

  Example:

  @test_util.with_control_flow_v2
  class ControlFlowTest(test.TestCase):

    def testEnabledForV2(self):
      ...

    @test_util.disable_control_flow_v2("b/xyzabc")
    def testDisabledForV2(self):
      ...

  Generated class:
  class ControlFlowTest(test.TestCase):

    def testEnabledForV2(self):
      ...

    def testEnabledForV2WithControlFlowV2(self):
      // Enable V2 flags.
      testEnabledForV2(self)
      // Restore V2 flags.

    def testDisabledForV2(self):
      ...

  Args:
    cls: class to decorate

  Returns:
    cls with new test methods added
  """
  if control_flow_util.ENABLE_CONTROL_FLOW_V2:
    return cls

  for name, value in cls.__dict__.copy().items():
    if (callable(value) and
        name.startswith(unittest.TestLoader.testMethodPrefix) and
        not getattr(value, "_disable_control_flow_v2", False)):
      setattr(cls, name + "WithControlFlowV2", enable_control_flow_v2(value))
  return cls


def disable_control_flow_v2(unused_msg):
  """Decorator for a function in a with_control_flow_v2 enabled test class.

  Blocks the function from being run with v2 control flow ops.

  Args:
    unused_msg: Reason for disabling.

  Returns:
    The wrapped function with _disable_control_flow_v2 attr set to True.
  """
  def wrapper(func):
    func._disable_control_flow_v2 = True
    return func
  return wrapper


def assert_no_new_pyobjects_executing_eagerly(f):
  """Decorator for asserting that no new Python objects persist after a test.

  Runs the test multiple times executing eagerly, first as a warmup and then to
  let objects accumulate. The warmup helps ignore caches which do not grow as
  the test is run repeatedly.

  Useful for checking that there are no missing Py_DECREFs in the C exercised by
  a bit of Python.
  """

  def decorator(self, **kwargs):
    """Warms up, gets an object count, runs the test, checks for new objects."""
    with context.eager_mode():
      gc.disable()
      # Run the test 2 times as warmup, in an attempt to fill up caches, which
      # should not grow as the test is run repeatedly below.
      #
      # TODO(b/117156879): Running warmup twice is black magic; we have seen
      # tests that fail with 1 warmup run, and pass with 2, on various versions
      # of python2.7.x.
      for _ in range(2):
        f(self, **kwargs)
      gc.collect()
      previous_count = len(gc.get_objects())
      if ops.has_default_graph():
        collection_sizes_before = {
            collection: len(ops.get_collection(collection))
            for collection in ops.get_default_graph().collections
        }
      for _ in range(3):
        f(self, **kwargs)
      # Note that gc.get_objects misses anything that isn't subject to garbage
      # collection (C types). Collections are a common source of leaks, so we
      # test for collection sizes explicitly.
      if ops.has_default_graph():
        for collection_key in ops.get_default_graph().collections:
          collection = ops.get_collection(collection_key)
          size_before = collection_sizes_before.get(collection_key, 0)
          if len(collection) > size_before:
            raise AssertionError(
                ("Collection %s increased in size from "
                 "%d to %d (current items %s).") %
                (collection_key, size_before, len(collection), collection))
          # Make sure our collection checks don't show up as leaked memory by
          # removing references to temporary variables.
          del collection
          del collection_key
          del size_before
        del collection_sizes_before
      gc.collect()
      # There should be no new Python objects hanging around.
      new_count = len(gc.get_objects())
      # In some cases (specifacally on MacOS), new_count is somehow
      # smaller than previous_count.
      # Using plain assert because not all classes using this decorator
      # have assertLessEqual
      assert new_count <= previous_count, (
          "new_count(%d) is not less than or equal to previous_count(%d)" %
          (new_count, previous_count))
      gc.enable()

  return decorator


def assert_no_new_tensors(f):
  """Decorator for asserting that no new Tensors persist after a test.

  Mainly useful for checking that code using the Python C API has correctly
  manipulated reference counts.

  Clears the caches that it knows about, runs the garbage collector, then checks
  that there are no Tensor or Tensor-like objects still around. This includes
  Tensors to which something still has a reference (e.g. from missing
  Py_DECREFs) and uncollectable cycles (i.e. Python reference cycles where one
  of the objects has __del__ defined).

  Args:
    f: The test case to run.
  Returns:
    The decorated test case.
  """

  def decorator(self, **kwargs):
    """Finds existing Tensors, runs the test, checks for new Tensors."""

    def _is_tensorflow_object(obj):
      try:
        return isinstance(obj,
                          (ops.Tensor, variables.Variable,
                           tensor_shape.Dimension, tensor_shape.TensorShape))
      except ReferenceError:
        # If the object no longer exists, we don't care about it.
        return False

    tensors_before = set(
        id(obj) for obj in gc.get_objects() if _is_tensorflow_object(obj))
    outside_executed_eagerly = context.executing_eagerly()
    # Run the test in a new graph so that collections get cleared when it's
    # done, but inherit the graph key so optimizers behave.
    outside_graph_key = ops.get_default_graph()._graph_key
    with ops.Graph().as_default():
      ops.get_default_graph()._graph_key = outside_graph_key
      if outside_executed_eagerly:
        with context.eager_mode():
          f(self, **kwargs)
      else:
        f(self, **kwargs)
    # Make an effort to clear caches, which would otherwise look like leaked
    # Tensors.
    context.context()._clear_caches()  # pylint: disable=protected-access
    gc.collect()
    tensors_after = [
        obj for obj in gc.get_objects()
        if _is_tensorflow_object(obj) and id(obj) not in tensors_before
    ]
    if tensors_after:
      raise AssertionError(("%d Tensors not deallocated after test: %s" % (
          len(tensors_after),
          str(tensors_after),
      )))

  return decorator


def _find_reference_cycle(objects, idx):

  def get_ignore_reason(obj, blacklist):
    """Tests whether an object should be omitted from the dependency graph."""
    if len(blacklist) > 100:
      return "<depth limit>"
    if tf_inspect.isframe(obj):
      if "test_util.py" in tf_inspect.getframeinfo(obj)[0]:
        return "<test code>"
    for b in blacklist:
      if b is obj:
        return "<test code>"
    if obj is blacklist:
      return "<test code>"
    return None

  # Note: this function is meant to help with diagnostics. Its output is purely
  # a human readable representation, so you may freely modify it to suit your
  # needs.
  def describe(obj, blacklist, leaves_only=False):
    """Returns a custom human-readable summary of obj.

    Args:
      obj: the value to describe.
      blacklist: same as blacklist in get_ignore_reason.
      leaves_only: boolean flag used when calling describe recursively. Useful
        for summarizing collections.
    """
    if get_ignore_reason(obj, blacklist):
      return "{}{}".format(get_ignore_reason(obj, blacklist), type(obj))
    if tf_inspect.isframe(obj):
      return "frame: {}".format(tf_inspect.getframeinfo(obj))
    elif tf_inspect.ismodule(obj):
      return "module: {}".format(obj.__name__)
    else:
      if leaves_only:
        return "{}, {}".format(type(obj), id(obj))
      elif isinstance(obj, list):
        return "list({}): {}".format(
            id(obj), [describe(e, blacklist, leaves_only=True) for e in obj])
      elif isinstance(obj, tuple):
        return "tuple({}): {}".format(
            id(obj), [describe(e, blacklist, leaves_only=True) for e in obj])
      elif isinstance(obj, dict):
        return "dict({}): {} keys".format(id(obj), len(obj.keys()))
      elif tf_inspect.isfunction(obj):
        return "function({}) {}; globals ID: {}".format(
            id(obj), obj.__name__, id(obj.__globals__))
      else:
        return "{}, {}".format(type(obj), id(obj))

  def build_ref_graph(obj, graph, reprs, blacklist):
    """Builds a reference graph as <referrer> -> <list of refferents>.

    Args:
      obj: The object to start from. The graph will be built by recursively
        adding its referrers.
      graph: Dict holding the graph to be built. To avoid creating extra
        references, the graph holds object IDs rather than actual objects.
      reprs: Auxiliary structure that maps object IDs to their human-readable
        description.
      blacklist: List of objects to ignore.
    """
    referrers = gc.get_referrers(obj)
    blacklist = blacklist + (referrers,)

    obj_id = id(obj)
    for r in referrers:
      if get_ignore_reason(r, blacklist) is None:
        r_id = id(r)
        if r_id not in graph:
          graph[r_id] = []
        if obj_id not in graph[r_id]:
          graph[r_id].append(obj_id)
          build_ref_graph(r, graph, reprs, blacklist)
          reprs[r_id] = describe(r, blacklist)

  def find_cycle(el, graph, reprs, path):
    """Finds and prints a single cycle in the dependency graph."""
    if el not in graph:
      return
    for r in graph[el]:
      if r in path:
        logging.error("Reference cycle sample:")
        for p in path + (r,):
          logging.error(reprs.get(p, "unknown object " + str(p)))
        return True
      else:
        if find_cycle(r, graph, reprs, path + (r,)):
          return True
    return False

  obj = objects[idx]
  graph = {}  # referrer ID -> object ID
  reprs = {}  # object ID -> description
  build_ref_graph(obj, graph, reprs, (objects, graph, reprs, get_ignore_reason,
                                      describe, build_ref_graph, find_cycle))
  for k in graph:
    if find_cycle(k, graph, reprs, ()):
      return True
  return False


def assert_no_garbage_created(f):
  """Test method decorator to assert that no garbage has been created.

  Note that this decorator sets DEBUG_SAVEALL, which in some Python interpreters
  cannot be un-set (i.e. will disable garbage collection for any other unit
  tests in the same file/shard).

  Args:
    f: The function to decorate.
  Returns:
    The decorated function.
  """

  def decorator(self, **kwargs):
    """Sets DEBUG_SAVEALL, runs the test, and checks for new garbage."""
    # Force-load `distribution_strategy_context` to prevent GC at
    # test time when using eager. Remove once b/117329403 is resolved.
    tape.distribution_strategy_context.get_distribution_strategy()

    gc.disable()
    previous_debug_flags = gc.get_debug()
    gc.set_debug(gc.DEBUG_SAVEALL)
    gc.collect()
    previous_garbage = len(gc.garbage)
    f(self, **kwargs)
    gc.collect()
    new_garbage = len(gc.garbage)
    if new_garbage > previous_garbage:
      logging.error(
          "The decorated test created work for Python's garbage collector, "
          "likely due to a reference cycle. New objects in cycle(s):")
      for i, obj in enumerate(gc.garbage[previous_garbage:]):
        try:
          logging.error("Object %d of %d", i,
                        len(gc.garbage) - previous_garbage)

          def _safe_object_str(obj):
            return "<%s %d>" % (obj.__class__.__name__, id(obj))

          logging.error("  Object type: %s", _safe_object_str(obj))
          logging.error(
              "  Referrer types: %s", ", ".join(
                  [_safe_object_str(ref) for ref in gc.get_referrers(obj)]))
          logging.error(
              "  Referent types: %s", ", ".join(
                  [_safe_object_str(ref) for ref in gc.get_referents(obj)]))
          logging.error("  Object attribute names: %s", dir(obj))
          logging.error("  Object __str__:")
          logging.error(obj)
          logging.error("  Object __repr__:")
          logging.error(repr(obj))
        except Exception:  # pylint: disable=broad-except
          logging.error("(Exception while printing object)")

    # When garbage is created, this call can help identify reference cycles,
    # which are typically the cause of such garbage.
    if new_garbage > previous_garbage:
      for i in range(previous_garbage, new_garbage):
        if _find_reference_cycle(gc.garbage, i):
          break

    # This will fail if any garbage has been created, typically because of a
    # reference cycle.
    self.assertEqual(previous_garbage, new_garbage)
    # TODO(allenl): Figure out why this debug flag reset doesn't work. It would
    # be nice to be able to decorate arbitrary tests in a large test suite and
    # not hold on to every object in other tests.
    gc.set_debug(previous_debug_flags)
    gc.enable()

  return decorator


def _combine_named_parameters(**kwargs):
  """Generate combinations based on its keyword arguments.

  Two sets of returned combinations can be concatenated using +.  Their product
  can be computed using `times()`.

  Args:
    **kwargs: keyword arguments of form `option=[possibilities, ...]`
         or `option=the_only_possibility`.

  Returns:
    a list of dictionaries for each combination. Keys in the dictionaries are
    the keyword argument names.  Each key has one value - one of the
    corresponding keyword argument values.
  """
  if not kwargs:
    return [OrderedDict()]

  sort_by_key = lambda k: k[0][0]
  kwargs = OrderedDict(sorted(kwargs.items(), key=sort_by_key))
  first = list(kwargs.items())[0]

  rest = dict(list(kwargs.items())[1:])
  rest_combined = _combine_named_parameters(**rest)

  key = first[0]
  values = first[1]
  if not isinstance(values, list):
    values = [values]

  combinations = [
      OrderedDict(sorted(list(combined.items()) + [(key, v)], key=sort_by_key))
      for v in values
      for combined in rest_combined
  ]
  return combinations


def generate_combinations_with_testcase_name(**kwargs):
  """Generate combinations based on its keyword arguments using combine().

  This function calls combine() and appends a testcase name to the list of
  dictionaries returned. The 'testcase_name' key is a required for named
  parameterized tests.

  Args:
    **kwargs: keyword arguments of form `option=[possibilities, ...]`
         or `option=the_only_possibility`.

  Returns:
    a list of dictionaries for each combination. Keys in the dictionaries are
    the keyword argument names.  Each key has one value - one of the
    corresponding keyword argument values.
  """
  combinations = _combine_named_parameters(**kwargs)
  named_combinations = []
  for combination in combinations:
    assert isinstance(combination, OrderedDict)
    name = "".join([
        "_{}_{}".format("".join(filter(str.isalnum, key)), "".join(
            filter(str.isalnum, str(value))))
        for key, value in combination.items()
    ])
    named_combinations.append(
        OrderedDict(
            list(combination.items()) + [("testcase_name",
                                          "_test{}".format(name))]))

  return named_combinations


def run_all_in_graph_and_eager_modes(cls):
  """Execute all test methods in the given class with and without eager."""
  base_decorator = run_in_graph_and_eager_modes
  for name, value in cls.__dict__.copy().items():
    if (callable(value) and
        name.startswith(unittest.TestLoader.testMethodPrefix) and
        not (name.startswith("testSkipEager")
             or name.startswith("test_skip_eager"))):
      setattr(cls, name, base_decorator(value))
  return cls


def run_in_graph_and_eager_modes(func=None,
                                 config=None,
                                 use_gpu=True,
                                 reset_test=True,
                                 assert_no_eager_garbage=False):
  """Execute the decorated test with and without enabling eager execution.

  This function returns a decorator intended to be applied to test methods in
  a `tf.test.TestCase` class. Doing so will cause the contents of the test
  method to be executed twice - once normally, and once with eager execution
  enabled. This allows unittests to confirm the equivalence between eager
  and graph execution (see `tf.enable_eager_execution`).

  For example, consider the following unittest:

  ```python
  class MyTests(tf.test.TestCase):

    @run_in_graph_and_eager_modes
    def test_foo(self):
      x = tf.constant([1, 2])
      y = tf.constant([3, 4])
      z = tf.add(x, y)
      self.assertAllEqual([4, 6], self.evaluate(z))

  if __name__ == "__main__":
    tf.test.main()
  ```

  This test validates that `tf.add()` has the same behavior when computed with
  eager execution enabled as it does when constructing a TensorFlow graph and
  executing the `z` tensor in a session.


  Args:
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.
    config: An optional config_pb2.ConfigProto to use to configure the
      session when executing graphs.
    use_gpu: If True, attempt to run as many operations as possible on GPU.
    reset_test: If True, tearDown and SetUp the test case between the two
      executions of the test (once with and once without eager execution).
    assert_no_eager_garbage: If True, sets DEBUG_SAVEALL on the garbage
      collector and asserts that no extra garbage has been created when running
      the test with eager execution enabled. This will fail if there are
      reference cycles (e.g. a = []; a.append(a)). Off by default because some
      tests may create garbage for legitimate reasons (e.g. they define a class
      which inherits from `object`), and because DEBUG_SAVEALL is sticky in some
      Python interpreters (meaning that tests which rely on objects being
      collected elsewhere in the unit test file will not work). Additionally,
      checks that nothing still has a reference to Tensors that the test
      allocated.
  Returns:
    Returns a decorator that will run the decorated test method twice:
    once by constructing and executing a graph in a session and once with
    eager execution enabled.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      raise ValueError(
          "`run_in_graph_and_eager_modes` only supports test methods. "
          "Did you mean to use `run_all_in_graph_and_eager_modes`?")

    def decorated(self, *args, **kwargs):
      try:
        with context.graph_mode():
          with self.test_session(use_gpu=use_gpu, config=config):
            f(self, *args, **kwargs)
      except unittest.case.SkipTest:
        pass

      def run_eagerly(self, **kwargs):
        if not use_gpu:
          with ops.device("/device:CPU:0"):
            f(self, *args, **kwargs)
        else:
          f(self, *args, **kwargs)

      if assert_no_eager_garbage:
        ops.reset_default_graph()
        run_eagerly = assert_no_new_tensors(
            assert_no_garbage_created(run_eagerly))

      if reset_test:
        # This decorator runs the wrapped test twice.
        # Reset the test environment between runs.
        self.tearDown()
        self._tempdir = None
      # Create a new graph for the eagerly executed version of this test for
      # better isolation.
      graph_for_eager_test = ops.Graph()
      with graph_for_eager_test.as_default(), context.eager_mode():
        if reset_test:
          self.setUp()
        run_eagerly(self, **kwargs)
      ops.dismantle_graph(graph_for_eager_test)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


def py_func_if_in_function(f):

  def decorated(*args, **kwds):
    if not ops.get_default_graph()._building_function:
      return f(*args, **kwds)

    tensor_args, tensor_indices = zip(
        *[(x, i) for i, x in enumerate(args)
          if isinstance(x, (ops.Tensor, variables.Variable))])

    def inner_f(*inner_tensor_args):
      my_args = list(args)
      for i, n in zip(tensor_indices, inner_tensor_args):
        my_args[i] = n
      return f(*my_args, **kwds)

    return script_ops.py_func(inner_f, tensor_args, [])

  return tf_decorator.make_decorator(f, decorated)


def also_run_as_tf_function(f):
  """Runs the decorated test twice--once as is, once inside a tf.function.

  This allows you to run a test both in eager execution and inside a
  tf.function, exercising the two execution modes supported in tf 2.0. The test
  assertions are automatically done inside tf.py_funcs, and tf.function ensures
  that they run in the proper order and with the proper side effects.

  Currently variable creation is not supported in tests annotated with this
  decorator since it's tricky to ensure the variable doesn't get repeatedly
  created when retracing the tf.function.

  Args:
    f: the test method to be decorated

  Returns:
    The decorated test method, which will run both in eager and inside a
    tf.function.
  """

  def decorated(*args, **kwds):
    def bound_f():
      f(*args, **kwds)
    with context.eager_mode():
      # Running in eager mode
      bound_f()
      # Running as TF function
      def_function.function(bound_f)()

  return decorated


def run_deprecated_v1(func=None):
  """Execute the decorated test in graph mode.

  This function returns a decorator intended to be applied to tests that have
  not been updated to a style that is compatible with both TensorFlow 1.x and
  2.x. When this decorated is applied, the test body will be run in
  an environment where API calls construct graphs instead of executing eagerly.

  Args:
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.
  Returns:
    Returns a decorator that will run the decorated test method in graph mode.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      raise ValueError("`run_deprecated_v1` only supports test methods.")

    def decorated(self, *args, **kwargs):
      if tf2.enabled():
        with context.graph_mode():
          f(self, *args, **kwargs)
      else:
        f(self, *args, **kwargs)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


def run_v1_only(reason, func=None):
  """Execute the decorated test only if running in v1 mode.

  This function is intended to be applied to tests that exercise v1 only
  functionality. If the test is run in v2 mode it will simply be skipped.

  Args:
    reason: string giving a reason for limiting the test to v1 only.
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.

  Returns:
    Returns a decorator that will conditionally skip the decorated test method.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      setup = f.__dict__.get("setUp")
      if setup is not None:
        setattr(f, "setUp", decorator(setup))

      for name, value in f.__dict__.copy().items():
        if (callable(value) and
            name.startswith(unittest.TestLoader.testMethodPrefix)):
          setattr(f, name, decorator(value))

      return f

    def decorated(self, *args, **kwargs):
      if tf2.enabled():
        self.skipTest(reason)

      f(self, *args, **kwargs)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


def run_v2_only(func=None):
  """Execute the decorated test only if running in v2 mode.

  This function is intended to be applied to tests that exercise v2 only
  functionality. If the test is run in v1 mode it will simply be skipped.

  Args:
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.

  Returns:
    Returns a decorator that will conditionally skip the decorated test method.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      raise ValueError("`run_v2_only` only supports test methods.")

    def decorated(self, *args, **kwargs):
      if not tf2.enabled():
        self.skipTest("Test is only comptaible in v2")

      f(self, *args, **kwargs)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


def run_gpu_only(func=None):
  """Execute the decorated test only if a GPU is available.

  This function is intended to be applied to tests that require the precense
  of a GPU. If a GPU is absent, it will simply be skipped.

  Args:
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.

  Returns:
    Returns a decorator that will conditionally skip the decorated test method.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      raise ValueError("`run_gpu_only` only supports test methods.")

    def decorated(self, *args, **kwargs):
      if not is_gpu_available():
        self.skipTest("Test requires GPU")

      f(self, *args, **kwargs)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


def run_cuda_only(func=None):
  """Execute the decorated test only if a GPU is available.

  This function is intended to be applied to tests that require the precense
  of a CUDA GPU. If a CUDA GPU is absent, it will simply be skipped.

  Args:
    func: function to be annotated. If `func` is None, this method returns a
      decorator the can be applied to a function. If `func` is not None this
      returns the decorator applied to `func`.

  Returns:
    Returns a decorator that will conditionally skip the decorated test method.
  """

  def decorator(f):
    if tf_inspect.isclass(f):
      raise ValueError("`run_cuda_only` only supports test methods.")

    def decorated(self, *args, **kwargs):
      if not is_gpu_available(cuda_only=True):
        self.skipTest("Test requires CUDA GPU")

      f(self, *args, **kwargs)

    return decorated

  if func is not None:
    return decorator(func)

  return decorator


@tf_export("test.is_gpu_available")
def is_gpu_available(cuda_only=False, min_cuda_compute_capability=None):
  """Returns whether TensorFlow can access a GPU.

  Args:
    cuda_only: limit the search to CUDA gpus.
    min_cuda_compute_capability: a (major,minor) pair that indicates the minimum
      CUDA compute capability required, or None if no requirement.

  Returns:
    True iff a gpu device of the requested kind is available.
  """

  def compute_capability_from_device_desc(device_desc):
    # TODO(jingyue): The device description generator has to be in sync with
    # this file. Another option is to put compute capability in
    # DeviceAttributes, but I avoided that to keep DeviceAttributes
    # target-independent. Reconsider this option when we have more things like
    # this to keep in sync.
    # LINT.IfChange
    match = re.search(r"compute capability: (\d+)\.(\d+)", device_desc)
    # LINT.ThenChange(//tensorflow/core/\
    #                 common_runtime/gpu/gpu_device.cc)
    if not match:
      return 0, 0
    return int(match.group(1)), int(match.group(2))

  try:
    for local_device in device_lib.list_local_devices():
      if local_device.device_type == "GPU":
        if (min_cuda_compute_capability is None or
            compute_capability_from_device_desc(
                local_device.physical_device_desc) >=
            min_cuda_compute_capability):
          return True
      if local_device.device_type == "SYCL" and not cuda_only:
        return True
    return False
  except errors_impl.NotFoundError as e:
    if not all(x in str(e) for x in ["CUDA", "not find"]):
      raise e
    else:
      logging.error(str(e))
      return False


@contextlib.contextmanager
def device(use_gpu):
  """Uses gpu when requested and available."""
  if use_gpu and is_gpu_available():
    dev = "/device:GPU:0"
  else:
    dev = "/device:CPU:0"
  with ops.device(dev):
    yield


@contextlib.contextmanager
def use_gpu():
  """Uses gpu when requested and available."""
  with device(use_gpu=True):
    yield


@contextlib.contextmanager
def force_gpu():
  """Force the gpu to be used."""
  with ops.device("/device:GPU:0"):
    yield


@contextlib.contextmanager
def force_cpu():
  """Force the cpu to be used."""
  with ops.device("/device:CPU:0"):
    yield


class CapturedWrites(object):
  """A utility class to load the captured writes made to a stream."""

  def __init__(self, capture_location):
    self.capture_location = capture_location

  def contents(self):
    """Get the captured writes as a single string."""
    with open(self.capture_location) as tmp_file:
      output_data = "".join(tmp_file.readlines())
    return output_data


class FakeEagerSession(object):
  """Fake session so tests that conditionally use placeholders can use eager.

  There are a number of tests that conditionally use placeholders for shape
  inference. The pattern is demonstrated here:

  ```python
  with self.cached_session() as sess:
    if static_shape:
      y = math_ops.matmul(x, ...)
      feed_dict = {}
    else:
      x_ph = array_ops.placeholder(...)
      y = math_ops.matmul(x_ph, ...)
      feed_dict = {x_ph: x}
    val = sess.run(y, feed_dict=feed_dict)
  ```

  Since the feed_dict is empty when not using placeholders we should be able to
  call self.evaluate(), however this requires rewriting the test case.
  This class shold be considered a stop-gap solution to get tests running with
  eager with minimal changes to the actual test.
  """

  def __init__(self, test_case):
    self._test_case = test_case

  def run(self, fetches, *args, **kwargs):
    """Evalaute `fetches`.

    Fail if additional args are specified.

    Args:
      fetches: A Tensor or a nested list/tuple of Tensors.
      *args: Positional arguments
      **kwargs: Keyword arguments

    Raises:
      RuntimeError: If args or kwargs are specified.

    Returns:
      Tensors as numpy values.
    """
    feed_dict = kwargs.pop("feed_dict", {})
    if feed_dict:
      raise RuntimeError(
          "feed_dict is not supported when eager execution is enabled "
          "(in this case, sess.run(t) is shorthand for t.numpy()")

    if args or kwargs:
      raise RuntimeError(
          "Optional args are not supported when eager execution is enabled "
          "(in this case, sess.run(t) is shorthand for t.numpy()")

    return self._test_case.evaluate(fetches)


class ErrorLoggingSession(session.Session):
  """Wrapper around a Session that logs errors in run().
  """

  def run(self, *args, **kwargs):
    try:
      return super(ErrorLoggingSession, self).run(*args, **kwargs)
    except Exception as e:  # pylint: disable=broad-except
      # Note: disable the logging for OutOfRangeError, which makes the output
      # of tf.data tests hard to read, because OutOfRangeError is used as the
      # signal completion
      if not isinstance(e, errors.OutOfRangeError):
        logging.error(str(e))
      raise


@tf_export("test.TestCase")
class TensorFlowTestCase(googletest.TestCase):
  """Base class for tests that need to test TensorFlow.
  """

  def __init__(self, methodName="runTest"):  # pylint: disable=invalid-name
    super(TensorFlowTestCase, self).__init__(methodName)
    self._threads = []
    self._tempdir = None
    self._cached_session = None

  def setUp(self):
    self._ClearCachedSession()
    random.seed(random_seed.DEFAULT_GRAPH_SEED)
    np.random.seed(random_seed.DEFAULT_GRAPH_SEED)
    # Note: The following line is necessary because some test methods may error
    # out from within nested graph contexts (e.g., via assertRaises and
    # assertRaisesRegexp), which may leave ops._default_graph_stack non-empty
    # under certain versions of Python. That would cause
    # ops.reset_default_graph() to throw an exception if the stack were not
    # cleared first.
    ops._default_graph_stack.reset()  # pylint: disable=protected-access
    ops.reset_default_graph()
    random_seed.set_random_seed(random_seed.DEFAULT_GRAPH_SEED)

    # Avoiding calling setUp() for the poorly named test_session method.
    if self.id().endswith(".test_session"):
      self.skipTest("Not a test.")

  def tearDown(self):
    for thread in self._threads:
      thread.check_termination()

    self._ClearCachedSession()

  def _ClearCachedSession(self):
    if self._cached_session is not None:
      self._cached_session.close()
      self._cached_session = None

  def get_temp_dir(self):
    """Returns a unique temporary directory for the test to use.

    If you call this method multiple times during in a test, it will return the
    same folder. However, across different runs the directories will be
    different. This will ensure that across different runs tests will not be
    able to pollute each others environment.
    If you need multiple unique directories within a single test, you should
    use tempfile.mkdtemp as follows:
      tempfile.mkdtemp(dir=self.get_temp_dir()):

    Returns:
      string, the path to the unique temporary directory created for this test.
    """
    if not self._tempdir:
      self._tempdir = tempfile.mkdtemp(dir=googletest.GetTempDir())
    return self._tempdir

  @contextlib.contextmanager
  def captureWritesToStream(self, stream):
    """A context manager that captures the writes to a given stream.

    This context manager captures all writes to a given stream inside of a
    `CapturedWrites` object. When this context manager is created, it yields
    the `CapturedWrites` object. The captured contents can be accessed  by
    calling `.contents()` on the `CapturedWrites`.

    For this function to work, the stream must have a file descriptor that
    can be modified using `os.dup` and `os.dup2`, and the stream must support
    a `.flush()` method. The default python sys.stdout and sys.stderr are
    examples of this. Note that this does not work in Colab or Jupyter
    notebooks, because those use alternate stdout streams.

    Example:
    ```python
    class MyOperatorTest(test_util.TensorFlowTestCase):
      def testMyOperator(self):
        input = [1.0, 2.0, 3.0, 4.0, 5.0]
        with self.captureWritesToStream(sys.stdout) as captured:
          result = MyOperator(input).eval()
        self.assertStartsWith(captured.contents(), "This was printed.")
    ```

    Args:
      stream: The stream whose writes should be captured. This
        stream must have a file descriptor, support writing via using that
        file descriptor, and must have a `.flush()` method.

    Yields:
      A `CapturedWrites` object that contains all writes to the specified stream
      made during this context.
    """
    stream.flush()
    fd = stream.fileno()
    tmp_file_path = tempfile.mktemp(dir=self.get_temp_dir())
    tmp_file = open(tmp_file_path, "w")
    orig_fd = os.dup(fd)
    os.dup2(tmp_file.fileno(), fd)
    try:
      yield CapturedWrites(tmp_file_path)
    finally:
      tmp_file.close()
      os.dup2(orig_fd, fd)

  def _AssertProtoEquals(self, a, b, msg=None):
    """Asserts that a and b are the same proto.

    Uses ProtoEq() first, as it returns correct results
    for floating point attributes, and then use assertProtoEqual()
    in case of failure as it provides good error messages.

    Args:
      a: a proto.
      b: another proto.
      msg: Optional message to report on failure.
    """
    if not compare.ProtoEq(a, b):
      compare.assertProtoEqual(self, a, b, normalize_numbers=True, msg=msg)

  def assertProtoEquals(self, expected_message_maybe_ascii, message, msg=None):
    """Asserts that message is same as parsed expected_message_ascii.

    Creates another prototype of message, reads the ascii message into it and
    then compares them using self._AssertProtoEqual().

    Args:
      expected_message_maybe_ascii: proto message in original or ascii form.
      message: the message to validate.
      msg: Optional message to report on failure.
    """
    msg = msg if msg else ""
    if isinstance(expected_message_maybe_ascii, type(message)):
      expected_message = expected_message_maybe_ascii
      self._AssertProtoEquals(expected_message, message)
    elif isinstance(expected_message_maybe_ascii, str):
      expected_message = type(message)()
      text_format.Merge(
          expected_message_maybe_ascii,
          expected_message,
          descriptor_pool=descriptor_pool.Default())
      self._AssertProtoEquals(expected_message, message, msg=msg)
    else:
      assert False, ("Can't compare protos of type %s and %s. %s" %
                     (type(expected_message_maybe_ascii), type(message), msg))

  def assertProtoEqualsVersion(
      self,
      expected,
      actual,
      producer=versions.GRAPH_DEF_VERSION,
      min_consumer=versions.GRAPH_DEF_VERSION_MIN_CONSUMER,
      msg=None):
    expected = "versions { producer: %d min_consumer: %d };\n%s" % (
        producer, min_consumer, expected)
    self.assertProtoEquals(expected, actual, msg=msg)

  def assertStartsWith(self, actual, expected_start, msg=None):
    """Assert that actual.startswith(expected_start) is True.

    Args:
      actual: str
      expected_start: str
      msg: Optional message to report on failure.
    """
    if not actual.startswith(expected_start):
      fail_msg = "%r does not start with %r" % (actual, expected_start)
      fail_msg += " : %r" % (msg) if msg else ""
      self.fail(fail_msg)

  def _eval_tensor(self, tensor):
    if tensor is None:
      return None
    elif callable(tensor):
      return self._eval_helper(tensor())
    else:
      try:
        if sparse_tensor.is_sparse(tensor):
          return sparse_tensor.SparseTensorValue(tensor.indices, tensor.values,
                                                 tensor.dense_shape)
        return tensor.numpy()
      except AttributeError as e:
        six.raise_from(ValueError("Unsupported type %s." % type(tensor)), e)

  def _eval_helper(self, tensors):
    if tensors is None:
      return None
    return nest.map_structure(self._eval_tensor, tensors)

  def evaluate(self, tensors):
    """Evaluates tensors and returns numpy values.

    Args:
      tensors: A Tensor or a nested list/tuple of Tensors.

    Returns:
      tensors numpy values.
    """
    if context.executing_eagerly():
      return self._eval_helper(tensors)
    else:
      sess = ops.get_default_session()
      if sess is None:
        with self.test_session() as sess:
          return sess.run(tensors)
      else:
        return sess.run(tensors)

  # pylint: disable=g-doc-return-or-yield
  @contextlib.contextmanager
  def session(self, graph=None, config=None, use_gpu=False, force_gpu=False):
    """Returns a TensorFlow Session for use in executing tests.

    Note that this will set this session and the graph as global defaults.

    Use the `use_gpu` and `force_gpu` options to control where ops are run. If
    `force_gpu` is True, all ops are pinned to `/device:GPU:0`. Otherwise, if
    `use_gpu` is True, TensorFlow tries to run as many ops on the GPU as
    possible. If both `force_gpu and `use_gpu` are False, all ops are pinned to
    the CPU.

    Example:
    ```python
    class MyOperatorTest(test_util.TensorFlowTestCase):
      def testMyOperator(self):
        with self.session(use_gpu=True):
          valid_input = [1.0, 2.0, 3.0, 4.0, 5.0]
          result = MyOperator(valid_input).eval()
          self.assertEqual(result, [1.0, 2.0, 3.0, 5.0, 8.0]
          invalid_input = [-1.0, 2.0, 7.0]
          with self.assertRaisesOpError("negative input not supported"):
            MyOperator(invalid_input).eval()
    ```

    Args:
      graph: Optional graph to use during the returned session.
      config: An optional config_pb2.ConfigProto to use to configure the
        session.
      use_gpu: If True, attempt to run as many ops as possible on GPU.
      force_gpu: If True, pin all ops to `/device:GPU:0`.

    Yields:
      A Session object that should be used as a context manager to surround
      the graph building and execution code in a test case.
    """
    if context.executing_eagerly():
      yield None
    else:
      with self._create_session(graph, config, force_gpu) as sess:
        with self._constrain_devices_and_set_default(sess, use_gpu, force_gpu):
          yield sess

  @contextlib.contextmanager
  def cached_session(self,
                     graph=None,
                     config=None,
                     use_gpu=False,
                     force_gpu=False):
    """Returns a TensorFlow Session for use in executing tests.

    This method behaves differently than self.session(): for performance reasons
    `cached_session` will by default reuse the same session within the same
    test. The session returned by this function will only be closed at the end
    of the test (in the TearDown function).

    Use the `use_gpu` and `force_gpu` options to control where ops are run. If
    `force_gpu` is True, all ops are pinned to `/device:GPU:0`. Otherwise, if
    `use_gpu` is True, TensorFlow tries to run as many ops on the GPU as
    possible. If both `force_gpu and `use_gpu` are False, all ops are pinned to
    the CPU.

    Example:
    ```python
    class MyOperatorTest(test_util.TensorFlowTestCase):
      def testMyOperator(self):
        with self.cached_session(use_gpu=True) as sess:
          valid_input = [1.0, 2.0, 3.0, 4.0, 5.0]
          result = MyOperator(valid_input).eval()
          self.assertEqual(result, [1.0, 2.0, 3.0, 5.0, 8.0]
          invalid_input = [-1.0, 2.0, 7.0]
          with self.assertRaisesOpError("negative input not supported"):
            MyOperator(invalid_input).eval()
    ```

    Args:
      graph: Optional graph to use during the returned session.
      config: An optional config_pb2.ConfigProto to use to configure the
        session.
      use_gpu: If True, attempt to run as many ops as possible on GPU.
      force_gpu: If True, pin all ops to `/device:GPU:0`.

    Yields:
      A Session object that should be used as a context manager to surround
      the graph building and execution code in a test case.
    """
    if context.executing_eagerly():
      yield FakeEagerSession(self)
    else:
      sess = self._get_cached_session(
          graph, config, force_gpu, crash_if_inconsistent_args=True)
      with self._constrain_devices_and_set_default(sess, use_gpu,
                                                   force_gpu) as cached:
        yield cached

  @contextlib.contextmanager
  @deprecation.deprecated(None, "Use `self.session()` or "
                          "`self.cached_session()` instead.")
  def test_session(self,
                   graph=None,
                   config=None,
                   use_gpu=False,
                   force_gpu=False):
    """Use cached_session instead."""
    if self.id().endswith(".test_session"):
      self.skipTest("Not a test.")
    if context.executing_eagerly():
      yield None
    else:
      if graph is None:
        sess = self._get_cached_session(
            graph, config, force_gpu, crash_if_inconsistent_args=False)
        with self._constrain_devices_and_set_default(sess, use_gpu,
                                                     force_gpu) as cached:
          yield cached
      else:
        with self.session(graph, config, use_gpu, force_gpu) as sess:
          yield sess

  # pylint: enable=g-doc-return-or-yield

  class _CheckedThread(object):
    """A wrapper class for Thread that asserts successful completion.

    This class should be created using the TensorFlowTestCase.checkedThread()
    method.
    """

    def __init__(self, testcase, target, args=None, kwargs=None):
      """Constructs a new instance of _CheckedThread.

      Args:
        testcase: The TensorFlowTestCase for which this thread is being created.
        target: A callable object representing the code to be executed in the
          thread.
        args: A tuple of positional arguments that will be passed to target.
        kwargs: A dictionary of keyword arguments that will be passed to target.
      """
      self._testcase = testcase
      self._target = target
      self._args = () if args is None else args
      self._kwargs = {} if kwargs is None else kwargs
      self._thread = threading.Thread(target=self._protected_run)
      self._exception = None

      self._is_thread_joined = False

    def _protected_run(self):
      """Target for the wrapper thread. Sets self._exception on failure."""
      try:
        self._target(*self._args, **self._kwargs)
      except Exception as e:  # pylint: disable=broad-except
        self._exception = e

    def start(self):
      """Starts the thread's activity.

      This must be called at most once per _CheckedThread object. It arranges
      for the object's target to be invoked in a separate thread of control.
      """
      self._thread.start()

    def join(self):
      """Blocks until the thread terminates.

      Raises:
        self._testcase.failureException: If the thread terminates with due to
          an exception.
      """
      self._is_thread_joined = True
      self._thread.join()
      if self._exception is not None:
        self._testcase.fail("Error in checkedThread: %s" % str(self._exception))

    def is_alive(self):
      """Returns whether the thread is alive.

      This method returns True just before the run() method starts
      until just after the run() method terminates.

      Returns:
        True if the thread is alive, otherwise False.
      """
      return self._thread.is_alive()

    def check_termination(self):
      """Returns whether the checked thread was properly used and did terminate.

      Every checked thread should be "join"ed after starting, and before the
      test tears down. If it is not joined, it is possible the thread will hang
      and cause flaky failures in tests.

      Raises:
        self._testcase.failureException: If check_termination was called before
        thread was joined.

        RuntimeError: If the thread is not terminated. This means thread was not
        joined with the main thread.
      """
      if self._is_thread_joined:
        if self.is_alive():
          raise RuntimeError(
              "Thread was not joined with main thread, and is still running "
              "when the test finished.")
      else:
        self._testcase.fail("A checked thread was not joined.")

  def checkedThread(self, target, args=None, kwargs=None):
    """Returns a Thread wrapper that asserts 'target' completes successfully.

    This method should be used to create all threads in test cases, as
    otherwise there is a risk that a thread will silently fail, and/or
    assertions made in the thread will not be respected.

    Args:
      target: A callable object to be executed in the thread.
      args: The argument tuple for the target invocation. Defaults to ().
      kwargs: A dictionary of keyword arguments for the target invocation.
        Defaults to {}.

    Returns:
      A wrapper for threading.Thread that supports start() and join() methods.
    """
    ret = TensorFlowTestCase._CheckedThread(self, target, args, kwargs)
    self._threads.append(ret)
    return ret


  # pylint: enable=invalid-name
  @py_func_if_in_function
  def assertNear(self, f1, f2, err, msg=None):
    """Asserts that two floats are near each other.

    Checks that |f1 - f2| < err and asserts a test failure
    if not.

    Args:
      f1: A float value.
      f2: A float value.
      err: A float value.
      msg: An optional string message to append to the failure message.
    """
    # f1 == f2 is needed here as we might have: f1, f2 = inf, inf
    self.assertTrue(
        f1 == f2 or math.fabs(f1 - f2) <= err,
        "%f != %f +/- %f%s" % (f1, f2, err, " (%s)" % msg
                               if msg is not None else ""))

  @py_func_if_in_function
  def assertArrayNear(self, farray1, farray2, err, msg=None):
    """Asserts that two float arrays are near each other.

    Checks that for all elements of farray1 and farray2
    |f1 - f2| < err.  Asserts a test failure if not.

    Args:
      farray1: a list of float values.
      farray2: a list of float values.
      err: a float value.
      msg: Optional message to report on failure.
    """
    self.assertEqual(len(farray1), len(farray2), msg=msg)
    for f1, f2 in zip(farray1, farray2):
      self.assertNear(float(f1), float(f2), err, msg=msg)

  def _NDArrayNear(self, ndarray1, ndarray2, err):
    return np.linalg.norm(ndarray1 - ndarray2) < err

  @py_func_if_in_function
  def assertNDArrayNear(self, ndarray1, ndarray2, err, msg=None):
    """Asserts that two numpy arrays have near values.

    Args:
      ndarray1: a numpy ndarray.
      ndarray2: a numpy ndarray.
      err: a float. The maximum absolute difference allowed.
      msg: Optional message to report on failure.
    """
    self.assertTrue(self._NDArrayNear(ndarray1, ndarray2, err), msg=msg)

  def _GetNdArray(self, a):
    # If a is a tensor then convert it to ndarray
    if isinstance(a, ops.Tensor):
      if isinstance(a, ops._EagerTensorBase):
        a = a.numpy()
      else:
        a = self.evaluate(a)
    if not isinstance(a, np.ndarray):
      return np.array(a)
    return a

  def _assertArrayLikeAllClose(self, a, b, rtol=1e-6, atol=1e-6, msg=None):
    a = self._GetNdArray(a)
    b = self._GetNdArray(b)
    # When the array rank is small, print its contents. Numpy array printing is
    # implemented using inefficient recursion so prints can cause tests to
    # time out.
    if a.shape != b.shape and (b.ndim <= 3 or b.size < 500):
      shape_mismatch_msg = ("Shape mismatch: expected %s, got %s with contents "
                            "%s.") % (a.shape, b.shape, b)
    else:
      shape_mismatch_msg = "Shape mismatch: expected %s, got %s." % (a.shape,
                                                                     b.shape)
    self.assertEqual(a.shape, b.shape, shape_mismatch_msg)

    msgs = [msg]
    if not np.allclose(a, b, rtol=rtol, atol=atol):
      # Adds more details to np.testing.assert_allclose.
      #
      # NOTE: numpy.allclose (and numpy.testing.assert_allclose)
      # checks whether two arrays are element-wise equal within a
      # tolerance. The relative difference (rtol * abs(b)) and the
      # absolute difference atol are added together to compare against
      # the absolute difference between a and b.  Here, we want to
      # tell user which elements violate such conditions.
      cond = np.logical_or(
          np.abs(a - b) > atol + rtol * np.abs(b),
          np.isnan(a) != np.isnan(b))
      if a.ndim:
        x = a[np.where(cond)]
        y = b[np.where(cond)]
        msgs.append("not close where = {}".format(np.where(cond)))
      else:
        # np.where is broken for scalars
        x, y = a, b
      msgs.append("not close lhs = {}".format(x))
      msgs.append("not close rhs = {}".format(y))
      msgs.append("not close dif = {}".format(np.abs(x - y)))
      msgs.append("not close tol = {}".format(atol + rtol * np.abs(y)))
      msgs.append("dtype = {}, shape = {}".format(a.dtype, a.shape))
      # TODO(xpan): There seems to be a bug:
      # tensorflow/compiler/tests:binary_ops_test pass with float32
      # nan even though the equal_nan is False by default internally.
      np.testing.assert_allclose(
          a, b, rtol=rtol, atol=atol, err_msg="\n".join(msgs), equal_nan=True)

  def _assertAllCloseRecursive(self,
                               a,
                               b,
                               rtol=1e-6,
                               atol=1e-6,
                               path=None,
                               msg=None):
    path = path or []
    path_str = (("[" + "][".join([str(p) for p in path]) + "]") if path else "")
    msg = msg if msg else ""

    # Check if a and/or b are namedtuples.
    if hasattr(a, "_asdict"):
      a = a._asdict()
    if hasattr(b, "_asdict"):
      b = b._asdict()
    a_is_dict = isinstance(a, collections.Mapping)
    if a_is_dict != isinstance(b, collections.Mapping):
      raise ValueError("Can't compare dict to non-dict, a%s vs b%s. %s" %
                       (path_str, path_str, msg))
    if a_is_dict:
      self.assertItemsEqual(
          a.keys(),
          b.keys(),
          msg="mismatched keys: a%s has keys %s, but b%s has keys %s. %s" %
          (path_str, a.keys(), path_str, b.keys(), msg))
      for k in a:
        path.append(k)
        self._assertAllCloseRecursive(
            a[k], b[k], rtol=rtol, atol=atol, path=path, msg=msg)
        del path[-1]
    elif isinstance(a, (list, tuple)):
      # Try to directly compare a, b as ndarrays; if not work, then traverse
      # through the sequence, which is more expensive.
      try:
        a_as_ndarray = self._GetNdArray(a)
        b_as_ndarray = self._GetNdArray(b)
        self._assertArrayLikeAllClose(
            a_as_ndarray,
            b_as_ndarray,
            rtol=rtol,
            atol=atol,
            msg="Mismatched value: a%s is different from b%s. %s" %
            (path_str, path_str, msg))
      except (ValueError, TypeError) as e:
        if len(a) != len(b):
          raise ValueError(
              "Mismatched length: a%s has %d items, but b%s has %d items. %s" %
              (path_str, len(a), path_str, len(b), msg))
        for idx, (a_ele, b_ele) in enumerate(zip(a, b)):
          path.append(str(idx))
          self._assertAllCloseRecursive(
              a_ele, b_ele, rtol=rtol, atol=atol, path=path, msg=msg)
          del path[-1]
    # a and b are ndarray like objects
    else:
      try:
        self._assertArrayLikeAllClose(
            a,
            b,
            rtol=rtol,
            atol=atol,
            msg=("Mismatched value: a%s is different from b%s. %s" %
                 (path_str, path_str, msg)))
      except TypeError as e:
        msg = ("Error: a%s has %s, but b%s has %s. %s" %
               (path_str, type(a), path_str, type(b), msg))
        e.args = ((e.args[0] + " : " + msg,) + e.args[1:])
        raise

  @py_func_if_in_function
  def assertAllClose(self, a, b, rtol=1e-6, atol=1e-6, msg=None):
    """Asserts that two structures of numpy arrays or Tensors, have near values.

    `a` and `b` can be arbitrarily nested structures. A layer of a nested
    structure can be a `dict`, `namedtuple`, `tuple` or `list`.

    Args:
      a: The expected numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor), or any arbitrarily nested of
         structure of these.
      b: The actual numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor), or any arbitrarily nested of
         structure of these.
      rtol: relative tolerance.
      atol: absolute tolerance.
      msg: Optional message to report on failure.

    Raises:
      ValueError: if only one of `a[p]` and `b[p]` is a dict or
          `a[p]` and `b[p]` have different length, where `[p]` denotes a path
          to the nested structure, e.g. given `a = [(1, 1), {'d': (6, 7)}]` and
          `[p] = [1]['d']`, then `a[p] = (6, 7)`.
    """
    self._assertAllCloseRecursive(a, b, rtol=rtol, atol=atol, msg=msg)

  @py_func_if_in_function
  def assertAllCloseAccordingToType(self,
                                    a,
                                    b,
                                    rtol=1e-6,
                                    atol=1e-6,
                                    float_rtol=1e-6,
                                    float_atol=1e-6,
                                    half_rtol=1e-3,
                                    half_atol=1e-3,
                                    bfloat16_rtol=1e-2,
                                    bfloat16_atol=1e-2,
                                    msg=None):
    """Like assertAllClose, but also suitable for comparing fp16 arrays.

    In particular, the tolerance is reduced to 1e-3 if at least
    one of the arguments is of type float16.

    Args:
      a: the expected numpy ndarray or anything can be converted to one.
      b: the actual numpy ndarray or anything can be converted to one.
      rtol: relative tolerance.
      atol: absolute tolerance.
      float_rtol: relative tolerance for float32.
      float_atol: absolute tolerance for float32.
      half_rtol: relative tolerance for float16.
      half_atol: absolute tolerance for float16.
      bfloat16_rtol: relative tolerance for bfloat16.
      bfloat16_atol: absolute tolerance for bfloat16.
      msg: Optional message to report on failure.
    """
    a = self._GetNdArray(a)
    b = self._GetNdArray(b)
    # types with lower tol are put later to overwrite previous ones.
    if (a.dtype == np.float32 or b.dtype == np.float32 or
        a.dtype == np.complex64 or b.dtype == np.complex64):
      rtol = max(rtol, float_rtol)
      atol = max(atol, float_atol)
    if a.dtype == np.float16 or b.dtype == np.float16:
      rtol = max(rtol, half_rtol)
      atol = max(atol, half_atol)
    if (a.dtype == dtypes.bfloat16.as_numpy_dtype or
        b.dtype == dtypes.bfloat16.as_numpy_dtype):
      rtol = max(rtol, bfloat16_rtol)
      atol = max(atol, bfloat16_atol)

    self.assertAllClose(a, b, rtol=rtol, atol=atol, msg=msg)

  @py_func_if_in_function
  def assertNotAllClose(self, a, b, **kwargs):
    """Assert that two numpy arrays, or or Tensors, do not have near values.

    Args:
      a: the first value to compare.
      b: the second value to compare.
      **kwargs: additional keyword arguments to be passed to the underlying
        `assertAllClose` call.

    Raises:
      AssertionError: If `a` and `b` are unexpectedly close at all elements.
    """
    try:
      self.assertAllClose(a, b, **kwargs)
    except AssertionError:
      return
    raise AssertionError("The two values are close at all elements")

  @py_func_if_in_function
  def assertAllEqual(self, a, b, msg=None):
    """Asserts that two numpy arrays or Tensors have the same values.

    Args:
      a: the expected numpy ndarray or anything can be converted to one.
      b: the actual numpy ndarray or anything can be converted to one.
      msg: Optional message to report on failure.
    """
    msg = msg if msg else ""
    a = self._GetNdArray(a)
    b = self._GetNdArray(b)
    # Arbitrary bounds so that we don't print giant tensors.
    if (b.ndim <= 3 or b.size < 500):
      self.assertEqual(
          a.shape, b.shape, "Shape mismatch: expected %s, got %s."
          " Contents: %s. \n%s." % (a.shape, b.shape, b, msg))
    else:
      self.assertEqual(
          a.shape, b.shape, "Shape mismatch: expected %s, got %s."
          " %s" % (a.shape, b.shape, msg))

    same = (a == b)

    if (a.dtype in [
        np.float16, np.float32, np.float64, dtypes.bfloat16.as_numpy_dtype
    ]):
      same = np.logical_or(same, np.logical_and(np.isnan(a), np.isnan(b)))
    msgs = [msg]
    if not np.all(same):
      # Adds more details to np.testing.assert_array_equal.
      diff = np.logical_not(same)
      if a.ndim:
        x = a[np.where(diff)]
        y = b[np.where(diff)]
        msgs.append("not equal where = {}".format(np.where(diff)))
      else:
        # np.where is broken for scalars
        x, y = a, b
      msgs.append("not equal lhs = {}".format(x))
      msgs.append("not equal rhs = {}".format(y))
      np.testing.assert_array_equal(a, b, err_msg="\n".join(msgs))

  @py_func_if_in_function
  def assertAllGreater(self, a, comparison_target):
    """Assert element values are all greater than a target value.

    Args:
      a: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      comparison_target: The target value of comparison.
    """
    a = self._GetNdArray(a)
    self.assertGreater(np.min(a), comparison_target)

  @py_func_if_in_function
  def assertAllLess(self, a, comparison_target):
    """Assert element values are all less than a target value.

    Args:
      a: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      comparison_target: The target value of comparison.
    """
    a = self._GetNdArray(a)
    self.assertLess(np.max(a), comparison_target)

  @py_func_if_in_function
  def assertAllGreaterEqual(self, a, comparison_target):
    """Assert element values are all greater than or equal to a target value.

    Args:
      a: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      comparison_target: The target value of comparison.
    """
    a = self._GetNdArray(a)
    self.assertGreaterEqual(np.min(a), comparison_target)

  @py_func_if_in_function
  def assertAllLessEqual(self, a, comparison_target):
    """Assert element values are all less than or equal to a target value.

    Args:
      a: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      comparison_target: The target value of comparison.
    """
    a = self._GetNdArray(a)
    self.assertLessEqual(np.max(a), comparison_target)

  def _format_subscripts(self, subscripts, value, limit=10, indent=2):
    """Generate a summary of ndarray subscripts as a list of str.

    If limit == N, this method will print up to the first N subscripts on
    separate
    lines. A line of ellipses (...) will be appended at the end if the number of
    subscripts exceeds N.

    Args:
      subscripts: The tensor (np.ndarray) subscripts, of the same format as
        np.where()'s return value, i.e., a tuple of arrays with each array
        corresponding to a dimension. E.g., (array([1, 1]), array([0, 1])).
      value: (np.ndarray) value of the tensor.
      limit: (int) The maximum number of indices to print.
      indent: (int) Number of characters to indent at the beginning of each
        line.

    Returns:
      (list of str) the multi-line representation of the subscripts and values,
        potentially with omission at the end.
    """
    lines = []
    subscripts = np.transpose(subscripts)
    prefix = " " * indent
    for subscript in itertools.islice(subscripts, limit):
      lines.append(prefix + str(subscript) + " : " +
                   str(value[tuple(subscript)]))
    if len(subscripts) > limit:
      lines.append(prefix + "...")
    return lines

  @py_func_if_in_function
  def assertAllInRange(self,
                       target,
                       lower_bound,
                       upper_bound,
                       open_lower_bound=False,
                       open_upper_bound=False):
    """Assert that elements in a Tensor are all in a given range.

    Args:
      target: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      lower_bound: lower bound of the range
      upper_bound: upper bound of the range
      open_lower_bound: (`bool`) whether the lower bound is open (i.e., > rather
        than the default >=)
      open_upper_bound: (`bool`) whether the upper bound is open (i.e., < rather
        than the default <=)

    Raises:
      AssertionError:
        if the value tensor does not have an ordered numeric type (float* or
          int*), or
        if there are nan values, or
        if any of the elements do not fall in the specified range.
    """
    target = self._GetNdArray(target)
    if not (np.issubdtype(target.dtype, np.floating) or
            np.issubdtype(target.dtype, np.integer)):
      raise AssertionError(
          "The value of %s does not have an ordered numeric type, instead it "
          "has type: %s" % (target, target.dtype))

    nan_subscripts = np.where(np.isnan(target))
    if np.size(nan_subscripts):
      raise AssertionError(
          "%d of the %d element(s) are NaN. "
          "Subscripts(s) and value(s) of the NaN element(s):\n" %
          (len(nan_subscripts[0]), np.size(target)) +
          "\n".join(self._format_subscripts(nan_subscripts, target)))

    range_str = (("(" if open_lower_bound else "[") + str(lower_bound) + ", " +
                 str(upper_bound) + (")" if open_upper_bound else "]"))

    violations = (
        np.less_equal(target, lower_bound)
        if open_lower_bound else np.less(target, lower_bound))
    violations = np.logical_or(
        violations,
        np.greater_equal(target, upper_bound)
        if open_upper_bound else np.greater(target, upper_bound))
    violation_subscripts = np.where(violations)
    if np.size(violation_subscripts):
      raise AssertionError(
          "%d of the %d element(s) are outside the range %s. " %
          (len(violation_subscripts[0]), np.size(target), range_str) +
          "Subscript(s) and value(s) of the offending elements:\n" +
          "\n".join(self._format_subscripts(violation_subscripts, target)))

  @py_func_if_in_function
  def assertAllInSet(self, target, expected_set):
    """Assert that elements of a Tensor are all in a given closed set.

    Args:
      target: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      expected_set: (`list`, `tuple` or `set`) The closed set that the elements
        of the value of `target` are expected to fall into.

    Raises:
      AssertionError:
        if any of the elements do not fall into `expected_set`.
    """
    target = self._GetNdArray(target)

    # Elements in target that are not in expected_set.
    diff = np.setdiff1d(target.flatten(), list(expected_set))
    if np.size(diff):
      raise AssertionError("%d unique element(s) are not in the set %s: %s" %
                           (np.size(diff), expected_set, diff))

  @py_func_if_in_function
  def assertDTypeEqual(self, target, expected_dtype):
    """Assert ndarray data type is equal to expected.

    Args:
      target: The numpy `ndarray`, or anything that can be converted into a
         numpy `ndarray` (including Tensor).
      expected_dtype: Expected data type.
    """
    target = self._GetNdArray(target)
    if not isinstance(target, list):
      arrays = [target]
    for arr in arrays:
      self.assertEqual(arr.dtype, expected_dtype)

  # pylint: disable=g-doc-return-or-yield
  @contextlib.contextmanager
  def assertRaisesWithPredicateMatch(self, exception_type,
                                     expected_err_re_or_predicate):
    """Returns a context manager to enclose code expected to raise an exception.

    If the exception is an OpError, the op stack is also included in the message
    predicate search.

    Args:
      exception_type: The expected type of exception that should be raised.
      expected_err_re_or_predicate: If this is callable, it should be a function
        of one argument that inspects the passed-in exception and
        returns True (success) or False (please fail the test). Otherwise, the
        error message is expected to match this regular expression partially.

    Returns:
      A context manager to surround code that is expected to raise an
      exception.
    """
    if callable(expected_err_re_or_predicate):
      predicate = expected_err_re_or_predicate
    else:

      def predicate(e):
        err_str = e.message if isinstance(e, errors.OpError) else str(e)
        op = e.op if isinstance(e, errors.OpError) else None
        while op is not None:
          err_str += "\nCaused by: " + op.name
          op = op._original_op  # pylint: disable=protected-access
        logging.info("Searching within error strings: '%s' within '%s'",
                     expected_err_re_or_predicate, err_str)
        return re.search(expected_err_re_or_predicate, err_str)

    try:
      yield
      self.fail(exception_type.__name__ + " not raised")
    except Exception as e:  # pylint: disable=broad-except
      if not isinstance(e, exception_type) or not predicate(e):
        raise AssertionError(
            "Exception of type %s: %s" % (str(type(e)), str(e)))

  # pylint: enable=g-doc-return-or-yield

  def assertRaisesOpError(self, expected_err_re_or_predicate):
    return self.assertRaisesWithPredicateMatch(errors.OpError,
                                               expected_err_re_or_predicate)

  def assertShapeEqual(self, np_array, tf_tensor, msg=None):
    """Asserts that a Numpy ndarray and a TensorFlow tensor have the same shape.

    Args:
      np_array: A Numpy ndarray or Numpy scalar.
      tf_tensor: A Tensor.
      msg: Optional message to report on failure.

    Raises:
      TypeError: If the arguments have the wrong type.
    """
    if not isinstance(np_array, (np.ndarray, np.generic)):
      raise TypeError("np_array must be a Numpy ndarray or Numpy scalar")
    if not isinstance(tf_tensor, ops.Tensor):
      raise TypeError("tf_tensor must be a Tensor")
    self.assertAllEqual(
        np_array.shape, tf_tensor.get_shape().as_list(), msg=msg)

  def assertDeviceEqual(self, device1, device2, msg=None):
    """Asserts that the two given devices are the same.

    Args:
      device1: A string device name or TensorFlow `DeviceSpec` object.
      device2: A string device name or TensorFlow `DeviceSpec` object.
      msg: Optional message to report on failure.
    """
    device1 = pydev.canonical_name(device1)
    device2 = pydev.canonical_name(device2)
    self.assertEqual(
        device1, device2,
        "Devices %s and %s are not equal. %s" % (device1, device2, msg))

  # Fix Python 3 compatibility issues
  if six.PY3:
    # pylint: disable=invalid-name

    # Silence a deprecation warning
    assertRaisesRegexp = googletest.TestCase.assertRaisesRegex

    # assertItemsEqual is assertCountEqual as of 3.2.
    assertItemsEqual = googletest.TestCase.assertCountEqual

    # pylint: enable=invalid-name

  @contextlib.contextmanager
  def _constrain_devices_and_set_default(self, sess, use_gpu, force_gpu):
    """Set the session and its graph to global default and constrain devices."""
    if context.executing_eagerly():
      yield None
    else:
      with sess.graph.as_default(), sess.as_default():
        if force_gpu:
          # Use the name of an actual device if one is detected, or
          # '/device:GPU:0' otherwise
          gpu_name = gpu_device_name()
          if not gpu_name:
            gpu_name = "/device:GPU:0"
          with sess.graph.device(gpu_name):
            yield sess
        elif use_gpu:
          yield sess
        else:
          with sess.graph.device("/device:CPU:0"):
            yield sess

  def _create_session(self, graph, config, force_gpu):
    """See session() for details."""
    def prepare_config(config):
      """Returns a config for sessions.

      Args:
        config: An optional config_pb2.ConfigProto to use to configure the
          session.

      Returns:
        A config_pb2.ConfigProto object.
      """
      # TODO(b/114333779): Enforce allow_soft_placement=False when
      # use_gpu=False. Currently many tests rely on the fact that any device
      # will be used even when a specific device is supposed to be used.
      allow_soft_placement = not force_gpu
      if config is None:
        config = config_pb2.ConfigProto()
        config.allow_soft_placement = allow_soft_placement
        config.gpu_options.per_process_gpu_memory_fraction = 0.3
      elif not allow_soft_placement and config.allow_soft_placement:
        config_copy = config_pb2.ConfigProto()
        config_copy.CopyFrom(config)
        config = config_copy
        config.allow_soft_placement = False
      # Don't perform optimizations for tests so we don't inadvertently run
      # gpu ops on cpu
      config.graph_options.optimizer_options.opt_level = -1
      # Disable Grappler constant folding since some tests & benchmarks
      # use constant input and become meaningless after constant folding.
      # DO NOT DISABLE GRAPPLER OPTIMIZERS WITHOUT CONSULTING WITH THE
      # GRAPPLER TEAM.
      config.graph_options.rewrite_options.constant_folding = (
          rewriter_config_pb2.RewriterConfig.OFF)
      config.graph_options.rewrite_options.pin_to_host_optimization = (
          rewriter_config_pb2.RewriterConfig.OFF)
      return config

    return ErrorLoggingSession(graph=graph, config=prepare_config(config))

  def _get_cached_session(self,
                          graph=None,
                          config=None,
                          force_gpu=False,
                          crash_if_inconsistent_args=True):
    """See cached_session() for documentation."""
    if self._cached_session is None:
      sess = self._create_session(
          graph=graph, config=config, force_gpu=force_gpu)
      self._cached_session = sess
      self._cached_graph = graph
      self._cached_config = config
      self._cached_force_gpu = force_gpu
      return sess
    else:
      if crash_if_inconsistent_args and self._cached_graph is not graph:
        raise ValueError("The graph used to get the cached session is "
                         "different than the one that was used to create the "
                         "session. Maybe create a new session with "
                         "self.session()")
      if crash_if_inconsistent_args and self._cached_config is not config:
        raise ValueError("The config used to get the cached session is "
                         "different than the one that was used to create the "
                         "session. Maybe create a new session with "
                         "self.session()")
      if crash_if_inconsistent_args and (self._cached_force_gpu is
                                         not force_gpu):
        raise ValueError(
            "The force_gpu value used to get the cached session is "
            "different than the one that was used to create the "
            "session. Maybe create a new session with "
            "self.session()")
      return self._cached_session


@tf_export("test.create_local_cluster")
def create_local_cluster(num_workers,
                         num_ps,
                         protocol="grpc",
                         worker_config=None,
                         ps_config=None):
  """Create and start local servers and return the associated `Server` objects.

  Example:
  ```python
  workers, _ = tf.test.create_local_cluster(num_workers=2, num_ps=2)

  worker_sessions = [tf.Session(w.target) for w in workers]

  with tf.device("/job:ps/task:0"):
    ...
  with tf.device("/job:ps/task:1"):
    ...
  with tf.device("/job:worker/task:0"):
    ...
  with tf.device("/job:worker/task:1"):
    ...

  worker_sessions[0].run(...)
  ```

  Args:
    num_workers: Number of worker servers to start.
    num_ps: Number of PS servers to start.
    protocol: Communication protocol.  Allowed values are documented in
      the documentation of `tf.train.Server`.
    worker_config: (optional) ConfigProto to initialize workers. Can be used
      to instantiate multiple devices etc.
    ps_config: (optional) ConfigProto to initialize PS servers.

  Returns:
    A tuple `(worker_servers, ps_servers)`.  `worker_servers` is a list
    of `num_workers` objects of type `tf.train.Server` (all running locally);
    and `ps_servers` is a list of `num_ps` objects of similar type.

  Raises:
    ImportError: if portpicker module was not found at load time
  """
  if _portpicker_import_error:
    raise _portpicker_import_error  # pylint: disable=raising-bad-type
  worker_ports = [portpicker.pick_unused_port() for _ in range(num_workers)]
  ps_ports = [portpicker.pick_unused_port() for _ in range(num_ps)]
  cluster_dict = {
      "worker": ["localhost:%s" % port for port in worker_ports],
      "ps": ["localhost:%s" % port for port in ps_ports]
  }
  cs = server_lib.ClusterSpec(cluster_dict)

  workers = [
      server_lib.Server(
          cs,
          job_name="worker",
          protocol=protocol,
          task_index=ix,
          config=worker_config,
          start=True) for ix in range(num_workers)
  ]
  ps_servers = [
      server_lib.Server(
          cs,
          job_name="ps",
          protocol=protocol,
          task_index=ix,
          config=ps_config,
          start=True) for ix in range(num_ps)
  ]

  return workers, ps_servers


def get_node_def_from_graph(node_name, graph_def):
  """Returns the `NodeDef` instance for given node name in the graph def.

  This method explores only the NodeDefs in `graph_def.node`.

  Args:
    node_name: Name of the NodeDef to search for.
    graph_def: An instance of `GraphDef` proto.

  Returns:
    the `NodeDef` instance whose name field matches the given node_name or None.
  """
  for node_def in graph_def.node:
    if node_def.name == node_name:
      return node_def
  return None


def set_producer_version(graph, producer_version):
  """Sets graph.graph_def_versions.producer to `producer_version`."""
  # The C API doesn't expose altering GraphDefVersions. We can indirectly set
  # it via import_graph_def though.
  graph_def = graph_pb2.GraphDef()
  graph_def.versions.producer = producer_version
  with graph.as_default():
    importer.import_graph_def(graph_def)
  assert graph.graph_def_versions.producer, producer_version


def dismantle_func_graph(func_graph):
  """Removes reference cycles in `func_graph` FuncGraph.

  Helpful for making sure the garbage collector doesn't need to run when
  the FuncGraph goes out of scope, e.g. in tests using defun with
  @test_util.run_in_graph_and_eager_modes(assert_no_eager_garbage=True).

  Args:
    func_graph: A `FuncGraph` object to destroy. `func_graph` is unusable
      after this function.
  """
  # TODO(b/115366440): Delete this method when a custom OrderedDict is added.
  # Clearing captures using clear() leaves some cycles around.
  while func_graph.captures:
    func_graph.captures.popitem()
  memory.dismantle_ordered_dict(func_graph.captures)
  ops.dismantle_graph(func_graph)


def dismantle_polymorphic_function(func):
  """Removes reference cycles in PolymorphicFunction `func`.

  Helpful for making sure the garbage collector doesn't need to run when
  PolymorphicFunction goes out of scope, e.g. in tests using defun with
  @test_util.run_in_graph_and_eager_modes(assert_no_eager_garbage=True).

  Args:
    func: A `PolymorphicFunction` object to destroy. `func` is unusable
      after this function.
  """
  # TODO(b/115366440): Delete this method when a custom OrderedDict is added
  cache = func._function_cache  # pylint: disable=protected-access
  for concrete_func in cache.values():
    dismantle_func_graph(concrete_func.graph)
  while cache:
    cache.popitem()
  memory.dismantle_ordered_dict(cache)
