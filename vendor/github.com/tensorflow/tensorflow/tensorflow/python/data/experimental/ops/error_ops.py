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
"""Ignore_errors dataset transformations."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.ops import gen_experimental_dataset_ops
from tensorflow.python.util.tf_export import tf_export


@tf_export("data.experimental.ignore_errors")
def ignore_errors():
  """Creates a `Dataset` from another `Dataset` and silently ignores any errors.

  Use this transformation to produce a dataset that contains the same elements
  as the input, but silently drops any elements that caused an error. For
  example:

  ```python
  dataset = tf.data.Dataset.from_tensor_slices([1., 2., 0., 4.])

  # Computing `tf.check_numerics(1. / 0.)` will raise an InvalidArgumentError.
  dataset = dataset.map(lambda x: tf.check_numerics(1. / x, "error"))

  # Using `ignore_errors()` will drop the element that causes an error.
  dataset =
      dataset.apply(tf.data.experimental.ignore_errors())  # ==> {1., 0.5, 0.2}
  ```

  Returns:
    A `Dataset` transformation function, which can be passed to
    `tf.data.Dataset.apply`.
  """

  def _apply_fn(dataset):
    return _IgnoreErrorsDataset(dataset)

  return _apply_fn


class _IgnoreErrorsDataset(dataset_ops.UnaryUnchangedStructureDataset):
  """A `Dataset` that silently ignores errors when computing its input."""

  def __init__(self, input_dataset):
    """See `Dataset.ignore_errors()` for details."""
    super(_IgnoreErrorsDataset, self).__init__(input_dataset)
    self._input_dataset = input_dataset

  def _as_variant_tensor(self):
    return gen_experimental_dataset_ops.experimental_ignore_errors_dataset(
        self._input_dataset._as_variant_tensor(),  # pylint: disable=protected-access
        **dataset_ops.flat_structure(self))
