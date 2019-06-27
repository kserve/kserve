# Copyright 2018 The TensorFlow Authors. All Rights Reserved.
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
# ===================================================================
"""TPU system metadata and associated tooling."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from contextlib import contextmanager
import copy

from tensorflow.contrib.tpu.python.tpu import device_assignment  as tpu_device_assignment
from tensorflow.contrib.tpu.python.tpu import tpu_config
from tensorflow.contrib.tpu.python.tpu import tpu_system_metadata as tpu_system_metadata_lib
from tensorflow.python.estimator import model_fn as model_fn_lib
from tensorflow.python.platform import tf_logging as logging


_DEFAULT_JOB_NAME = 'tpu_worker'
_DEFAULT_COORDINATOR_JOB_NAME = 'coordinator'
_LOCAL_MASTERS = ('', 'local')
_NUM_CORES_TO_COMPUTATION_SHAPE = {
    1: [1, 1, 1],
    2: [1, 1, 2],
    4: [1, 2, 2],
    8: [2, 2, 2],
    16: [4, 2, 2],
}


class TPUContext(object):
  """A context that holds the current configuration of the TPU computation."""

  def __init__(self,
               internal_ctx,
               input_device=None,
               invocation_index=None,
               call_from_input_fn=True):
    self._internal_ctx = internal_ctx
    self._input_device = input_device
    self._invocation_index = invocation_index
    self._call_from_input_fn = call_from_input_fn

  def current_input_fn_deployment(self):
    """The configuration of the current input_fn invocation.

    The configuration depends on `TPUConfig.per_host_input_for_training`. See
    `TPUConfig` for details.

    Only set in params dict of input_fn

    Returns:
      A tuple of
        1. Device spec string: String, is the current CPU host where the
           input_fn is invoked.
        2. Current invocation index: Int, 0-based index of the input_fn
           invocation. See next item for details.
        3. Total invocation count: Int, the total number of times to invoke the
           input_fn on all CPU hosts. Each invocation will be passed with a new
           `TPUContext` instance with current invocation index set properly.
        4. Total number of replicas consumed by current_invocation: Int, the
           number of replicas fed by the data returned by current input_fn. For
           example, for per_core input pipeline deployment
           and non-model-parallelism, total invocation count is equal to
           the number of cores in the system and num replicas consumed by
           current invocation is 1. For per-host v2 input pipeline deployment,
           total invocation count is equal to the number of hosts in the system
           and num replicas consumed by current invocation is equal to number of
           cores per host.

    Raises:
      RuntimeError: If this method must not be called from input_fn.
    """
    if not self._call_from_input_fn:
      raise RuntimeError('This TPUContext instance must not be called from'
                         ' model_fn.')

    if self._internal_ctx.is_input_sharded_per_core():
      total_invocation_count = (self._internal_ctx.num_hosts
                                * self._internal_ctx.num_of_replicas_per_host)
      replicas_consumed = 1
    elif self._internal_ctx.is_input_broadcast_with_iterators():
      total_invocation_count = 1
      replicas_consumed = self._internal_ctx.num_replicas
    else:
      total_invocation_count = self._internal_ctx.num_hosts
      replicas_consumed = self._internal_ctx.num_of_replicas_per_host
    return (self._input_device, self._invocation_index,
            total_invocation_count, replicas_consumed)

  @property
  def num_replicas(self):
    """The total number of replicas.

    For non-model-parallelism, num_replicas should be the total num of TPU
    cores in the system.

    Returns:
      The number of replicas.
    """
    return self._internal_ctx.num_replicas

  @property
  def num_hosts(self):
    """The number of hosts for the TPU system."""
    return self._internal_ctx.num_hosts

  @property
  def current_host(self):
    """The current host index for the TPU system."""
    return self._invocation_index

  @property
  def num_of_replicas_per_host(self):
    """The number of replicas for each host."""
    if self._internal_ctx.model_parallelism_enabled:
      raise ValueError(
          'num_of_replicas_per_host is not supported for model_parallelism')
    return self._internal_ctx.num_of_replicas_per_host

  @property
  def device_assignment(self):
    """Returns device_assignment object."""
    if self._call_from_input_fn:
      raise RuntimeError('This TPUContext instance must not be called from'
                         ' input_fn.')
    return self._internal_ctx.device_assignment

  def device_for_replica(self, replica_id):
    """Returns the tuple of (CPU device and device ordinal) for replica.

    This should be used for full replicate for non-model-parallelism.

    Args:
       replica_id: Int, the replica index.

    Returns:
       A tuple of device spec for CPU device and int device ordinal.
    """
    # Note that: For the non-model parallelism, the mapping could be
    # a random permutation. The order should not matter in most cases
    # as far as model is replicated to all cores in the system.
    return self._internal_ctx.device_for_replica(replica_id)

  @property
  def tpu_host_placement_function(self):
    """Returns the TPU host place function.

    The place function takes host_id as the input and returns the TF device
    for the correspoding host.
    """

    def _placement_function(host_id):
      """Return the host device given host_id."""
      return self._internal_ctx.tpu_host_placement_function(host_id=host_id)

    return _placement_function


class _InternalTPUContext(object):
  """A context holds immutable states of TPU computation.

  This immutable object holds TPUEstimator config, train/eval batch size, and
  `TPUEstimator.use_tpu`, which is expected to be passed around. It also
  provides utility functions, based on the current state, to determine other
  information commonly required by TPU computation, such as TPU device names,
  TPU hosts, shard batch size, etc.

  if eval_on_tpu is False, then execution of eval on TPU is disabled.
  if eval_on_tpu is True, but use_tpu is False, a warning is issued,
  and TPU execution is disabled for all modes.

  N.B. As `mode` is not immutable state in Estimator, but essential to
  distinguish between TPU training and evaluation, a common usage for
  _InternalTPUContext with `mode` is as follows:
  ```
  with _ctx.with_mode(mode) as ctx:
    if ctx.is_running_on_cpu():
       ...
  ```
  """

  def __init__(self, config, train_batch_size, eval_batch_size,
               predict_batch_size, use_tpu, eval_on_tpu=True):
    self._config = config
    self._train_batch_size = train_batch_size
    self._eval_batch_size = eval_batch_size
    self._predict_batch_size = predict_batch_size
    self._use_tpu = use_tpu
    logging.info('_TPUContext: eval_on_tpu %s', eval_on_tpu)
    if not use_tpu and eval_on_tpu:
      logging.warning('eval_on_tpu ignored because use_tpu is False.')

    self._eval_on_tpu = eval_on_tpu
    self._model_parallelism_enabled = (
        use_tpu and config.tpu_config.num_cores_per_replica)
    self._mode = None
    num_cores_per_replica = config.tpu_config.num_cores_per_replica
    if num_cores_per_replica:
      self._computation_shape = _NUM_CORES_TO_COMPUTATION_SHAPE[
          num_cores_per_replica]
    else:
      self._computation_shape = None
    self._lazy_tpu_system_metadata_dict = {}  # key by master address
    self._lazy_device_assignment_dict = {}  # key by master address
    self._lazy_validation_dict = {}  # key by ModeKeys

  def _assert_mode(self):
    if self._mode is None:
      raise RuntimeError(
          '`mode` needs to be set via contextmanager `with_mode`.')
    return self._mode

  @contextmanager
  def with_mode(self, mode):
    # NOTE(xiejw): Shallow copy is enough. It will share he lazy dictionaries,
    # such as _lazy_tpu_system_metadata_dict between new copy and the original
    # one. Note that all lazy states stored in properties _lazy_foo are sort of
    # immutable as they should be same for the process lifetime.
    new_ctx = copy.copy(self)
    new_ctx._mode = mode  # pylint: disable=protected-access
    yield new_ctx

  @property
  def mode(self):
    return self._assert_mode()

  def _get_master_address(self):
    mode = self._assert_mode()
    config = self._config
    master = (
        config.master
        if mode != model_fn_lib.ModeKeys.EVAL else config.evaluation_master)
    return master

  def _get_tpu_system_metadata(self):
    """Gets the (maybe cached) TPU system metadata."""
    master = self._get_master_address()
    tpu_system_metadata = self._lazy_tpu_system_metadata_dict.get(master)
    if tpu_system_metadata is not None:
      return tpu_system_metadata

    cluster_def = None
    if (self._config.session_config and
        self._config.session_config.cluster_def.job):
      cluster_def = self._config.session_config.cluster_def

    # pylint: disable=protected-access
    tpu_system_metadata = (
        tpu_system_metadata_lib._query_tpu_system_metadata(
            master,
            cluster_def=cluster_def,
            query_topology=self.model_parallelism_enabled))

    self._lazy_tpu_system_metadata_dict[master] = tpu_system_metadata
    return tpu_system_metadata

  def _get_device_assignment(self):
    """Gets the (maybe cached) TPU device assignment."""
    master = self._get_master_address()
    device_assignment = self._lazy_device_assignment_dict.get(master)
    if device_assignment is not None:
      return device_assignment

    tpu_system_metadata = self._get_tpu_system_metadata()

    device_assignment = tpu_device_assignment.device_assignment(
        tpu_system_metadata.topology,
        computation_shape=self._computation_shape,
        num_replicas=self.num_replicas)

    logging.info('num_cores_per_replica: %s',
                 str(self._config.tpu_config.num_cores_per_replica))
    logging.info('computation_shape: %s', str(self._computation_shape))
    logging.info('num_replicas: %d', self.num_replicas)
    logging.info('device_assignment.topology.device_coordinates: %s',
                 str(device_assignment.topology.device_coordinates))
    logging.info('device_assignment.core_assignment: %s',
                 str(device_assignment.core_assignment))

    self._lazy_device_assignment_dict[master] = device_assignment
    return device_assignment

  @property
  def model_parallelism_enabled(self):
    return self._model_parallelism_enabled

  @property
  def input_partition_dims(self):
    return self._config.tpu_config.input_partition_dims

  @property
  def device_assignment(self):
    return (self._get_device_assignment()
            if self._model_parallelism_enabled else None)

  @property
  def num_of_cores_per_host(self):
    metadata = self._get_tpu_system_metadata()
    return metadata.num_of_cores_per_host

  @property
  def num_cores(self):
    metadata = self._get_tpu_system_metadata()
    return metadata.num_cores

  @property
  def num_of_replicas_per_host(self):
    """Return the number of replicas per host."""
    if self.model_parallelism_enabled:
      return self.num_replicas // self.num_hosts
    else:
      return self.num_of_cores_per_host

  @property
  def num_replicas(self):
    num_cores_in_system = self.num_cores

    if self.model_parallelism_enabled:
      num_cores_per_replica = self._config.tpu_config.num_cores_per_replica
      if num_cores_per_replica > num_cores_in_system:
        raise ValueError(
            'The num of cores required by the model parallelism, specified by '
            'TPUConfig.num_cores_per_replica, is larger than the total num of '
            'TPU cores in the system. num_cores_per_replica: {}, num cores '
            'in the system: {}'.format(num_cores_per_replica,
                                       num_cores_in_system))

      if num_cores_in_system % num_cores_per_replica != 0:
        raise RuntimeError(
            'The num of cores in the system ({}) is not divisible by the num '
            'of cores ({}) required by the model parallelism, specified by '
            'TPUConfig.num_cores_per_replica. This should never happen!'.format(
                num_cores_in_system, num_cores_per_replica))

      return num_cores_in_system // num_cores_per_replica
    else:
      return num_cores_in_system

  @property
  def num_hosts(self):
    metadata = self._get_tpu_system_metadata()
    return metadata.num_hosts

  @property
  def config(self):
    return self._config

  def is_input_sharded_per_core(self):
    """Return true if input_fn is invoked per-core (other than per-host)."""
    mode = self._assert_mode()
    return (mode == model_fn_lib.ModeKeys.TRAIN and
            (self._config.tpu_config.per_host_input_for_training is
             tpu_config.InputPipelineConfig.PER_SHARD_V1))

  def is_input_per_host_with_iterators(self):
    """Return true if input_fn should be run in the per-host v2 config."""
    return (self._config.tpu_config.per_host_input_for_training is
            tpu_config.InputPipelineConfig.PER_HOST_V2)

  def is_input_broadcast_with_iterators(self):
    """Return true if input_fn should be run in the full_replicae config."""
    return (self._config.tpu_config.per_host_input_for_training is
            tpu_config.InputPipelineConfig.BROADCAST)

  def is_running_on_cpu(self, is_export_mode=False):
    """Determines whether the input_fn and model_fn should be invoked on CPU.

    This API also validates user provided configuration, such as batch size,
    according the lazy initialized TPU system metadata.

    Args:
      is_export_mode: Indicates whether the current mode is for exporting the
        model, when mode == PREDICT. Only with this bool, we could
        tell whether user is calling the Estimator.predict or
        Estimator.export_savedmodel, which are running on TPU and CPU
        respectively. Parent class Estimator does not distinguish these two.

    Returns:
      bool, whether current input_fn or model_fn should be running on CPU.

    Raises:
      ValueError: any configuration is invalid.
    """

    is_running_on_cpu = self._is_running_on_cpu(is_export_mode)
    if not is_running_on_cpu:
      self._validate_tpu_configuration()
    return is_running_on_cpu

  def _is_running_on_cpu(self, is_export_mode):
    """Determines whether the input_fn and model_fn should be invoked on CPU."""
    mode = self._assert_mode()

    if not self._use_tpu:
      return True

    if mode == model_fn_lib.ModeKeys.EVAL and not self._eval_on_tpu:
      logging.info('_is_running_on_cpu: eval_on_tpu disabled')
      return True

    if is_export_mode:
      return True

    return False

  @property
  def global_batch_size(self):
    mode = self._assert_mode()
    if mode == model_fn_lib.ModeKeys.TRAIN:
      return self._train_batch_size
    elif mode == model_fn_lib.ModeKeys.EVAL:
      return self._eval_batch_size
    elif mode == model_fn_lib.ModeKeys.PREDICT:
      return self._predict_batch_size
    else:
      return None

  @property
  def batch_size_for_input_fn(self):
    """Returns the shard batch size for `input_fn`."""
    global_batch_size = self.global_batch_size

    if (self.is_running_on_cpu() or self.is_input_broadcast_with_iterators()):
      return global_batch_size

    # On TPU
    if self.is_input_sharded_per_core() or (
        self.is_input_per_host_with_iterators()):
      return global_batch_size // self.num_replicas
    else:
      return global_batch_size // self.num_hosts

  @property
  def batch_size_for_model_fn(self):
    """Returns the shard batch size for `model_fn`."""
    global_batch_size = self.global_batch_size

    if (self.is_running_on_cpu() or self.is_input_broadcast_with_iterators()):
      return global_batch_size

    # On TPU. always sharded per shard.
    return global_batch_size // self.num_replicas

  @property
  def master_job(self):
    """Returns the job name to use to place TPU computations on.

    Returns:
      A string containing the job name, or None if no job should be specified.

    Raises:
      ValueError: If the user needs to specify a tpu_job_name, because we are
        unable to infer the job name automatically, or if the user-specified job
        names are inappropriate.
    """
    run_config = self._config
    # If the user specifies the tpu_job_name, use that.
    if run_config.tpu_config.tpu_job_name:
      return run_config.tpu_config.tpu_job_name

    # The tpu job is determined by the run_config. Right now, this method is
    # required as tpu_config is not part of the RunConfig.
    mode = self._assert_mode()
    master = (
        run_config.evaluation_master
        if mode == model_fn_lib.ModeKeys.EVAL else run_config.master)
    if master in _LOCAL_MASTERS:
      return None

    if (not run_config.session_config or
        not run_config.session_config.cluster_def.job):
      return _DEFAULT_JOB_NAME
    cluster_def = run_config.session_config.cluster_def
    job_names = set([job.name for job in cluster_def.job])
    if _DEFAULT_JOB_NAME in job_names:
      # b/37868888 tracks allowing ClusterSpec propagation to reuse job names.
      raise ValueError('Currently, tpu_worker is not an allowed job name.')
    if len(job_names) == 1:
      return cluster_def.job[0].name
    if len(job_names) == 2:
      if _DEFAULT_COORDINATOR_JOB_NAME in job_names:
        job_names.remove(_DEFAULT_COORDINATOR_JOB_NAME)
        return job_names.pop()
      # TODO(b/67716447): Include more sophisticated heuristics.
    raise ValueError(
        'Could not infer TPU job name. Please specify a tpu_job_name as part '
        'of your TPUConfig.')

  @property
  def tpu_host_placement_function(self):
    """Returns the TPU host place function."""

    master = self.master_job

    def _placement_function(_sentinal=None, replica_id=None, host_id=None):  # pylint: disable=invalid-name
      """Return the host device given replica_id or host_id."""
      assert _sentinal is None
      if replica_id is not None and host_id is not None:
        raise RuntimeError(
            'replica_id and host_id can have only one non-None value.')

      if master is None:
        return '/replica:0/task:0/device:CPU:0'
      else:
        if replica_id is not None:
          if self.model_parallelism_enabled:
            return self.device_assignment.host_device(
                replica=replica_id, job=master)
          else:
            host_id = replica_id / self.num_of_cores_per_host

        return '/job:%s/task:%d/device:CPU:0' % (master, host_id)

    return _placement_function

  @property
  def tpu_device_placement_function(self):
    """Returns a TPU device placement Fn."""
    master = self.master_job
    job_device = '' if master is None else ('/job:%s' % master)

    def _placement_function(i):
      if self.model_parallelism_enabled:
        return self.device_assignment.tpu_device(replica=i, job=master)
      else:
        num_of_cores_per_host = self.num_of_cores_per_host
        host_id = i / num_of_cores_per_host
        ordinal_id = i % num_of_cores_per_host
        return '%s/task:%d/device:TPU:%d' % (job_device, host_id, ordinal_id)

    return _placement_function

  def tpu_ordinal_function(self, host_id):
    """Returns the TPU ordinal fn."""

    def _tpu_ordinal_function(shard_index_in_host):
      """Return the TPU ordinal associated with a shard.

      Required because the enqueue ops are placed on CPU.

      Args:
        shard_index_in_host: the shard index

      Returns:
        The ordinal of the TPU device the shard's infeed should be placed on.
      """
      if self.model_parallelism_enabled:
        # We put both enqueue/dequeue ops at tpu.core(0) in each replica.
        replica = self.device_assignment.lookup_replicas(host_id,
                                                         0)[shard_index_in_host]
        return self.device_assignment.tpu_ordinal(replica=replica)
      else:
        return shard_index_in_host % self.num_of_cores_per_host

    return _tpu_ordinal_function

  def _validate_tpu_configuration(self):
    """Validates the configuration based on the TPU system metadata."""
    mode = self._assert_mode()
    if self._lazy_validation_dict.get(mode):
      return

    # All following information is obtained from TPU system metadata.
    num_cores = self.num_cores
    num_replicas = self.num_replicas
    num_hosts = self.num_hosts

    if not num_cores:
      tpu_system_metadata = self._get_tpu_system_metadata()
      raise RuntimeError(
          'Cannot find any TPU cores in the system. Please double check '
          'Tensorflow master address and TPU worker(s). Available devices '
          'are {}.'.format(tpu_system_metadata.devices))

    if self._config.tpu_config.num_shards:
      user_provided_num_replicas = self._config.tpu_config.num_shards
      if user_provided_num_replicas != num_replicas:
        message = (
            'TPUConfig.num_shards is not set correctly. According to TPU '
            'system metadata for Tensorflow master ({}): num_replicas should '
            'be ({}), got ({}). For non-model-parallelism, num_replicas should '
            'be the total num of TPU cores in the system. For '
            'model-parallelism, the total number of TPU cores should be '
            'num_cores_per_replica * num_replicas. Please set it '
            'accordingly or leave it as `None`'.format(
                self._get_master_address(), num_replicas,
                user_provided_num_replicas))

        raise ValueError(message)

    if self._config.tpu_config.num_cores_per_replica:
      num_cores_per_replica = self._config.tpu_config.num_cores_per_replica
      num_cores_per_host = self._get_tpu_system_metadata().num_of_cores_per_host
      if num_cores_per_replica > num_cores_per_host:
        raise ValueError(
            'The num of cores required by the model parallelism, specified by '
            'TPUConfig.num_cores_per_replica, is larger than the '
            'num_cores_per_host. num_cores_per_replica: {}, '
            'num_cores_per_host: {}'.format(num_cores_per_replica,
                                            num_cores_per_host))

    if mode == model_fn_lib.ModeKeys.TRAIN:
      if (self._train_batch_size % num_replicas != 0 and
          not self.is_input_broadcast_with_iterators()):
        raise ValueError(
            'train batch size {} must be divisible by number of replicas {}'
            .format(self._train_batch_size, num_replicas))

    elif mode == model_fn_lib.ModeKeys.EVAL:
      if self._eval_batch_size is None:
        raise ValueError(
            'eval_batch_size in TPUEstimator constructor cannot be `None`'
            'if .evaluate is running on TPU.')
      if (self._eval_batch_size % num_replicas != 0 and
          not self.is_input_broadcast_with_iterators()):
        raise ValueError(
            'eval batch size {} must be divisible by number of replicas {}'
            .format(self._eval_batch_size, num_replicas))
      if num_hosts > 1 and not self.is_input_broadcast_with_iterators():
        raise ValueError(
            'TPUEstimator.evaluate should be running on single TPU'
            ' instead of a Pod.')
    else:
      assert mode == model_fn_lib.ModeKeys.PREDICT
      if self._predict_batch_size is None:
        raise ValueError(
            'predict_batch_size in TPUEstimator constructor should not be '
            '`None` if .predict is running on TPU.')
      if (self._predict_batch_size % num_replicas != 0 and
          not self.is_input_broadcast_with_iterators()):
        raise ValueError(
            'predict batch size {} must be divisible by number of replicas {}'
            .format(self._predict_batch_size, num_replicas))
      if num_hosts > 1 and not self.is_input_broadcast_with_iterators():
        raise ValueError(
            'TPUEstimator.predict should be running on single TPU worker. '
            'got {}.'.format(num_hosts))

    # Record the state "validated" into lazy dictionary.
    self._lazy_validation_dict[mode] = True

  def device_for_replica(self, replica_id):
    """Returns the tuple of (CPU device and device ordinal) for replica.

    This should be used for full replicate for non-model-parallelism.

    Args:
       replica_id: Int, the replica index.

    Returns:
       A tuple of device spec for CPU device and int device ordinal.
    """
    master = self.master_job

    if self.model_parallelism_enabled:
      return (self.device_assignment.host_device(
          replica=replica_id, job=master),
              self.device_assignment.tpu_ordinal(replica=replica_id))

    job_device = '' if master is None else ('/job:%s' % master)

    num_of_replicas_per_host = self.num_of_replicas_per_host
    host_id = replica_id / num_of_replicas_per_host
    ordinal_id = replica_id % num_of_replicas_per_host

    host_device = '%s/task:%d/device:CPU:0' % (job_device, host_id)
    return (host_device, ordinal_id)


class _OneCoreTPUContext(_InternalTPUContext):
  """Special _InternalTPUContext for one core usage."""

  def __init__(self, config, train_batch_size, eval_batch_size,
               predict_batch_size, use_tpu):

    super(_OneCoreTPUContext, self).__init__(
        config, train_batch_size, eval_batch_size,
        predict_batch_size, use_tpu)

  def _get_tpu_system_metadata(self):
    """Gets the (maybe cached) TPU system metadata."""
    master = self._get_master_address()
    tpu_system_metadata = self._lazy_tpu_system_metadata_dict.get(master)
    if tpu_system_metadata is not None:
      return tpu_system_metadata

    tpu_system_metadata = (
        tpu_system_metadata_lib._TPUSystemMetadata(  # pylint: disable=protected-access
            num_cores=1,
            num_hosts=1,
            num_of_cores_per_host=1,
            topology=None,
            devices=[]))

    self._lazy_tpu_system_metadata_dict[master] = tpu_system_metadata
    return tpu_system_metadata


def _get_tpu_context(config, train_batch_size, eval_batch_size,
                     predict_batch_size, use_tpu, eval_on_tpu):
  """Returns an instance of `_InternalTPUContext`."""

  if (config.tpu_config.num_shards == 1 and
      config.tpu_config.num_cores_per_replica is None):
    logging.warning(
        'Setting TPUConfig.num_shards==1 is an unsupported behavior. '
        'Please fix as soon as possible (leaving num_shards as None.)')
    return _OneCoreTPUContext(config, train_batch_size, eval_batch_size,
                              predict_batch_size, use_tpu)

  return _InternalTPUContext(config, train_batch_size, eval_batch_size,
                             predict_batch_size, use_tpu, eval_on_tpu)
