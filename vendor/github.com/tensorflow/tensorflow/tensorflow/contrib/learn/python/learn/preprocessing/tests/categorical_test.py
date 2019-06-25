# encoding: utf-8
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
"""Categorical tests."""

#  limitations under the License.
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.contrib.learn.python.learn.learn_io import HAS_PANDAS
from tensorflow.contrib.learn.python.learn.preprocessing import categorical
from tensorflow.python.platform import test


class CategoricalTest(test.TestCase):
  """Categorical tests."""

  def testSingleCategoricalProcessor(self):
    cat_processor = categorical.CategoricalProcessor(min_frequency=1)
    x = cat_processor.fit_transform([["0"], [1], [float("nan")], ["C"], ["C"],
                                     [1], ["0"], [np.nan], [3]])
    self.assertAllEqual(list(x), [[2], [1], [0], [3], [3], [1], [2], [0], [0]])

  def testSingleCategoricalProcessorPandasSingleDF(self):
    if HAS_PANDAS:
      import pandas as pd  # pylint: disable=g-import-not-at-top
      cat_processor = categorical.CategoricalProcessor()
      data = pd.DataFrame({"Gender": ["Male", "Female", "Male"]})
      x = list(cat_processor.fit_transform(data))
      self.assertAllEqual(list(x), [[1], [2], [1]])

  def testMultiCategoricalProcessor(self):
    cat_processor = categorical.CategoricalProcessor(
        min_frequency=0, share=False)
    x = cat_processor.fit_transform([["0", "Male"], [1, "Female"],
                                     ["3", "Male"]])
    self.assertAllEqual(list(x), [[1, 1], [2, 2], [3, 1]])


if __name__ == "__main__":
  test.main()
