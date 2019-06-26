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
"""Implementation of tf.metrics module."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.distribute import distribution_strategy_context
from tensorflow.python.eager import context
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import check_ops
from tensorflow.python.ops import confusion_matrix
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn
from tensorflow.python.ops import sets
from tensorflow.python.ops import sparse_ops
from tensorflow.python.ops import state_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops import weights_broadcast_ops
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.util.deprecation import deprecated
from tensorflow.python.util.tf_export import tf_export


def metric_variable(shape, dtype, validate_shape=True, name=None):
  """Create variable in `GraphKeys.(LOCAL|METRIC_VARIABLES)` collections.

  If running in a `DistributionStrategy` context, the variable will be
  "replica local". This means:

  *   The returned object will be a container with separate variables
      per replica of the model.

  *   When writing to the variable, e.g. using `assign_add` in a metric
      update, the update will be applied to the variable local to the
      replica.

  *   To get a metric's result value, we need to sum the variable values
      across the replicas before computing the final answer. Furthermore,
      the final answer should be computed once instead of in every
      replica. Both of these are accomplished by running the computation
      of the final result value inside
      `distribution_strategy_context.get_replica_context().merge_call(fn)`.
      Inside the `merge_call()`, ops are only added to the graph once
      and access to a replica-local variable in a computation returns
      the sum across all replicas.

  Args:
    shape: Shape of the created variable.
    dtype: Type of the created variable.
    validate_shape: (Optional) Whether shape validation is enabled for
      the created variable.
    name: (Optional) String name of the created variable.

  Returns:
    A (non-trainable) variable initialized to zero, or if inside a
    `DistributionStrategy` scope a replica-local variable container.
  """
  # Note that synchronization "ON_READ" implies trainable=False.
  return variable_scope.variable(
      lambda: array_ops.zeros(shape, dtype),
      collections=[
          ops.GraphKeys.LOCAL_VARIABLES, ops.GraphKeys.METRIC_VARIABLES
      ],
      validate_shape=validate_shape,
      synchronization=variable_scope.VariableSynchronization.ON_READ,
      aggregation=variable_scope.VariableAggregation.SUM,
      name=name)


def _remove_squeezable_dimensions(predictions, labels, weights):
  """Squeeze or expand last dim if needed.

  Squeezes last dim of `predictions` or `labels` if their rank differs by 1
  (using confusion_matrix.remove_squeezable_dimensions).
  Squeezes or expands last dim of `weights` if its rank differs by 1 from the
  new rank of `predictions`.

  If `weights` is scalar, it is kept scalar.

  This will use static shape if available. Otherwise, it will add graph
  operations, which could result in a performance hit.

  Args:
    predictions: Predicted values, a `Tensor` of arbitrary dimensions.
    labels: Optional label `Tensor` whose dimensions match `predictions`.
    weights: Optional weight scalar or `Tensor` whose dimensions match
      `predictions`.

  Returns:
    Tuple of `predictions`, `labels` and `weights`. Each of them possibly has
    the last dimension squeezed, `weights` could be extended by one dimension.
  """
  predictions = ops.convert_to_tensor(predictions)
  if labels is not None:
    labels, predictions = confusion_matrix.remove_squeezable_dimensions(
        labels, predictions)
    predictions.get_shape().assert_is_compatible_with(labels.get_shape())

  if weights is None:
    return predictions, labels, None

  weights = ops.convert_to_tensor(weights)
  weights_shape = weights.get_shape()
  weights_rank = weights_shape.ndims
  if weights_rank == 0:
    return predictions, labels, weights

  predictions_shape = predictions.get_shape()
  predictions_rank = predictions_shape.ndims
  if (predictions_rank is not None) and (weights_rank is not None):
    # Use static rank.
    if weights_rank - predictions_rank == 1:
      weights = array_ops.squeeze(weights, [-1])
    elif predictions_rank - weights_rank == 1:
      weights = array_ops.expand_dims(weights, [-1])
  else:
    # Use dynamic rank.
    weights_rank_tensor = array_ops.rank(weights)
    rank_diff = weights_rank_tensor - array_ops.rank(predictions)

    def _maybe_expand_weights():
      return control_flow_ops.cond(
          math_ops.equal(rank_diff, -1),
          lambda: array_ops.expand_dims(weights, [-1]), lambda: weights)

    # Don't attempt squeeze if it will fail based on static check.
    if ((weights_rank is not None) and
        (not weights_shape.dims[-1].is_compatible_with(1))):
      maybe_squeeze_weights = lambda: weights
    else:
      maybe_squeeze_weights = lambda: array_ops.squeeze(weights, [-1])

    def _maybe_adjust_weights():
      return control_flow_ops.cond(
          math_ops.equal(rank_diff, 1), maybe_squeeze_weights,
          _maybe_expand_weights)

    # If weights are scalar, do nothing. Otherwise, try to add or remove a
    # dimension to match predictions.
    weights = control_flow_ops.cond(
        math_ops.equal(weights_rank_tensor, 0), lambda: weights,
        _maybe_adjust_weights)
  return predictions, labels, weights


def _maybe_expand_labels(labels, predictions):
  """If necessary, expand `labels` along last dimension to match `predictions`.

  Args:
    labels: `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN]. The latter implies
      num_labels=1, in which case the result is an expanded `labels` with shape
      [D1, ... DN, 1].
    predictions: `Tensor` with shape [D1, ... DN, num_classes].

  Returns:
    `labels` with the same rank as `predictions`.

  Raises:
    ValueError: if `labels` has invalid shape.
  """
  with ops.name_scope(None, 'expand_labels', (labels, predictions)) as scope:
    labels = sparse_tensor.convert_to_tensor_or_sparse_tensor(labels)

    # If sparse, expand sparse shape.
    if isinstance(labels, sparse_tensor.SparseTensor):
      return control_flow_ops.cond(
          math_ops.equal(
              array_ops.rank(predictions),
              array_ops.size(labels.dense_shape) + 1),
          lambda: sparse_ops.sparse_reshape(  # pylint: disable=g-long-lambda
              labels,
              shape=array_ops.concat((labels.dense_shape, (1,)), 0),
              name=scope),
          lambda: labels)

    # Otherwise, try to use static shape.
    labels_rank = labels.get_shape().ndims
    if labels_rank is not None:
      predictions_rank = predictions.get_shape().ndims
      if predictions_rank is not None:
        if predictions_rank == labels_rank:
          return labels
        if predictions_rank == labels_rank + 1:
          return array_ops.expand_dims(labels, -1, name=scope)
        raise ValueError(
            'Unexpected labels shape %s for predictions shape %s.' %
            (labels.get_shape(), predictions.get_shape()))

    # Otherwise, use dynamic shape.
    return control_flow_ops.cond(
        math_ops.equal(array_ops.rank(predictions),
                       array_ops.rank(labels) + 1),
        lambda: array_ops.expand_dims(labels, -1, name=scope), lambda: labels)


def _safe_scalar_div(numerator, denominator, name):
  """Divides two values, returning 0 if the denominator is 0.

  Args:
    numerator: A scalar `float64` `Tensor`.
    denominator: A scalar `float64` `Tensor`.
    name: Name for the returned op.

  Returns:
    0 if `denominator` == 0, else `numerator` / `denominator`
  """
  numerator.get_shape().with_rank_at_most(1)
  denominator.get_shape().with_rank_at_most(1)
  return math_ops.div_no_nan(numerator, denominator, name=name)


def _streaming_confusion_matrix(labels, predictions, num_classes, weights=None):
  """Calculate a streaming confusion matrix.

  Calculates a confusion matrix. For estimation over a stream of data,
  the function creates an  `update_op` operation.

  Args:
    labels: A `Tensor` of ground truth labels with shape [batch size] and of
      type `int32` or `int64`. The tensor will be flattened if its rank > 1.
    predictions: A `Tensor` of prediction results for semantic labels, whose
      shape is [batch size] and type `int32` or `int64`. The tensor will be
      flattened if its rank > 1.
    num_classes: The possible number of labels the prediction task can
      have. This value must be provided, since a confusion matrix of
      dimension = [num_classes, num_classes] will be allocated.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).

  Returns:
    total_cm: A `Tensor` representing the confusion matrix.
    update_op: An operation that increments the confusion matrix.
  """
  # Local variable to accumulate the predictions in the confusion matrix.
  total_cm = metric_variable(
      [num_classes, num_classes], dtypes.float64, name='total_confusion_matrix')

  # Cast the type to int64 required by confusion_matrix_ops.
  predictions = math_ops.to_int64(predictions)
  labels = math_ops.to_int64(labels)
  num_classes = math_ops.to_int64(num_classes)

  # Flatten the input if its rank > 1.
  if predictions.get_shape().ndims > 1:
    predictions = array_ops.reshape(predictions, [-1])

  if labels.get_shape().ndims > 1:
    labels = array_ops.reshape(labels, [-1])

  if (weights is not None) and (weights.get_shape().ndims > 1):
    weights = array_ops.reshape(weights, [-1])

  # Accumulate the prediction to current confusion matrix.
  current_cm = confusion_matrix.confusion_matrix(
      labels, predictions, num_classes, weights=weights, dtype=dtypes.float64)
  update_op = state_ops.assign_add(total_cm, current_cm)
  return total_cm, update_op


def _aggregate_across_replicas(metrics_collections, metric_value_fn, *args):
  """Aggregate metric value across replicas."""
  def fn(distribution, *a):
    """Call `metric_value_fn` in the correct control flow context."""
    if hasattr(distribution.extended, '_outer_control_flow_context'):
      # If there was an outer context captured before this method was called,
      # then we enter that context to create the metric value op. If the
      # caputred context is `None`, ops.control_dependencies(None) gives the
      # desired behavior. Else we use `Enter` and `Exit` to enter and exit the
      # captured context.
      # This special handling is needed because sometimes the metric is created
      # inside a while_loop (and perhaps a TPU rewrite context). But we don't
      # want the value op to be evaluated every step or on the TPU. So we
      # create it outside so that it can be evaluated at the end on the host,
      # once the update ops have been evaluted.

      # pylint: disable=protected-access
      if distribution.extended._outer_control_flow_context is None:
        with ops.control_dependencies(None):
          metric_value = metric_value_fn(distribution, *a)
      else:
        distribution.extended._outer_control_flow_context.Enter()
        metric_value = metric_value_fn(distribution, *a)
        distribution.extended._outer_control_flow_context.Exit()
        # pylint: enable=protected-access
    else:
      metric_value = metric_value_fn(distribution, *a)
    if metrics_collections:
      ops.add_to_collections(metrics_collections, metric_value)
    return metric_value

  return distribution_strategy_context.get_replica_context().merge_call(
      fn, args=args)


@tf_export(v1=['metrics.mean'])
def mean(values,
         weights=None,
         metrics_collections=None,
         updates_collections=None,
         name=None):
  """Computes the (weighted) mean of the given values.

  The `mean` function creates two local variables, `total` and `count`
  that are used to compute the average of `values`. This average is ultimately
  returned as `mean` which is an idempotent operation that simply divides
  `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `mean`.
  `update_op` increments `total` with the reduced sum of the product of `values`
  and `weights`, and it increments `count` with the reduced sum of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    values: A `Tensor` of arbitrary dimensions.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `values`, and must be broadcastable to `values` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `values` dimension).
    metrics_collections: An optional list of collections that `mean`
      should be added to.
    updates_collections: An optional list of collections that `update_op`
      should be added to.
    name: An optional variable_scope name.

  Returns:
    mean: A `Tensor` representing the current mean, the value of `total` divided
      by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `mean_value`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match `values`,
      or if either `metrics_collections` or `updates_collections` are not a list
      or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean is not supported when eager execution '
                       'is enabled.')

  with variable_scope.variable_scope(name, 'mean', (values, weights)):
    values = math_ops.to_float(values)

    total = metric_variable([], dtypes.float32, name='total')
    count = metric_variable([], dtypes.float32, name='count')

    if weights is None:
      num_values = math_ops.to_float(array_ops.size(values))
    else:
      values, _, weights = _remove_squeezable_dimensions(
          predictions=values, labels=None, weights=weights)
      weights = weights_broadcast_ops.broadcast_weights(
          math_ops.to_float(weights), values)
      values = math_ops.multiply(values, weights)
      num_values = math_ops.reduce_sum(weights)

    update_total_op = state_ops.assign_add(total, math_ops.reduce_sum(values))
    with ops.control_dependencies([values]):
      update_count_op = state_ops.assign_add(count, num_values)

    def compute_mean(_, t, c):
      return math_ops.div_no_nan(t, math_ops.maximum(c, 0), name='value')

    mean_t = _aggregate_across_replicas(
        metrics_collections, compute_mean, total, count)
    update_op = math_ops.div_no_nan(
        update_total_op, math_ops.maximum(update_count_op, 0), name='update_op')

    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return mean_t, update_op


