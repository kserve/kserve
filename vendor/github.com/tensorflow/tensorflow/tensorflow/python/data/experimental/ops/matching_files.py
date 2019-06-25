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
# ==============================================================================
"""Experimental API for matching input filenames."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.data.util import structure
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.ops import gen_experimental_dataset_ops as ged_ops


class MatchingFilesDataset(dataset_ops.DatasetSource):
  """A `Dataset` that list the files according to the input patterns."""

  def __init__(self, patterns):
    super(MatchingFilesDataset, self).__init__()
    self._patterns = ops.convert_to_tensor(
        patterns, dtype=dtypes.string, name="patterns")

  def _as_variant_tensor(self):
    return ged_ops.experimental_matching_files_dataset(self._patterns)

  @property
  def _element_structure(self):
    return structure.TensorStructure(dtypes.string, [])
