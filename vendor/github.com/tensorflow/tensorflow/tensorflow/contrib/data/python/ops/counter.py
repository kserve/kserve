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
"""The Counter Dataset."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.data.experimental.ops import counter
from tensorflow.python.framework import dtypes
from tensorflow.python.util import deprecation


@deprecation.deprecated(None, "Use `tf.data.experimental.Counter(...)`.")
def Counter(start=0, step=1, dtype=dtypes.int64):
  """Creates a `Dataset` that counts from `start` in steps of size `step`.

  For example:

  ```python
  Dataset.count() == [0, 1, 2, ...)
  Dataset.count(2) == [2, 3, ...)
  Dataset.count(2, 5) == [2, 7, 12, ...)
  Dataset.count(0, -1) == [0, -1, -2, ...)
  Dataset.count(10, -1) == [10, 9, ...)
  ```

  Args:
    start: (Optional.) The starting value for the counter. Defaults to 0.
    step: (Optional.) The step size for the counter. Defaults to 1.
    dtype: (Optional.) The data type for counter elements. Defaults to
      `tf.int64`.

  Returns:
    A `Dataset` of scalar `dtype` elements.
  """
  return counter.Counter(start, step, dtype)
