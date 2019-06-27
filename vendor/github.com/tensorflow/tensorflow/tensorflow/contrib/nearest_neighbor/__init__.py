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
"""Ops for (approximate) nearest neighbor look-ups.

## Ops for (approximate) nearest neighbor look-ups

This package provides several ops for efficient (approximate) nearest
neighbor look-ups.

### LSH multiprobe ops

The following ops generate multiprobe sequences for various hash families.

@@hyperplane_lsh_hash

"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# pylint: disable=unused-import,wildcard-import, line-too-long
from tensorflow.contrib.nearest_neighbor.python.ops.nearest_neighbor_ops import *
# pylint: enable=unused-import,wildcard-import,line-too-long
