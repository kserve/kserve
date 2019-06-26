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
"""Tests for `tf.data.Options`."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.data.experimental.ops import optimization_options
from tensorflow.python.data.experimental.ops import stats_options
from tensorflow.python.data.experimental.ops import threading_options
from tensorflow.python.data.kernel_tests import test_base
from tensorflow.python.data.ops import dataset_ops
from tensorflow.python.platform import test


class OptionsTest(test_base.DatasetTestBase):

  def testOptionsDefault(self):
    ds = dataset_ops.Dataset.range(0)
    self.assertEqual(dataset_ops.Options(), ds.options())

  def testOptionsOnce(self):
    options = dataset_ops.Options()
    ds = dataset_ops.Dataset.range(0).with_options(options).cache()
    self.assertEqual(options, ds.options())

  def testOptionsTwiceSame(self):
    options = dataset_ops.Options()
    options.experimental_autotune = True
    ds = dataset_ops.Dataset.range(0).with_options(options).with_options(
        options)
    self.assertEqual(options, ds.options())

  def testOptionsTwiceDifferent(self):
    options1 = dataset_ops.Options()
    options1.experimental_autotune = True
    options2 = dataset_ops.Options()
    options2.experimental_deterministic = False
    ds = dataset_ops.Dataset.range(0).with_options(options1).with_options(
        options2)
    self.assertTrue(ds.options().experimental_autotune)
    # Explicitly check that flag is False since assertFalse allows None
    self.assertIs(ds.options().experimental_deterministic, False)

  def testOptionsTwiceDifferentError(self):
    options1 = dataset_ops.Options()
    options1.experimental_autotune = True
    options2 = dataset_ops.Options()
    options2.experimental_autotune = False
    with self.assertRaisesRegexp(ValueError,
                                 "Cannot merge incompatible values"):
      dataset_ops.Dataset.range(0).with_options(options1).with_options(options2)

  def testOptionsMergeOptionsFromMultipleInputs(self):
    options1 = dataset_ops.Options()
    options1.experimental_autotune = True
    options2 = dataset_ops.Options()
    options2.experimental_deterministic = True
    ds = dataset_ops.Dataset.zip(
        (dataset_ops.Dataset.range(0).with_options(options1),
         dataset_ops.Dataset.range(0).with_options(options2)))
    self.assertTrue(ds.options().experimental_autotune)
    self.assertTrue(ds.options().experimental_deterministic)

  def testOptionsHaveDefaults(self):
    options1 = dataset_ops.Options()
    options2 = dataset_ops.Options()
    self.assertIsNot(options1.experimental_optimization,
                     options2.experimental_optimization)
    self.assertIsNot(options1.experimental_stats,
                     options2.experimental_stats)
    self.assertIsNot(options1.experimental_threading,
                     options2.experimental_threading)
    self.assertEquals(options1.experimental_optimization,
                      optimization_options.OptimizationOptions())
    self.assertEquals(options1.experimental_stats,
                      stats_options.StatsOptions())
    self.assertEquals(options1.experimental_threading,
                      threading_options.ThreadingOptions())


if __name__ == "__main__":
  test.main()
