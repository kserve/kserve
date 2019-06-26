# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
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

"""Momentum for TensorFlow."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.optimizer_v2 import optimizer_v2
from tensorflow.python.training import training_ops


class MomentumOptimizer(optimizer_v2.OptimizerV2):
  """Optimizer that implements the Momentum algorithm.

  Computes (if `use_nesterov = False`):

  ```
  accumulation = momentum * accumulation + gradient
  variable -= learning_rate * accumulation
  ```

  Note that in the dense version of this algorithm, `accumulation` is updated
  and applied regardless of a gradient's value, whereas the sparse version (when
  the gradient is an `IndexedSlices`, typically because of `tf.gather` or an
  embedding) only updates variable slices and corresponding `accumulation` terms
  when that part of the variable was used in the forward pass.
  """

  def __init__(self, learning_rate, momentum,
               use_locking=False, name="Momentum", use_nesterov=False):
    """Construct a new Momentum optimizer.

    Some of the args below are hyperparameters, where a hyperparameter is
    defined as a scalar Tensor, a regular Python value or a callable (which
    will be evaluated when `apply_gradients` is called) returning a scalar
    Tensor or a Python value.

    Args:
      learning_rate: A float hyperparameter. The learning rate.
      momentum: A float hyperparameter. The momentum.
      use_locking: If `True` use locks for update operations.
      name: Optional name prefix for the operations created when applying
        gradients.  Defaults to "Momentum".
      use_nesterov: If `True` use Nesterov Momentum.
        See [Sutskever et al., 2013](
        http://jmlr.org/proceedings/papers/v28/sutskever13.pdf).
        This implementation always computes gradients at the value of the
        variable(s) passed to the optimizer. Using Nesterov Momentum makes the
        variable(s) track the values called `theta_t + mu*v_t` in the paper.

    @compatibility(eager)
    When eager execution is enabled, learning_rate and momentum can each be a
    callable that takes no arguments and returns the actual value to use. This
    can be useful for changing these values across different invocations of
    optimizer functions.
    @end_compatibility
    """
    super(MomentumOptimizer, self).__init__(use_locking, name)
    self._set_hyper("learning_rate", learning_rate)
    self._set_hyper("momentum", momentum)
    self._use_nesterov = use_nesterov

  def _create_vars(self, var_list, state):
    for v in var_list:
      state.zeros_slot(v, "momentum")

  def _apply_dense(self, grad, var, state):
    mom = state.get_slot(var, "momentum")
    return training_ops.apply_momentum(
        var,
        mom,
        state.get_hyper("learning_rate", var.dtype.base_dtype),
        grad,
        state.get_hyper("momentum", var.dtype.base_dtype),
        use_locking=self._use_locking,
        use_nesterov=self._use_nesterov).op

  def _resource_apply_dense(self, grad, var, state):
    mom = state.get_slot(var, "momentum")
    return training_ops.resource_apply_momentum(
        var.handle,
        mom.handle,
        state.get_hyper("learning_rate", var.dtype.base_dtype),
        grad,
        state.get_hyper("momentum", var.dtype.base_dtype),
        use_locking=self._use_locking,
        use_nesterov=self._use_nesterov)

  def _apply_sparse(self, grad, var, state):
    mom = state.get_slot(var, "momentum")
    return training_ops.sparse_apply_momentum(
        var,
        mom,
        state.get_hyper("learning_rate", var.dtype.base_dtype),
        grad.values,
        grad.indices,
        state.get_hyper("momentum", var.dtype.base_dtype),
        use_locking=self._use_locking,
        use_nesterov=self._use_nesterov).op

  def _resource_apply_sparse(self, grad, var, indices, state):
    mom = state.get_slot(var, "momentum")
    return training_ops.resource_sparse_apply_momentum(
        var.handle,
        mom.handle,
        state.get_hyper("learning_rate", var.dtype.base_dtype),
        grad,
        indices,
        state.get_hyper("momentum", var.dtype.base_dtype),
        use_locking=self._use_locking,
        use_nesterov=self._use_nesterov)
