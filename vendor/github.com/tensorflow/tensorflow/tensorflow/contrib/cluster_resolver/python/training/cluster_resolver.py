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
"""Stub file for ClusterResolver to maintain backwards compatibility."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# This file (and all files in this directory in general) is a backwards
# compatibility shim that exists to re-export ClusterResolvers such that
# existing OSS code will not be broken.

# pylint: disable=unused-import
from tensorflow.python.distribute.cluster_resolver.cluster_resolver import ClusterResolver
from tensorflow.python.distribute.cluster_resolver.cluster_resolver import SimpleClusterResolver
from tensorflow.python.distribute.cluster_resolver.cluster_resolver import UnionClusterResolver
# pylint: enable=unused-import

from tensorflow.python.util.all_util import remove_undocumented

_allowed_symbols = [
    'ClusterResolver',
    'SimpleClusterResolver',
    'UnionClusterResolver',
]

remove_undocumented(__name__, _allowed_symbols)

