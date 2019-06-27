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

"""Operations to emit summaries."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import getpass
import os
import re
import time

import six

from tensorflow.core.framework import graph_pb2
from tensorflow.python.eager import context
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import smart_cond
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import gen_summary_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.ops import summary_op_util
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.training import training_util
from tensorflow.python.util import deprecation
from tensorflow.python.util import tf_contextlib
from tensorflow.python.util.tf_export import tf_export


# Dictionary mapping graph keys to a boolean Tensor (or callable returning
# a boolean Tensor) indicating whether we should record summaries for the
# graph identified by the key of the dictionary.
_SHOULD_RECORD_SUMMARIES = {}

# A global dictionary mapping graph keys to a list of summary writer init ops.
_SUMMARY_WRITER_INIT_OP = {}

_EXPERIMENT_NAME_PATTERNS = re.compile(r"^[^\x00-\x1F<>]{0,256}$")
_RUN_NAME_PATTERNS = re.compile(r"^[^\x00-\x1F<>]{0,512}$")
_USER_NAME_PATTERNS = re.compile(r"^[a-z]([-a-z0-9]{0,29}[a-z0-9])?$", re.I)


def _should_record_summaries_internal():
  """Returns boolean Tensor if summaries should/shouldn't be recorded, or None.
  """
  global _SHOULD_RECORD_SUMMARIES
  key = ops.get_default_graph()._graph_key  # pylint: disable=protected-access
  should = _SHOULD_RECORD_SUMMARIES.get(key)
  return should() if callable(should) else should


def _should_record_summaries_v2():
  """Returns boolean Tensor which is true if summaries should be recorded.

  If no recording status has been set, this defaults to True, unlike the public
  should_record_summaries().
  """
  result = _should_record_summaries_internal()
  return True if result is None else result


def should_record_summaries():
  """Returns boolean Tensor which is true if summaries should be recorded."""
  result = _should_record_summaries_internal()
  return False if result is None else result


@tf_contextlib.contextmanager
def _record_summaries(boolean=True):
  """Sets summary recording on or off per the provided boolean value.

  The provided value can be a python boolean, a scalar boolean Tensor, or
  or a callable providing such a value; if a callable is passed it will be
  invoked each time should_record_summaries() is called to determine whether
  summary writing should be enabled.

  Args:
    boolean: can be True, False, a bool Tensor, or a callable providing such.
      Defaults to True.

  Yields:
    Returns a context manager that sets this value on enter and restores the
    previous value on exit.
  """
  # TODO(nickfelt): make this threadlocal
  global _SHOULD_RECORD_SUMMARIES
  key = ops.get_default_graph()._graph_key  # pylint: disable=protected-access
  old = _SHOULD_RECORD_SUMMARIES.setdefault(key, None)
  try:
    _SHOULD_RECORD_SUMMARIES[key] = boolean
    yield
  finally:
    _SHOULD_RECORD_SUMMARIES[key] = old


# TODO(apassos) consider how to handle local step here.
def record_summaries_every_n_global_steps(n, global_step=None):
  """Sets the should_record_summaries Tensor to true if global_step % n == 0."""
  if global_step is None:
    global_step = training_util.get_or_create_global_step()
  with ops.device("cpu:0"):
    should = lambda: math_ops.equal(global_step % n, 0)
    if not context.executing_eagerly():
      should = should()
  return _record_summaries(should)


def always_record_summaries():
  """Sets the should_record_summaries Tensor to always true."""
  return _record_summaries(True)


def never_record_summaries():
  """Sets the should_record_summaries Tensor to always false."""
  return _record_summaries(False)


@tf_export("summary.SummaryWriter", v1=[])
class SummaryWriter(object):
  """Encapsulates a stateful summary writer resource.

  See also:
  - `tf.summary.create_file_writer`
  - `tf.summary.create_db_writer`
  """

  def  __init__(self, resource, init_op_fn):
    self._resource = resource
    # TODO(nickfelt): cache constructed ops in graph mode
    self._init_op_fn = init_op_fn
    if context.executing_eagerly() and self._resource is not None:
      self._resource_deleter = resource_variable_ops.EagerResourceDeleter(
          handle=self._resource, handle_device="cpu:0")

  def set_as_default(self):
    """Enables this summary writer for the current thread."""
    context.context().summary_writer_resource = self._resource

  @tf_contextlib.contextmanager
  def as_default(self):
    """Enables summary writing within a `with` block."""
    if self._resource is None:
      yield self
    else:
      old = context.context().summary_writer_resource
      try:
        context.context().summary_writer_resource = self._resource
        yield self
        # Flushes the summary writer in eager mode or in graph functions, but
        # not in legacy graph mode (you're on your own there).
        with ops.device("cpu:0"):
          gen_summary_ops.flush_summary_writer(self._resource)
      finally:
        context.context().summary_writer_resource = old

  def init(self):
    """Operation to initialize the summary writer resource."""
    if self._resource is not None:
      return self._init_op_fn()

  def _flush(self):
    return _flush_fn(writer=self)

  def flush(self):
    """Operation to force the summary writer to flush any buffered data."""
    if self._resource is not None:
      return self._flush()

  def _close(self):
    with ops.control_dependencies([self.flush()]):
      with ops.device("cpu:0"):
        return gen_summary_ops.close_summary_writer(self._resource)

  def close(self):
    """Operation to flush and close the summary writer resource."""
    if self._resource is not None:
      return self._close()


def initialize(
    graph=None,  # pylint: disable=redefined-outer-name
    session=None):
  """Initializes summary writing for graph execution mode.

  This helper method provides a higher-level alternative to using
  `tf.contrib.summary.summary_writer_initializer_op` and
  `tf.contrib.summary.graph`.

  Most users will also want to call `tf.train.create_global_step`
  which can happen before or after this function is called.

  Args:
    graph: A `tf.Graph` or `tf.GraphDef` to output to the writer.
      This function will not write the default graph by default. When
      writing to an event log file, the associated step will be zero.
    session: So this method can call `tf.Session.run`. This defaults
      to `tf.get_default_session`.

  Raises:
    RuntimeError: If  the current thread has no default
      `tf.contrib.summary.SummaryWriter`.
    ValueError: If session wasn't passed and no default session.
  """
  if context.executing_eagerly():
    return
  if context.context().summary_writer_resource is None:
    raise RuntimeError("No default tf.contrib.summary.SummaryWriter found")
  if session is None:
    session = ops.get_default_session()
    if session is None:
      raise ValueError("session must be passed if no default session exists")
  session.run(summary_writer_initializer_op())
  if graph is not None:
    data = _serialize_graph(graph)
    x = array_ops.placeholder(dtypes.string)
    session.run(_graph(x, 0), feed_dict={x: data})


@tf_export("summary.create_file_writer", v1=[])
def create_file_writer(logdir,
                       max_queue=None,
                       flush_millis=None,
                       filename_suffix=None,
                       name=None):
  """Creates a summary file writer in the current context under the given name.

  Args:
    logdir: a string, or None. If a string, creates a summary file writer
     which writes to the directory named by the string. If None, returns
     a mock object which acts like a summary writer but does nothing,
     useful to use as a context manager.
    max_queue: the largest number of summaries to keep in a queue; will
     flush once the queue gets bigger than this. Defaults to 10.
    flush_millis: the largest interval between flushes. Defaults to 120,000.
    filename_suffix: optional suffix for the event file name. Defaults to `.v2`.
    name: Shared name for this SummaryWriter resource stored to default
      Graph. Defaults to the provided logdir prefixed with `logdir:`. Note: if a
      summary writer resource with this shared name already exists, the returned
      SummaryWriter wraps that resource and the other arguments have no effect.

  Returns:
    Either a summary writer or an empty object which can be used as a
    summary writer.
  """
  if logdir is None:
    return SummaryWriter(None, None)
  logdir = str(logdir)
  with ops.device("cpu:0"):
    if max_queue is None:
      max_queue = constant_op.constant(10)
    if flush_millis is None:
      flush_millis = constant_op.constant(2 * 60 * 1000)
    if filename_suffix is None:
      filename_suffix = constant_op.constant(".v2")
    if name is None:
      name = "logdir:" + logdir
    return _make_summary_writer(
        name,
        gen_summary_ops.create_summary_file_writer,
        logdir=logdir,
        max_queue=max_queue,
        flush_millis=flush_millis,
        filename_suffix=filename_suffix)


def create_db_writer(db_uri,
                     experiment_name=None,
                     run_name=None,
                     user_name=None,
                     name=None):
  """Creates a summary database writer in the current context.

  This can be used to write tensors from the execution graph directly
  to a database. Only SQLite is supported right now. This function
  will create the schema if it doesn't exist. Entries in the Users,
  Experiments, and Runs tables will be created automatically if they
  don't already exist.

  Args:
    db_uri: For example "file:/tmp/foo.sqlite".
    experiment_name: Defaults to YYYY-MM-DD in local time if None.
      Empty string means the Run will not be associated with an
      Experiment. Can't contain ASCII control characters or <>. Case
      sensitive.
    run_name: Defaults to HH:MM:SS in local time if None. Empty string
      means a Tag will not be associated with any Run. Can't contain
      ASCII control characters or <>. Case sensitive.
    user_name: Defaults to system username if None. Empty means the
      Experiment will not be associated with a User. Must be valid as
      both a DNS label and Linux username.
    name: Shared name for this SummaryWriter resource stored to default
      `tf.Graph`.

  Returns:
    A `tf.summary.SummaryWriter` instance.
  """
  with ops.device("cpu:0"):
    if experiment_name is None:
      experiment_name = time.strftime("%Y-%m-%d", time.localtime(time.time()))
    if run_name is None:
      run_name = time.strftime("%H:%M:%S", time.localtime(time.time()))
    if user_name is None:
      user_name = getpass.getuser()
    experiment_name = _cleanse_string(
        "experiment_name", _EXPERIMENT_NAME_PATTERNS, experiment_name)
    run_name = _cleanse_string("run_name", _RUN_NAME_PATTERNS, run_name)
    user_name = _cleanse_string("user_name", _USER_NAME_PATTERNS, user_name)
    return _make_summary_writer(
        name,
        gen_summary_ops.create_summary_db_writer,
        db_uri=db_uri,
        experiment_name=experiment_name,
        run_name=run_name,
        user_name=user_name)


def _make_summary_writer(name, factory, **kwargs):
  resource = gen_summary_ops.summary_writer(shared_name=name)
  init_op_fn = lambda: factory(resource, **kwargs)
  init_op = init_op_fn()
  if not context.executing_eagerly():
    # TODO(apassos): Consider doing this instead.
    #   ops.get_default_session().run(init_op)
    global _SUMMARY_WRITER_INIT_OP
    key = ops.get_default_graph()._graph_key  # pylint: disable=protected-access
    _SUMMARY_WRITER_INIT_OP.setdefault(key, []).append(init_op)
  return SummaryWriter(resource, init_op_fn)


def _cleanse_string(name, pattern, value):
  if isinstance(value, six.string_types) and pattern.search(value) is None:
    raise ValueError("%s (%s) must match %s" % (name, value, pattern.pattern))
  return ops.convert_to_tensor(value, dtypes.string)


def _nothing():
  """Convenient else branch for when summaries do not record."""
  return constant_op.constant(False)


def all_summary_ops():
  """Graph-mode only. Returns all summary ops.

  Please note this excludes `tf.summary.graph` ops.

  Returns:
    The summary ops.
  """
  if context.executing_eagerly():
    return None
  return ops.get_collection(ops.GraphKeys._SUMMARY_COLLECTION)  # pylint: disable=protected-access


def summary_writer_initializer_op():
  """Graph-mode only. Returns the list of ops to create all summary writers.

  Returns:
    The initializer ops.

  Raises:
    RuntimeError: If in Eager mode.
  """
  if context.executing_eagerly():
    raise RuntimeError(
        "tf.contrib.summary.summary_writer_initializer_op is only "
        "supported in graph mode.")
  global _SUMMARY_WRITER_INIT_OP
  key = ops.get_default_graph()._graph_key  # pylint: disable=protected-access
  return _SUMMARY_WRITER_INIT_OP.setdefault(key, [])


_INVALID_SCOPE_CHARACTERS = re.compile(r"[^-_/.A-Za-z0-9]")


@tf_export("summary.summary_scope", v1=[])
@tf_contextlib.contextmanager
def summary_scope(name, default_name="summary", values=None):
  """A context manager for use when defining a custom summary op.

  This behaves similarly to `tf.name_scope`, except that it returns a generated
  summary tag in addition to the scope name. The tag is structurally similar to
  the scope name - derived from the user-provided name, prefixed with enclosing
  name scopes if any - but we relax the constraint that it be uniquified, as
  well as the character set limitation (so the user-provided name can contain
  characters not legal for scope names; in the scope name these are removed).

  This makes the summary tag more predictable and consistent for the user.

  For example, to define a new summary op called `my_op`:

  ```python
  def my_op(name, my_value, step):
    with tf.summary.summary_scope(name, "MyOp", [my_value]) as (tag, scope):
      my_value = tf.convert_to_tensor(my_value)
      return tf.summary.write(tag, my_value, step=step)
  ```

  Args:
    name: string name for the summary.
    default_name: Optional; if provided, used as default name of the summary.
    values: Optional; passed as `values` parameter to name_scope.

  Yields:
    A tuple `(tag, scope)` as described above.
  """
  name = name or default_name
  current_scope = ops.get_name_scope()
  tag = current_scope + "/" + name if current_scope else name
  # Strip illegal characters from the scope name, and if that leaves nothing,
  # use None instead so we pick up the default name.
  name = _INVALID_SCOPE_CHARACTERS.sub("", name) or None
  with ops.name_scope(name, default_name, values) as scope:
    yield tag, scope


@tf_export("summary.write", v1=[])
def write(tag, tensor, step, metadata=None, name=None):
  """Writes a generic summary to the default SummaryWriter if one exists.

  This exists primarily to support the definition of type-specific summary ops
  like scalar() and image(), and is not intended for direct use unless defining
  a new type-specific summary op.

  Args:
    tag: string tag used to identify the summary (e.g. in TensorBoard), usually
      generated with `tf.summary.summary_scope`
    tensor: the Tensor holding the summary data to write
    step: `int64`-castable monotic step value for this summary
    metadata: Optional SummaryMetadata, as a proto or serialized bytes
    name: Optional string name for this op.

  Returns:
    True on success, or false if no summary was written because no default
    summary writer was available.
  """
  with ops.name_scope(name, "write_summary") as scope:
    if context.context().summary_writer_resource is None:
      return constant_op.constant(False)
    if metadata is None:
      serialized_metadata = constant_op.constant(b"")
    elif hasattr(metadata, "SerializeToString"):
      serialized_metadata = constant_op.constant(metadata.SerializeToString())
    else:
      serialized_metadata = metadata

    def record():
      """Record the actual summary and return True."""
      # Note the identity to move the tensor to the CPU.
      with ops.device("cpu:0"):
        write_summary_op = gen_summary_ops.write_summary(
            context.context().summary_writer_resource,
            step,
            array_ops.identity(tensor),
            tag,
            serialized_metadata,
            name=scope)
        with ops.control_dependencies([write_summary_op]):
          return constant_op.constant(True)

    return smart_cond.smart_cond(
        _should_record_summaries_v2(), record, _nothing, name="summary_cond")


def summary_writer_function(name, tensor, function, family=None):
  """Helper function to write summaries.

  Args:
    name: name of the summary
    tensor: main tensor to form the summary
    function: function taking a tag and a scope which writes the summary
    family: optional, the summary's family

  Returns:
    The result of writing the summary.
  """
  name_scope = ops.get_name_scope()
  if name_scope:
    # Add a slash to allow reentering the name scope.
    name_scope += "/"
  def record():
    with ops.name_scope(name_scope), summary_op_util.summary_scope(
        name, family, values=[tensor]) as (tag, scope):
      with ops.control_dependencies([function(tag, scope)]):
        return constant_op.constant(True)

  if context.context().summary_writer_resource is None:
    return control_flow_ops.no_op()
  with ops.device("cpu:0"):
    op = smart_cond.smart_cond(
        should_record_summaries(), record, _nothing, name="")
    if not context.executing_eagerly():
      ops.add_to_collection(ops.GraphKeys._SUMMARY_COLLECTION, op)  # pylint: disable=protected-access
  return op


def generic(name, tensor, metadata=None, family=None, step=None):
  """Writes a tensor summary if possible."""

  def function(tag, scope):
    if metadata is None:
      serialized_metadata = constant_op.constant("")
    elif hasattr(metadata, "SerializeToString"):
      serialized_metadata = constant_op.constant(metadata.SerializeToString())
    else:
      serialized_metadata = metadata
    # Note the identity to move the tensor to the CPU.
    return gen_summary_ops.write_summary(
        context.context().summary_writer_resource,
        _choose_step(step),
        array_ops.identity(tensor),
        tag,
        serialized_metadata,
        name=scope)
  return summary_writer_function(name, tensor, function, family=family)


def scalar(name, tensor, family=None, step=None):
  """Writes a scalar summary if possible.

  Unlike `tf.contrib.summary.generic` this op may change the dtype
  depending on the writer, for both practical and efficiency concerns.

  Args:
    name: An arbitrary name for this summary.
    tensor: A `tf.Tensor` Must be one of the following types:
      `float32`, `float64`, `int32`, `int64`, `uint8`, `int16`,
      `int8`, `uint16`, `half`, `uint32`, `uint64`.
    family: Optional, the summary's family.
    step: The `int64` monotonic step variable, which defaults
      to `tf.train.get_global_step`.

  Returns:
    The created `tf.Operation` or a `tf.no_op` if summary writing has
    not been enabled for this context.
  """

  def function(tag, scope):
    # Note the identity to move the tensor to the CPU.
    return gen_summary_ops.write_scalar_summary(
        context.context().summary_writer_resource,
        _choose_step(step),
        tag,
        array_ops.identity(tensor),
        name=scope)

  return summary_writer_function(name, tensor, function, family=family)


def histogram(name, tensor, family=None, step=None):
  """Writes a histogram summary if possible."""

  def function(tag, scope):
    # Note the identity to move the tensor to the CPU.
    return gen_summary_ops.write_histogram_summary(
        context.context().summary_writer_resource,
        _choose_step(step),
        tag,
        array_ops.identity(tensor),
        name=scope)

  return summary_writer_function(name, tensor, function, family=family)


def image(name, tensor, bad_color=None, max_images=3, family=None, step=None):
  """Writes an image summary if possible."""

  def function(tag, scope):
    bad_color_ = (constant_op.constant([255, 0, 0, 255], dtype=dtypes.uint8)
                  if bad_color is None else bad_color)
    # Note the identity to move the tensor to the CPU.
    return gen_summary_ops.write_image_summary(
        context.context().summary_writer_resource,
        _choose_step(step),
        tag,
        array_ops.identity(tensor),
        bad_color_,
        max_images,
        name=scope)

  return summary_writer_function(name, tensor, function, family=family)


def audio(name, tensor, sample_rate, max_outputs, family=None, step=None):
  """Writes an audio summary if possible."""

  def function(tag, scope):
    # Note the identity to move the tensor to the CPU.
    return gen_summary_ops.write_audio_summary(
        context.context().summary_writer_resource,
        _choose_step(step),
        tag,
        array_ops.identity(tensor),
        sample_rate=sample_rate,
        max_outputs=max_outputs,
        name=scope)

  return summary_writer_function(name, tensor, function, family=family)


def graph(param, step=None, name=None):
  """Writes a TensorFlow graph to the summary interface.

  The graph summary is, strictly speaking, not a summary. Conditions
  like `tf.summary.should_record_summaries` do not apply. Only
  a single graph can be associated with a particular run. If multiple
  graphs are written, then only the last one will be considered by
  TensorBoard.

  When not using eager execution mode, the user should consider passing
  the `graph` parameter to `tf.contrib.summary.initialize` instead of
  calling this function. Otherwise special care needs to be taken when
  using the graph to record the graph.

  Args:
    param: A `tf.Tensor` containing a serialized graph proto. When
      eager execution is enabled, this function will automatically
      coerce `tf.Graph`, `tf.GraphDef`, and string types.
    step: The global step variable. This doesn't have useful semantics
      for graph summaries, but is used anyway, due to the structure of
      event log files. This defaults to the global step.
    name: A name for the operation (optional).

  Returns:
    The created `tf.Operation` or a `tf.no_op` if summary writing has
    not been enabled for this context.

  Raises:
    TypeError: If `param` isn't already a `tf.Tensor` in graph mode.
  """
  if not context.executing_eagerly() and not isinstance(param, ops.Tensor):
    raise TypeError("graph() needs a tf.Tensor (e.g. tf.placeholder) in graph "
                    "mode, but was: %s" % type(param))
  writer = context.context().summary_writer_resource
  if writer is None:
    return control_flow_ops.no_op()
  with ops.device("cpu:0"):
    if isinstance(param, (ops.Graph, graph_pb2.GraphDef)):
      tensor = ops.convert_to_tensor(_serialize_graph(param), dtypes.string)
    else:
      tensor = array_ops.identity(param)
    return gen_summary_ops.write_graph_summary(
        writer, _choose_step(step), tensor, name=name)


_graph = graph  # for functions with a graph parameter


@tf_export("summary.import_event", v1=[])
def import_event(tensor, name=None):
  """Writes a `tf.Event` binary proto.

  This can be used to import existing event logs into a new summary writer sink.
  Please note that this is lower level than the other summary functions and
  will ignore the `tf.summary.should_record_summaries` setting.

  Args:
    tensor: A `tf.Tensor` of type `string` containing a serialized
      `tf.Event` proto.
    name: A name for the operation (optional).

  Returns:
    The created `tf.Operation`.
  """
  return gen_summary_ops.import_event(
      context.context().summary_writer_resource, tensor, name=name)


@tf_export("summary.flush", v1=[])
def flush(writer=None, name=None):
  """Forces summary writer to send any buffered data to storage.

  This operation blocks until that finishes.

  Args:
    writer: The `tf.summary.SummaryWriter` resource to flush.
      The thread default will be used if this parameter is None.
      Otherwise a `tf.no_op` is returned.
    name: A name for the operation (optional).

  Returns:
    The created `tf.Operation`.
  """
  if writer is None:
    writer = context.context().summary_writer_resource
    if writer is None:
      return control_flow_ops.no_op()
  else:
    if isinstance(writer, SummaryWriter):
      writer = writer._resource  # pylint: disable=protected-access
  with ops.device("cpu:0"):
    return gen_summary_ops.flush_summary_writer(writer, name=name)


_flush_fn = flush  # for within SummaryWriter.flush()


def eval_dir(model_dir, name=None):
  """Construct a logdir for an eval summary writer."""
  return os.path.join(model_dir, "eval" if not name else "eval_" + name)


@deprecation.deprecated(date=None,
                        instructions="Renamed to create_file_writer().")
def create_summary_file_writer(*args, **kwargs):
  """Please use `tf.contrib.summary.create_file_writer`."""
  logging.warning("Deprecation Warning: create_summary_file_writer was renamed "
                  "to create_file_writer")
  return create_file_writer(*args, **kwargs)


def _serialize_graph(arbitrary_graph):
  if isinstance(arbitrary_graph, ops.Graph):
    return arbitrary_graph.as_graph_def(add_shapes=True).SerializeToString()
  else:
    return arbitrary_graph.SerializeToString()


def _choose_step(step):
  if step is None:
    return training_util.get_or_create_global_step()
  if not isinstance(step, ops.Tensor):
    return ops.convert_to_tensor(step, dtypes.int64)
  return step
