# Copyright 2018 The TensorFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ==============================================================================
"""Library Imports for Cluster Resolvers."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.distribute.cluster_resolver import cluster_resolver
from tensorflow.python.distribute.cluster_resolver import gce_cluster_resolver
from tensorflow.python.distribute.cluster_resolver import kubernetes_cluster_resolver
from tensorflow.python.distribute.cluster_resolver import slurm_cluster_resolver
from tensorflow.python.distribute.cluster_resolver import tfconfig_cluster_resolver
from tensorflow.python.distribute.cluster_resolver import tpu_cluster_resolver

from tensorflow.python.distribute.cluster_resolver.cluster_resolver import ClusterResolver
from tensorflow.python.distribute.cluster_resolver.cluster_resolver import SimpleClusterResolver
from tensorflow.python.distribute.cluster_resolver.cluster_resolver import UnionClusterResolver
from tensorflow.python.distribute.cluster_resolver.gce_cluster_resolver import GceClusterResolver
from tensorflow.python.distribute.cluster_resolver.kubernetes_cluster_resolver import KubernetesClusterResolver
from tensorflow.python.distribute.cluster_resolver.slurm_cluster_resolver import SlurmClusterResolver
from tensorflow.python.distribute.cluster_resolver.tfconfig_cluster_resolver import TFConfigClusterResolver
from tensorflow.python.distribute.cluster_resolver.tpu_cluster_resolver import TPUClusterResolver

from tensorflow.python.util.all_util import remove_undocumented

_allowed_symbols = [
    'cluster_resolver',
    'gce_cluster_resolver',
    'kubernetes_cluster_resolver',
    'slurm_cluster_resolver',
    'tfconfig_cluster_resolver',
    'tpu_cluster_resolver',
    'ClusterResolver',
    'SimpleClusterResolver',
    'UnionClusterResolver',
    'GceClusterResolver',
    'KubernetesClusterResolver',
    'TFConfigClusterResolver',
    'TPUClusterResolver',
    'SlurmClusterResolver',
]

remove_undocumented(__name__, _allowed_symbols)

