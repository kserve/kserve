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
"""Wrappers for bucketization operations."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.ops import math_ops


def bucketize(input_tensor, boundaries, name=None):
  """Bucketizes input_tensor by given boundaries.

  See bucketize_op.cc for more details.

  Args:
    input_tensor: A `Tensor` which will be bucketize.
    boundaries: A list of floats gives the boundaries. It has to be sorted.
    name: A name prefix for the returned tensors (optional).

  Returns:
    A `Tensor` with type int32 which indicates the corresponding bucket for
      each value in `input_tensor`.

  Raises:
    TypeError: If boundaries is not a list.
  """
  return math_ops._bucketize(  # pylint: disable=protected-access
      input_tensor, boundaries=boundaries, name=name)