@tf_export(v1=['metrics.accuracy'])
def accuracy(labels,
             predictions,
             weights=None,
             metrics_collections=None,
             updates_collections=None,
             name=None):
  """Calculates how often `predictions` matches `labels`.

  The `accuracy` function creates two local variables, `total` and
  `count` that are used to compute the frequency with which `predictions`
  matches `labels`. This frequency is ultimately returned as `accuracy`: an
  idempotent operation that simply divides `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `accuracy`.
  Internally, an `is_correct` operation computes a `Tensor` with elements 1.0
  where the corresponding elements of `predictions` and `labels` match and 0.0
  otherwise. Then `update_op` increments `total` with the reduced sum of the
  product of `weights` and `is_correct`, and it increments `count` with the
  reduced sum of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose shape matches
      `predictions`.
    predictions: The predicted values, a `Tensor` of any shape.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `accuracy` should
      be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    accuracy: A `Tensor` representing the accuracy, the value of `total` divided
      by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `accuracy`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.accuracy is not supported when eager '
                       'execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)
  predictions.get_shape().assert_is_compatible_with(labels.get_shape())
  if labels.dtype != predictions.dtype:
    predictions = math_ops.cast(predictions, labels.dtype)
  is_correct = math_ops.to_float(math_ops.equal(predictions, labels))
  return mean(is_correct, weights, metrics_collections, updates_collections,
              name or 'accuracy')


def _confusion_matrix_at_thresholds(labels,
                                    predictions,
                                    thresholds,
                                    weights=None,
                                    includes=None):
  """Computes true_positives, false_negatives, true_negatives, false_positives.

  This function creates up to four local variables, `true_positives`,
  `true_negatives`, `false_positives` and `false_negatives`.
  `true_positive[i]` is defined as the total weight of values in `predictions`
  above `thresholds[i]` whose corresponding entry in `labels` is `True`.
  `false_negatives[i]` is defined as the total weight of values in `predictions`
  at most `thresholds[i]` whose corresponding entry in `labels` is `True`.
  `true_negatives[i]` is defined as the total weight of values in `predictions`
  at most `thresholds[i]` whose corresponding entry in `labels` is `False`.
  `false_positives[i]` is defined as the total weight of values in `predictions`
  above `thresholds[i]` whose corresponding entry in `labels` is `False`.

  For estimation of these metrics over a stream of data, for each metric the
  function respectively creates an `update_op` operation that updates the
  variable and returns its value.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    includes: Tuple of keys to return, from 'tp', 'fn', 'tn', fp'. If `None`,
        default to all four.

  Returns:
    values: Dict of variables of shape `[len(thresholds)]`. Keys are from
        `includes`.
    update_ops: Dict of operations that increments the `values`. Keys are from
        `includes`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      `includes` contains invalid keys.
  """
  all_includes = ('tp', 'fn', 'tn', 'fp')
  if includes is None:
    includes = all_includes
  else:
    for include in includes:
      if include not in all_includes:
        raise ValueError('Invalid key: %s.' % include)

  with ops.control_dependencies([
      check_ops.assert_greater_equal(
          predictions,
          math_ops.cast(0.0, dtype=predictions.dtype),
          message='predictions must be in [0, 1]'),
      check_ops.assert_less_equal(
          predictions,
          math_ops.cast(1.0, dtype=predictions.dtype),
          message='predictions must be in [0, 1]')
  ]):
    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.to_float(predictions),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)

  num_thresholds = len(thresholds)

  # Reshape predictions and labels.
  predictions_2d = array_ops.reshape(predictions, [-1, 1])
  labels_2d = array_ops.reshape(
      math_ops.cast(labels, dtype=dtypes.bool), [1, -1])

  # Use static shape if known.
  num_predictions = predictions_2d.get_shape().as_list()[0]

  # Otherwise use dynamic shape.
  if num_predictions is None:
    num_predictions = array_ops.shape(predictions_2d)[0]
  thresh_tiled = array_ops.tile(
      array_ops.expand_dims(array_ops.constant(thresholds), [1]),
      array_ops.stack([1, num_predictions]))

  # Tile the predictions after thresholding them across different thresholds.
  pred_is_pos = math_ops.greater(
      array_ops.tile(array_ops.transpose(predictions_2d), [num_thresholds, 1]),
      thresh_tiled)
  if ('fn' in includes) or ('tn' in includes):
    pred_is_neg = math_ops.logical_not(pred_is_pos)

  # Tile labels by number of thresholds
  label_is_pos = array_ops.tile(labels_2d, [num_thresholds, 1])
  if ('fp' in includes) or ('tn' in includes):
    label_is_neg = math_ops.logical_not(label_is_pos)

  if weights is not None:
    weights = weights_broadcast_ops.broadcast_weights(
        math_ops.to_float(weights), predictions)
    weights_tiled = array_ops.tile(
        array_ops.reshape(weights, [1, -1]), [num_thresholds, 1])
    thresh_tiled.get_shape().assert_is_compatible_with(
        weights_tiled.get_shape())
  else:
    weights_tiled = None

  values = {}
  update_ops = {}

  if 'tp' in includes:
    true_p = metric_variable(
        [num_thresholds], dtypes.float32, name='true_positives')
    is_true_positive = math_ops.to_float(
        math_ops.logical_and(label_is_pos, pred_is_pos))
    if weights_tiled is not None:
      is_true_positive *= weights_tiled
    update_ops['tp'] = state_ops.assign_add(true_p,
                                            math_ops.reduce_sum(
                                                is_true_positive, 1))
    values['tp'] = true_p

  if 'fn' in includes:
    false_n = metric_variable(
        [num_thresholds], dtypes.float32, name='false_negatives')
    is_false_negative = math_ops.to_float(
        math_ops.logical_and(label_is_pos, pred_is_neg))
    if weights_tiled is not None:
      is_false_negative *= weights_tiled
    update_ops['fn'] = state_ops.assign_add(false_n,
                                            math_ops.reduce_sum(
                                                is_false_negative, 1))
    values['fn'] = false_n

  if 'tn' in includes:
    true_n = metric_variable(
        [num_thresholds], dtypes.float32, name='true_negatives')
    is_true_negative = math_ops.to_float(
        math_ops.logical_and(label_is_neg, pred_is_neg))
    if weights_tiled is not None:
      is_true_negative *= weights_tiled
    update_ops['tn'] = state_ops.assign_add(true_n,
                                            math_ops.reduce_sum(
                                                is_true_negative, 1))
    values['tn'] = true_n

  if 'fp' in includes:
    false_p = metric_variable(
        [num_thresholds], dtypes.float32, name='false_positives')
    is_false_positive = math_ops.to_float(
        math_ops.logical_and(label_is_neg, pred_is_pos))
    if weights_tiled is not None:
      is_false_positive *= weights_tiled
    update_ops['fp'] = state_ops.assign_add(false_p,
                                            math_ops.reduce_sum(
                                                is_false_positive, 1))
    values['fp'] = false_p

  return values, update_ops


def _aggregate_variable(v, collections):
  f = lambda distribution, value: distribution.read_var(value)
  return _aggregate_across_replicas(collections, f, v)


@tf_export(v1=['metrics.auc'])
def auc(labels,
        predictions,
        weights=None,
        num_thresholds=200,
        metrics_collections=None,
        updates_collections=None,
        curve='ROC',
        name=None,
        summation_method='trapezoidal'):
  """Computes the approximate AUC via a Riemann sum.

  The `auc` function creates four local variables, `true_positives`,
  `true_negatives`, `false_positives` and `false_negatives` that are used to
  compute the AUC. To discretize the AUC curve, a linearly spaced set of
  thresholds is used to compute pairs of recall and precision values. The area
  under the ROC-curve is therefore computed using the height of the recall
  values by the false positive rate, while the area under the PR-curve is the
  computed using the height of the precision values by the recall.

  This value is ultimately returned as `auc`, an idempotent operation that
  computes the area under a discretized curve of precision versus recall values
  (computed using the aforementioned variables). The `num_thresholds` variable
  controls the degree of discretization with larger numbers of thresholds more
  closely approximating the true AUC. The quality of the approximation may vary
  dramatically depending on `num_thresholds`.

  For best results, `predictions` should be distributed approximately uniformly
  in the range [0, 1] and not peaked around 0 or 1. The quality of the AUC
  approximation may be poor if this is not the case. Setting `summation_method`
  to 'minoring' or 'majoring' can help quantify the error in the approximation
  by providing lower or upper bound estimate of the AUC.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `auc`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    num_thresholds: The number of thresholds to use when discretizing the roc
      curve.
    metrics_collections: An optional list of collections that `auc` should be
      added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    curve: Specifies the name of the curve to be computed, 'ROC' [default] or
      'PR' for the Precision-Recall-curve.
    name: An optional variable_scope name.
    summation_method: Specifies the Riemann summation method used
      (https://en.wikipedia.org/wiki/Riemann_sum): 'trapezoidal' [default] that
      applies the trapezoidal rule; 'careful_interpolation', a variant of it
      differing only by a more correct interpolation scheme for PR-AUC -
      interpolating (true/false) positives but not the ratio that is precision;
      'minoring' that applies left summation for increasing intervals and right
      summation for decreasing intervals; 'majoring' that does the opposite.
      Note that 'careful_interpolation' is strictly preferred to 'trapezoidal'
      (to be deprecated soon) as it applies the same method for ROC, and a
      better one (see Davis & Goadrich 2006 for details) for the PR curve.

  Returns:
    auc: A scalar `Tensor` representing the current area-under-curve.
    update_op: An operation that increments the `true_positives`,
      `true_negatives`, `false_positives` and `false_negatives` variables
      appropriately and whose value matches `auc`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.auc is not supported when eager execution '
                       'is enabled.')

  with variable_scope.variable_scope(name, 'auc',
                                     (labels, predictions, weights)):
    if curve != 'ROC' and curve != 'PR':
      raise ValueError('curve must be either ROC or PR, %s unknown' % (curve))
    kepsilon = 1e-7  # to account for floating point imprecisions
    thresholds = [
        (i + 1) * 1.0 / (num_thresholds - 1) for i in range(num_thresholds - 2)
    ]
    thresholds = [0.0 - kepsilon] + thresholds + [1.0 + kepsilon]

    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights)

    # Add epsilons to avoid dividing by 0.
    epsilon = 1.0e-6

    def interpolate_pr_auc(tp, fp, fn):
      """Interpolation formula inspired by section 4 of Davis & Goadrich 2006.

      Note here we derive & use a closed formula not present in the paper
      - as follows:
      Modeling all of TP (true positive weight),
      FP (false positive weight) and their sum P = TP + FP (positive weight)
      as varying linearly within each interval [A, B] between successive
      thresholds, we get
        Precision = (TP_A + slope * (P - P_A)) / P
      with slope = dTP / dP = (TP_B - TP_A) / (P_B - P_A).
      The area within the interval is thus (slope / total_pos_weight) times
        int_A^B{Precision.dP} = int_A^B{(TP_A + slope * (P - P_A)) * dP / P}
        int_A^B{Precision.dP} = int_A^B{slope * dP + intercept * dP / P}
      where intercept = TP_A - slope * P_A = TP_B - slope * P_B, resulting in
        int_A^B{Precision.dP} = TP_B - TP_A + intercept * log(P_B / P_A)
      Bringing back the factor (slope / total_pos_weight) we'd put aside, we get
         slope * [dTP + intercept *  log(P_B / P_A)] / total_pos_weight
      where dTP == TP_B - TP_A.
      Note that when P_A == 0 the above calculation simplifies into
        int_A^B{Precision.dTP} = int_A^B{slope * dTP} = slope * (TP_B - TP_A)
      which is really equivalent to imputing constant precision throughout the
      first bucket having >0 true positives.

      Args:
        tp: true positive counts
        fp: false positive counts
        fn: false negative counts
      Returns:
        pr_auc: an approximation of the area under the P-R curve.
      """
      dtp = tp[:num_thresholds - 1] - tp[1:]
      p = tp + fp
      prec_slope = math_ops.div_no_nan(
          dtp,
          math_ops.maximum(p[:num_thresholds - 1] - p[1:], 0),
          name='prec_slope')
      intercept = tp[1:] - math_ops.multiply(prec_slope, p[1:])
      safe_p_ratio = array_ops.where(
          math_ops.logical_and(p[:num_thresholds - 1] > 0, p[1:] > 0),
          math_ops.div_no_nan(
              p[:num_thresholds - 1],
              math_ops.maximum(p[1:], 0),
              name='recall_relative_ratio'), array_ops.ones_like(p[1:]))
      return math_ops.reduce_sum(
          math_ops.div_no_nan(
              prec_slope * (dtp + intercept * math_ops.log(safe_p_ratio)),
              math_ops.maximum(tp[1:] + fn[1:], 0),
              name='pr_auc_increment'),
          name='interpolate_pr_auc')

    def compute_auc(tp, fn, tn, fp, name):
      """Computes the roc-auc or pr-auc based on confusion counts."""
      if curve == 'PR':
        if summation_method == 'trapezoidal':
          logging.warning(
              'Trapezoidal rule is known to produce incorrect PR-AUCs; '
              'please switch to "careful_interpolation" instead.')
        elif summation_method == 'careful_interpolation':
          # This one is a bit tricky and is handled separately.
          return interpolate_pr_auc(tp, fp, fn)
      rec = math_ops.div(tp + epsilon, tp + fn + epsilon)
      if curve == 'ROC':
        fp_rate = math_ops.div(fp, fp + tn + epsilon)
        x = fp_rate
        y = rec
      else:  # curve == 'PR'.
        prec = math_ops.div(tp + epsilon, tp + fp + epsilon)
        x = rec
        y = prec
      if summation_method in ('trapezoidal', 'careful_interpolation'):
        # Note that the case ('PR', 'careful_interpolation') has been handled
        # above.
        return math_ops.reduce_sum(
            math_ops.multiply(x[:num_thresholds - 1] - x[1:],
                              (y[:num_thresholds - 1] + y[1:]) / 2.),
            name=name)
      elif summation_method == 'minoring':
        return math_ops.reduce_sum(
            math_ops.multiply(x[:num_thresholds - 1] - x[1:],
                              math_ops.minimum(y[:num_thresholds - 1], y[1:])),
            name=name)
      elif summation_method == 'majoring':
        return math_ops.reduce_sum(
            math_ops.multiply(x[:num_thresholds - 1] - x[1:],
                              math_ops.maximum(y[:num_thresholds - 1], y[1:])),
            name=name)
      else:
        raise ValueError('Invalid summation_method: %s' % summation_method)

    # sum up the areas of all the trapeziums
    def compute_auc_value(_, values):
      return compute_auc(values['tp'], values['fn'], values['tn'], values['fp'],
                         'value')

    auc_value = _aggregate_across_replicas(
        metrics_collections, compute_auc_value, values)
    update_op = compute_auc(update_ops['tp'], update_ops['fn'],
                            update_ops['tn'], update_ops['fp'], 'update_op')

    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return auc_value, update_op


