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
"""Tests for mfcc_ops."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.framework import dtypes
from tensorflow.python.framework import tensor_shape
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import spectral_ops_test_util
from tensorflow.python.ops.signal import mfcc_ops
from tensorflow.python.platform import test


# TODO(rjryan): We have no open source tests for MFCCs at the moment. Internally
# at Google, this code is tested against a reference implementation that follows
# HTK conventions.
class MFCCTest(test.TestCase):

  @test_util.run_deprecated_v1
  def test_error(self):
    # num_mel_bins must be positive.
    with self.assertRaises(ValueError):
      signal = array_ops.zeros((2, 3, 0))
      mfcc_ops.mfccs_from_log_mel_spectrograms(signal)

    # signal must be float32
    with self.assertRaises(ValueError):
      signal = array_ops.zeros((2, 3, 5), dtype=dtypes.float64)
      mfcc_ops.mfccs_from_log_mel_spectrograms(signal)

  @test_util.run_deprecated_v1
  def test_basic(self):
    """A basic test that the op runs on random input."""
    with spectral_ops_test_util.fft_kernel_label_map():
      with self.session(use_gpu=True):
        signal = random_ops.random_normal((2, 3, 5))
        mfcc_ops.mfccs_from_log_mel_spectrograms(signal).eval()

  @test_util.run_deprecated_v1
  def test_unknown_shape(self):
    """A test that the op runs when shape and rank are unknown."""
    with spectral_ops_test_util.fft_kernel_label_map():
      with self.session(use_gpu=True):
        signal = array_ops.placeholder_with_default(
            random_ops.random_normal((2, 3, 5)), tensor_shape.TensorShape(None))
        self.assertIsNone(signal.shape.ndims)
        mfcc_ops.mfccs_from_log_mel_spectrograms(signal).eval()

if __name__ == "__main__":
  test.main()
