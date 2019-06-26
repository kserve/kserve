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
"""Sigmoid bijector."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops.distributions import bijector
from tensorflow.python.util import deprecation


__all__ = [
    "Sigmoid",
]


class Sigmoid(bijector.Bijector):
  """Bijector which computes `Y = g(X) = 1 / (1 + exp(-X))`."""

  @deprecation.deprecated(
      "2018-10-01",
      "The TensorFlow Distributions library has moved to "
      "TensorFlow Probability "
      "(https://github.com/tensorflow/probability). You "
      "should update all references to use `tfp.distributions` "
      "instead of `tf.contrib.distributions`.",
      warn_once=True)
  def __init__(self, validate_args=False, name="sigmoid"):
    super(Sigmoid, self).__init__(
        forward_min_event_ndims=0,
        validate_args=validate_args,
        name=name)

  def _forward(self, x):
    return math_ops.sigmoid(x)

  def _inverse(self, y):
    return math_ops.log(y) - math_ops.log1p(-y)

  def _inverse_log_det_jacobian(self, y):
    return -math_ops.log(y) - math_ops.log1p(-y)

  def _forward_log_det_jacobian(self, x):
    return -nn_ops.softplus(-x) - nn_ops.softplus(x)