@tf_export(v1=['metrics.mean_absolute_error'])
def mean_absolute_error(labels,
                        predictions,
                        weights=None,
                        metrics_collections=None,
                        updates_collections=None,
                        name=None):
  """Computes the mean absolute error between the labels and predictions.

  The `mean_absolute_error` function creates two local variables,
  `total` and `count` that are used to compute the mean absolute error. This
  average is weighted by `weights`, and it is ultimately returned as
  `mean_absolute_error`: an idempotent operation that simply divides `total` by
  `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `mean_absolute_error`. Internally, an `absolute_errors` operation computes the
  absolute value of the differences between `predictions` and `labels`. Then
  `update_op` increments `total` with the reduced sum of the product of
  `weights` and `absolute_errors`, and it increments `count` with the reduced
  sum of `weights`

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of the same shape as `predictions`.
    predictions: A `Tensor` of arbitrary shape.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that
      `mean_absolute_error` should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    mean_absolute_error: A `Tensor` representing the current mean, the value of
      `total` divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `mean_absolute_error`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_absolute_error is not supported '
                       'when eager execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)
  absolute_errors = math_ops.abs(predictions - labels)
  return mean(absolute_errors, weights, metrics_collections,
              updates_collections, name or 'mean_absolute_error')


@tf_export(v1=['metrics.mean_cosine_distance'])
def mean_cosine_distance(labels,
                         predictions,
                         dim,
                         weights=None,
                         metrics_collections=None,
                         updates_collections=None,
                         name=None):
  """Computes the cosine distance between the labels and predictions.

  The `mean_cosine_distance` function creates two local variables,
  `total` and `count` that are used to compute the average cosine distance
  between `predictions` and `labels`. This average is weighted by `weights`,
  and it is ultimately returned as `mean_distance`, which is an idempotent
  operation that simply divides `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `mean_distance`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of arbitrary shape.
    predictions: A `Tensor` of the same shape as `labels`.
    dim: The dimension along which the cosine distance is computed.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension). Also,
      dimension `dim` must be `1`.
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    mean_distance: A `Tensor` representing the current mean, the value of
      `total` divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_cosine_distance is not supported when '
                       'eager execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)
  radial_diffs = math_ops.multiply(predictions, labels)
  radial_diffs = math_ops.reduce_sum(
      radial_diffs, axis=[
          dim,
      ], keepdims=True)
  mean_distance, update_op = mean(radial_diffs, weights, None, None, name or
                                  'mean_cosine_distance')
  mean_distance = math_ops.subtract(1.0, mean_distance)
  update_op = math_ops.subtract(1.0, update_op)

  if metrics_collections:
    ops.add_to_collections(metrics_collections, mean_distance)

  if updates_collections:
    ops.add_to_collections(updates_collections, update_op)

  return mean_distance, update_op


@tf_export(v1=['metrics.mean_per_class_accuracy'])
def mean_per_class_accuracy(labels,
                            predictions,
                            num_classes,
                            weights=None,
                            metrics_collections=None,
                            updates_collections=None,
                            name=None):
  """Calculates the mean of the per-class accuracies.

  Calculates the accuracy for each class, then takes the mean of that.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates the accuracy of each class and returns
  them.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of ground truth labels with shape [batch size] and of
      type `int32` or `int64`. The tensor will be flattened if its rank > 1.
    predictions: A `Tensor` of prediction results for semantic labels, whose
      shape is [batch size] and type `int32` or `int64`. The tensor will be
      flattened if its rank > 1.
    num_classes: The possible number of labels the prediction task can
      have. This value must be provided, since two variables with shape =
      [num_classes] will be allocated.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that
      `mean_per_class_accuracy'
      should be added to.
    updates_collections: An optional list of collections `update_op` should be
      added to.
    name: An optional variable_scope name.

  Returns:
    mean_accuracy: A `Tensor` representing the mean per class accuracy.
    update_op: An operation that updates the accuracy tensor.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_per_class_accuracy is not supported '
                       'when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'mean_accuracy',
                                     (predictions, labels, weights)):
    labels = math_ops.to_int64(labels)

    # Flatten the input if its rank > 1.
    if labels.get_shape().ndims > 1:
      labels = array_ops.reshape(labels, [-1])

    if predictions.get_shape().ndims > 1:
      predictions = array_ops.reshape(predictions, [-1])

    # Check if shape is compatible.
    predictions.get_shape().assert_is_compatible_with(labels.get_shape())

    total = metric_variable([num_classes], dtypes.float32, name='total')
    count = metric_variable([num_classes], dtypes.float32, name='count')

    ones = array_ops.ones([array_ops.size(labels)], dtypes.float32)

    if labels.dtype != predictions.dtype:
      predictions = math_ops.cast(predictions, labels.dtype)
    is_correct = math_ops.to_float(math_ops.equal(predictions, labels))

    if weights is not None:
      if weights.get_shape().ndims > 1:
        weights = array_ops.reshape(weights, [-1])
      weights = math_ops.to_float(weights)

      is_correct *= weights
      ones *= weights

    update_total_op = state_ops.scatter_add(total, labels, ones)
    update_count_op = state_ops.scatter_add(count, labels, is_correct)

    def compute_mean_accuracy(_, count, total):
      per_class_accuracy = math_ops.div_no_nan(
          count, math_ops.maximum(total, 0), name=None)
      mean_accuracy_v = math_ops.reduce_mean(
          per_class_accuracy, name='mean_accuracy')
      return mean_accuracy_v

    mean_accuracy_v = _aggregate_across_replicas(
        metrics_collections, compute_mean_accuracy, count, total)

    update_op = math_ops.div_no_nan(
        update_count_op, math_ops.maximum(update_total_op, 0), name='update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return mean_accuracy_v, update_op


@tf_export(v1=['metrics.mean_iou'])
def mean_iou(labels,
             predictions,
             num_classes,
             weights=None,
             metrics_collections=None,
             updates_collections=None,
             name=None):
  """Calculate per-step mean Intersection-Over-Union (mIOU).

  Mean Intersection-Over-Union is a common evaluation metric for
  semantic image segmentation, which first computes the IOU for each
  semantic class and then computes the average over classes.
  IOU is defined as follows:
    IOU = true_positive / (true_positive + false_positive + false_negative).
  The predictions are accumulated in a confusion matrix, weighted by `weights`,
  and mIOU is then calculated from it.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `mean_iou`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of ground truth labels with shape [batch size] and of
      type `int32` or `int64`. The tensor will be flattened if its rank > 1.
    predictions: A `Tensor` of prediction results for semantic labels, whose
      shape is [batch size] and type `int32` or `int64`. The tensor will be
      flattened if its rank > 1.
    num_classes: The possible number of labels the prediction task can
      have. This value must be provided, since a confusion matrix of
      dimension = [num_classes, num_classes] will be allocated.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `mean_iou`
      should be added to.
    updates_collections: An optional list of collections `update_op` should be
      added to.
    name: An optional variable_scope name.

  Returns:
    mean_iou: A `Tensor` representing the mean intersection-over-union.
    update_op: An operation that increments the confusion matrix.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_iou is not supported when '
                       'eager execution is enabled.')

  with variable_scope.variable_scope(name, 'mean_iou',
                                     (predictions, labels, weights)):
    # Check if shape is compatible.
    predictions.get_shape().assert_is_compatible_with(labels.get_shape())

    total_cm, update_op = _streaming_confusion_matrix(labels, predictions,
                                                      num_classes, weights)

    def compute_mean_iou(_, total_cm):
      """Compute the mean intersection-over-union via the confusion matrix."""
      sum_over_row = math_ops.to_float(math_ops.reduce_sum(total_cm, 0))
      sum_over_col = math_ops.to_float(math_ops.reduce_sum(total_cm, 1))
      cm_diag = math_ops.to_float(array_ops.diag_part(total_cm))
      denominator = sum_over_row + sum_over_col - cm_diag

      # The mean is only computed over classes that appear in the
      # label or prediction tensor. If the denominator is 0, we need to
      # ignore the class.
      num_valid_entries = math_ops.reduce_sum(
          math_ops.cast(
              math_ops.not_equal(denominator, 0), dtype=dtypes.float32))

      # If the value of the denominator is 0, set it to 1 to avoid
      # zero division.
      denominator = array_ops.where(
          math_ops.greater(denominator, 0), denominator,
          array_ops.ones_like(denominator))
      iou = math_ops.div(cm_diag, denominator)

      # If the number of valid entries is 0 (no classes) we return 0.
      result = array_ops.where(
          math_ops.greater(num_valid_entries, 0),
          math_ops.reduce_sum(iou, name='mean_iou') / num_valid_entries, 0)
      return result

    # TODO(priyag): Use outside_compilation if in TPU context.
    mean_iou_v = _aggregate_across_replicas(
        metrics_collections, compute_mean_iou, total_cm)

    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return mean_iou_v, update_op


@tf_export(v1=['metrics.mean_relative_error'])
def mean_relative_error(labels,
                        predictions,
                        normalizer,
                        weights=None,
                        metrics_collections=None,
                        updates_collections=None,
                        name=None):
  """Computes the mean relative error by normalizing with the given values.

  The `mean_relative_error` function creates two local variables,
  `total` and `count` that are used to compute the mean relative absolute error.
  This average is weighted by `weights`, and it is ultimately returned as
  `mean_relative_error`: an idempotent operation that simply divides `total` by
  `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `mean_reative_error`. Internally, a `relative_errors` operation divides the
  absolute value of the differences between `predictions` and `labels` by the
  `normalizer`. Then `update_op` increments `total` with the reduced sum of the
  product of `weights` and `relative_errors`, and it increments `count` with the
  reduced sum of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of the same shape as `predictions`.
    predictions: A `Tensor` of arbitrary shape.
    normalizer: A `Tensor` of the same shape as `predictions`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that
      `mean_relative_error` should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    mean_relative_error: A `Tensor` representing the current mean, the value of
      `total` divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `mean_relative_error`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_relative_error is not supported when '
                       'eager execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)

  predictions, normalizer = confusion_matrix.remove_squeezable_dimensions(
      predictions, normalizer)
  predictions.get_shape().assert_is_compatible_with(normalizer.get_shape())
  relative_errors = array_ops.where(
      math_ops.equal(normalizer, 0.0), array_ops.zeros_like(labels),
      math_ops.div(math_ops.abs(labels - predictions), normalizer))
  return mean(relative_errors, weights, metrics_collections,
              updates_collections, name or 'mean_relative_error')


@tf_export(v1=['metrics.mean_squared_error'])
def mean_squared_error(labels,
                       predictions,
                       weights=None,
                       metrics_collections=None,
                       updates_collections=None,
                       name=None):
  """Computes the mean squared error between the labels and predictions.

  The `mean_squared_error` function creates two local variables,
  `total` and `count` that are used to compute the mean squared error.
  This average is weighted by `weights`, and it is ultimately returned as
  `mean_squared_error`: an idempotent operation that simply divides `total` by
  `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `mean_squared_error`. Internally, a `squared_error` operation computes the
  element-wise square of the difference between `predictions` and `labels`. Then
  `update_op` increments `total` with the reduced sum of the product of
  `weights` and `squared_error`, and it increments `count` with the reduced sum
  of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of the same shape as `predictions`.
    predictions: A `Tensor` of arbitrary shape.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that
      `mean_squared_error` should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    mean_squared_error: A `Tensor` representing the current mean, the value of
      `total` divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `mean_squared_error`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_squared_error is not supported when '
                       'eager execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)
  squared_error = math_ops.square(labels - predictions)
  return mean(squared_error, weights, metrics_collections, updates_collections,
              name or 'mean_squared_error')


@tf_export(v1=['metrics.mean_tensor'])
def mean_tensor(values,
                weights=None,
                metrics_collections=None,
                updates_collections=None,
                name=None):
  """Computes the element-wise (weighted) mean of the given tensors.

  In contrast to the `mean` function which returns a scalar with the
  mean,  this function returns an average tensor with the same shape as the
  input tensors.

  The `mean_tensor` function creates two local variables,
  `total_tensor` and `count_tensor` that are used to compute the average of
  `values`. This average is ultimately returned as `mean` which is an idempotent
  operation that simply divides `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `mean`.
  `update_op` increments `total` with the reduced sum of the product of `values`
  and `weights`, and it increments `count` with the reduced sum of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    values: A `Tensor` of arbitrary dimensions.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `values`, and must be broadcastable to `values` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `values` dimension).
    metrics_collections: An optional list of collections that `mean`
      should be added to.
    updates_collections: An optional list of collections that `update_op`
      should be added to.
    name: An optional variable_scope name.

  Returns:
    mean: A float `Tensor` representing the current mean, the value of `total`
      divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `mean_value`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match `values`,
      or if either `metrics_collections` or `updates_collections` are not a list
      or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.mean_tensor is not supported when '
                       'eager execution is enabled.')

  with variable_scope.variable_scope(name, 'mean', (values, weights)):
    values = math_ops.to_float(values)
    total = metric_variable(
        values.get_shape(), dtypes.float32, name='total_tensor')
    count = metric_variable(
        values.get_shape(), dtypes.float32, name='count_tensor')

    num_values = array_ops.ones_like(values)
    if weights is not None:
      values, _, weights = _remove_squeezable_dimensions(
          predictions=values, labels=None, weights=weights)
      weights = weights_broadcast_ops.broadcast_weights(
          math_ops.to_float(weights), values)
      values = math_ops.multiply(values, weights)
      num_values = math_ops.multiply(num_values, weights)

    update_total_op = state_ops.assign_add(total, values)
    with ops.control_dependencies([values]):
      update_count_op = state_ops.assign_add(count, num_values)

    compute_mean = lambda _, t, c: math_ops.div_no_nan(  # pylint: disable=g-long-lambda
        t, math_ops.maximum(c, 0), name='value')

    mean_t = _aggregate_across_replicas(
        metrics_collections, compute_mean, total, count)

    update_op = math_ops.div_no_nan(
        update_total_op, math_ops.maximum(update_count_op, 0), name='update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return mean_t, update_op


@tf_export(v1=['metrics.percentage_below'])
def percentage_below(values,
                     threshold,
                     weights=None,
                     metrics_collections=None,
                     updates_collections=None,
                     name=None):
  """Computes the percentage of values less than the given threshold.

  The `percentage_below` function creates two local variables,
  `total` and `count` that are used to compute the percentage of `values` that
  fall below `threshold`. This rate is weighted by `weights`, and it is
  ultimately returned as `percentage` which is an idempotent operation that
  simply divides `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `percentage`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    values: A numeric `Tensor` of arbitrary size.
    threshold: A scalar threshold.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `values`, and must be broadcastable to `values` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `values` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    percentage: A `Tensor` representing the current mean, the value of `total`
      divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match `values`,
      or if either `metrics_collections` or `updates_collections` are not a list
      or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.percentage_below is not supported when '
                       'eager execution is enabled.')

  is_below_threshold = math_ops.to_float(math_ops.less(values, threshold))
  return mean(is_below_threshold, weights, metrics_collections,
              updates_collections, name or 'percentage_below_threshold')


def _count_condition(values,
                     weights=None,
                     metrics_collections=None,
                     updates_collections=None):
  """Sums the weights of cases where the given values are True.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    values: A `bool` `Tensor` of arbitrary size.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `values`, and must be broadcastable to `values` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `values` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.

  Returns:
    value_tensor: A `Tensor` representing the current value of the metric.
    update_op: An operation that accumulates the error from a batch of data.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match `values`,
      or if either `metrics_collections` or `updates_collections` are not a list
      or tuple.
  """
  check_ops.assert_type(values, dtypes.bool)
  count = metric_variable([], dtypes.float32, name='count')

  values = math_ops.to_float(values)
  if weights is not None:
    with ops.control_dependencies((check_ops.assert_rank_in(
        weights, (0, array_ops.rank(values))),)):
      weights = math_ops.to_float(weights)
      values = math_ops.multiply(values, weights)

  value_tensor = _aggregate_variable(count, metrics_collections)

  update_op = state_ops.assign_add(count, math_ops.reduce_sum(values))
  if updates_collections:
    ops.add_to_collections(updates_collections, update_op)

  return value_tensor, update_op


@tf_export(v1=['metrics.false_negatives'])
def false_negatives(labels,
                    predictions,
                    weights=None,
                    metrics_collections=None,
                    updates_collections=None,
                    name=None):
  """Computes the total number of false negatives.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    value_tensor: A `Tensor` representing the current value of the metric.
    update_op: An operation that accumulates the error from a batch of data.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match `values`,
      or if either `metrics_collections` or `updates_collections` are not a list
      or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.false_negatives is not supported when '
                       'eager execution is enabled.')

  with variable_scope.variable_scope(name, 'false_negatives',
                                     (predictions, labels, weights)):

    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)
    is_false_negative = math_ops.logical_and(
        math_ops.equal(labels, True), math_ops.equal(predictions, False))
    return _count_condition(is_false_negative, weights, metrics_collections,
                            updates_collections)


@tf_export(v1=['metrics.false_negatives_at_thresholds'])
def false_negatives_at_thresholds(labels,
                                  predictions,
                                  thresholds,
                                  weights=None,
                                  metrics_collections=None,
                                  updates_collections=None,
                                  name=None):
  """Computes false negatives at provided threshold values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `false_negatives`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    false_negatives:  A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that updates the `false_negatives` variable and
      returns its current value.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.false_negatives_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'false_negatives',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights=weights, includes=('fn',))

    fn_value = _aggregate_variable(values['fn'], metrics_collections)

    if updates_collections:
      ops.add_to_collections(updates_collections, update_ops['fn'])

    return fn_value, update_ops['fn']


@tf_export(v1=['metrics.false_positives'])
def false_positives(labels,
                    predictions,
                    weights=None,
                    metrics_collections=None,
                    updates_collections=None,
                    name=None):
  """Sum the weights of false positives.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    value_tensor: A `Tensor` representing the current value of the metric.
    update_op: An operation that accumulates the error from a batch of data.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.false_positives is not supported when '
                       'eager execution is enabled.')

  with variable_scope.variable_scope(name, 'false_positives',
                                     (predictions, labels, weights)):

    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)
    is_false_positive = math_ops.logical_and(
        math_ops.equal(labels, False), math_ops.equal(predictions, True))
    return _count_condition(is_false_positive, weights, metrics_collections,
                            updates_collections)


@tf_export(v1=['metrics.false_positives_at_thresholds'])
def false_positives_at_thresholds(labels,
                                  predictions,
                                  thresholds,
                                  weights=None,
                                  metrics_collections=None,
                                  updates_collections=None,
                                  name=None):
  """Computes false positives at provided threshold values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `false_positives`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    false_positives:  A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that updates the `false_positives` variable and
      returns its current value.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.false_positives_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'false_positives',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights=weights, includes=('fp',))

    fp_value = _aggregate_variable(values['fp'], metrics_collections)

    if updates_collections:
      ops.add_to_collections(updates_collections, update_ops['fp'])

    return fp_value, update_ops['fp']


@tf_export(v1=['metrics.true_negatives'])
def true_negatives(labels,
                   predictions,
                   weights=None,
                   metrics_collections=None,
                   updates_collections=None,
                   name=None):
  """Sum the weights of true_negatives.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    value_tensor: A `Tensor` representing the current value of the metric.
    update_op: An operation that accumulates the error from a batch of data.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.true_negatives is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'true_negatives',
                                     (predictions, labels, weights)):

    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)
    is_true_negative = math_ops.logical_and(
        math_ops.equal(labels, False), math_ops.equal(predictions, False))
    return _count_condition(is_true_negative, weights, metrics_collections,
                            updates_collections)


@tf_export(v1=['metrics.true_negatives_at_thresholds'])
def true_negatives_at_thresholds(labels,
                                 predictions,
                                 thresholds,
                                 weights=None,
                                 metrics_collections=None,
                                 updates_collections=None,
                                 name=None):
  """Computes true negatives at provided threshold values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `true_negatives`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    true_negatives:  A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that updates the `true_negatives` variable and
      returns its current value.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.true_negatives_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'true_negatives',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights=weights, includes=('tn',))

    tn_value = _aggregate_variable(values['tn'], metrics_collections)

    if updates_collections:
      ops.add_to_collections(updates_collections, update_ops['tn'])

    return tn_value, update_ops['tn']


@tf_export(v1=['metrics.true_positives'])
def true_positives(labels,
                   predictions,
                   weights=None,
                   metrics_collections=None,
                   updates_collections=None,
                   name=None):
  """Sum the weights of true_positives.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that the metric
      value variable should be added to.
    updates_collections: An optional list of collections that the metric update
      ops should be added to.
    name: An optional variable_scope name.

  Returns:
    value_tensor: A `Tensor` representing the current value of the metric.
    update_op: An operation that accumulates the error from a batch of data.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.true_positives is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'true_positives',
                                     (predictions, labels, weights)):

    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)
    is_true_positive = math_ops.logical_and(
        math_ops.equal(labels, True), math_ops.equal(predictions, True))
    return _count_condition(is_true_positive, weights, metrics_collections,
                            updates_collections)


@tf_export(v1=['metrics.true_positives_at_thresholds'])
def true_positives_at_thresholds(labels,
                                 predictions,
                                 thresholds,
                                 weights=None,
                                 metrics_collections=None,
                                 updates_collections=None,
                                 name=None):
  """Computes true positives at provided threshold values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` whose shape matches `predictions`. Will be cast to
      `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `true_positives`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    true_positives:  A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that updates the `true_positives` variable and
      returns its current value.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.true_positives_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'true_positives',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights=weights, includes=('tp',))

    tp_value = _aggregate_variable(values['tp'], metrics_collections)

    if updates_collections:
      ops.add_to_collections(updates_collections, update_ops['tp'])

    return tp_value, update_ops['tp']


@tf_export(v1=['metrics.precision'])
def precision(labels,
              predictions,
              weights=None,
              metrics_collections=None,
              updates_collections=None,
              name=None):
  """Computes the precision of the predictions with respect to the labels.

  The `precision` function creates two local variables,
  `true_positives` and `false_positives`, that are used to compute the
  precision. This value is ultimately returned as `precision`, an idempotent
  operation that simply divides `true_positives` by the sum of `true_positives`
  and `false_positives`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `precision`. `update_op` weights each prediction by the corresponding value in
  `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `precision` should
      be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    precision: Scalar float `Tensor` with the value of `true_positives`
      divided by the sum of `true_positives` and `false_positives`.
    update_op: `Operation` that increments `true_positives` and
      `false_positives` variables appropriately and whose value matches
      `precision`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.precision is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'precision',
                                     (predictions, labels, weights)):

    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)

    true_p, true_positives_update_op = true_positives(
        labels,
        predictions,
        weights,
        metrics_collections=None,
        updates_collections=None,
        name=None)
    false_p, false_positives_update_op = false_positives(
        labels,
        predictions,
        weights,
        metrics_collections=None,
        updates_collections=None,
        name=None)

    def compute_precision(tp, fp, name):
      return array_ops.where(
          math_ops.greater(tp + fp, 0), math_ops.div(tp, tp + fp), 0, name)

    def once_across_replicas(_, true_p, false_p):
      return compute_precision(true_p, false_p, 'value')

    p = _aggregate_across_replicas(metrics_collections, once_across_replicas,
                                   true_p, false_p)

    update_op = compute_precision(true_positives_update_op,
                                  false_positives_update_op, 'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return p, update_op


@tf_export(v1=['metrics.precision_at_thresholds'])
def precision_at_thresholds(labels,
                            predictions,
                            thresholds,
                            weights=None,
                            metrics_collections=None,
                            updates_collections=None,
                            name=None):
  """Computes precision values for different `thresholds` on `predictions`.

  The `precision_at_thresholds` function creates four local variables,
  `true_positives`, `true_negatives`, `false_positives` and `false_negatives`
  for various values of thresholds. `precision[i]` is defined as the total
  weight of values in `predictions` above `thresholds[i]` whose corresponding
  entry in `labels` is `True`, divided by the total weight of values in
  `predictions` above `thresholds[i]` (`true_positives[i] / (true_positives[i] +
  false_positives[i])`).

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `precision`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `auc` should be
      added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    precision: A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that increments the `true_positives`,
      `true_negatives`, `false_positives` and `false_negatives` variables that
      are used in the computation of `precision`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.precision_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'precision_at_thresholds',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights, includes=('tp', 'fp'))

    # Avoid division by zero.
    epsilon = 1e-7

    def compute_precision(tp, fp, name):
      return math_ops.div(tp, epsilon + tp + fp, name='precision_' + name)

    def precision_across_replicas(_, values):
      return compute_precision(values['tp'], values['fp'], 'value')

    prec = _aggregate_across_replicas(
        metrics_collections, precision_across_replicas, values)

    update_op = compute_precision(update_ops['tp'], update_ops['fp'],
                                  'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return prec, update_op


@tf_export(v1=['metrics.recall'])
def recall(labels,
           predictions,
           weights=None,
           metrics_collections=None,
           updates_collections=None,
           name=None):
  """Computes the recall of the predictions with respect to the labels.

  The `recall` function creates two local variables, `true_positives`
  and `false_negatives`, that are used to compute the recall. This value is
  ultimately returned as `recall`, an idempotent operation that simply divides
  `true_positives` by the sum of `true_positives` and `false_negatives`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` that updates these variables and returns the `recall`. `update_op`
  weights each prediction by the corresponding value in `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: The predicted values, a `Tensor` of arbitrary dimensions. Will
      be cast to `bool`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `recall` should
      be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    recall: Scalar float `Tensor` with the value of `true_positives` divided
      by the sum of `true_positives` and `false_negatives`.
    update_op: `Operation` that increments `true_positives` and
      `false_negatives` variables appropriately and whose value matches
      `recall`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.recall is not supported is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'recall',
                                     (predictions, labels, weights)):
    predictions, labels, weights = _remove_squeezable_dimensions(
        predictions=math_ops.cast(predictions, dtype=dtypes.bool),
        labels=math_ops.cast(labels, dtype=dtypes.bool),
        weights=weights)

    true_p, true_positives_update_op = true_positives(
        labels,
        predictions,
        weights,
        metrics_collections=None,
        updates_collections=None,
        name=None)
    false_n, false_negatives_update_op = false_negatives(
        labels,
        predictions,
        weights,
        metrics_collections=None,
        updates_collections=None,
        name=None)

    def compute_recall(true_p, false_n, name):
      return array_ops.where(
          math_ops.greater(true_p + false_n, 0),
          math_ops.div(true_p, true_p + false_n), 0, name)

    def once_across_replicas(_, true_p, false_n):
      return compute_recall(true_p, false_n, 'value')

    rec = _aggregate_across_replicas(
        metrics_collections, once_across_replicas, true_p, false_n)

    update_op = compute_recall(true_positives_update_op,
                               false_negatives_update_op, 'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return rec, update_op


def _at_k_name(name, k=None, class_id=None):
  if k is not None:
    name = '%s_at_%d' % (name, k)
  else:
    name = '%s_at_k' % (name)
  if class_id is not None:
    name = '%s_class%d' % (name, class_id)
  return name


def _select_class_id(ids, selected_id):
  """Filter all but `selected_id` out of `ids`.

  Args:
    ids: `int64` `Tensor` or `SparseTensor` of IDs.
    selected_id: Int id to select.

  Returns:
    `SparseTensor` of same dimensions as `ids`. This contains only the entries
    equal to `selected_id`.
  """
  ids = sparse_tensor.convert_to_tensor_or_sparse_tensor(ids)
  if isinstance(ids, sparse_tensor.SparseTensor):
    return sparse_ops.sparse_retain(ids, math_ops.equal(ids.values,
                                                        selected_id))

  # TODO(ptucker): Make this more efficient, maybe add a sparse version of
  # tf.equal and tf.reduce_any?

  # Shape of filled IDs is the same as `ids` with the last dim collapsed to 1.
  ids_shape = array_ops.shape(ids, out_type=dtypes.int64)
  ids_last_dim = array_ops.size(ids_shape) - 1
  filled_selected_id_shape = math_ops.reduced_shape(ids_shape,
                                                    array_ops.reshape(
                                                        ids_last_dim, [1]))

  # Intersect `ids` with the selected ID.
  filled_selected_id = array_ops.fill(filled_selected_id_shape,
                                      math_ops.to_int64(selected_id))
  result = sets.set_intersection(filled_selected_id, ids)
  return sparse_tensor.SparseTensor(
      indices=result.indices, values=result.values, dense_shape=ids_shape)


def _maybe_select_class_id(labels, predictions_idx, selected_id=None):
  """If class ID is specified, filter all other classes.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: `int64` `Tensor` of class IDs, with shape [D1, ... DN, k]
      where N >= 1. Commonly, N=1 and `predictions_idx` has shape
      [batch size, k].
    selected_id: Int id to select.

  Returns:
    Tuple of `labels` and `predictions_idx`, possibly with classes removed.
  """
  if selected_id is None:
    return labels, predictions_idx
  return (_select_class_id(labels, selected_id),
          _select_class_id(predictions_idx, selected_id))


def _sparse_true_positive_at_k(labels,
                               predictions_idx,
                               class_id=None,
                               weights=None,
                               name=None):
  """Calculates true positives for recall@k and precision@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels_sparse`.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    name: Name of operation.

  Returns:
    A [D1, ... DN] `Tensor` of true positive counts.
  """
  with ops.name_scope(name, 'true_positives',
                      (predictions_idx, labels, weights)):
    labels, predictions_idx = _maybe_select_class_id(labels, predictions_idx,
                                                     class_id)
    tp = sets.set_size(sets.set_intersection(predictions_idx, labels))
    tp = math_ops.to_double(tp)
    if weights is not None:
      with ops.control_dependencies((weights_broadcast_ops.assert_broadcastable(
          weights, tp),)):
        weights = math_ops.to_double(weights)
        tp = math_ops.multiply(tp, weights)
    return tp


def _streaming_sparse_true_positive_at_k(labels,
                                         predictions_idx,
                                         k=None,
                                         class_id=None,
                                         weights=None,
                                         name=None):
  """Calculates weighted per step true positives for recall@k and precision@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    k: Integer, k for @k metric. This is only used for default op name.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    name: Name of new variable, and namespace for other dependent ops.

  Returns:
    A tuple of `Variable` and update `Operation`.

  Raises:
    ValueError: If `weights` is not `None` and has an incompatible shape.
  """
  with ops.name_scope(name, _at_k_name('true_positive', k, class_id=class_id),
                      (predictions_idx, labels, weights)) as scope:
    tp = _sparse_true_positive_at_k(
        predictions_idx=predictions_idx,
        labels=labels,
        class_id=class_id,
        weights=weights)
    batch_total_tp = math_ops.to_double(math_ops.reduce_sum(tp))

    var = metric_variable([], dtypes.float64, name=scope)
    return var, state_ops.assign_add(var, batch_total_tp, name='update')


def _sparse_false_negative_at_k(labels,
                                predictions_idx,
                                class_id=None,
                                weights=None):
  """Calculates false negatives for recall@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels_sparse`.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).

  Returns:
    A [D1, ... DN] `Tensor` of false negative counts.
  """
  with ops.name_scope(None, 'false_negatives',
                      (predictions_idx, labels, weights)):
    labels, predictions_idx = _maybe_select_class_id(labels, predictions_idx,
                                                     class_id)
    fn = sets.set_size(
        sets.set_difference(predictions_idx, labels, aminusb=False))
    fn = math_ops.to_double(fn)
    if weights is not None:
      with ops.control_dependencies((weights_broadcast_ops.assert_broadcastable(
          weights, fn),)):
        weights = math_ops.to_double(weights)
        fn = math_ops.multiply(fn, weights)
    return fn


def _streaming_sparse_false_negative_at_k(labels,
                                          predictions_idx,
                                          k,
                                          class_id=None,
                                          weights=None,
                                          name=None):
  """Calculates weighted per step false negatives for recall@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    k: Integer, k for @k metric. This is only used for default op name.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    name: Name of new variable, and namespace for other dependent ops.

  Returns:
    A tuple of `Variable` and update `Operation`.

  Raises:
    ValueError: If `weights` is not `None` and has an incompatible shape.
  """
  with ops.name_scope(name, _at_k_name('false_negative', k, class_id=class_id),
                      (predictions_idx, labels, weights)) as scope:
    fn = _sparse_false_negative_at_k(
        predictions_idx=predictions_idx,
        labels=labels,
        class_id=class_id,
        weights=weights)
    batch_total_fn = math_ops.to_double(math_ops.reduce_sum(fn))

    var = metric_variable([], dtypes.float64, name=scope)
    return var, state_ops.assign_add(var, batch_total_fn, name='update')


@tf_export(v1=['metrics.recall_at_k'])
def recall_at_k(labels,
                predictions,
                k,
                class_id=None,
                weights=None,
                metrics_collections=None,
                updates_collections=None,
                name=None):
  """Computes recall@k of the predictions with respect to sparse labels.

  If `class_id` is specified, we calculate recall by considering only the
      entries in the batch for which `class_id` is in the label, and computing
      the fraction of them for which `class_id` is in the top-k `predictions`.
  If `class_id` is not specified, we'll calculate recall as how often on
      average a class among the labels of a batch entry is in the top-k
      `predictions`.

  `sparse_recall_at_k` creates two local variables,
  `true_positive_at_<k>` and `false_negative_at_<k>`, that are used to compute
  the recall_at_k frequency. This frequency is ultimately returned as
  `recall_at_<k>`: an idempotent operation that simply divides
  `true_positive_at_<k>` by total (`true_positive_at_<k>` +
  `false_negative_at_<k>`).

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `recall_at_<k>`. Internally, a `top_k` operation computes a `Tensor`
  indicating the top `k` `predictions`. Set operations applied to `top_k` and
  `labels` calculate the true positives and false negatives weighted by
  `weights`. Then `update_op` increments `true_positive_at_<k>` and
  `false_negative_at_<k>` using these values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions`. Values
      should be in range [0, num_classes), where num_classes is the last
      dimension of `predictions`. Values outside this range always count
      towards `false_negative_at_<k>`.
    predictions: Float `Tensor` with shape [D1, ... DN, num_classes] where
      N >= 1. Commonly, N=1 and predictions has shape [batch size, num_classes].
      The final dimension contains the logit values for each class. [D1, ... DN]
      must match `labels`.
    k: Integer, k for @k metric.
    class_id: Integer class ID for which we want binary metrics. This should be
      in range [0, num_classes), where num_classes is the last dimension of
      `predictions`. If class_id is outside this range, the method returns NAN.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    recall: Scalar `float64` `Tensor` with the value of `true_positives` divided
      by the sum of `true_positives` and `false_negatives`.
    update_op: `Operation` that increments `true_positives` and
      `false_negatives` variables appropriately, and whose value matches
      `recall`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match
    `predictions`, or if either `metrics_collections` or `updates_collections`
    are not a list or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.recall_at_k is not '
                       'supported when eager execution is enabled.')

  with ops.name_scope(name, _at_k_name('recall', k, class_id=class_id),
                      (predictions, labels, weights)) as scope:
    _, top_k_idx = nn.top_k(predictions, k)
    return recall_at_top_k(
        labels=labels,
        predictions_idx=top_k_idx,
        k=k,
        class_id=class_id,
        weights=weights,
        metrics_collections=metrics_collections,
        updates_collections=updates_collections,
        name=scope)


@tf_export(v1=['metrics.recall_at_top_k'])
def recall_at_top_k(labels,
                    predictions_idx,
                    k=None,
                    class_id=None,
                    weights=None,
                    metrics_collections=None,
                    updates_collections=None,
                    name=None):
  """Computes recall@k of top-k predictions with respect to sparse labels.

  Differs from `recall_at_k` in that predictions must be in the form of top `k`
  class indices, whereas `recall_at_k` expects logits. Refer to `recall_at_k`
  for more details.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions`. Values
      should be in range [0, num_classes), where num_classes is the last
      dimension of `predictions`. Values outside this range always count
      towards `false_negative_at_<k>`.
    predictions_idx: Integer `Tensor` with shape [D1, ... DN, k] where N >= 1.
      Commonly, N=1 and predictions has shape [batch size, k]. The final
      dimension contains the top `k` predicted class indices. [D1, ... DN] must
      match `labels`.
    k: Integer, k for @k metric. Only used for the default op name.
    class_id: Integer class ID for which we want binary metrics. This should be
      in range [0, num_classes), where num_classes is the last dimension of
      `predictions`. If class_id is outside this range, the method returns NAN.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    recall: Scalar `float64` `Tensor` with the value of `true_positives` divided
      by the sum of `true_positives` and `false_negatives`.
    update_op: `Operation` that increments `true_positives` and
      `false_negatives` variables appropriately, and whose value matches
      `recall`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match
    `predictions`, or if either `metrics_collections` or `updates_collections`
    are not a list or tuple.
  """
  with ops.name_scope(name, _at_k_name('recall', k, class_id=class_id),
                      (predictions_idx, labels, weights)) as scope:
    labels = _maybe_expand_labels(labels, predictions_idx)
    top_k_idx = math_ops.to_int64(predictions_idx)
    tp, tp_update = _streaming_sparse_true_positive_at_k(
        predictions_idx=top_k_idx,
        labels=labels,
        k=k,
        class_id=class_id,
        weights=weights)
    fn, fn_update = _streaming_sparse_false_negative_at_k(
        predictions_idx=top_k_idx,
        labels=labels,
        k=k,
        class_id=class_id,
        weights=weights)

    def compute_recall(_, tp, fn):
      return math_ops.div(tp, math_ops.add(tp, fn), name=scope)

    metric = _aggregate_across_replicas(
        metrics_collections, compute_recall, tp, fn)

    update = math_ops.div(
        tp_update, math_ops.add(tp_update, fn_update), name='update')
    if updates_collections:
      ops.add_to_collections(updates_collections, update)
    return metric, update


@tf_export(v1=['metrics.recall_at_thresholds'])
def recall_at_thresholds(labels,
                         predictions,
                         thresholds,
                         weights=None,
                         metrics_collections=None,
                         updates_collections=None,
                         name=None):
  """Computes various recall values for different `thresholds` on `predictions`.

  The `recall_at_thresholds` function creates four local variables,
  `true_positives`, `true_negatives`, `false_positives` and `false_negatives`
  for various values of thresholds. `recall[i]` is defined as the total weight
  of values in `predictions` above `thresholds[i]` whose corresponding entry in
  `labels` is `True`, divided by the total weight of `True` values in `labels`
  (`true_positives[i] / (true_positives[i] + false_negatives[i])`).

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the `recall`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    thresholds: A python list or tuple of float thresholds in `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that `recall` should be
      added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    recall: A float `Tensor` of shape `[len(thresholds)]`.
    update_op: An operation that increments the `true_positives`,
      `true_negatives`, `false_positives` and `false_negatives` variables that
      are used in the computation of `recall`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.recall_at_thresholds is not '
                       'supported when eager execution is enabled.')

  with variable_scope.variable_scope(name, 'recall_at_thresholds',
                                     (predictions, labels, weights)):
    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights, includes=('tp', 'fn'))

    # Avoid division by zero.
    epsilon = 1e-7

    def compute_recall(tp, fn, name):
      return math_ops.div(tp, epsilon + tp + fn, name='recall_' + name)

    def recall_across_replicas(_, values):
      return compute_recall(values['tp'], values['fn'], 'value')

    rec = _aggregate_across_replicas(
        metrics_collections, recall_across_replicas, values)

    update_op = compute_recall(update_ops['tp'], update_ops['fn'], 'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return rec, update_op


@tf_export(v1=['metrics.root_mean_squared_error'])
def root_mean_squared_error(labels,
                            predictions,
                            weights=None,
                            metrics_collections=None,
                            updates_collections=None,
                            name=None):
  """Computes the root mean squared error between the labels and predictions.

  The `root_mean_squared_error` function creates two local variables,
  `total` and `count` that are used to compute the root mean squared error.
  This average is weighted by `weights`, and it is ultimately returned as
  `root_mean_squared_error`: an idempotent operation that takes the square root
  of the division of `total` by `count`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `root_mean_squared_error`. Internally, a `squared_error` operation computes
  the element-wise square of the difference between `predictions` and `labels`.
  Then `update_op` increments `total` with the reduced sum of the product of
  `weights` and `squared_error`, and it increments `count` with the reduced sum
  of `weights`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: A `Tensor` of the same shape as `predictions`.
    predictions: A `Tensor` of arbitrary shape.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    metrics_collections: An optional list of collections that
      `root_mean_squared_error` should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    root_mean_squared_error: A `Tensor` representing the current mean, the value
      of `total` divided by `count`.
    update_op: An operation that increments the `total` and `count` variables
      appropriately and whose value matches `root_mean_squared_error`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, or if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      either `metrics_collections` or `updates_collections` are not a list or
      tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.root_mean_squared_error is not '
                       'supported when eager execution is enabled.')

  predictions, labels, weights = _remove_squeezable_dimensions(
      predictions=predictions, labels=labels, weights=weights)
  mse, update_mse_op = mean_squared_error(labels, predictions, weights, None,
                                          None, name or
                                          'root_mean_squared_error')

  once_across_replicas = lambda _, mse: math_ops.sqrt(mse)
  rmse = _aggregate_across_replicas(
      metrics_collections, once_across_replicas, mse)

  update_rmse_op = math_ops.sqrt(update_mse_op)
  if updates_collections:
    ops.add_to_collections(updates_collections, update_rmse_op)

  return rmse, update_rmse_op


@tf_export(v1=['metrics.sensitivity_at_specificity'])
def sensitivity_at_specificity(labels,
                               predictions,
                               specificity,
                               weights=None,
                               num_thresholds=200,
                               metrics_collections=None,
                               updates_collections=None,
                               name=None):
  """Computes the specificity at a given sensitivity.

  The `sensitivity_at_specificity` function creates four local
  variables, `true_positives`, `true_negatives`, `false_positives` and
  `false_negatives` that are used to compute the sensitivity at the given
  specificity value. The threshold for the given specificity value is computed
  and used to evaluate the corresponding sensitivity.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `sensitivity`. `update_op` increments the `true_positives`, `true_negatives`,
  `false_positives` and `false_negatives` counts with the weight of each case
  found in the `predictions` and `labels`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  For additional information about specificity and sensitivity, see the
  following: https://en.wikipedia.org/wiki/Sensitivity_and_specificity

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    specificity: A scalar value in range `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    num_thresholds: The number of thresholds to use for matching the given
      specificity.
    metrics_collections: An optional list of collections that `sensitivity`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    sensitivity: A scalar `Tensor` representing the sensitivity at the given
      `specificity` value.
    update_op: An operation that increments the `true_positives`,
      `true_negatives`, `false_positives` and `false_negatives` variables
      appropriately and whose value matches `sensitivity`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      `specificity` is not between 0 and 1, or if either `metrics_collections`
      or `updates_collections` are not a list or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.sensitivity_at_specificity is not '
                       'supported when eager execution is enabled.')

  if specificity < 0 or specificity > 1:
    raise ValueError('`specificity` must be in the range [0, 1].')

  with variable_scope.variable_scope(name, 'sensitivity_at_specificity',
                                     (predictions, labels, weights)):
    kepsilon = 1e-7  # to account for floating point imprecisions
    thresholds = [
        (i + 1) * 1.0 / (num_thresholds - 1) for i in range(num_thresholds - 2)
    ]
    thresholds = [0.0 - kepsilon] + thresholds + [1.0 + kepsilon]

    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights)

    def compute_sensitivity_at_specificity(tp, tn, fp, fn, name):
      specificities = math_ops.div(tn, tn + fp + kepsilon)
      tf_index = math_ops.argmin(math_ops.abs(specificities - specificity), 0)
      tf_index = math_ops.cast(tf_index, dtypes.int32)

      # Now, we have the implicit threshold, so compute the sensitivity:
      return math_ops.div(tp[tf_index], tp[tf_index] + fn[tf_index] + kepsilon,
                          name)

    def sensitivity_across_replicas(_, values):
      return compute_sensitivity_at_specificity(
          values['tp'], values['tn'], values['fp'], values['fn'], 'value')

    sensitivity = _aggregate_across_replicas(
        metrics_collections, sensitivity_across_replicas, values)

    update_op = compute_sensitivity_at_specificity(
        update_ops['tp'], update_ops['tn'], update_ops['fp'], update_ops['fn'],
        'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return sensitivity, update_op


def _expand_and_tile(tensor, multiple, dim=0, name=None):
  """Slice `tensor` shape in 2, then tile along the sliced dimension.

  A new dimension is inserted in shape of `tensor` before `dim`, then values are
  tiled `multiple` times along the new dimension.

  Args:
    tensor: Input `Tensor` or `SparseTensor`.
    multiple: Integer, number of times to tile.
    dim: Integer, dimension along which to tile.
    name: Name of operation.

  Returns:
    `Tensor` result of expanding and tiling `tensor`.

  Raises:
    ValueError: if `multiple` is less than 1, or `dim` is not in
    `[-rank(tensor), rank(tensor)]`.
  """
  if multiple < 1:
    raise ValueError('Invalid multiple %s, must be > 0.' % multiple)
  with ops.name_scope(name, 'expand_and_tile',
                      (tensor, multiple, dim)) as scope:
    # Sparse.
    tensor = sparse_tensor.convert_to_tensor_or_sparse_tensor(tensor)
    if isinstance(tensor, sparse_tensor.SparseTensor):
      if dim < 0:
        expand_dims = array_ops.reshape(
            array_ops.size(tensor.dense_shape) + dim, [1])
      else:
        expand_dims = [dim]
      expanded_shape = array_ops.concat(
          (array_ops.slice(tensor.dense_shape, [0], expand_dims), [1],
           array_ops.slice(tensor.dense_shape, expand_dims, [-1])),
          0,
          name='expanded_shape')
      expanded = sparse_ops.sparse_reshape(
          tensor, shape=expanded_shape, name='expand')
      if multiple == 1:
        return expanded
      return sparse_ops.sparse_concat(
          dim - 1 if dim < 0 else dim, [expanded] * multiple, name=scope)

    # Dense.
    expanded = array_ops.expand_dims(
        tensor, dim if (dim >= 0) else (dim - 1), name='expand')
    if multiple == 1:
      return expanded
    ones = array_ops.ones_like(array_ops.shape(tensor))
    tile_multiples = array_ops.concat(
        (ones[:dim], (multiple,), ones[dim:]), 0, name='multiples')
    return array_ops.tile(expanded, tile_multiples, name=scope)


def _num_relevant(labels, k):
  """Computes number of relevant values for each row in labels.

  For labels with shape [D1, ... DN, num_labels], this is the minimum of
  `num_labels` and `k`.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels].
    k: Integer, k for @k metric.

  Returns:
    Integer `Tensor` of shape [D1, ... DN], where each value is the number of
    relevant values for that row.

  Raises:
    ValueError: if inputs have invalid dtypes or values.
  """
  if k < 1:
    raise ValueError('Invalid k=%s.' % k)
  with ops.name_scope(None, 'num_relevant', (labels,)) as scope:
    # For SparseTensor, calculate separate count for each row.
    labels = sparse_tensor.convert_to_tensor_or_sparse_tensor(labels)
    if isinstance(labels, sparse_tensor.SparseTensor):
      return math_ops.minimum(sets.set_size(labels), k, name=scope)

    # For dense Tensor, calculate scalar count based on last dimension, and
    # tile across labels shape.
    labels_shape = array_ops.shape(labels)
    labels_size = labels_shape[-1]
    num_relevant_scalar = math_ops.minimum(labels_size, k)
    return array_ops.fill(labels_shape[0:-1], num_relevant_scalar, name=scope)


def _sparse_average_precision_at_top_k(labels, predictions_idx):
  """Computes average precision@k of predictions with respect to sparse labels.

  From en.wikipedia.org/wiki/Information_retrieval#Average_precision, formula
  for each row is:

    AveP = sum_{i=1...k} P_{i} * rel_{i} / num_relevant_items

  A "row" is the elements in dimension [D1, ... DN] of `predictions_idx`,
  `labels`, and the result `Tensors`. In the common case, this is [batch_size].
  Each row of the results contains the average precision for that row.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions_idx`.
      Values should be in range [0, num_classes).
    predictions_idx: Integer `Tensor` with shape [D1, ... DN, k] where N >= 1.
      Commonly, N=1 and `predictions_idx` has shape [batch size, k]. The final
      dimension must be set and contains the top `k` predicted class indices.
      [D1, ... DN] must match `labels`. Values should be in range
      [0, num_classes).

  Returns:
    `float64` `Tensor` of shape [D1, ... DN], where each value is the average
    precision for that row.

  Raises:
    ValueError: if the last dimension of predictions_idx is not set.
  """
  with ops.name_scope(None, 'average_precision',
                      (predictions_idx, labels)) as scope:
    predictions_idx = math_ops.to_int64(predictions_idx, name='predictions_idx')
    if predictions_idx.get_shape().ndims == 0:
      raise ValueError('The rank of predictions_idx must be at least 1.')
    k = predictions_idx.get_shape().as_list()[-1]
    if k is None:
      raise ValueError('The last dimension of predictions_idx must be set.')
    labels = _maybe_expand_labels(labels, predictions_idx)

    # Expand dims to produce [D1, ... DN, k, 1] tensor. This gives us a separate
    # prediction for each k, so we can calculate separate true positive values
    # for each k.
    predictions_idx_per_k = array_ops.expand_dims(
        predictions_idx, -1, name='predictions_idx_per_k')

    # Replicate labels k times to produce [D1, ... DN, k, num_labels] tensor.
    labels_per_k = _expand_and_tile(
        labels, multiple=k, dim=-1, name='labels_per_k')

    # The following tensors are all of shape [D1, ... DN, k], containing values
    # per row, per k value.
    # `relevant_per_k` (int32) - Relevance indicator, 1 if the prediction at
    #     that k value is correct, 0 otherwise. This is the "rel_{i}" term from
    #     the formula above.
    # `tp_per_k` (int32) - True positive counts.
    # `retrieved_per_k` (int32) - Number of predicted values at each k. This is
    #     the precision denominator.
    # `precision_per_k` (float64) - Precision at each k. This is the "P_{i}"
    #     term from the formula above.
    # `relevant_precision_per_k` (float64) - Relevant precisions; i.e.,
    #     precisions at all k for which relevance indicator is true.
    relevant_per_k = _sparse_true_positive_at_k(
        labels_per_k, predictions_idx_per_k, name='relevant_per_k')
    tp_per_k = math_ops.cumsum(relevant_per_k, axis=-1, name='tp_per_k')
    retrieved_per_k = math_ops.cumsum(
        array_ops.ones_like(relevant_per_k), axis=-1, name='retrieved_per_k')
    precision_per_k = math_ops.div(
        math_ops.to_double(tp_per_k),
        math_ops.to_double(retrieved_per_k),
        name='precision_per_k')
    relevant_precision_per_k = math_ops.multiply(
        precision_per_k,
        math_ops.to_double(relevant_per_k),
        name='relevant_precision_per_k')

    # Reduce along k dimension to get the sum, yielding a [D1, ... DN] tensor.
    precision_sum = math_ops.reduce_sum(
        relevant_precision_per_k, axis=(-1,), name='precision_sum')

    # Divide by number of relevant items to get average precision. These are
    # the "num_relevant_items" and "AveP" terms from the formula above.
    num_relevant_items = math_ops.to_double(_num_relevant(labels, k))
    return math_ops.div(precision_sum, num_relevant_items, name=scope)


def _streaming_sparse_average_precision_at_top_k(labels,
                                                 predictions_idx,
                                                 weights=None,
                                                 metrics_collections=None,
                                                 updates_collections=None,
                                                 name=None):
  """Computes average precision@k of predictions with respect to sparse labels.

  `sparse_average_precision_at_top_k` creates two local variables,
  `average_precision_at_<k>/total` and `average_precision_at_<k>/max`, that
  are used to compute the frequency. This frequency is ultimately returned as
  `average_precision_at_<k>`: an idempotent operation that simply divides
  `average_precision_at_<k>/total` by `average_precision_at_<k>/max`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `precision_at_<k>`. Set operations applied to `top_k` and `labels` calculate
  the true positives and false positives weighted by `weights`. Then `update_op`
  increments `true_positive_at_<k>` and `false_positive_at_<k>` using these
  values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions_idx`.
      Values should be in range [0, num_classes).
    predictions_idx: Integer `Tensor` with shape [D1, ... DN, k] where N >= 1.
      Commonly, N=1 and `predictions_idx` has shape [batch size, k]. The final
      dimension contains the top `k` predicted class indices. [D1, ... DN] must
      match `labels`. Values should be in range [0, num_classes).
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    mean_average_precision: Scalar `float64` `Tensor` with the mean average
      precision values.
    update: `Operation` that increments variables appropriately, and whose
      value matches `metric`.
  """
  with ops.name_scope(name, 'average_precision_at_top_k',
                      (predictions_idx, labels, weights)) as scope:
    # Calculate per-example average precision, and apply weights.
    average_precision = _sparse_average_precision_at_top_k(
        predictions_idx=predictions_idx, labels=labels)
    if weights is not None:
      weights = weights_broadcast_ops.broadcast_weights(
          math_ops.to_double(weights), average_precision)
      average_precision = math_ops.multiply(average_precision, weights)

    # Create accumulation variables and update ops for max average precision and
    # total average precision.
    with ops.name_scope(None, 'max', (average_precision,)) as max_scope:
      # `max` is the max possible precision. Since max for any row is 1.0:
      # - For the unweighted case, this is just the number of rows.
      # - For the weighted case, it's the sum of the weights broadcast across
      #   `average_precision` rows.
      max_var = metric_variable([], dtypes.float64, name=max_scope)
      if weights is None:
        batch_max = math_ops.to_double(
            array_ops.size(average_precision, name='batch_max'))
      else:
        batch_max = math_ops.reduce_sum(weights, name='batch_max')
      max_update = state_ops.assign_add(max_var, batch_max, name='update')
    with ops.name_scope(None, 'total', (average_precision,)) as total_scope:
      total_var = metric_variable([], dtypes.float64, name=total_scope)
      batch_total = math_ops.reduce_sum(average_precision, name='batch_total')
      total_update = state_ops.assign_add(total_var, batch_total, name='update')

    # Divide total by max to get mean, for both vars and the update ops.
    def precision_across_replicas(_, total_var, max_var):
      return _safe_scalar_div(total_var, max_var, name='mean')

    mean_average_precision = _aggregate_across_replicas(
        metrics_collections, precision_across_replicas, total_var, max_var)

    update = _safe_scalar_div(total_update, max_update, name=scope)
    if updates_collections:
      ops.add_to_collections(updates_collections, update)

    return mean_average_precision, update


@tf_export(v1=['metrics.sparse_average_precision_at_k'])
@deprecated(None, 'Use average_precision_at_k instead')
def sparse_average_precision_at_k(labels,
                                  predictions,
                                  k,
                                  weights=None,
                                  metrics_collections=None,
                                  updates_collections=None,
                                  name=None):
  """Renamed to `average_precision_at_k`, please use that method instead."""
  return average_precision_at_k(
      labels=labels,
      predictions=predictions,
      k=k,
      weights=weights,
      metrics_collections=metrics_collections,
      updates_collections=updates_collections,
      name=name)


@tf_export(v1=['metrics.average_precision_at_k'])
def average_precision_at_k(labels,
                           predictions,
                           k,
                           weights=None,
                           metrics_collections=None,
                           updates_collections=None,
                           name=None):
  """Computes average precision@k of predictions with respect to sparse labels.

  `average_precision_at_k` creates two local variables,
  `average_precision_at_<k>/total` and `average_precision_at_<k>/max`, that
  are used to compute the frequency. This frequency is ultimately returned as
  `average_precision_at_<k>`: an idempotent operation that simply divides
  `average_precision_at_<k>/total` by `average_precision_at_<k>/max`.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `precision_at_<k>`. Internally, a `top_k` operation computes a `Tensor`
  indicating the top `k` `predictions`. Set operations applied to `top_k` and
  `labels` calculate the true positives and false positives weighted by
  `weights`. Then `update_op` increments `true_positive_at_<k>` and
  `false_positive_at_<k>` using these values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions`. Values
      should be in range [0, num_classes), where num_classes is the last
      dimension of `predictions`. Values outside this range are ignored.
    predictions: Float `Tensor` with shape [D1, ... DN, num_classes] where
      N >= 1. Commonly, N=1 and `predictions` has shape
      [batch size, num_classes]. The final dimension contains the logit values
      for each class. [D1, ... DN] must match `labels`.
    k: Integer, k for @k metric. This will calculate an average precision for
      range `[1,k]`, as documented above.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    mean_average_precision: Scalar `float64` `Tensor` with the mean average
      precision values.
    update: `Operation` that increments variables appropriately, and whose
      value matches `metric`.

  Raises:
    ValueError: if k is invalid.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.sparse_average_precision_at_k is not '
                       'supported when eager execution is enabled.')

  if k < 1:
    raise ValueError('Invalid k=%s.' % k)
  with ops.name_scope(name, _at_k_name('average_precision', k),
                      (predictions, labels, weights)) as scope:
    # Calculate top k indices to produce [D1, ... DN, k] tensor.
    _, predictions_idx = nn.top_k(predictions, k)
    return _streaming_sparse_average_precision_at_top_k(
        labels=labels,
        predictions_idx=predictions_idx,
        weights=weights,
        metrics_collections=metrics_collections,
        updates_collections=updates_collections,
        name=scope)


