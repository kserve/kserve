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
"""Tests for initializers."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np
from scipy import stats
from tensorflow.contrib.distributions.python.ops import chi2 as chi2_lib
from tensorflow.python.framework import constant_op
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import test


class Chi2Test(test.TestCase):

  def testChi2LogPDF(self):
    with self.cached_session():
      batch_size = 6
      df = constant_op.constant([2.0] * batch_size, dtype=np.float64)
      df_v = 2.0
      x = np.array([2.5, 2.5, 4.0, 0.1, 1.0, 2.0], dtype=np.float64)
      chi2 = chi2_lib.Chi2(df=df)
      expected_log_pdf = stats.chi2.logpdf(x, df_v)

      log_pdf = chi2.log_prob(x)
      self.assertEqual(log_pdf.get_shape(), (6,))
      self.assertAllClose(log_pdf.eval(), expected_log_pdf)

      pdf = chi2.prob(x)
      self.assertEqual(pdf.get_shape(), (6,))
      self.assertAllClose(pdf.eval(), np.exp(expected_log_pdf))

  def testChi2CDF(self):
    with self.cached_session():
      batch_size = 6
      df = constant_op.constant([2.0] * batch_size, dtype=np.float64)
      df_v = 2.0
      x = np.array([2.5, 2.5, 4.0, 0.1, 1.0, 2.0], dtype=np.float64)

      chi2 = chi2_lib.Chi2(df=df)
      expected_cdf = stats.chi2.cdf(x, df_v)

      cdf = chi2.cdf(x)
      self.assertEqual(cdf.get_shape(), (6,))
      self.assertAllClose(cdf.eval(), expected_cdf)

  def testChi2Mean(self):
    with self.cached_session():
      df_v = np.array([1., 3, 5], dtype=np.float64)
      expected_mean = stats.chi2.mean(df_v)
      chi2 = chi2_lib.Chi2(df=df_v)
      self.assertEqual(chi2.mean().get_shape(), (3,))
      self.assertAllClose(chi2.mean().eval(), expected_mean)

  def testChi2Variance(self):
    with self.cached_session():
      df_v = np.array([1., 3, 5], np.float64)
      expected_variances = stats.chi2.var(df_v)
      chi2 = chi2_lib.Chi2(df=df_v)
      self.assertEqual(chi2.variance().get_shape(), (3,))
      self.assertAllClose(chi2.variance().eval(), expected_variances)

  def testChi2Entropy(self):
    with self.cached_session():
      df_v = np.array([1., 3, 5], dtype=np.float64)
      expected_entropy = stats.chi2.entropy(df_v)
      chi2 = chi2_lib.Chi2(df=df_v)
      self.assertEqual(chi2.entropy().get_shape(), (3,))
      self.assertAllClose(chi2.entropy().eval(), expected_entropy)

  def testChi2WithAbsDf(self):
    with self.cached_session():
      df_v = np.array([-1.3, -3.2, 5], dtype=np.float64)
      chi2 = chi2_lib.Chi2WithAbsDf(df=df_v)
      self.assertAllClose(
          math_ops.floor(math_ops.abs(df_v)).eval(),
          chi2.df.eval())


if __name__ == "__main__":
  test.main()
