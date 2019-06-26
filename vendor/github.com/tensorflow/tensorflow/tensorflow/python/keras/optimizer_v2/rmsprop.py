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
"""RMSprop for TensorFlow."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.framework import ops
from tensorflow.python.keras.optimizer_v2 import optimizer_v2
from tensorflow.python.training import training_ops
from tensorflow.python.util.tf_export import tf_export


@tf_export("keras.optimizers.RMSprop", v1=[])
class RMSprop(optimizer_v2.OptimizerV2):
  r"""Optimizer that implements the RMSprop algorithm.

  A detailed description of rmsprop.

    - maintain a moving (discounted) average of the square of gradients
    - divide gradient by the root of this average

  $$mean_square_t = rho * mean_square{t-1} + (1-rho) * gradient ** 2$$
  $$mom_t = momentum * mom_{t-1} + learning_rate * gradient / \sqrt{ /
      mean_square_t + \epsilon}$$
  $$variable_t := variable_{t-1} - mom_t

  This implementation of RMSprop uses plain momentum, not Nesterov momentum.

  The centered version additionally maintains a moving average of the
  gradients, and uses that average to estimate the variance:

  $$mean_grad_t = rho * mean_grad_{t-1} + (1-rho) * gradient$$
  $$mean_square_t = rho * mean_square_{t-1} + (1-rho) * gradient ** 2$$
  $$mom_t = momentum * mom_{t-1} + learning_rate * gradient /
      sqrt(mean_square_t - mean_grad_t**2 + epsilon)$$
  $$variable_t := variable_{t-1} - mom_t

  References
    See ([pdf]
      http://www.cs.toronto.edu/~tijmen/csc321/slides/lecture_slides_lec6.pdf).
  """

  def __init__(self,
               learning_rate=0.001,
               rho=0.9,
               momentum=0.0,
               epsilon=1e-7,
               centered=False,
               name="RMSprop",
               **kwargs):
    """Construct a new RMSprop optimizer.

    Note that in the dense implementation of this algorithm, variables and their
    corresponding accumulators (momentum, gradient moving average, square
    gradient moving average) will be updated even if the gradient is zero
    (i.e. accumulators will decay, momentum will be applied). The sparse
    implementation (used when the gradient is an `IndexedSlices` object,
    typically because of `tf.gather` or an embedding lookup in the forward pass)
    will not update variable slices or their accumulators unless those slices
    were used in the forward pass (nor is there an "eventual" correction to
    account for these omitted updates). This leads to more efficient updates for
    large embedding lookup tables (where most of the slices are not accessed in
    a particular graph execution), but differs from the published algorithm.

    Args:
      learning_rate: A Tensor or a floating point value.  The learning rate.
      rho: Discounting factor for the history/coming gradient
      momentum: A scalar tensor.
      epsilon: Small value to avoid zero denominator.
      centered: If True, gradients are normalized by the estimated variance of
        the gradient; if False, by the uncentered second moment. Setting this to
        True may help with training, but is slightly more expensive in terms of
        computation and memory. Defaults to False.
      name: Optional name prefix for the operations created when applying
        gradients. Defaults to "RMSprop".  @compatibility(eager) When eager
        execution is enabled, `learning_rate`, `decay`, `momentum`, and
        `epsilon` can each be a callable that takes no arguments and returns the
        actual value to use. This can be useful for changing these values across
        different invocations of optimizer functions. @end_compatibility
      **kwargs: keyword arguments. Allowed to be {`decay`}
    """
    super(RMSprop, self).__init__(name, **kwargs)
    self._set_hyper("learning_rate", kwargs.get("lr", learning_rate))
    self._set_hyper("decay", self._initial_decay)
    self._set_hyper("rho", rho)

    self._momentum = False
    if isinstance(momentum, ops.Tensor) or callable(momentum) or momentum > 0:
      self._momentum = True
    if isinstance(momentum, (int, float)) and (momentum < 0 or momentum > 1):
      raise ValueError("`momentum` must be between [0, 1].")
    self._set_hyper("momentum", momentum)

    self._set_hyper("epsilon", epsilon)
    self.centered = centered

  def _create_slots(self, var_list):
    for var in var_list:
      self.add_slot(var, "rms")
      self.add_slot(var, "momentum")
      if self.centered:
        self.add_slot(var, "mg")

  def _resource_apply_dense(self, grad, var):
    var_dtype = var.dtype.base_dtype
    lr_t = self._decayed_lr(var_dtype)
    rms = self.get_slot(var, "rms")
    mom = self.get_slot(var, "momentum")
    rho = self._get_hyper("rho", var_dtype)
    momentum = self._get_hyper("momentum", var_dtype)
    epsilon = self._get_hyper("epsilon", var_dtype)
    if self.centered:
      mg = self.get_slot(var, "mg")
      return training_ops.resource_apply_centered_rms_prop(
          var.handle,
          mg.handle,
          rms.handle,
          mom.handle,
          lr_t,
          rho,
          momentum,
          epsilon,
          grad,
          use_locking=self._use_locking)
    else:
      return training_ops.resource_apply_rms_prop(
          var.handle,
          rms.handle,
          mom.handle,
          lr_t,
          rho,
          momentum,
          epsilon,
          grad,
          use_locking=self._use_locking)

  def _resource_apply_sparse(self, grad, var, indices):
    var_dtype = var.dtype.base_dtype
    lr_t = self._decayed_lr(var_dtype)
    rms = self.get_slot(var, "rms")
    mom = self.get_slot(var, "momentum")
    rho = self._get_hyper("rho", var_dtype)
    momentum = self._get_hyper("momentum", var_dtype)
    epsilon = self._get_hyper("epsilon", var_dtype)
    if self.centered:
      mg = self.get_slot(var, "mg")
      return training_ops.resource_sparse_apply_centered_rms_prop(
          var.handle,
          mg.handle,
          rms.handle,
          mom.handle,
          lr_t,
          rho,
          momentum,
          epsilon,
          grad,
          indices,
          use_locking=self._use_locking)
    else:
      return training_ops.resource_sparse_apply_rms_prop(
          var.handle,
          rms.handle,
          mom.handle,
          lr_t,
          rho,
          momentum,
          epsilon,
          grad,
          indices,
          use_locking=self._use_locking)

  def get_config(self):
    config = super(RMSprop, self).get_config()
    config.update({
        "learning_rate": self._serialize_hyperparameter("learning_rate"),
        "decay": self._serialize_hyperparameter("decay"),
        "rho": self._serialize_hyperparameter("rho"),
        "momentum": self._serialize_hyperparameter("momentum"),
        "epsilon": self._serialize_hyperparameter("epsilon"),
        "centered": self.centered,
    })
    return config


RMSProp = RMSprop