def _sparse_false_positive_at_k(labels,
                                predictions_idx,
                                class_id=None,
                                weights=None):
  """Calculates false positives for precision@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels_sparse`.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).

  Returns:
    A [D1, ... DN] `Tensor` of false positive counts.
  """
  with ops.name_scope(None, 'false_positives',
                      (predictions_idx, labels, weights)):
    labels, predictions_idx = _maybe_select_class_id(labels, predictions_idx,
                                                     class_id)
    fp = sets.set_size(
        sets.set_difference(predictions_idx, labels, aminusb=True))
    fp = math_ops.to_double(fp)
    if weights is not None:
      with ops.control_dependencies((weights_broadcast_ops.assert_broadcastable(
          weights, fp),)):
        weights = math_ops.to_double(weights)
        fp = math_ops.multiply(fp, weights)
    return fp


def _streaming_sparse_false_positive_at_k(labels,
                                          predictions_idx,
                                          k=None,
                                          class_id=None,
                                          weights=None,
                                          name=None):
  """Calculates weighted per step false positives for precision@k.

  If `class_id` is specified, calculate binary true positives for `class_id`
      only.
  If `class_id` is not specified, calculate metrics for `k` predicted vs
      `n` label classes, where `n` is the 2nd dimension of `labels`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels], where N >= 1 and num_labels is the number of
      target classes for the associated prediction. Commonly, N=1 and `labels`
      has shape [batch_size, num_labels]. [D1, ... DN] must match
      `predictions_idx`.
    predictions_idx: 1-D or higher `int64` `Tensor` with last dimension `k`,
      top `k` predicted classes. For rank `n`, the first `n-1` dimensions must
      match `labels`.
    k: Integer, k for @k metric. This is only used for default op name.
    class_id: Class for which we want binary metrics.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    name: Name of new variable, and namespace for other dependent ops.

  Returns:
    A tuple of `Variable` and update `Operation`.

  Raises:
    ValueError: If `weights` is not `None` and has an incompatible shape.
  """
  with ops.name_scope(name, _at_k_name('false_positive', k, class_id=class_id),
                      (predictions_idx, labels, weights)) as scope:
    fp = _sparse_false_positive_at_k(
        predictions_idx=predictions_idx,
        labels=labels,
        class_id=class_id,
        weights=weights)
    batch_total_fp = math_ops.to_double(math_ops.reduce_sum(fp))

    var = metric_variable([], dtypes.float64, name=scope)
    return var, state_ops.assign_add(var, batch_total_fp, name='update')


