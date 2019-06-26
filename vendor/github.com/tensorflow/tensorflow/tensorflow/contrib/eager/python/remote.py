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
"""Helpers to connect to remote servers."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import os

from tensorflow.core.protobuf.cluster_pb2 import ClusterDef
from tensorflow.core.protobuf.tensorflow_server_pb2 import ServerDef
from tensorflow.python.eager import context


def connect_to_remote_host(remote_host=None, job_name="worker"):
  """Connects to a single machine to enable remote execution on it.

  Will make devices on the remote host available to use. Note that calling this
  more than once will work, but will invalidate any tensor handles on the old
  remote devices.

  Using the default job_name of worker, you can schedule ops to run remotely as
  follows:
  ```python
  # Enable eager execution, and connect to the remote host.
  tf.enable_eager_execution()
  tf.contrib.eager.connect_to_remote_host("exampleaddr.com:9876")

  with ops.device("job:worker/replica:0/task:1/device:CPU:0"):
    # The following tensors should be resident on the remote device, and the op
    # will also execute remotely.
    x1 = array_ops.ones([2, 2])
    x2 = array_ops.ones([2, 2])
    y = math_ops.matmul(x1, x2)
  ```

  Args:
    remote_host: The addr of the remote server in host-port format.
    job_name: The job name under which the new server will be accessible.

  Raises:
    ValueError: if remote_host is None.
  """
  if remote_host is None:
    raise ValueError("Must provide an remote_host")
  cluster_def = ClusterDef()
  job_def = cluster_def.job.add()
  job_def.name = job_name
  job_def.tasks[0] = "127.0.0.1:0"
  job_def.tasks[1] = remote_host

  server_def = ServerDef(
      cluster=cluster_def,
      job_name=job_name,
      task_index=0,
      protocol="grpc")

  # TODO(nareshmodi): Make this default since it works in more situations.
  os.environ["TF_EAGER_REMOTE_USE_SEND_TENSOR_RPC"] = "1"
  context.set_server_def(server_def)
