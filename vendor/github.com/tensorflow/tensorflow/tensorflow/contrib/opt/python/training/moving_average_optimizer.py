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
"""Moving average optimizer."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import six

from tensorflow.python.framework import ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import variables
from tensorflow.python.training import moving_averages
from tensorflow.python.training import optimizer
from tensorflow.python.training import saver
from tensorflow.python.training.saving import saveable_object_util


class MovingAverageOptimizer(optimizer.Optimizer):
  """Optimizer that computes a moving average of the variables.

  Empirically it has been found that using the moving average of the trained
  parameters of a deep network is better than using its trained parameters
  directly. This optimizer allows you to compute this moving average and swap
  the variables at save time so that any code outside of the training loop will
  use by default the averaged values instead of the original ones.

  Example of usage:

  ```python

  // Encapsulate your favorite optimizer (here the momentum one)
  // inside the MovingAverageOptimizer.
  opt = tf.train.MomentumOptimizer(learning_rate, FLAGS.momentum)
  opt = tf.contrib.opt.MovingAverageOptimizer(opt)
  // Then create your model and all its variables.
  model = build_model()
  // Add the training op that optimizes using opt.
  // This needs to be called before swapping_saver().
  opt.minimize(cost, var_list)
  // Then create your saver like this:
  saver = opt.swapping_saver()
  // Pass it to your training loop.
      slim.learning.train(
          model,
          ...
          saver=saver)
  ```

  Note that for evaluation, the normal saver should be used instead of
  swapping_saver().
  """

  def __init__(self, opt, average_decay=0.9999, num_updates=None,
               sequential_update=True):
    """Construct a new MovingAverageOptimizer.

    Args:
      opt: A tf.Optimizer that will be used to compute and apply gradients.
      average_decay: Float.  Decay to use to maintain the moving averages
                     of trained variables.
                     See tf.train.ExponentialMovingAverage for details.
      num_updates: Optional count of number of updates applied to variables.
                   See tf.train.ExponentialMovingAverage for details.
      sequential_update: Bool. If False, will compute the moving average at the
                         same time as the model is updated, potentially doing
                         benign data races.
                         If True, will update the moving average after gradient
                         updates.
    """
    self._optimizer = opt
    self._ema = moving_averages.ExponentialMovingAverage(
        average_decay, num_updates=num_updates)
    self._swapped_variable_name_map = None
    self._sequential_update = sequential_update

  def compute_gradients(self, *args, **kwargs):
    return self._optimizer.compute_gradients(*args, **kwargs)

  def apply_gradients(self, grads_and_vars, global_step=None, name=None):
    train_op = self._optimizer.apply_gradients(
        grads_and_vars, global_step=global_step, name=name)
    var_list = [x[1] for x in grads_and_vars if x[0] is not None]
    self._swapped_variable_name_map = {}
    if self._sequential_update:
      with ops.control_dependencies([train_op]):
        ma_op = self._ema.apply(var_list)
    else:
      ma_op = self._ema.apply(var_list)

    for v in var_list:
      v_avg = self._ema.average(v)
      self._swapped_variable_name_map[v.op.name] = v_avg.op.name
      self._swapped_variable_name_map[v_avg.op.name] = v.op.name
    return control_flow_ops.group(train_op, ma_op, name='train_with_avg')

  def _find_swapped_variable(self, v_name_to_tensor, v_name, tensor):
    """Returns name of swapped variable for given tensor.

    Args:
      v_name_to_tensor: Mapping from variable names to tensors.
      v_name: name of the variable for which swapped variable should be returned
      tensor: Tensor which correspond to variable for which swapped variable
        should be returned.

    Returns:
      Tensor which correspond to swapped variable.

    Raises:
      ValueError: If swapped variable could not be found in v_name_to_tensor.
    """
    swapped_v_name = self._swapped_variable_name_map.get(v_name, None)
    if swapped_v_name is None:
      return tensor
    else:
      if swapped_v_name in v_name_to_tensor:
        return v_name_to_tensor[swapped_v_name]
      else:
        raise ValueError(
            ('Variable to swap %s is not part of variables to save. '
             'This breaks MovingAverageOptimizer.') % swapped_v_name)

  def swapping_saver(self, var_list=None, name='swapping_saver', **kwargs):
    """Create a saver swapping moving averages and variables.

    You should use this saver during training.  It will save the moving averages
    of the trained parameters under the original parameter names.  For
    evaluations or inference you should use a regular saver and it will
    automatically use the moving averages for the trained variable.

    You must call this function after all variables have been created and after
    you have called Optimizer.minimize().

    Args:
      var_list: List of variables to save, as per `Saver()`.
                If set to None, will save all the variables that have been
                created before this call.
      name: The name of the saver.
      **kwargs: Keyword arguments of `Saver()`.

    Returns:
      A `tf.train.Saver` object.

    Raises:
      RuntimeError: If apply_gradients or minimize has not been called before.
      ValueError: If var_list is provided and contains some variables but not
        their moving average counterpart.
    """

    if self._swapped_variable_name_map is None:
      raise RuntimeError('Must call apply_gradients or minimize before '
                         'creating the swapping_saver')
    if var_list is None:
      var_list = variables.global_variables()
    if not isinstance(var_list, dict):
      var_list = saveable_object_util.op_list_to_dict(var_list)

    v_name_to_tensor = {}
    for k, tensor_or_list in six.iteritems(var_list):
      # For each partitioned variable OpListToDict returns list of constituent
      # parts instead of single tensor.
      if (isinstance(tensor_or_list, list)
          or isinstance(tensor_or_list, variables.PartitionedVariable)):
        for tensor in tensor_or_list:
          v_name = tensor.op.name
          v_name_to_tensor[v_name] = tensor
      else:
        v_name_to_tensor[k] = tensor_or_list

    # Now swap variables and moving averages
    swapped_var_list = {}
    for k, tensor_or_list in six.iteritems(var_list):
      if isinstance(tensor_or_list, list):
        tensor_list_to_save = []
        for tensor in tensor_or_list:
          v_name = tensor.op.name
          swapped_variable = self._find_swapped_variable(v_name_to_tensor,
                                                         v_name,
                                                         tensor)
          tensor_list_to_save.append(swapped_variable)
        swapped_var_list[k] = tensor_list_to_save
      else:
        swapped_var_list[k] = self._find_swapped_variable(
            v_name_to_tensor, k, tensor_or_list)

    # Build the swapping saver.
    return saver.Saver(swapped_var_list, name=name, **kwargs)
