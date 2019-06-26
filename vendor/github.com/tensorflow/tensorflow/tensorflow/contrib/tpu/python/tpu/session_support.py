# Copyright 2017 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the 'License');
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an 'AS IS' BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ======================================
"""Operations for handling session logging and shutdown notifications."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import threading

import time
from google.protobuf import text_format

from tensorflow.contrib.tpu.python.ops import tpu_ops
from tensorflow.core.protobuf import config_pb2
from tensorflow.core.util import event_pb2
from tensorflow.python.client import session as session_lib
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.training import session_run_hook
from tensorflow.python.training import training_util

_WATCHDOG = None


class CoordinatorShutdownException(Exception):
  """Raised when the coordinator needs to shutdown."""
  pass


def _clone_session(session, graph=None):
  return session_lib.Session(
      target=session.sess_str,
      config=session._config,  # pylint: disable=protected-access
      graph=graph if graph else session.graph)


def _make_heartbeat_op(session, device, request_ph):
  """Return a heartbeat op or None if heartbeats are not supported by device."""
  try:
    # Test if we can connect in a isolated graph + session
    with ops.Graph().as_default():
      with _clone_session(session) as temp_session:
        with ops.device(device):
          heartbeat_op = tpu_ops.worker_heartbeat('')
          options = config_pb2.RunOptions(timeout_in_ms=5000)
          temp_session.run(heartbeat_op, options=options)
  except errors.InvalidArgumentError as _:
    logging.warning('Error running heartbeat on %s', device)
    return None
  except errors.DeadlineExceededError as _:
    logging.warning('Timeout connecting to %s when testing heartbeat', device)
    return None

  # If we successfully connected and pinged the worker, go ahead and construct
  # the operation.
  with ops.device(device):
    return tpu_ops.worker_heartbeat(request_ph)


class WorkerHeartbeatManager(object):
  """Manages the status/heartbeat monitor for a set of workers."""

  def __init__(self, session, devices, heartbeat_ops, request_placeholder):
    """Construct a new WorkerHeartbeatManager.

    (Prefer using `WorkerHeartbeatManager.from_devices` when possible.)

    Args:
      session: `tf.Session`, session to use for heartbeat operations.
      devices: `list[string]` Set of devices to connect to.
      heartbeat_ops: `list[tf.Operation]` Heartbeat operations.
      request_placeholder: `tf.Placeholder[String]` Placeholder used to specify
        the WorkerHeartbeatRequest protocol buffer.
    """
    self._session = session
    self._devices = devices
    self._ops = heartbeat_ops
    self._request_placeholder = request_placeholder

  @staticmethod
  def from_devices(session, devices):
    """Construct a heartbeat manager for the given devices."""
    if not devices:
      logging.error('Trying to create heartbeat manager with no devices?')

    logging.info('Creating heartbeat manager for %s', devices)
    request_placeholder = array_ops.placeholder(
        name='worker_heartbeat_request', dtype=dtypes.string)

    heartbeat_ops = []
    kept_devices = []
    for device in devices:
      heartbeat_op = _make_heartbeat_op(session, device, request_placeholder)
      if heartbeat_op is not None:
        kept_devices.append(device)
        heartbeat_ops.append(heartbeat_op)
      else:
        logging.warning('Heartbeat support not available for %s', device)

    return WorkerHeartbeatManager(session, kept_devices, heartbeat_ops,
                                  request_placeholder)

  def num_workers(self):
    return len(self._devices)

  def configure(self, message):
    """Configure heartbeat manager for all devices.

    Args:
      message: `event_pb2.WorkerHeartbeatRequest`
    Returns: `None`
    """
    logging.info('Configuring worker heartbeat: %s',
                 text_format.MessageToString(message))
    self._session.run(self._ops,
                      {self._request_placeholder: message.SerializeToString()})

  def ping(self, request=None, timeout_in_ms=5000):
    """Ping all workers, returning the parsed status results."""
    if request is None:
      request = event_pb2.WorkerHeartbeatRequest()

    options = config_pb2.RunOptions(timeout_in_ms=timeout_in_ms)
    results = self._session.run(
        self._ops,
        feed_dict={self._request_placeholder: request.SerializeToString()},
        options=options)
    parsed_results = [
        event_pb2.WorkerHeartbeatResponse.FromString(res_pb)
        for res_pb in results
    ]
    logging.debug('Ping results: %s', parsed_results)
    return parsed_results

  def lame_workers(self):
    """Ping all workers, returning manager containing lame workers (or None)."""
    ping_results = self.ping()
    lame_workers = []

    for ping_response, device, op in zip(ping_results, self._devices,
                                         self._ops):
      if ping_response.health_status != event_pb2.OK:
        lame_workers.append((device, op))

    if not lame_workers:
      return None

    bad_devices, bad_ops = zip(*lame_workers)
    return WorkerHeartbeatManager(self._session, bad_devices, bad_ops,
                                  self._request_placeholder)

  def __repr__(self):
    return 'HeartbeatManager(%s)' % ','.join(self._devices)

  def shutdown(self, timeout_ms=10000):
    """Shutdown all workers after `shutdown_timeout_secs`."""
    logging.info('Shutting down %s.', self)
    req = event_pb2.WorkerHeartbeatRequest(
        watchdog_config=event_pb2.WatchdogConfig(timeout_ms=timeout_ms))
    self.configure(req)

    # Wait for workers to shutdown.  This isn't strictly required
    # but it avoids triggering multiple checkpoints with the same lame worker.
    logging.info('Waiting %dms for worker shutdown.', timeout_ms)
    time.sleep(timeout_ms / 1000)


def all_worker_devices(session):
  """Return a list of devices for each worker in the system."""
  devices = session.list_devices()
  return [
      device.name for device in devices
      if ':CPU:' in device.name and 'coordinator' not in device.name
  ]


class WatchdogManager(threading.Thread):
  """Configures worker watchdog timer and handles periodic pings.

  Usage:
    # Ping workers every minute, shutting down workers if they haven't received
    # a ping after 1 hour.
    watchdog_manager = WatchdogManager(
      ping_interval=60, shutdown_timeout=3600
    )

    # Use as a context manager, resetting watchdog on context exit:
    with watchdog_manager:
      session.run(...)

    # Or setup globally; watchdog will remain active until program exit.
    watchdog_manager.configure_and_run()
  """

  def __init__(self,
               session,
               devices=None,
               ping_interval=60,
               shutdown_timeout=3600):
    """Initialize a watchdog manager.

    Args:
      session: Session connected to worker devices.  A cloned session and graph
        will be created for managing worker pings.
      devices: Set of devices to monitor.  If none, all workers will be
        monitored.
      ping_interval: Time, in seconds, between watchdog pings.
      shutdown_timeout: Time, in seconds, before watchdog timeout.
    """
    threading.Thread.__init__(self)
    self.ping_interval = ping_interval
    self.shutdown_timeout = shutdown_timeout
    self.daemon = True
    self._config = session._config  # pylint: disable=protected-access
    self._target = session.sess_str
    self._running = False
    self._devices = devices

    self._graph = None
    self._session = None
    self._worker_manager = None

  def _reset_manager(self):
    """Reset the graph, session and worker manager."""
    self._graph = ops.Graph()
    self._session = session_lib.Session(
        target=self._target,
        graph=self._graph,
        config=self._config,
    )

    if self._devices is None:
      self._devices = all_worker_devices(self._session)

    with self._graph.as_default():
      self._worker_manager = WorkerHeartbeatManager.from_devices(
          self._session, self._devices)

    self._worker_manager.configure(
        event_pb2.WorkerHeartbeatRequest(
            watchdog_config=event_pb2.WatchdogConfig(
                timeout_ms=self.shutdown_timeout * 1000,)))

  def configure_and_run(self):
    logging.info('Enabling watchdog timer with %d second timeout '
                 'and %d second ping interval.',
                 self.shutdown_timeout, self.ping_interval)
    self._reset_manager()
    self._running = True
    self.start()

  def stop(self):
    logging.info('Stopping worker watchdog.')
    self._worker_manager.configure(
        event_pb2.WorkerHeartbeatRequest(
            watchdog_config=event_pb2.WatchdogConfig(timeout_ms=-1,)))
    self._running = False
    self.join()

  def __enter__(self):
    self.configure_and_run()

  def __exit__(self, exc_type, exc_val, exc_tb):
    self.stop()

  def run(self):
    # Don't fetch logs or adjust timing: just ping the watchdog.
    #
    # If we hit an exception, reset our session as it is likely broken.
    while self._running:
      try:
        self._worker_manager.ping(request=None)
        time.sleep(self.ping_interval)
      except errors.OpError as e:
        # Catch any TF errors that occur so we don't stop sending heartbeats
        logging.debug('Caught error while sending heartbeat: %s', e)
        self._reset_manager()


def start_worker_watchdog(session,
                          devices=None,
                          ping_interval=60,
                          shutdown_timeout=3600):
  """Start global worker watchdog to shutdown workers on coordinator exit."""
  global _WATCHDOG
  if _WATCHDOG is None:
    # Ensure we can send a few pings before we timeout!
    ping_interval = min(shutdown_timeout / 10., ping_interval)
    _WATCHDOG = WatchdogManager(session, devices, ping_interval,
                                shutdown_timeout)
    _WATCHDOG.configure_and_run()


class GracefulShutdownHook(session_run_hook.SessionRunHook):
  """Session hook that watches for shutdown events.

  If a shutdown is indicated, `saver.save(checkpoint_prefix)` is executed, and a
  SystemShutdown exception is raised to terminate the main session.  If `saver`
  is None the `SAVERS` collection will be read to find a saver.

  `on_shutdown_hooks` is an optional list of functions that should be called
  after checkpointing.  The function is called with (`run_context`,
  `all_workers`, `lame_workers`).

  If `heartbeat_group` is not specified, it will default to all CPU workers
  in the system.
  """

  def __init__(self, checkpoint_prefix, saver=None, on_shutdown_hooks=None):
    self._saver = saver
    self._checkpoint_prefix = checkpoint_prefix
    self._on_shutdown_hooks = on_shutdown_hooks if on_shutdown_hooks else []

    # Worker heartbeats are managed independently of the main training graph.
    self._graph = ops.Graph()
    self._workers = None
    self._session = None
    self._heartbeat_supported = False

  def after_create_session(self, training_session, coord):  # pylint: disable=unused-argument
    # N.B. We have to pull the global step here to avoid it being unavailable
    # at checkpoint time; the graph has been frozen at that point.
    if training_util.get_global_step() is None and self.saver() is not None:
      raise ValueError(
          'Saver defined but no global step.  Run `get_or_create_global_step()`'
          ' in your model definition to allow checkpointing.')

    with self._graph.as_default():
      logging.info('Installing graceful shutdown hook.')
      self._session = _clone_session(training_session, self._graph)
      self._workers = WorkerHeartbeatManager.from_devices(
          self._session, all_worker_devices(self._session))
      self._heartbeat_supported = self._workers.num_workers() > 0
      if self._heartbeat_supported:
        self._workers.configure(
            event_pb2.WorkerHeartbeatRequest(
                shutdown_mode=event_pb2.WAIT_FOR_COORDINATOR))
      else:
        logging.warn(
            'No workers support hearbeats. Failure handling will be disabled.')

  def saver(self):
    if self._saver:
      return self._saver

    savers = ops.get_collection(ops.GraphKeys.SAVERS)
    if not savers:
      return None

    if not isinstance(savers, list):
      return savers

    if len(savers) > 1:
      logging.error(
          'Multiple savers in the SAVERS collection.  On-demand checkpointing '
          'will be disabled. Pass an explicit `saver` to the constructor to '
          'override this behavior.')
      return None

    return savers[0]

  def after_run(self, run_context, run_values):
    del run_values

    if not self._heartbeat_supported:
      return

    lame_workers = self._workers.lame_workers()
    if lame_workers:
      logging.info('ShutdownHook: lame workers found: %s', lame_workers)

      if self.saver():
        logging.info('ShutdownHook: saving checkpoint to %s',
                     self._checkpoint_prefix)
        self.saver().save(
            run_context.session,
            self._checkpoint_prefix,
            global_step=training_util.get_global_step(),
            write_state=True,
        )
      else:
        logging.info('ShutdownHook: no Saver defined.')

      for fn in self._on_shutdown_hooks:
        fn(run_context, self._workers, lame_workers)


class RestartComputation(object):
  """Restart the entire computation.

  This hook shuts down all workers and returns control to the top-level by
  throwing a CoordinatorShutdownException.
  """

  def __init__(self, timeout_ms=10000):
    self.timeout_ms = timeout_ms

  def __call__(self, run_context, all_workers, lame_workers):
    del run_context, lame_workers
    all_workers.shutdown(timeout_ms=self.timeout_ms)

    logging.info('Terminating coordinator.')
    raise CoordinatorShutdownException()


class ShutdownLameWorkers(object):
  """Shutdown lamed workers.

  Processing will continue normally (typically by waiting for the down
  workers to be restarted).
  """

  def __init__(self, timeout_ms=10000):
    self.timeout_in_ms = timeout_ms

  def __call__(self, run_context, all_workers, lame_workers):
    lame_workers.shutdown(timeout_ms=self.timeout_in_ms)
