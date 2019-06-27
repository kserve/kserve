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
"""Loads the _boosted_trees_ops.so when the binary is not statically linked."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.util import loader
from tensorflow.python.framework import errors
from tensorflow.python.platform import resource_loader

# Conditionally load ops, they might already be statically linked in.
try:
  loader.load_op_library(
      resource_loader.get_path_to_datafile('_boosted_trees_ops.so'))
except (errors.NotFoundError, IOError):
  print('Error loading _boosted_trees_ops.so')
