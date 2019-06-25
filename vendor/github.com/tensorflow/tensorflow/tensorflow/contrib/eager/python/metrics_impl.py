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
"""Metrics classes for computing the output of an evaluation."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import re

from tensorflow.python.eager import context
from tensorflow.python.eager import function
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import smart_cond
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import check_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import summary_ops_v2 as summary_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.training.checkpointable import base as checkpointable

_to_replace = re.compile("[^A-Za-z0-9.]")


class Metric(checkpointable.CheckpointableBase):
  """A metric holds state for aggregating statistics over an evaluation run.

  Example use with eager execution:

  ```python
  m = SomeMetric(...)
  for input in ...:
    m(input)
  print(m.result())
  ```

  Example use with graph execution:

  ```python
  m = SomeMetric(...)
  inputs = ... # Some tensors to compute the metric on.
  m_update = m(inputs)
  # Variables defined in first call, so get the initialization op afterwards.
  m_init = m.init_variables()  # or tf.global_variables_initializer()
  m_result = m.result()
  with tf.Session() as sess:
    sess.run(m_init)
    for input in ...:
      sess.run(m_update)
    print(sess.run(m_result))
  ```
  Example use with graph execution with placeholders and feed_dict:
  ```python
  m = SomeMetric(...)
  m_placeholder = tf.placeholder(...)
  m_update = m(m_placeholder)
  # Variables defined in first call, so get the initialization op afterwards.
  m_init = m.init_variables()  # or tf.global_variables_initializer()
  m_result = m.result()
  with tf.Session() as sess:
    sess.run(m_init)
    for input in ...:
      sess.run(m_update, feed_dict={m_placeholder: input})
    print(sess.run(m_result))
  ```

  Descendants will implement:
  * `build()`: All variables should be created in this method, by calling
    `self.add_variable()` as in: `self.var = self.add_variable(...)`
    build() will be called in the first invocation of `__call__()`, with
    the same arguments passed `call()`.
  * `call()`: Has all updates to variables, as in:
      self.var.assign_add(...)
  * `result()`: Computes and returns a final value for the metric
    from the variables in `self`.

  Descendants may override `aggregate()`, but usually won't need to.  It
  adds in the state from a list of metrics of the same type as `self`.
  (Default is to sum all the variables.) Note that users should not call
  `aggregate()`, it is for use by TensorFlow infrastructure.
  """

  def __init__(self, name=None, use_global_variables=False):
    self._built = False
    self._vars = []
    self._initial_values = {}
    self._updates = []
    self._use_global_variables = use_global_variables
    name = name or self.__class__.__name__
    # Replace things like spaces in name to create a valid scope name.
    scope_name = _to_replace.sub("_", name)
    # We create the variable scope now to get the unique name that will
    # be used as a variable prefix when build() calls add_variable().
    with variable_scope.variable_scope(
        scope_name, use_resource=True, reuse=False) as scope:
      pos = scope.name.rfind(scope_name)
      self._name = name + scope.name[pos + len(scope_name):]
      self._scope = scope

    # Ensures that if the user calls build directly we still set self._built to
    # True to prevent variables from being recreated.
    self._build = self.build

    def actual_build(*args, **kwargs):
      self._build(*args, **kwargs)
      self._built = True
    self.build = actual_build
    self.build.__doc__ = self._build.__doc__

    # Captures construction scope for proper initialization.
    if context.executing_eagerly():
      self._construction_scope = context.eager_mode
    else:
      # We make self.call() into a graph callable here, so that we can
      # return a single op that performs all of the variable updates.
      self._construction_scope = ops.get_default_graph().as_default
      self.call = function.defun(self.call)

  # ---- API for users ----
  def __call__(self, *args, **kwargs):
    """Returns op to execute to update this metric for these inputs.

    Returns None if eager execution is enabled.
    Returns a graph-mode function if graph execution is enabled.

    Args:
      *args:
      **kwargs: A mini-batch of inputs to the Metric, passed on to `call()`.
    """
    if not self._built:
      with variable_scope.variable_scope(
          self._scope), self._construction_scope():
        self.build(*args, **kwargs)
      self._built = True
    return self.call(*args, **kwargs)

  @property
  def name(self):
    return self._name

  @property
  def variables(self):
    return self._vars

  def init_variables(self):
    """Initializes this Metric's variables.

    Should be called after variables are created in the first execution
    of `__call__()`. If using graph execution, the return value should be
    `run()` in a session before running the op returned by `__call__()`.
    (See example above.)

    Returns:
      If using graph execution, this returns an op to perform the
      initialization. Under eager execution, the variables are reset to their
      initial values as a side effect and this function returns None.
    """
    if context.executing_eagerly():
      for v in self._vars:
        v.assign(self._initial_values[v])
    else:
      return control_flow_ops.group([v.initializer for v in self._vars])

  # ---- To be implemented by descendants ---
  def build(self, *args, **kwargs):
    """Method to create variables.

    Called by `__call__()` before `call()` for the first time.

    Args:
      *args:
      **kwargs: The arguments to the first invocation of `__call__()`.
       `build()` may use the shape and/or dtype of these arguments
       when deciding how to create variables.
    """
    raise NotImplementedError("Metrics must define a build() member function")

  def call(self, *args, **kwargs):
    """Accumulates statistics for the metric. Users should use __call__ instead.

    Note: This function is executed as a graph function in graph mode.
    This means:
    a) Operations on the same resource are executed in textual order.
       This should make it easier to do things like add the updated
       value of a variable to another, for example.
    b) You don't need to worry about collecting the update ops to execute.
       All update ops added to the graph by this function will be executed.
    As a result, code should generally work the same way with graph or
    eager execution.

    Args:
      *args:
      **kwargs: A mini-batch of inputs to the Metric, as passed to
        `__call__()`.
    """
    raise NotImplementedError("Metrics must define a call() member function")

  def result(self):  # TODO(josh11b): Add an optional summary_writer parameter.
    """Computes and returns a final value for the metric."""
    raise NotImplementedError("Metrics must define a result() member function")

  def value(self):
    """In graph mode returns the result Tensor while in eager the callable."""
    if context.executing_eagerly():
      return self.result
    else:
      return self.result()

  # We can support two different strategies of for doing data-parallel
  # distributed metric computations:
  # * Put metric variables on the first device and rely on small
  #   bandwidth needed to do updates. (Doesn't require any particular
  #   code in Metric implementations.)
  # * Ask each type of metric to define an aggregation method to run
  #   at the end of eval to merge across devices. Note: this is good
  #   for the use case where they want to record the metric's state
  #   for each example and then later decide which examples they want
  #   to aggregate over. (Recommended -- not too much harder and adds
  #   flexibility over previous option.)
  # I'm going with the second strategy since we can define a default
  # implementation of aggregate() that will work for most descendants.
  def aggregate(self, metrics):
    """Adds in the state from a list of metrics.

    Default implementation sums all the metric variables.

    Args:
      metrics: A list of metrics with the same type as `self`.

    Raises:
      ValueError: If metrics contains invalid data.
    """
    for m in metrics:
      if type(self) != type(m):  # pylint: disable=unidiomatic-typecheck
        raise TypeError("All metrics must be the same type, '%s' != '%s'." %
                        (type(self), type(m)))
    # pylint: disable=protected-access
    for i in range(len(self._vars)):
      if any(m._vars[i].name != self._vars[i].name for m in metrics):
        raise ValueError("All metrics must have variables in the same order.")
      self._vars[i].assign_add(math_ops.add_n([m._vars[i] for m in metrics]))
    # pylint: enable=protected-access

  # ---- For use by descendants ---
  def add_variable(self, name, shape=None, dtype=None, initializer=None):
    """***Only for use by descendants of Metric***."""
    if self._built:
      raise RuntimeError("Can't call add_variable() except in build().")
    if context.executing_eagerly():
      collections = None
    else:
      if self._use_global_variables:
        collections = [ops.GraphKeys.GLOBAL_VARIABLES]
      else:
        collections = [ops.GraphKeys.LOCAL_VARIABLES]
      collections += [ops.GraphKeys.METRIC_VARIABLES]
    # Variables are Checkpointable dependencies of Metrics regardless of the
    # global/local distinction. Users can avoid saving variables by not adding a
    # dependency on the Metric.
    v = self._add_variable_with_custom_getter(
        name=name,
        shape=shape,
        dtype=dtype,
        initializer=initializer,
        trainable=False,
        collections=collections,
        use_resource=True,
        getter=variable_scope.get_variable,
        # Raise duplicate variable exceptions from get_variable rather than
        # Checkpointable.
        overwrite=True)
    self._vars.append(v)
    if context.executing_eagerly():
      self._initial_values[v] = v.value()
    return v


class Mean(Metric):
  """Computes the (weighted) mean of the given values."""

  def __init__(self, name=None, dtype=dtypes.float64,
               use_global_variables=False):
    super(Mean, self).__init__(name=name,
                               use_global_variables=use_global_variables)
    self.dtype = dtype

  def build(self, *args, **kwargs):
    # build() does not use call's arguments, by using *args, **kwargs
    # we make it easier to inherit from Mean().
    del args, kwargs
    self.numer = self.add_variable(name="numer", shape=(),
                                   dtype=self.dtype,
                                   initializer=init_ops.zeros_initializer)
    self.denom = self.add_variable(name="denom", shape=(),
                                   dtype=self.dtype,
                                   initializer=init_ops.zeros_initializer)

  def call(self, values, weights=None):
    """Accumulate statistics for computing the mean.

    For example, if values is [1, 3, 5, 7] then the mean is 4.
    If the weights were specified as [1, 1, 0, 0] then the mean would be 2.

    Args:
      values: Tensor with the per-example value.
      weights: Optional weighting of each example. Defaults to 1.

    Returns:
      The arguments, for easy chaining.
    """
    if weights is None:
      self.denom.assign_add(
          math_ops.cast(array_ops.identity(array_ops.size(values)), self.dtype))
      values = math_ops.reduce_sum(values)
      self.numer.assign_add(math_ops.cast(values, self.dtype))
    else:
      weights = math_ops.cast(weights, self.dtype)
      self.denom.assign_add(math_ops.reduce_sum(weights))
      values = math_ops.cast(values, self.dtype) * weights
      self.numer.assign_add(math_ops.reduce_sum(values))
    if weights is None:
      return values
    return values, weights

  def result(self, write_summary=True):
    """Returns the result of the Metric.

    Args:
      write_summary: bool indicating whether to feed the result to the summary
        before returning.
    Returns:
      aggregated metric as float.
    Raises:
      ValueError: if the optional argument is not bool
    """
    # Convert the boolean to tensor for tf.cond, if it is not.
    if not isinstance(write_summary, ops.Tensor):
      write_summary = ops.convert_to_tensor(write_summary)
    t = self.numer / self.denom
    def write_summary_f():
      summary_ops.scalar(name=self.name, tensor=t)
      return t
    smart_cond.smart_cond(write_summary,
                          write_summary_f,
                          lambda: t,
                          name="")
    return t


class Accuracy(Mean):
  """Calculates how often `predictions` matches `labels`.
  Attributes:
    name: name of the accuracy object
    dtype: data type of the tensor
  """

  def __init__(self, name=None, dtype=dtypes.float64):
    """Inits Accuracy class with name and dtype."""
    super(Accuracy, self).__init__(name=name, dtype=dtype)

  def call(self, labels, predictions, weights=None):
    """Accumulate accuracy statistics.

    For example, if labels is [1, 2, 3, 4] and predictions is [0, 2, 3, 4]
    then the accuracy is 3/4 or .75.  If the weights were specified as
    [1, 1, 0, 0] then the accuracy would be 1/2 or .5.

    `labels` and `predictions` should have the same shape and type.

    Args:
      labels: Tensor with the true labels for each example.  One example
        per element of the Tensor.
      predictions: Tensor with the predicted label for each example.
      weights: Optional weighting of each example. Defaults to 1.

    Returns:
      The arguments, for easy chaining.
    """
    check_ops.assert_equal(
        array_ops.shape(labels), array_ops.shape(predictions),
        message="Shapes of labels and predictions are unequal")
    matches = math_ops.equal(labels, predictions)
    matches = math_ops.cast(matches, self.dtype)
    super(Accuracy, self).call(matches, weights=weights)
    if weights is None:
      return labels, predictions
    return labels, predictions, weights


class CategoricalAccuracy(Mean):
  """Calculates how often `predictions` matches `labels`.

  This class is compatible with `tf.keras.losses.categorical_crossentropy`,
  `tf.nn.softmax_cross_entropy_with_logits_v2`,
  `tf.losses.softmax_cross_entropy`.

  Attributes:
    name: name of the accuracy object.
    dtype: data type of tensor.
  """

  def __init__(self, name=None, dtype=dtypes.float64):
    """Inits CategoricalAccuracy with name and dtype."""
    super(CategoricalAccuracy, self).__init__(name=name, dtype=dtype)

  def call(self, labels, predictions, weights=None):
    """Accumulate accuracy statistics.

    `labels` and `predictions` should have the same shape.
    As argmax is being done here, labels and predictions type
    can be different.

    Args:
      labels: One-hot Tensor.
      predictions: Tensor with the logits or probabilities for each example.
      weights: Optional weighting of each example. Defaults to 1.

    Returns:
      The arguments, for easy chaining.
    """
    check_ops.assert_equal(
        array_ops.shape(labels), array_ops.shape(predictions),
        message="Shapes of labels and predictions are unequal")
    labels = math_ops.argmax(labels, axis=-1)
    predictions = math_ops.argmax(predictions, axis=-1)
    matches = math_ops.equal(labels, predictions)
    matches = math_ops.cast(matches, self.dtype)
    super(CategoricalAccuracy, self).call(matches, weights=weights)
    if weights is None:
      return labels, predictions
    return labels, predictions, weights


class BinaryAccuracy(Mean):
  """Calculates how often `predictions` matches `labels`.

  This class is compatible with `tf.keras.losses.binary_crossentropy`,
  `tf.losses.sigmoid_cross_entropy`,
  `tf.nn.sigmoid_cross_entropy_with_logits`.
  If there is more than one label, this will become multi-label classification.

  Attributes:
    name: name of the accuracy object.
    threshold: Used for rounding off the predictions.
               If the predictions are,
                1. probabilities then set the threshold to 0.5.
                2. logits then set the threshold to 0.
              You can set the threshold appropriately,
              to trade off with precision and recall.
    dtype: data type of tensor.
  """

  def __init__(self, threshold, name=None, dtype=dtypes.float64):
    """Inits BinaryAccuracy with name, threshold and dtype."""

    super(BinaryAccuracy, self).__init__(name=name, dtype=dtype)
    self.threshold = threshold

  def call(self, labels, predictions, weights=None):
    """Accumulate accuracy statistics.

    `labels` and `predictions` should have the same shape and type.

    Args:
      labels: Binary Tensor(containing 0 or 1).
      predictions: Tensor with probabilities or logits.
      weights: Optional weighting of each example. Defaults to 1.

    Returns:
      The arguments, for easy chaining.
    """
    check_ops.assert_equal(
        array_ops.shape(labels), array_ops.shape(predictions),
        message="Shapes of labels and predictions are unequal")
    predictions = ops.convert_to_tensor(predictions)
    predictions = predictions > self.threshold
    # Convert labels to bool to match predictions.
    labels = math_ops.cast(labels, dtypes.bool)
    matches = math_ops.equal(labels, predictions)
    matches = math_ops.cast(matches, self.dtype)
    super(BinaryAccuracy, self).call(matches, weights=weights)
    if weights is None:
      return labels, predictions
    return labels, predictions, weights


class SparseAccuracy(Mean):
  """Calculates how often `predictions` matches `labels`.

  This class is compatible with
  `tf.keras.losses.sparse_categorical_crossentropy`,
  `tf.nn.sparse_softmax_cross_entropy_with_logits`,
  `tf.losses.sparse_softmax_cross_entropy`.

  Attributes:
    name: name of the accuracy object
    dtype: data type of tensor.
  """

  def __init__(self, name=None, dtype=dtypes.float64):
    """Inits SparseAccuracy with name and dtype."""

    super(SparseAccuracy, self).__init__(name=name, dtype=dtype)

  def call(self, labels, predictions, weights=None):
    """Accumulate accuracy statistics.

    `labels` and `predictions` should have the same shape except the
    predictions must have one additional trailing dimension equal to the
    number of classes(you want to predict).

    Type of labels and predictions can be different.

    Args:
      labels: Tensor of shape (batch_size, ) containing integers
      predictions: Tensor with the logits or probabilities for each example.
      weights: Optional weighting of each example. Defaults to 1.

    Returns:
      The arguments, for easy chaining.
    """
    check_ops.assert_equal(
        array_ops.shape(labels), array_ops.shape(predictions)[0],
        message="First axis of labels and predictions is unequal")
    predictions = math_ops.argmax(predictions, axis=-1)
    labels = math_ops.cast(labels, dtypes.int64)
    matches = math_ops.equal(labels, predictions)
    matches = math_ops.cast(matches, self.dtype)
    super(SparseAccuracy, self).call(matches, weights=weights)
    if weights is None:
      return labels, predictions
    return labels, predictions, weights
