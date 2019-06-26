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

"""AdaMax for TensorFlow."""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.eager import context
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import resource_variable_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.training import adam
from tensorflow.python.training import training_ops


class AdaMaxOptimizer(adam.AdamOptimizer):
  """Optimizer that implements the AdaMax algorithm.

  Adamax is sometimes superior to adam, specially in models with embeddings,
  see [Kingma et al., 2014](http://arxiv.org/abs/1412.6980)
  ([pdf](http://arxiv.org/pdf/1412.6980.pdf)).
  """

  def __init__(self, learning_rate=0.001, beta1=0.9, beta2=0.999, epsilon=1e-8,
               use_locking=False, name="AdaMax"):
    """Construct a new AdaMax optimizer.

    Initialization:

    ```
    m_0 <- 0 (Initialize initial 1st moment vector)
    v_0 <- 0 (Initialize the exponentially weighted infinity norm)
    t <- 0 (Initialize timestep)
    ```

    The update rule for `variable` with gradient `g` uses an optimization
    described at the end of section 7.1 of the paper:

    ```
    t <- t + 1

    m_t <- beta1 * m_{t-1} + (1 - beta1) * g
    v_t <- max(beta2 * v_{t-1}, abs(g))
    variable <- variable - learning_rate / (1 - beta1^t) * m_t / (v_t + epsilon)
    ```

    Similar to AdamOptimizer, the epsilon is added for numerical stability
    (especially to get rid of division by zero when v_t = 0).

    Contrast to AdamOptimizer, the sparse implementation of this algorithm
    (used when the gradient is an IndexedSlices object, typically because of
    `tf.gather` or an embedding lookup in the forward pass) only updates
    variable slices and corresponding `m_t`, `v_t` terms when that part of
    the variable was used in the forward pass. This means that the sparse
    behavior is contrast to the dense behavior (similar to some momentum
    implementations which ignore momentum unless a variable slice was actually
    used).

    Args:
      learning_rate: A Tensor or a floating point value.  The learning rate.
      beta1: A float value or a constant float tensor.
        The exponential decay rate for the 1st moment estimates.
      beta2: A float value or a constant float tensor.
        The exponential decay rate for the exponentially weighted infinity norm.
      epsilon: A small constant for numerical stability.
      use_locking: If True use locks for update operations.
      name: Optional name for the operations created when applying gradients.
        Defaults to "AdaMax".
    """
    super(AdaMaxOptimizer, self).__init__(learning_rate, beta1, beta2,
                                          epsilon, use_locking, name)

  def _get_beta_accumulators(self):
    if context.executing_eagerly():
      graph = None
    else:
      graph = ops.get_default_graph()
    return self._get_non_slot_variable("beta1_power", graph=graph)

  def _create_slots(self, var_list):
    # Create the beta1 accumulators on the same device as the first
    # variable. Sort the var_list to make sure this device is consistent across
    # workers (these need to go on the same PS, otherwise some updates are
    # silently ignored).
    first_var = min(var_list, key=lambda x: x.name)
    self._create_non_slot_variable(initial_value=self._beta1,
                                   name="beta1_power",
                                   colocate_with=first_var)

    # Create slots for the first and second moments.
    for v in var_list:
      self._zeros_slot(v, "m", self._name)
      self._zeros_slot(v, "v", self._name)

  def _apply_dense(self, grad, var):
    m = self.get_slot(var, "m")
    v = self.get_slot(var, "v")
    beta1_power = self._get_beta_accumulators()
    return training_ops.apply_ada_max(
        var, m, v,
        math_ops.cast(beta1_power, var.dtype.base_dtype),
        math_ops.cast(self._lr_t, var.dtype.base_dtype),
        math_ops.cast(self._beta1_t, var.dtype.base_dtype),
        math_ops.cast(self._beta2_t, var.dtype.base_dtype),
        math_ops.cast(self._epsilon_t, var.dtype.base_dtype),
        grad, use_locking=self._use_locking).op

  def _resource_apply_dense(self, grad, var):
    m = self.get_slot(var, "m")
    v = self.get_slot(var, "v")
    beta1_power = self._get_beta_accumulators()
    return training_ops.resource_apply_ada_max(
        var.handle, m.handle, v.handle,
        math_ops.cast(beta1_power, grad.dtype.base_dtype),
        math_ops.cast(self._lr_t, grad.dtype.base_dtype),
        math_ops.cast(self._beta1_t, grad.dtype.base_dtype),
        math_ops.cast(self._beta2_t, grad.dtype.base_dtype),
        math_ops.cast(self._epsilon_t, grad.dtype.base_dtype),
        grad, use_locking=self._use_locking)

  def _apply_sparse_shared(self, grad, var, indices,
                           scatter_add, scatter_update):
    beta1_power = self._get_beta_accumulators()
    beta1_power = math_ops.cast(beta1_power, var.dtype.base_dtype)
    lr_t = math_ops.cast(self._lr_t, var.dtype.base_dtype)
    beta1_t = math_ops.cast(self._beta1_t, var.dtype.base_dtype)
    beta2_t = math_ops.cast(self._beta2_t, var.dtype.base_dtype)
    epsilon_t = math_ops.cast(self._epsilon_t, var.dtype.base_dtype)
    # m_t = beta1 * m + (1 - beta1) * g_t
    m = self.get_slot(var, "m")
    m_slice = array_ops.gather(m, indices)
    m_t_slice = m_slice * beta1_t + grad * (1 - beta1_t)
    with ops.control_dependencies([m_t_slice]):
      m_t = scatter_update(m, indices, m_t_slice)
    # u_t = max(beta2 * u, abs(g_t))
    v = self.get_slot(var, "v")
    v_slice = array_ops.gather(v, indices)
    v_t_slice = math_ops.maximum(v_slice * beta2_t, math_ops.abs(grad))
    with ops.control_dependencies([v_t_slice]):
      v_t = scatter_update(v, indices, v_t_slice)
    # theta_t = theta - lr / (1 - beta1^t) * m_t / u_t
    var_slice = -lr_t / (1 - beta1_power) * (m_t_slice /
                                             (v_t_slice + epsilon_t))
    with ops.control_dependencies([var_slice]):
      var_update = scatter_add(var, indices, var_slice)
    return control_flow_ops.group(*[var_update, m_t, v_t])

  def _apply_sparse(self, grad, var):
    return self._apply_sparse_shared(
        grad.values, var, grad.indices,
        lambda x, i, v: state_ops.scatter_add(  # pylint: disable=g-long-lambda
            x, i, v, use_locking=self._use_locking),
        lambda x, i, v: state_ops.scatter_update(  # pylint: disable=g-long-lambda
            x, i, v, use_locking=self._use_locking))

  def _resource_scatter_update(self, x, i, v):
    with ops.control_dependencies(
        [resource_variable_ops.resource_scatter_update(
            x.handle, i, v)]):
      return x.value()

  def _resource_apply_sparse(self, grad, var, indices):
    return self._apply_sparse_shared(
        grad, var, indices,
        self._resource_scatter_add, self._resource_scatter_update)

  def _finish(self, update_ops, name_scope):
    # Update the power accumulators.
    with ops.control_dependencies(update_ops):
      beta1_power = self._get_beta_accumulators()
      with ops.colocate_with(beta1_power):
        update_beta1 = beta1_power.assign(
            beta1_power * self._beta1_t, use_locking=self._use_locking)
    return control_flow_ops.group(*update_ops + [update_beta1],
                                  name=name_scope)
