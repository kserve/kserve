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
"""Tests for Bijector."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.distributions.python.ops import bijectors
from tensorflow.python.framework import tensor_shape
from tensorflow.python.ops import array_ops
from tensorflow.python.ops.distributions import gamma as gamma_lib
from tensorflow.python.ops.distributions import transformed_distribution as transformed_distribution_lib
from tensorflow.python.ops.distributions.bijector_test_util import assert_scalar_congruency
from tensorflow.python.platform import test


class InvertBijectorTest(test.TestCase):
  """Tests the correctness of the Y = Invert(bij) transformation."""

  def testBijector(self):
    with self.cached_session():
      for fwd in [
          bijectors.Identity(),
          bijectors.Exp(),
          bijectors.Affine(shift=[0., 1.], scale_diag=[2., 3.]),
          bijectors.Softplus(),
          bijectors.SoftmaxCentered(),
      ]:
        rev = bijectors.Invert(fwd)
        self.assertEqual("_".join(["invert", fwd.name]), rev.name)
        x = [[[1., 2.],
              [2., 3.]]]
        self.assertAllClose(fwd.inverse(x).eval(), rev.forward(x).eval())
        self.assertAllClose(fwd.forward(x).eval(), rev.inverse(x).eval())
        self.assertAllClose(
            fwd.forward_log_det_jacobian(x, event_ndims=1).eval(),
            rev.inverse_log_det_jacobian(x, event_ndims=1).eval())
        self.assertAllClose(
            fwd.inverse_log_det_jacobian(x, event_ndims=1).eval(),
            rev.forward_log_det_jacobian(x, event_ndims=1).eval())

  def testScalarCongruency(self):
    with self.cached_session():
      bijector = bijectors.Invert(bijectors.Exp())
      assert_scalar_congruency(
          bijector, lower_x=1e-3, upper_x=1.5, rtol=0.05)

  def testShapeGetters(self):
    with self.cached_session():
      bijector = bijectors.Invert(bijectors.SoftmaxCentered(validate_args=True))
      x = tensor_shape.TensorShape([2])
      y = tensor_shape.TensorShape([1])
      self.assertAllEqual(y, bijector.forward_event_shape(x))
      self.assertAllEqual(
          y.as_list(),
          bijector.forward_event_shape_tensor(x.as_list()).eval())
      self.assertAllEqual(x, bijector.inverse_event_shape(y))
      self.assertAllEqual(
          x.as_list(),
          bijector.inverse_event_shape_tensor(y.as_list()).eval())

  def testDocstringExample(self):
    with self.cached_session():
      exp_gamma_distribution = (
          transformed_distribution_lib.TransformedDistribution(
              distribution=gamma_lib.Gamma(concentration=1., rate=2.),
              bijector=bijectors.Invert(bijectors.Exp())))
      self.assertAllEqual(
          [], array_ops.shape(exp_gamma_distribution.sample()).eval())


if __name__ == "__main__":
  test.main()