@tf_export(v1=['metrics.precision_at_top_k'])
def precision_at_top_k(labels,
                       predictions_idx,
                       k=None,
                       class_id=None,
                       weights=None,
                       metrics_collections=None,
                       updates_collections=None,
                       name=None):
  """Computes precision@k of the predictions with respect to sparse labels.

  Differs from `sparse_precision_at_k` in that predictions must be in the form
  of top `k` class indices, whereas `sparse_precision_at_k` expects logits.
  Refer to `sparse_precision_at_k` for more details.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions`. Values
      should be in range [0, num_classes), where num_classes is the last
      dimension of `predictions`. Values outside this range are ignored.
    predictions_idx: Integer `Tensor` with shape [D1, ... DN, k] where
      N >= 1. Commonly, N=1 and predictions has shape [batch size, k].
      The final dimension contains the top `k` predicted class indices.
      [D1, ... DN] must match `labels`.
    k: Integer, k for @k metric. Only used for the default op name.
    class_id: Integer class ID for which we want binary metrics. This should be
      in range [0, num_classes], where num_classes is the last dimension of
      `predictions`. If `class_id` is outside this range, the method returns
      NAN.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    precision: Scalar `float64` `Tensor` with the value of `true_positives`
      divided by the sum of `true_positives` and `false_positives`.
    update_op: `Operation` that increments `true_positives` and
      `false_positives` variables appropriately, and whose value matches
      `precision`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match
      `predictions`, or if either `metrics_collections` or `updates_collections`
      are not a list or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.precision_at_top_k is not '
                       'supported when eager execution is enabled.')

  with ops.name_scope(name, _at_k_name('precision', k, class_id=class_id),
                      (predictions_idx, labels, weights)) as scope:
    labels = _maybe_expand_labels(labels, predictions_idx)
    top_k_idx = math_ops.to_int64(predictions_idx)
    tp, tp_update = _streaming_sparse_true_positive_at_k(
        predictions_idx=top_k_idx,
        labels=labels,
        k=k,
        class_id=class_id,
        weights=weights)
    fp, fp_update = _streaming_sparse_false_positive_at_k(
        predictions_idx=top_k_idx,
        labels=labels,
        k=k,
        class_id=class_id,
        weights=weights)

    def precision_across_replicas(_, tp, fp):
      return math_ops.div(tp, math_ops.add(tp, fp), name=scope)

    metric = _aggregate_across_replicas(
        metrics_collections, precision_across_replicas, tp, fp)

    update = math_ops.div(
        tp_update, math_ops.add(tp_update, fp_update), name='update')
    if updates_collections:
      ops.add_to_collections(updates_collections, update)
    return metric, update


@tf_export(v1=['metrics.sparse_precision_at_k'])
@deprecated(None, 'Use precision_at_k instead')
def sparse_precision_at_k(labels,
                          predictions,
                          k,
                          class_id=None,
                          weights=None,
                          metrics_collections=None,
                          updates_collections=None,
                          name=None):
  """Renamed to `precision_at_k`, please use that method instead."""
  return precision_at_k(
      labels=labels,
      predictions=predictions,
      k=k,
      class_id=class_id,
      weights=weights,
      metrics_collections=metrics_collections,
      updates_collections=updates_collections,
      name=name)


@tf_export(v1=['metrics.precision_at_k'])
def precision_at_k(labels,
                   predictions,
                   k,
                   class_id=None,
                   weights=None,
                   metrics_collections=None,
                   updates_collections=None,
                   name=None):
  """Computes precision@k of the predictions with respect to sparse labels.

  If `class_id` is specified, we calculate precision by considering only the
      entries in the batch for which `class_id` is in the top-k highest
      `predictions`, and computing the fraction of them for which `class_id` is
      indeed a correct label.
  If `class_id` is not specified, we'll calculate precision as how often on
      average a class among the top-k classes with the highest predicted values
      of a batch entry is correct and can be found in the label for that entry.

  `precision_at_k` creates two local variables,
  `true_positive_at_<k>` and `false_positive_at_<k>`, that are used to compute
  the precision@k frequency. This frequency is ultimately returned as
  `precision_at_<k>`: an idempotent operation that simply divides
  `true_positive_at_<k>` by total (`true_positive_at_<k>` +
  `false_positive_at_<k>`).

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `precision_at_<k>`. Internally, a `top_k` operation computes a `Tensor`
  indicating the top `k` `predictions`. Set operations applied to `top_k` and
  `labels` calculate the true positives and false positives weighted by
  `weights`. Then `update_op` increments `true_positive_at_<k>` and
  `false_positive_at_<k>` using these values.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  Args:
    labels: `int64` `Tensor` or `SparseTensor` with shape
      [D1, ... DN, num_labels] or [D1, ... DN], where the latter implies
      num_labels=1. N >= 1 and num_labels is the number of target classes for
      the associated prediction. Commonly, N=1 and `labels` has shape
      [batch_size, num_labels]. [D1, ... DN] must match `predictions`. Values
      should be in range [0, num_classes), where num_classes is the last
      dimension of `predictions`. Values outside this range are ignored.
    predictions: Float `Tensor` with shape [D1, ... DN, num_classes] where
      N >= 1. Commonly, N=1 and predictions has shape [batch size, num_classes].
      The final dimension contains the logit values for each class. [D1, ... DN]
      must match `labels`.
    k: Integer, k for @k metric.
    class_id: Integer class ID for which we want binary metrics. This should be
      in range [0, num_classes], where num_classes is the last dimension of
      `predictions`. If `class_id` is outside this range, the method returns
      NAN.
    weights: `Tensor` whose rank is either 0, or n-1, where n is the rank of
      `labels`. If the latter, it must be broadcastable to `labels` (i.e., all
      dimensions must be either `1`, or the same as the corresponding `labels`
      dimension).
    metrics_collections: An optional list of collections that values should
      be added to.
    updates_collections: An optional list of collections that updates should
      be added to.
    name: Name of new update operation, and namespace for other dependent ops.

  Returns:
    precision: Scalar `float64` `Tensor` with the value of `true_positives`
      divided by the sum of `true_positives` and `false_positives`.
    update_op: `Operation` that increments `true_positives` and
      `false_positives` variables appropriately, and whose value matches
      `precision`.

  Raises:
    ValueError: If `weights` is not `None` and its shape doesn't match
      `predictions`, or if either `metrics_collections` or `updates_collections`
      are not a list or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.sparse_precision_at_k is not '
                       'supported when eager execution is enabled.')

  with ops.name_scope(name, _at_k_name('precision', k, class_id=class_id),
                      (predictions, labels, weights)) as scope:
    _, top_k_idx = nn.top_k(predictions, k)
    return precision_at_top_k(
        labels=labels,
        predictions_idx=top_k_idx,
        k=k,
        class_id=class_id,
        weights=weights,
        metrics_collections=metrics_collections,
        updates_collections=updates_collections,
        name=scope)


