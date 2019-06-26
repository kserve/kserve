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
"""Timeline visualization for TensorFlow using Chrome Trace Format."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections
import copy
import json
import re

# The timeline target is usually imported as part of BUILD target
# "platform_test", which includes also includes the "platform"
# dependency.  This is why the logging import here is okay.
from tensorflow.python.platform import tf_logging as logging


class AllocationMaximum(collections.namedtuple(
    'AllocationMaximum', ('timestamp', 'num_bytes', 'tensors'))):
  """Stores the maximum allocation for a given allocator within the timelne.

  Parameters:
    timestamp: `tensorflow::Env::NowMicros()` when this maximum was reached.
    num_bytes: the total memory used at this time.
    tensors: the set of tensors allocated at this time.
  """
  pass


class StepStatsAnalysis(collections.namedtuple(
    'StepStatsAnalysis', ('chrome_trace', 'allocator_maximums'))):
  """Stores the step stats analysis output.

  Parameters:
    chrome_trace: A dict containing the chrome trace analysis.
    allocator_maximums: A dict mapping allocator names to AllocationMaximum.
  """
  pass


class _ChromeTraceFormatter(object):
  """A helper class for generating traces in Chrome Trace Format."""

  def __init__(self, show_memory=False):
    """Constructs a new Chrome Trace formatter."""
    self._show_memory = show_memory
    self._events = []
    self._metadata = []

  def _create_event(self, ph, category, name, pid, tid, timestamp):
    """Creates a new Chrome Trace event.

    For details of the file format, see:
    https://github.com/catapult-project/catapult/blob/master/tracing/README.md

    Args:
      ph:  The type of event - usually a single character.
      category: The event category as a string.
      name:  The event name as a string.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      timestamp:  The timestamp of this event as a long integer.

    Returns:
      A JSON compatible event object.
    """
    event = {}
    event['ph'] = ph
    event['cat'] = category
    event['name'] = name
    event['pid'] = pid
    event['tid'] = tid
    event['ts'] = timestamp
    return event

  def emit_pid(self, name, pid):
    """Adds a process metadata event to the trace.

    Args:
      name:  The process name as a string.
      pid:  Identifier of the process as an integer.
    """
    event = {}
    event['name'] = 'process_name'
    event['ph'] = 'M'
    event['pid'] = pid
    event['args'] = {'name': name}
    self._metadata.append(event)

  def emit_tid(self, name, pid, tid):
    """Adds a thread metadata event to the trace.

    Args:
      name:  The thread name as a string.
      pid:  Identifier of the process as an integer.
      tid:  Identifier of the thread as an integer.
    """
    event = {}
    event['name'] = 'thread_name'
    event['ph'] = 'M'
    event['pid'] = pid
    event['tid'] = tid
    event['args'] = {'name': name}
    self._metadata.append(event)

  def emit_region(self, timestamp, duration, pid, tid, category, name, args):
    """Adds a region event to the trace.

    Args:
      timestamp:  The start timestamp of this region as a long integer.
      duration:  The duration of this region as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      category: The event category as a string.
      name:  The event name as a string.
      args:  A JSON-compatible dictionary of event arguments.
    """
    event = self._create_event('X', category, name, pid, tid, timestamp)
    event['dur'] = duration
    event['args'] = args
    self._events.append(event)

  def emit_obj_create(self, category, name, timestamp, pid, tid, object_id):
    """Adds an object creation event to the trace.

    Args:
      category: The event category as a string.
      name:  The event name as a string.
      timestamp:  The timestamp of this event as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      object_id: Identifier of the object as an integer.
    """
    event = self._create_event('N', category, name, pid, tid, timestamp)
    event['id'] = object_id
    self._events.append(event)

  def emit_obj_delete(self, category, name, timestamp, pid, tid, object_id):
    """Adds an object deletion event to the trace.

    Args:
      category: The event category as a string.
      name:  The event name as a string.
      timestamp:  The timestamp of this event as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      object_id: Identifier of the object as an integer.
    """
    event = self._create_event('D', category, name, pid, tid, timestamp)
    event['id'] = object_id
    self._events.append(event)

  def emit_obj_snapshot(self, category, name, timestamp, pid, tid, object_id,
                        snapshot):
    """Adds an object snapshot event to the trace.

    Args:
      category: The event category as a string.
      name:  The event name as a string.
      timestamp:  The timestamp of this event as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      object_id: Identifier of the object as an integer.
      snapshot:  A JSON-compatible representation of the object.
    """
    event = self._create_event('O', category, name, pid, tid, timestamp)
    event['id'] = object_id
    event['args'] = {'snapshot': snapshot}
    self._events.append(event)

  def emit_flow_start(self, name, timestamp, pid, tid, flow_id):
    """Adds a flow start event to the trace.

    When matched with a flow end event (with the same 'flow_id') this will
    cause the trace viewer to draw an arrow between the start and end events.

    Args:
      name:  The event name as a string.
      timestamp:  The timestamp of this event as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      flow_id: Identifier of the flow as an integer.
    """
    event = self._create_event('s', 'DataFlow', name, pid, tid, timestamp)
    event['id'] = flow_id
    self._events.append(event)

  def emit_flow_end(self, name, timestamp, pid, tid, flow_id):
    """Adds a flow end event to the trace.

    When matched with a flow start event (with the same 'flow_id') this will
    cause the trace viewer to draw an arrow between the start and end events.

    Args:
      name:  The event name as a string.
      timestamp:  The timestamp of this event as a long integer.
      pid:  Identifier of the process generating this event as an integer.
      tid:  Identifier of the thread generating this event as an integer.
      flow_id: Identifier of the flow as an integer.
    """
    event = self._create_event('t', 'DataFlow', name, pid, tid, timestamp)
    event['id'] = flow_id
    self._events.append(event)

  def emit_counter(self, category, name, pid, timestamp, counter, value):
    """Emits a record for a single counter.

    Args:
      category: The event category as a string.
      name:  The event name as a string.
      pid:  Identifier of the process generating this event as an integer.
      timestamp:  The timestamp of this event as a long integer.
      counter: Name of the counter as a string.
      value:  Value of the counter as an integer.
    """
    event = self._create_event('C', category, name, pid, 0, timestamp)
    event['args'] = {counter: value}
    self._events.append(event)

  def emit_counters(self, category, name, pid, timestamp, counters):
    """Emits a counter record for the dictionary 'counters'.

    Args:
      category: The event category as a string.
      name:  The event name as a string.
      pid:  Identifier of the process generating this event as an integer.
      timestamp:  The timestamp of this event as a long integer.
      counters: Dictionary of counter values.
    """
    event = self._create_event('C', category, name, pid, 0, timestamp)
    event['args'] = counters.copy()
    self._events.append(event)

  def format_to_string(self, pretty=False):
    """Formats the chrome trace to a string.

    Args:
      pretty: (Optional.)  If True, produce human-readable JSON output.

    Returns:
      A JSON-formatted string in Chrome Trace format.
    """
    trace = {}
    trace['traceEvents'] = self._metadata + self._events
    if pretty:
      return json.dumps(trace, indent=4, separators=(',', ': '))
    else:
      return json.dumps(trace, separators=(',', ':'))


class _TensorTracker(object):
  """An internal class to track the lifetime of a Tensor."""

  def __init__(self, name, object_id, timestamp, pid, allocator, num_bytes):
    """Creates an object to track tensor references.

    This class is not thread safe and is intended only for internal use by
    the 'Timeline' class in this file.

    Args:
      name:  The name of the Tensor as a string.
      object_id:  Chrome Trace object identifier assigned for this Tensor.
      timestamp:  The creation timestamp of this event as a long integer.
      pid:  Process identifier of the associated device, as an integer.
      allocator:  Name of the allocator used to create the Tensor.
      num_bytes:  Number of bytes allocated (long integer).

    Returns:
      A 'TensorTracker' object.
    """
    self._name = name
    self._pid = pid
    self._object_id = object_id
    self._create_time = timestamp
    self._allocator = allocator
    self._num_bytes = num_bytes
    self._ref_times = []
    self._unref_times = []

  @property
  def name(self):
    """Name of this tensor."""
    return self._name

  @property
  def pid(self):
    """ID of the process which created this tensor (an integer)."""
    return self._pid

  @property
  def create_time(self):
    """Timestamp when this tensor was created (long integer)."""
    return self._create_time

  @property
  def object_id(self):
    """Returns the object identifier of this tensor (integer)."""
    return self._object_id

  @property
  def num_bytes(self):
    """Size of this tensor in bytes (long integer)."""
    return self._num_bytes

  @property
  def allocator(self):
    """Name of the allocator used to create this tensor (string)."""
    return self._allocator

  @property
  def last_unref(self):
    """Last unreference timestamp of this tensor (long integer)."""
    return max(self._unref_times)

  def add_ref(self, timestamp):
    """Adds a reference to this tensor with the specified timestamp.

    Args:
      timestamp:  Timestamp of object reference as an integer.
    """
    self._ref_times.append(timestamp)

  def add_unref(self, timestamp):
    """Adds an unref to this tensor with the specified timestamp.

    Args:
      timestamp:  Timestamp of object unreference as an integer.
    """
    self._unref_times.append(timestamp)


class Timeline(object):
  """A class for visualizing execution timelines of TensorFlow steps."""

  def __init__(self, step_stats, graph=None):
    """Constructs a new Timeline.

    A 'Timeline' is used for visualizing the execution of a TensorFlow
    computation.  It shows the timings and concurrency of execution at
    the granularity of TensorFlow Ops.
    This class is not thread safe.

    Args:
      step_stats: The 'StepStats' proto recording execution times.
      graph: (Optional) The 'Graph' that was executed.
    """

    self._step_stats = step_stats
    self._graph = graph
    self._chrome_trace = _ChromeTraceFormatter()
    self._next_pid = 0
    self._device_pids = {}  # device name -> pid for compute activity.
    self._tensor_pids = {}  # device name -> pid for tensors.
    self._tensors = {}  # tensor_name -> TensorTracker
    self._next_flow_id = 0
    self._flow_starts = {}  # tensor_name -> (timestamp, pid, tid)
    self._alloc_times = {}  # tensor_name -> ( time, allocator, size )
    self._allocator_maximums = {}  # allocator name => maximum bytes long

  def _alloc_pid(self):
    """Allocate a process Id."""
    pid = self._next_pid
    self._next_pid += 1
    return pid

  def _alloc_flow_id(self):
    """Allocate a flow Id."""
    flow_id = self._next_flow_id
    self._next_flow_id += 1
    return flow_id

  def _parse_op_label(self, label):
    """Parses the fields in a node timeline label."""
    # Expects labels of the form: name = op(arg, arg, ...).
    match = re.match(r'(.*) = (.*)\((.*)\)', label)
    if match is None:
      return 'unknown', 'unknown', []
    nn, op, inputs = match.groups()
    if not inputs:
      inputs = []
    else:
      inputs = inputs.split(', ')
    return nn, op, inputs

  def _assign_lanes(self):
    """Assigns non-overlapping lanes for the activities on each device."""
    for device_stats in self._step_stats.dev_stats:
      # TODO(pbar): Genuine thread IDs in NodeExecStats might be helpful.
      lanes = [0]
      for ns in device_stats.node_stats:
        l = -1
        for (i, lts) in enumerate(lanes):
          if ns.all_start_micros > lts:
            l = i
            lanes[l] = ns.all_start_micros + ns.all_end_rel_micros
            break
        if l < 0:
          l = len(lanes)
          lanes.append(ns.all_start_micros + ns.all_end_rel_micros)
        ns.thread_id = l

  def _emit_op(self, nodestats, pid, is_gputrace):
    """Generates a Chrome Trace event to show Op execution.

    Args:
      nodestats: The 'NodeExecStats' proto recording op execution.
      pid: The pid assigned for the device where this op ran.
      is_gputrace: If True then this op came from the GPUTracer.
    """
    node_name = nodestats.node_name
    start = nodestats.all_start_micros
    duration = nodestats.all_end_rel_micros
    tid = nodestats.thread_id
    inputs = []
    if is_gputrace:
      # Node names should always have the form 'name:op'.
      fields = node_name.split(':') + ['unknown']
      node_name, op = fields[:2]
    elif node_name == 'RecvTensor':
      # RPC tracing does not use the standard timeline_label format.
      op = 'RecvTensor'
    else:
      _, op, inputs = self._parse_op_label(nodestats.timeline_label)
    args = {'name': node_name, 'op': op}
    for i, iname in enumerate(inputs):
      args['input%d' % i] = iname
    self._chrome_trace.emit_region(start, duration, pid, tid, 'Op', op, args)

  def _emit_tensor_snapshot(self, tensor, timestamp, pid, tid, value):
    """Generate Chrome Trace snapshot event for a computed Tensor.

    Args:
      tensor: A 'TensorTracker' object.
      timestamp:  The timestamp of this snapshot as a long integer.
      pid: The pid assigned for showing the device where this op ran.
      tid: The tid of the thread computing the tensor snapshot.
      value: A JSON-compliant snapshot of the object.
    """
    desc = str(value.tensor_description).replace('"', '')
    snapshot = {'tensor_description': desc}
    self._chrome_trace.emit_obj_snapshot('Tensor', tensor.name, timestamp, pid,
                                         tid, tensor.object_id, snapshot)

  def _produce_tensor(self, name, timestamp, tensors_pid, allocator, num_bytes):
    object_id = len(self._tensors)
    tensor = _TensorTracker(name, object_id, timestamp, tensors_pid, allocator,
                            num_bytes)
    self._tensors[name] = tensor
    return tensor

  def _is_gputrace_device(self, device_name):
    """Returns true if this device is part of the GPUTracer logging."""
    return '/stream:' in device_name or '/memcpy' in device_name

  def _allocate_pids(self):
    """Allocate fake process ids for each device in the StepStats."""
    self._allocators_pid = self._alloc_pid()
    self._chrome_trace.emit_pid('Allocators', self._allocators_pid)

    # Add processes in the Chrome trace to show compute and data activity.
    for dev_stats in self._step_stats.dev_stats:
      device_pid = self._alloc_pid()
      self._device_pids[dev_stats.device] = device_pid
      tensors_pid = self._alloc_pid()
      self._tensor_pids[dev_stats.device] = tensors_pid
      self._chrome_trace.emit_pid(dev_stats.device + ' Compute', device_pid)
      self._chrome_trace.emit_pid(dev_stats.device + ' Tensors', tensors_pid)

  def _analyze_tensors(self, show_memory):
    """Analyze tensor references to track dataflow."""
    for dev_stats in self._step_stats.dev_stats:
      device_pid = self._device_pids[dev_stats.device]
      tensors_pid = self._tensor_pids[dev_stats.device]
      for node_stats in dev_stats.node_stats:
        tid = node_stats.thread_id
        node_name = node_stats.node_name
        start_time = node_stats.all_start_micros
        end_time = node_stats.all_start_micros + node_stats.all_end_rel_micros
        for index, output in enumerate(node_stats.output):
          if index:
            output_name = '%s:%d' % (node_name, index)
          else:
            output_name = node_name

          allocation = output.tensor_description.allocation_description
          num_bytes = allocation.requested_bytes
          allocator_name = allocation.allocator_name
          tensor = self._produce_tensor(output_name, start_time, tensors_pid,
                                        allocator_name, num_bytes)
          tensor.add_ref(start_time)
          tensor.add_unref(end_time)
          self._flow_starts[output_name] = (end_time, device_pid, tid)

          if show_memory:
            self._chrome_trace.emit_obj_create('Tensor', output_name,
                                               start_time, tensors_pid, tid,
                                               tensor.object_id)
            self._emit_tensor_snapshot(tensor, end_time - 1, tensors_pid, tid,
                                       output)

  def _show_compute(self, show_dataflow):
    """Visualize the computation activity."""
    for dev_stats in self._step_stats.dev_stats:
      device_name = dev_stats.device
      device_pid = self._device_pids[device_name]
      is_gputrace = self._is_gputrace_device(device_name)

      for node_stats in dev_stats.node_stats:
        tid = node_stats.thread_id
        start_time = node_stats.all_start_micros
        end_time = node_stats.all_start_micros + node_stats.all_end_rel_micros
        self._emit_op(node_stats, device_pid, is_gputrace)

        if is_gputrace or node_stats.node_name == 'RecvTensor':
          continue

        _, _, inputs = self._parse_op_label(node_stats.timeline_label)
        for input_name in inputs:
          if input_name not in self._tensors:
            # This can happen when partitioning has inserted a Send/Recv.
            # We remove the numeric suffix so that the dataflow appears to
            # come from the original node.  Ideally, the StepStats would
            # contain logging for the Send and Recv nodes.
            index = input_name.rfind('/_')
            if index > 0:
              input_name = input_name[:index]

          if input_name in self._tensors:
            tensor = self._tensors[input_name]
            tensor.add_ref(start_time)
            tensor.add_unref(end_time - 1)

            if show_dataflow:
              # We use a different flow ID for every graph edge.
              create_time, create_pid, create_tid = self._flow_starts[
                  input_name]
              # Don't add flows when producer and consumer ops are on the same
              # pid/tid since the horizontal arrows clutter the visualization.
              if create_pid != device_pid or create_tid != tid:
                flow_id = self._alloc_flow_id()
                self._chrome_trace.emit_flow_start(input_name, create_time,
                                                   create_pid, create_tid,
                                                   flow_id)
                self._chrome_trace.emit_flow_end(input_name, start_time,
                                                 device_pid, tid, flow_id)
          else:
            logging.vlog(1, 'Can\'t find tensor %s - removed by CSE?',
                         input_name)

  def _show_memory_counters(self):
    """Produce a counter series for each memory allocator."""
    # Iterate over all tensor trackers to build a list of allocations and
    # frees for each allocator. Then sort the lists and emit a cumulative
    # counter series for each allocator.
    allocations = {}
    for name in self._tensors:
      tensor = self._tensors[name]
      self._chrome_trace.emit_obj_delete('Tensor', name, tensor.last_unref,
                                         tensor.pid, 0, tensor.object_id)
      allocator = tensor.allocator
      if allocator not in allocations:
        allocations[allocator] = []
      num_bytes = tensor.num_bytes
      allocations[allocator].append((tensor.create_time, num_bytes, name))
      allocations[allocator].append((tensor.last_unref, -num_bytes, name))

    alloc_maxes = {}

    # Generate a counter series showing total allocations for each allocator.
    for allocator in allocations:
      alloc_list = allocations[allocator]
      alloc_list.sort()
      total_bytes = 0
      alloc_tensor_set = set()
      alloc_maxes[allocator] = AllocationMaximum(
          timestamp=0, num_bytes=0, tensors=set())
      for time, num_bytes, name in sorted(
          alloc_list, key=lambda allocation: allocation[0]):
        total_bytes += num_bytes
        if num_bytes < 0:
          alloc_tensor_set.discard(name)
        else:
          alloc_tensor_set.add(name)

        if total_bytes > alloc_maxes[allocator].num_bytes:
          alloc_maxes[allocator] = AllocationMaximum(
              timestamp=time,
              num_bytes=total_bytes,
              tensors=copy.deepcopy(alloc_tensor_set))

        self._chrome_trace.emit_counter('Memory', allocator,
                                        self._allocators_pid, time, allocator,
                                        total_bytes)
    self._allocator_maximums = alloc_maxes

  def analyze_step_stats(self, show_dataflow=True, show_memory=True):
    self._allocate_pids()
    self._assign_lanes()
    self._analyze_tensors(show_memory)
    self._show_compute(show_dataflow)
    if show_memory:
      self._show_memory_counters()
    return StepStatsAnalysis(
        chrome_trace=self._chrome_trace,
        allocator_maximums=self._allocator_maximums)

  def generate_chrome_trace_format(self, show_dataflow=True, show_memory=False):
    """Produces a trace in Chrome Trace Format.

    Args:
      show_dataflow: (Optional.) If True, add flow events to the trace
        connecting producers and consumers of tensors.
      show_memory: (Optional.) If True, add object snapshot events to the trace
        showing the sizes and lifetimes of tensors.

    Returns:
      A JSON formatted string in Chrome Trace format.
    """
    step_stats_analysis = self.analyze_step_stats(
        show_dataflow=show_dataflow, show_memory=show_memory)

    return step_stats_analysis.chrome_trace.format_to_string(pretty=True)