@tf_export(v1=['metrics.specificity_at_sensitivity'])
def specificity_at_sensitivity(labels,
                               predictions,
                               sensitivity,
                               weights=None,
                               num_thresholds=200,
                               metrics_collections=None,
                               updates_collections=None,
                               name=None):
  """Computes the specificity at a given sensitivity.

  The `specificity_at_sensitivity` function creates four local
  variables, `true_positives`, `true_negatives`, `false_positives` and
  `false_negatives` that are used to compute the specificity at the given
  sensitivity value. The threshold for the given sensitivity value is computed
  and used to evaluate the corresponding specificity.

  For estimation of the metric over a stream of data, the function creates an
  `update_op` operation that updates these variables and returns the
  `specificity`. `update_op` increments the `true_positives`, `true_negatives`,
  `false_positives` and `false_negatives` counts with the weight of each case
  found in the `predictions` and `labels`.

  If `weights` is `None`, weights default to 1. Use weights of 0 to mask values.

  For additional information about specificity and sensitivity, see the
  following: https://en.wikipedia.org/wiki/Sensitivity_and_specificity

  Args:
    labels: The ground truth values, a `Tensor` whose dimensions must match
      `predictions`. Will be cast to `bool`.
    predictions: A floating point `Tensor` of arbitrary shape and whose values
      are in the range `[0, 1]`.
    sensitivity: A scalar value in range `[0, 1]`.
    weights: Optional `Tensor` whose rank is either 0, or the same rank as
      `labels`, and must be broadcastable to `labels` (i.e., all dimensions must
      be either `1`, or the same as the corresponding `labels` dimension).
    num_thresholds: The number of thresholds to use for matching the given
      sensitivity.
    metrics_collections: An optional list of collections that `specificity`
      should be added to.
    updates_collections: An optional list of collections that `update_op` should
      be added to.
    name: An optional variable_scope name.

  Returns:
    specificity: A scalar `Tensor` representing the specificity at the given
      `specificity` value.
    update_op: An operation that increments the `true_positives`,
      `true_negatives`, `false_positives` and `false_negatives` variables
      appropriately and whose value matches `specificity`.

  Raises:
    ValueError: If `predictions` and `labels` have mismatched shapes, if
      `weights` is not `None` and its shape doesn't match `predictions`, or if
      `sensitivity` is not between 0 and 1, or if either `metrics_collections`
      or `updates_collections` are not a list or tuple.
    RuntimeError: If eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('tf.metrics.specificity_at_sensitivity is not '
                       'supported when eager execution is enabled.')

  if sensitivity < 0 or sensitivity > 1:
    raise ValueError('`sensitivity` must be in the range [0, 1].')

  with variable_scope.variable_scope(name, 'specificity_at_sensitivity',
                                     (predictions, labels, weights)):
    kepsilon = 1e-7  # to account for floating point imprecisions
    thresholds = [
        (i + 1) * 1.0 / (num_thresholds - 1) for i in range(num_thresholds - 2)
    ]
    thresholds = [0.0 - kepsilon] + thresholds + [1.0 - kepsilon]

    values, update_ops = _confusion_matrix_at_thresholds(
        labels, predictions, thresholds, weights)

    def compute_specificity_at_sensitivity(tp, tn, fp, fn, name):
      """Computes the specificity at the given sensitivity.

      Args:
        tp: True positives.
        tn: True negatives.
        fp: False positives.
        fn: False negatives.
        name: The name of the operation.

      Returns:
        The specificity using the aggregated values.
      """
      sensitivities = math_ops.div(tp, tp + fn + kepsilon)

      # We'll need to use this trick until tf.argmax allows us to specify
      # whether we should use the first or last index in case of ties.
      min_val = math_ops.reduce_min(math_ops.abs(sensitivities - sensitivity))
      indices_at_minval = math_ops.equal(
          math_ops.abs(sensitivities - sensitivity), min_val)
      indices_at_minval = math_ops.to_int64(indices_at_minval)
      indices_at_minval = math_ops.cumsum(indices_at_minval)
      tf_index = math_ops.argmax(indices_at_minval, 0)
      tf_index = math_ops.cast(tf_index, dtypes.int32)

      # Now, we have the implicit threshold, so compute the specificity:
      return math_ops.div(tn[tf_index], tn[tf_index] + fp[tf_index] + kepsilon,
                          name)

    def specificity_across_replicas(_, values):
      return compute_specificity_at_sensitivity(
          values['tp'], values['tn'], values['fp'], values['fn'], 'value')

    specificity = _aggregate_across_replicas(
        metrics_collections, specificity_across_replicas, values)

    update_op = compute_specificity_at_sensitivity(
        update_ops['tp'], update_ops['tn'], update_ops['fp'], update_ops['fn'],
        'update_op')
    if updates_collections:
      ops.add_to_collections(updates_collections, update_op)

    return specificity, update_op
