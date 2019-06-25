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
"""Tests for contrib.losses.python.losses.loss_ops."""
# pylint: disable=unused-import,g-bad-import-order
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function
# pylint: enable=unused-import

import numpy as np
from tensorflow.contrib.framework.python.ops import arg_scope
from tensorflow.contrib.losses.python.losses import loss_ops
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import random_seed
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops import variables
from tensorflow.python.platform import test
from tensorflow.python.training import momentum as momentum_lib


class AbsoluteDifferenceLossTest(test.TestCase):

  def setUp(self):
    self._predictions = constant_op.constant([4, 8, 12, 8, 1, 3], shape=(2, 3))
    self._labels = constant_op.constant([1, 9, 2, -5, -2, 6], shape=(2, 3))

  def testValueErrorThrownWhenWeightIsNone(self):
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.absolute_difference(
            self._predictions, self._predictions, weights=None)

  def testAllCorrectNoLossWeight(self):
    loss = loss_ops.absolute_difference(self._predictions, self._predictions)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testNonZeroLoss(self):
    loss = loss_ops.absolute_difference(self._predictions, self._labels)
    with self.cached_session():
      self.assertAlmostEqual(5.5, loss.eval(), 3)

  def testNonZeroLossWithPythonScalarWeight(self):
    weights = 2.3
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(5.5 * weights, loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    weights = 2.3
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        constant_op.constant(weights))
    with self.cached_session():
      self.assertAlmostEqual(5.5 * weights, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    weights = constant_op.constant([1.2, 0.0], shape=[2,])
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(5.6, loss.eval(), 3)

  def testNonZeroLossWithTwoDimBatchSpecificWeights(self):
    weights = constant_op.constant([1.2, 0.0], shape=[2, 1])
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(5.6, loss.eval(), 3)

  def testNonZeroLossWithSampleSpecificWeights(self):
    weights = constant_op.constant([3, 6, 5, 0, 4, 2], shape=[2, 3])
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(16.6, loss.eval(), 3)

  def testNonZeroLossWithSampleSpecificWeightsMostZero(self):
    weights = constant_op.constant([0, 0, 0, 0, 0, 2], shape=[2, 3])
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(6.0, loss.eval(), 3)

  def testLossWithSampleSpecificWeightsAllZero(self):
    weights = array_ops.zeros((2, 3))
    loss = loss_ops.absolute_difference(self._predictions, self._labels,
                                        weights)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)


class SoftmaxCrossEntropyLossTest(test.TestCase):

  def testNoneWeightRaisesValueError(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[1, 0, 0],
                                   [0, 1, 0],
                                   [0, 0, 1]])
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.softmax_cross_entropy(logits, labels, weights=None)

  def testAllCorrect(self):
    with self.cached_session():
      logits = constant_op.constant([[10.0, 0.0, 0.0],
                                     [0.0, 10.0, 0.0],
                                     [0.0, 0.0, 10.0]])
      labels = constant_op.constant([[1, 0, 0],
                                     [0, 1, 0],
                                     [0, 0, 1]])
      loss = loss_ops.softmax_cross_entropy(logits, labels)
      self.assertEquals('softmax_cross_entropy_loss/value', loss.op.name)
      self.assertAlmostEqual(loss.eval(), 0.0, 3)

  def testAllWrong(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])

    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 10.0, 3)

  def testNonZeroLossWithPythonScalarWeight(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = 2.3
    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(weights * 10.0, loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = 2.3
    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels,
                                            constant_op.constant(weights))
      self.assertAlmostEqual(weights * 10.0, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = constant_op.constant([1.2, 3.4, 5.6], shape=[3])
    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual((1.2 + 3.4 + 5.6) * 10.0 / 3.0, loss.eval(), 3)

  def testAllWrongAllWeightsMissing(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = constant_op.constant([0, 0, 0], shape=[3])
    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testSomeWeightsMissing(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = constant_op.constant([1.2, 0, 0], shape=[3])
    with self.cached_session():
      loss = loss_ops.softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(12.0, loss.eval(), 3)

  def testSoftmaxWithMeasurementSpecificWeightsRaisesException(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[1, 0, 0],
                                     [0, 1, 0],
                                     [0, 0, 1]])
      weights = constant_op.constant([[3, 4, 5],
                                      [2, 6, 0],
                                      [8, 0, 1]])

      with self.assertRaises(ValueError):
        loss_ops.softmax_cross_entropy(logits, labels, weights=weights).eval()

  def testSoftmaxLabelSmoothing(self):
    with self.cached_session():
      # Softmax Cross Entropy Loss is:
      #   -\sum_i p_i \log q_i
      # where for a softmax activation
      # \log q_i = x_i - \log \sum_j \exp x_j
      #          = x_i - x_max - \log \sum_j \exp (x_j - x_max)
      # For our activations, [100, -100, -100] the log partion function becomes
      # \log ( exp(0) + exp(-200) + exp(-200) ) = 0
      # so our log softmaxes become: [0, -200, -200]
      # so our cross entropy loss is:
      # -(1 - L + L/n) * 0 + 400 * L/n = 400 L/n
      logits = constant_op.constant([[100.0, -100.0, -100.0]])
      labels = constant_op.constant([[1, 0, 0]])
      label_smoothing = 0.1
      loss = loss_ops.softmax_cross_entropy(
          logits, labels, label_smoothing=label_smoothing)
      self.assertEquals(loss.op.name, 'softmax_cross_entropy_loss/value')
      expected_value = 400.0 * label_smoothing / 3.0
      self.assertAlmostEqual(loss.eval(), expected_value, 3)

  def testLossWithDynamicallyShapedWeights1D(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = [2.3, 2.4, 2.5]
    weights_placeholder = array_ops.placeholder(dtypes.float32, shape=[None])
    loss = loss_ops.softmax_cross_entropy(logits, labels, weights_placeholder)
    with self.cached_session() as sess:
      loss = sess.run(loss, {weights_placeholder: weights})
      self.assertAlmostEqual(np.average(weights) * 10.0, loss, 3)

  def testLossWithDynamicallyShapedWeights2D(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0, 0, 1],
                                   [1, 0, 0],
                                   [0, 1, 0]])
    weights = [[2.3], [2.4], [2.5]]
    weights_placeholder = array_ops.placeholder(
        dtypes.float32, shape=[None, None])
    loss = loss_ops.softmax_cross_entropy(logits, labels, weights_placeholder)
    with self.cached_session() as sess:
      loss = sess.run(loss, {weights_placeholder: weights})
      self.assertAlmostEqual(np.average(weights) * 10.0, loss, 3)


class SparseSoftmaxCrossEntropyLossTest(test.TestCase):

  def testNoneWeightRaisesValueError(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[0], [1], [2]])
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(logits, labels, weights=None)

  def testAllCorrectInt32Labels(self):
    with self.cached_session():
      logits = constant_op.constant([[10.0, 0.0, 0.0],
                                     [0.0, 10.0, 0.0],
                                     [0.0, 0.0, 10.0]])
      labels = constant_op.constant([[0], [1], [2]], dtype=dtypes.int32)
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 0.0, 3)

  def testAllCorrectInt64Labels(self):
    with self.cached_session():
      logits = constant_op.constant([[10.0, 0.0, 0.0],
                                     [0.0, 10.0, 0.0],
                                     [0.0, 0.0, 10.0]])
      labels = constant_op.constant([[0], [1], [2]], dtype=dtypes.int64)
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 0.0, 3)

  def testAllCorrectNonColumnLabels(self):
    with self.cached_session():
      logits = constant_op.constant([[10.0, 0.0, 0.0],
                                     [0.0, 10.0, 0.0],
                                     [0.0, 0.0, 10.0]])
      labels = constant_op.constant([0, 1, 2])
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 0.0, 3)

  def testAllWrongInt32Labels(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]], dtype=dtypes.int32)

    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 10.0, 3)

  def testAllWrongInt64Labels(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]], dtype=dtypes.int64)

    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 10.0, 3)

  def testAllWrongNonColumnLabels(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([2, 0, 1])

    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sparse_softmax_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 10.0, 3)

  def testNonZeroLossWithPythonScalarWeight(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = 2.3
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(weights * 10.0, loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = 2.3
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(
          logits, labels, constant_op.constant(weights))
      self.assertAlmostEqual(weights * 10.0, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = constant_op.constant([1.2, 3.4, 5.6], shape=[3])
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual((1.2 + 3.4 + 5.6) * 10.0 / 3.0, loss.eval(), 3)

  def testNonZeroLossWithColumnWeights(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = constant_op.constant([[1.2], [3.4], [5.6]])
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual((1.2 + 3.4 + 5.6) * 10.0 / 3.0, loss.eval(), 3)

  def testAllWrongAllWeightsMissing(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = constant_op.constant([0, 0, 0], shape=[3])
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testSomeWeightsMissing(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([[2], [0], [1]])
    weights = constant_op.constant([1.2, 0, 0], shape=[3])
    with self.cached_session():
      loss = loss_ops.sparse_softmax_cross_entropy(logits, labels, weights)
      self.assertAlmostEqual(12.0, loss.eval(), 3)

  def testMeasurementSpecificWeightsRaisesException(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0], [1], [2]])
      weights = constant_op.constant([[3, 4, 5], [2, 6, 0], [8, 0, 1]])

      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(
            logits, labels, weights=weights).eval()

  def testInconsistentWeightSizeRaisesException(self):
    """The weight tensor has incorrect number of elements."""
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0], [1], [2]])
      weights = constant_op.constant([1.2, 3.4, 5.6, 7.8])

      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(
            logits, labels, weights=weights).eval()

  def testInconsistentLabelSizeRaisesException(self):
    """The label tensor has incorrect number of elements."""
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0], [1], [2], [3]])
      weights = constant_op.constant([1.2, 3.4, 5.6])

      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(
            logits, labels, weights=weights).eval()

  def testInconsistentWeightShapeRaisesException(self):
    """The weight tensor has incorrect shape."""
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0, -100.0],
                                     [-100.0, -100.0, 100.0, -100.0],
                                     [-100.0, -100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0], [1], [2], [3]])
      weights = constant_op.constant([[1.2, 3.4], [5.6, 7.8]])

      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(
            logits, labels, weights=weights).eval()

  def testInconsistentLabelShapeRaisesException(self):
    """The label tensor has incorrect shape."""
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0, -100.0],
                                     [-100.0, -100.0, 100.0, -100.0],
                                     [-100.0, -100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0, 1], [2, 3]])
      weights = constant_op.constant([1.2, 3.4, 5.6, 7.8])

      with self.assertRaises(ValueError):
        loss_ops.sparse_softmax_cross_entropy(
            logits, labels, weights=weights).eval()

  def testLossWithDynamicallyShapedWeights1D(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([2, 0, 1])
    weights = [2.3, 2.4, 2.5]
    weights_placeholder = array_ops.placeholder(
        dtypes.float32, shape=[None])
    loss = loss_ops.sparse_softmax_cross_entropy(
        logits, labels, weights_placeholder)
    with self.cached_session() as sess:
      loss = sess.run(loss, {weights_placeholder: weights})
      self.assertAlmostEqual(np.average(weights) * 10.0, loss, 3)

  def testLossWithDynamicallyShapedWeights2D(self):
    logits = constant_op.constant([[10.0, 0.0, 0.0],
                                   [0.0, 10.0, 0.0],
                                   [0.0, 0.0, 10.0]])
    labels = constant_op.constant([2, 0, 1])
    weights = [[2.3], [2.4], [2.5]]
    weights_placeholder = array_ops.placeholder(
        dtypes.float32, shape=[None, None])
    loss = loss_ops.sparse_softmax_cross_entropy(
        logits, labels, weights_placeholder)
    with self.cached_session() as sess:
      loss = sess.run(loss, {weights_placeholder: weights})
      self.assertAlmostEqual(np.average(weights) * 10.0, loss, 3)


class SigmoidCrossEntropyLossTest(test.TestCase):

  def testAllCorrectSigmoid(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[1, 0, 0], [0, 1, 0], [0, 0, 1]])
      loss = loss_ops.sigmoid_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sigmoid_cross_entropy_loss/value')
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testLossWithSingleDimPlaceholderForLogitsAndWeights1(self):
    logits = array_ops.placeholder(dtypes.float32, shape=(None, 1))
    labels = array_ops.placeholder(dtypes.float32, shape=(None, 1))
    weights = array_ops.ones_like(logits, dtype=dtypes.float32)

    loss = loss_ops.sigmoid_cross_entropy(logits, labels, weights)

    with self.cached_session() as sess:
      loss = sess.run(loss,
                      feed_dict={
                          logits: np.ones((32, 1)),
                          labels: np.ones((32, 1)),
                      })
      self.assertAlmostEqual(0.313, loss, 3)

  def testLossWithSingleDimPlaceholderForLogitsAndWeights2(self):
    logits = array_ops.placeholder(dtypes.float32, shape=(None, 2))
    labels = array_ops.placeholder(dtypes.float32, shape=(None, 2))
    weights = array_ops.ones_like(logits, dtype=dtypes.float32)

    loss = loss_ops.sigmoid_cross_entropy(logits, labels, weights)

    with self.cached_session() as sess:
      loss = sess.run(loss,
                      feed_dict={
                          logits: np.ones((32, 2)),
                          labels: np.ones((32, 2)),
                      })
      self.assertAlmostEqual(0.313, loss, 3)

  def testAllWrongSigmoid(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0, 0, 1],
                                     [1, 0, 0],
                                     [0, 1, 0]])
      loss = loss_ops.sigmoid_cross_entropy(logits, labels)
      self.assertEquals(loss.op.name, 'sigmoid_cross_entropy_loss/value')
      self.assertAlmostEqual(loss.eval(), 600.0 / 9.0, 3)

  def testAllWrongSigmoidWithMeasurementSpecificWeights(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0],
                                     [-100.0, 100.0, -100.0],
                                     [-100.0, -100.0, 100.0]])
      labels = constant_op.constant([[0, 0, 1],
                                     [1, 0, 0],
                                     [0, 1, 0]])
      weights = constant_op.constant([[3, 4, 5],
                                      [2, 6, 0],
                                      [8, 0, 1]])
      loss = loss_ops.sigmoid_cross_entropy(logits, labels, weights)
      self.assertEquals(loss.op.name, 'sigmoid_cross_entropy_loss/value')
      self.assertAlmostEqual(1700.0 / 7.0, loss.eval(), 3)

  def testMultiCorrectSigmoid(self):
    logits = constant_op.constant([[100.0, -100.0, 100.0],
                                   [100.0, 100.0, -100.0],
                                   [-100.0, 100.0, 100.0]])
    labels = constant_op.constant([[1, 0, 1],
                                   [1, 1, 0],
                                   [0, 1, 1]])
    loss = loss_ops.sigmoid_cross_entropy(logits, labels)
    self.assertEquals(loss.op.name, 'sigmoid_cross_entropy_loss/value')

    with self.cached_session():
      self.assertAlmostEqual(loss.eval(), 0.0, 3)

  def testSigmoidLabelSmoothingCorrect(self):
    with self.cached_session():
      logits = constant_op.constant([[100.0, -100.0, -100.0]])
      labels = constant_op.constant([[1, 0, 1]])
      # Sigmoid cross entropy loss is:
      #   max(x,0) - x*z + log(1 + exp(-abs(x)))
      # The new labels are:
      #    z' = z * (1 - L) + 0.5 L
      #    1 -> 1 - 0.5 L
      #    0 -> 0.5 L
      # here we expect:
      # 1/3 * (100 - 100 * (1 - 0.5 L)  + 0
      #       + 0  + 100 * (0.5 L)      + 0
      #       + 0  + 100 * (1 - 0.5 L)  + 0)
      # = 1/3 * (100 + 50 L)
      label_smoothing = 0.1
      loss = loss_ops.sigmoid_cross_entropy(
          logits, labels, label_smoothing=label_smoothing)
      self.assertEquals(loss.op.name, 'sigmoid_cross_entropy_loss/value')
      expected_value = (100.0 + 50.0 * label_smoothing) / 3.0
      self.assertAlmostEqual(loss.eval(), expected_value, 3)

  def testSigmoidLabelSmoothingEqualsSoftmaxTwoLabel(self):
    with self.cached_session():
      label_smoothing = 0.1
      sigmoid_logits = constant_op.constant([[100.0, -100.0, -100.0]])
      sigmoid_labels = constant_op.constant([[1, 0, 1]])
      sigmoid_loss = loss_ops.sigmoid_cross_entropy(
          sigmoid_logits, sigmoid_labels, label_smoothing=label_smoothing)

      softmax_logits = constant_op.constant(
          [[0.0, 100.0], [100.0, 0.0], [100.0, 0.0]])
      softmax_labels = constant_op.constant([[0, 1], [1, 0], [0, 1]])
      softmax_loss = loss_ops.softmax_cross_entropy(
          softmax_logits, softmax_labels, label_smoothing=label_smoothing)
      self.assertAlmostEqual(sigmoid_loss.eval(), softmax_loss.eval(), 3)


class LogLossTest(test.TestCase):

  def setUp(self):
    predictions = np.asarray([.9, .2, .2, .8, .4, .6]).reshape((2, 3))
    labels = np.asarray([1.0, 0.0, 1.0, 1.0, 0.0, 0.0]).reshape((2, 3))

    self._np_predictions = predictions
    self._np_labels = labels

    epsilon = 1e-7
    self._expected_losses = np.multiply(
        labels, np.log(predictions + epsilon)) + np.multiply(
            1 - labels, np.log(1 - predictions + epsilon))

    self._predictions = constant_op.constant(predictions)
    self._labels = constant_op.constant(labels)

  def testValueErrorThrownWhenWeightIsNone(self):
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.log_loss(self._labels, self._labels, weights=None)

  def testAllCorrectNoLossWeight(self):
    loss = loss_ops.log_loss(self._labels, self._labels)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testAllCorrectNoLossWeightWithPlaceholder(self):
    tf_predictions = array_ops.placeholder(
        dtypes.float32, shape=self._np_labels.shape)
    loss = loss_ops.log_loss(tf_predictions, self._labels)
    with self.cached_session():
      self.assertAlmostEqual(
          0.0, loss.eval(feed_dict={tf_predictions: self._np_labels}), 3)

  def testNonZeroLoss(self):
    loss = loss_ops.log_loss(self._predictions, self._labels)
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(self._expected_losses) / 6.0,
                             loss.eval(), 3)

  def testNonZeroLossWithPythonScalarWeight(self):
    weights = 2.3
    loss = loss_ops.log_loss(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(weights * -np.sum(self._expected_losses) / 6.0,
                             loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    weights = 2.3
    loss = loss_ops.log_loss(self._predictions, self._labels,
                             constant_op.constant(weights))
    with self.cached_session():
      self.assertAlmostEqual(weights * -np.sum(self._expected_losses) / 6.0,
                             loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeightAndPlaceholder(self):
    tf_predictions = array_ops.placeholder(
        dtypes.float32, shape=self._np_predictions.shape)
    weights = 2.3
    loss = loss_ops.log_loss(tf_predictions, self._labels,
                             constant_op.constant(weights))
    with self.cached_session() as sess:
      loss = sess.run(loss, feed_dict={tf_predictions: self._np_predictions})
      self.assertAlmostEqual(weights * -np.sum(self._expected_losses) / 6.0,
                             loss, 3)

  def testNonZeroLossWithScalarTensorWeightAndPlaceholderWithRankOnly(self):
    tf_predictions = array_ops.placeholder(dtypes.float32, shape=[None, None])
    weights = 2.3
    loss = loss_ops.log_loss(tf_predictions, self._labels,
                             constant_op.constant(weights))
    with self.cached_session() as sess:
      loss = sess.run(loss, feed_dict={tf_predictions: self._np_predictions})
      self.assertAlmostEqual(weights * -np.sum(self._expected_losses) / 6.0,
                             loss, 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    weights = constant_op.constant([1.2, 3.4], shape=[2])
    expected_losses = np.multiply(
        self._expected_losses,
        np.asarray([1.2, 1.2, 1.2, 3.4, 3.4, 3.4]).reshape((2, 3)))
    loss = loss_ops.log_loss(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(expected_losses) / 6.0, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeightsSomeZero(self):
    weights = constant_op.constant([1.2, 0], shape=[2])
    expected_losses = np.multiply(self._expected_losses,
                                  np.asarray([1.2, 1.2, 1.2, 0, 0, 0]).reshape(
                                      (2, 3)))
    loss = loss_ops.log_loss(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(expected_losses) / 3.0, loss.eval(), 3)

  def testNonZeroLossWithTwoDimBatchSpecificWeightsSomeZero(self):
    weights = constant_op.constant([1.2, 0], shape=[2, 1])
    expected_losses = np.multiply(self._expected_losses,
                                  np.asarray([1.2, 1.2, 1.2, 0, 0, 0]).reshape(
                                      (2, 3)))
    loss = loss_ops.log_loss(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(expected_losses) / 3.0, loss.eval(), 3)

  def testWeightsWithSameNumDimsButWrongShapeThrowsException(self):
    weights = constant_op.constant(np.random.normal(size=(2, 4)), shape=[2, 4])
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.log_loss(self._predictions, self._labels, weights)

  def testNonZeroLossWithMeasurementSpecificWeights(self):
    weights = np.array([3, 6, 5, 0, 4, 2]).reshape((2, 3))
    expected_losses = np.multiply(self._expected_losses, weights)

    loss = loss_ops.log_loss(
        self._predictions,
        self._labels,
        constant_op.constant(
            weights, shape=(2, 3)))
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(expected_losses) / 5.0, loss.eval(), 3)

  def testNonZeroLossWithMeasurementSpecificWeightsWithPlaceholder(self):
    weights = np.array([3, 6, 5, 0, 4, 2]).reshape((2, 3))
    expected_losses = np.multiply(self._expected_losses, weights)

    tf_predictions = array_ops.placeholder(dtypes.float32, shape=[2, 3])
    loss = loss_ops.log_loss(
        tf_predictions,
        self._labels,
        constant_op.constant(
            weights, shape=(2, 3)))

    with self.cached_session() as sess:
      loss = sess.run(loss, feed_dict={tf_predictions: self._np_predictions})
      self.assertAlmostEqual(-np.sum(expected_losses) / 5.0, loss, 3)

  def testNonZeroLossWithSampleSpecificWeightsMostZero(self):
    weights = np.array([0, 0, 0, 0, 0, 2]).reshape((2, 3))
    expected_losses = np.multiply(self._expected_losses, weights)

    loss = loss_ops.log_loss(
        self._predictions,
        self._labels,
        constant_op.constant(
            weights, shape=(2, 3)))
    with self.cached_session():
      self.assertAlmostEqual(-np.sum(expected_losses), loss.eval(), 3)

  def testNonZeroLossWithSampleSpecificWeightsMostZeroWithPlaceholder(self):
    weights = np.array([0, 0, 0, 0, 0, 2]).reshape((2, 3))
    expected_losses = np.multiply(self._expected_losses, weights)

    tf_predictions = array_ops.placeholder(dtypes.float32, shape=[2, 3])
    tf_weights = constant_op.constant(weights, shape=(2, 3))
    loss = loss_ops.log_loss(tf_predictions, self._labels, tf_weights)

    with self.cached_session() as sess:
      loss = sess.run(loss, feed_dict={tf_predictions: self._np_predictions})
      self.assertAlmostEqual(-np.sum(expected_losses), loss, 3)

  def testLossWithSampleSpecificWeightsAllZero(self):
    tf_weights = array_ops.zeros(shape=(2, 3))
    loss = loss_ops.log_loss(self._predictions, self._labels, tf_weights)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)


class HingeLossTest(test.TestCase):

  def testIncompatibleShapes(self):
    with self.cached_session():
      logits = constant_op.constant([[-1.0], [2.1]])
      labels = constant_op.constant([0.0, 1.0])
      with self.assertRaises(ValueError):
        _ = loss_ops.hinge_loss(logits, labels).eval()

  def testAllOutsideMargin(self):
    with self.cached_session():
      logits = constant_op.constant([1.2, -1.4, -1.0, 2.1])
      labels = constant_op.constant([1.0, 0.0, 0.0, 1.0])
      loss = loss_ops.hinge_loss(logits, labels)
      self.assertAllClose(loss.eval(), [0.0, 0.0, 0.0, 0.0], atol=1e-3)

  def testSomeInsideMargin(self):
    with self.cached_session():
      logits = constant_op.constant([[-0.7], [-1.4], [1.4], [0.6]])
      labels = constant_op.constant([[0.0], [0.0], [1.0], [1.0]])
      loss = loss_ops.hinge_loss(logits, labels)
      # Examples 1 and 4 are on the correct side of the hyperplane but within
      # the margin so they incur some (small) loss.
      self.assertAllClose(loss.eval(), [[0.3], [0.0], [0.0], [0.4]], atol=1e-3)

  def testSomeMisclassified(self):
    with self.cached_session():
      logits = constant_op.constant([[[1.2], [0.4], [-1.0], [-1.1]]])
      labels = constant_op.constant([[[1.0], [0.0], [0.0], [1.0]]])
      loss = loss_ops.hinge_loss(logits, labels)
      # Examples 2 and 4 are on the wrong side of the hyperplane so they incur
      # some (fairly large) loss.
      self.assertAllClose(
          loss.eval(), [[[0.0], [1.4], [0.0], [2.1]]], atol=1e-3)


class MeanSquaredErrorTest(test.TestCase):

  def setUp(self):
    self._predictions = constant_op.constant([4, 8, 12, 8, 1, 3], shape=(2, 3))
    self._labels = constant_op.constant([1, 9, 2, -5, -2, 6], shape=(2, 3))

  def testValueErrorThrownWhenWeightIsNone(self):
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.mean_squared_error(
            self._predictions, self._predictions, weights=None)

  def testAllCorrectNoLossWeight(self):
    loss = loss_ops.mean_squared_error(self._predictions, self._predictions)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testNonZeroLoss(self):
    loss = loss_ops.mean_squared_error(self._predictions, self._labels)
    with self.cached_session():
      self.assertAlmostEqual(49.5, loss.eval(), 3)

  def testNonZeroLossWithPythonScalarWeight(self):
    weights = 2.3
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(49.5 * weights, loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    weights = 2.3
    loss = loss_ops.mean_squared_error(self._predictions, self._labels,
                                       constant_op.constant(weights))
    with self.cached_session():
      self.assertAlmostEqual(49.5 * weights, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    weights = constant_op.constant([1.2, 3.4], shape=[2,])
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(767.8 / 6.0, loss.eval(), 3)

  def testNonZeroLossWithTwoDimBatchSpecificWeights(self):
    weights = constant_op.constant([1.2, 3.4], shape=[2, 1])
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(767.8 / 6.0, loss.eval(), 3)

  def testNonZeroLossWithSampleSpecificWeights(self):
    weights = constant_op.constant([3, 6, 5, 0, 4, 2], shape=[2, 3])
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(587 / 5.0, loss.eval(), 3)

  def testNonZeroLossWithSampleSpecificWeightsMostZero(self):
    weights = constant_op.constant([0, 0, 0, 0, 0, 2], shape=[2, 3])
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(18.0, loss.eval(), 3)

  def testLossWithSampleSpecificWeightsAllZero(self):
    weights = array_ops.zeros((2, 3))
    loss = loss_ops.mean_squared_error(self._predictions, self._labels, weights)
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)


class MeanPairwiseSquaresErrorTest(test.TestCase):

  def setUp(self):
    self._predictions = np.array([[4, 8, 12], [8, 1, 3]])
    self._labels = np.array([[1, 9, 2], [-5, -5, 7]])

    batch_size, dims = self._labels.shape

    # Compute the expected loss 'manually'.
    total = np.zeros((batch_size, 1))
    for b in range(batch_size):
      for i in range(dims):
        for j in range(dims):
          x = self._predictions[b, i].item() - self._predictions[b, j].item()
          y = self._labels[b, i].item() - self._labels[b, j].item()
          tmp = (x - y) * (x - y)
          total[b] += tmp

    self._expected_losses = np.divide(total, 9.0)

  def testValueErrorThrownWhenWeightIsNone(self):
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.mean_pairwise_squared_error(
            predictions=constant_op.constant(self._labels),
            labels=constant_op.constant(self._labels),
            weights=None)

  def testAllCorrectNoLossWeight(self):
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._labels),
        labels=constant_op.constant(self._labels))
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testNonZeroLoss(self):
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels))
    with self.cached_session():
      self.assertAlmostEqual(np.sum(self._expected_losses), loss.eval(), 3)

  def testGradientWithZeroWeight(self):
    with ops.Graph().as_default():
      random_seed.set_random_seed(0)

      inputs = array_ops.ones((2, 3))
      weights = variable_scope.get_variable(
          'weights',
          shape=[3, 4],
          initializer=init_ops.truncated_normal_initializer())
      predictions = math_ops.matmul(inputs, weights)

      optimizer = momentum_lib.MomentumOptimizer(
          learning_rate=0.001, momentum=0.9)
      loss = loss_ops.mean_pairwise_squared_error(predictions, predictions, 0)

      gradients_to_variables = optimizer.compute_gradients(loss)

      init_op = variables.global_variables_initializer()

      with self.cached_session() as sess:
        sess.run(init_op)
        for grad, _ in gradients_to_variables:
          np_grad = sess.run(grad)
          self.assertFalse(np.isnan(np_grad).any())

  def testNonZeroLossWithPythonScalarWeight(self):
    weights = 2.3
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=weights)
    with self.cached_session():
      self.assertAlmostEqual(weights * np.sum(self._expected_losses),
                             loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeight(self):
    weights = 2.3
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=constant_op.constant(weights))
    with self.cached_session():
      self.assertAlmostEqual(weights * np.sum(self._expected_losses),
                             loss.eval(), 3)

  def testNonZeroLossWithScalarZeroWeight(self):
    weights = 0
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=constant_op.constant(weights))
    with self.cached_session():
      self.assertAlmostEqual(0, loss.eval(), 3)

  def testNonZeroLossWithScalarTensorWeightWithPlaceholder(self):
    weights = 2.3
    tf_predictions = array_ops.placeholder(
        dtypes.float32, shape=self._predictions.shape)
    tf_labels = array_ops.placeholder(dtypes.float32, shape=self._labels.shape)
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=tf_predictions,
        labels=tf_labels,
        weights=constant_op.constant(weights))
    with self.cached_session() as sess:
      loss = sess.run(loss,
                      feed_dict={
                          tf_predictions: self._predictions,
                          tf_labels: self._labels,
                      })
      self.assertAlmostEqual(weights * np.sum(self._expected_losses), loss, 3)

  def testNonZeroLossWithOneDimBatchSpecificWeights(self):
    weights = np.asarray([2.0, 1.0]).reshape((2, 1))
    expected_losses = np.multiply(weights, self._expected_losses)

    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=constant_op.constant(
            weights, shape=[2]))
    with self.cached_session():
      self.assertAlmostEqual(np.sum(expected_losses), loss.eval(), 3)

  def testZeroLossWithOneDimBatchZeroWeights(self):
    weights = np.asarray([0.0, 0.0]).reshape((2, 1))
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=constant_op.constant(
            weights, shape=[2]))
    with self.cached_session():
      self.assertAlmostEqual(0, loss.eval(), 3)

  def testNonZeroLossWithOneDimBatchSpecificWeightsAndPlaceholders(self):
    weights = np.asarray([1.2, 3.4]).reshape((2, 1))
    expected_losses = np.multiply(weights, self._expected_losses)

    tf_predictions = array_ops.placeholder(
        dtypes.float32, shape=self._predictions.shape)
    tf_labels = array_ops.placeholder(dtypes.int32, shape=self._labels.shape)
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=tf_predictions,
        labels=tf_labels,
        weights=constant_op.constant(
            weights, shape=[2]))

    with self.cached_session() as sess:
      loss = sess.run(loss,
                      feed_dict={
                          tf_predictions: self._predictions,
                          tf_labels: self._labels,
                      })
      self.assertAlmostEqual(np.sum(expected_losses), loss, 3)

  def testLossWithAllZeroBatchSpecificWeights(self):
    weights = np.zeros((2, 1))
    loss = loss_ops.mean_pairwise_squared_error(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        weights=constant_op.constant(
            weights, shape=[2]))
    with self.cached_session():
      self.assertAlmostEqual(0.0, loss.eval(), 3)

  def testLossIsAssociativeAcrossBatchElements(self):
    with ops.Graph().as_default():
      random_seed.set_random_seed(0)

      height = 3
      width = 4
      shape = (1, height, width, 1)

      labels0 = random_ops.random_uniform(
          shape, minval=0, maxval=1, dtype=dtypes.float32)
      predictions0 = random_ops.random_uniform(
          shape, minval=0, maxval=1, dtype=dtypes.float32)

      labels1 = random_ops.random_uniform(
          shape, minval=0, maxval=1, dtype=dtypes.float32)
      predictions1 = random_ops.random_uniform(
          shape, minval=0, maxval=1, dtype=dtypes.float32)

      loss0 = loss_ops.mean_pairwise_squared_error(
          predictions=predictions0,
          labels=labels0)
      loss1 = loss_ops.mean_pairwise_squared_error(
          predictions=predictions1,
          labels=labels1)
      loss0_1 = loss_ops.mean_pairwise_squared_error(
          predictions=array_ops.concat([predictions0, predictions1], 0),
          labels=array_ops.concat([labels0, labels1], 0))

      with self.cached_session() as session:
        loss0, loss1, loss0_1 = session.run([loss0, loss1, loss0_1])

        self.assertTrue(loss0 > 0)
        self.assertTrue(loss1 > 0)
        self.assertAlmostEqual(loss0 + loss1, loss0_1, 5)


class CosineDistanceLossTest(test.TestCase):

  def setUp(self):
    self._predictions = np.asarray([
        [1, 0, 0],  # Batch 1
        [0, 0, -1],
        [1, 0, 0],  # Batch 2
        [1, 0, 0],
        [0, 0, -1],  # Batch 3
        [1, 0, 0]
    ]).reshape((3, 2, 3))

    self._labels = np.asarray([[1, 0, 0],
                               [0, 0, 1],
                               [0, 1, 0],
                               [1, 0, 0],
                               [0, 0, 1],
                               [0, 1, 0]]).reshape((3, 2, 3))

  def testValueErrorThrownWhenWeightIsNone(self):
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.cosine_distance(
            predictions=constant_op.constant(self._labels),
            labels=constant_op.constant(self._labels),
            dim=2,
            weights=None)

  def testAllCorrectNoWeights(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._labels),
        labels=constant_op.constant(self._labels),
        dim=2)
    with self.cached_session():
      self.assertAlmostEqual(0, loss.eval(), 5)

  def testPartiallyCorrectWithIntegerValues(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        dim=2)
    with self.cached_session():
      self.assertAlmostEqual(1, loss.eval(), 5)

  def testPartiallyCorrectFloatingPointValues(self):
    predictions = np.matrix(
        ('0.819031913261206 0.567041924552012 0.087465312324590;'
         '-0.665139432070255 -0.739487441769973 -0.103671883216994;'
         '0.707106781186548 -0.707106781186548 0'))
    labels = np.matrix(('0.819031913261206 0.567041924552012 0.087465312324590;'
                        '0.665139432070255 0.739487441769973 0.103671883216994;'
                        '0.707106781186548 0.707106781186548 0'))

    tf_preds = constant_op.constant(
        predictions, shape=(3, 1, 3), dtype=dtypes.float32)
    tf_labels = constant_op.constant(
        labels, shape=(3, 1, 3), dtype=dtypes.float32)
    loss = loss_ops.cosine_distance(tf_preds, tf_labels, dim=2)

    with self.cached_session():
      self.assertAlmostEqual(1.0, loss.eval(), 5)

  def testSampleSpecificWeights(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        dim=2,
        weights=constant_op.constant([1, 0, 0]))
    with self.cached_session():
      self.assertEqual(1.0, loss.eval())

  def testMeasurementSpecificWeights(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        dim=2,
        weights=constant_op.constant(
            [1, 0, 0, 1, 1, 1], shape=(3, 2)))
    with self.cached_session():
      self.assertEqual(3.0 / 4.0, loss.eval())

  def testValueErrorThrownWithShapelessPlaceholder(self):
    tf_predictions = array_ops.placeholder(dtypes.float32)
    with self.cached_session():
      with self.assertRaises(ValueError):
        loss_ops.cosine_distance(
            predictions=tf_predictions,
            labels=constant_op.constant(self._labels),
            dim=2,
            weights=constant_op.constant(
                [1, 0, 0, 1, 1, 1], shape=(3, 2)))

  def testMeasurementSpecificWeightsWithPlaceholderWithShape(self):
    tf_predictions = array_ops.placeholder(
        dtypes.float32, shape=self._labels.shape)
    loss = loss_ops.cosine_distance(
        predictions=tf_predictions,
        labels=constant_op.constant(self._labels),
        dim=2,
        weights=constant_op.constant(
            [1, 0, 0, 1, 1, 1], shape=(3, 2)))
    with self.cached_session() as sess:
      loss = sess.run(loss, feed_dict={tf_predictions: self._predictions})
      self.assertEqual(3.0 / 4.0, loss)

  def testZeroLossWhenAllSampleSpecificWeightsAreZero(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        dim=2,
        weights=array_ops.zeros((3,)))
    with self.cached_session():
      self.assertEqual(0, loss.eval())

  def testZeroLossWhenAllMeasurementSpecificWeightsAreZero(self):
    loss = loss_ops.cosine_distance(
        predictions=constant_op.constant(self._predictions),
        labels=constant_op.constant(self._labels),
        dim=2,
        weights=array_ops.zeros((3, 2)))
    with self.cached_session():
      self.assertEqual(0, loss.eval())


class ComputeWeightedLossTest(test.TestCase):

  def testHingeLoss(self):
    logits = constant_op.constant([1.2, 0.4, -1.0, -1.1])
    labels = constant_op.constant([1.0, 0.0, 0.0, 1.0])
    losses = loss_ops.hinge_loss(logits, labels)
    self.assertFalse(loss_ops.get_losses())
    loss = loss_ops.compute_weighted_loss(losses)
    self.assertTrue(loss_ops.get_losses())
    with self.cached_session():
      self.assertAllClose(losses.eval(), [0.0, 1.4, 0.0, 2.1], atol=1e-3)
      self.assertAllClose(loss.eval(), 3.5 / 4.0, atol=1e-3)


class AddLossTest(test.TestCase):

  def testAddExternalLoss(self):
    logits = constant_op.constant([[1.2, 0.4, -1.0, -1.1]])
    labels = constant_op.constant([[1.0, 0.0, 0.0, 1.0]])
    losses = loss_ops.hinge_loss(logits, labels)
    self.assertFalse(loss_ops.get_losses())
    loss_ops.add_loss(math_ops.reduce_mean(losses))
    self.assertTrue(loss_ops.get_losses())
    total_loss = loss_ops.get_total_loss()
    with self.cached_session():
      self.assertAllClose(losses.eval(), [[0.0, 1.4, 0.0, 2.1]], atol=1e-3)
      self.assertAllClose(total_loss.eval(), 3.5 / 4.0, atol=1e-3)

  def testNoneLossCollection(self):
    logits = constant_op.constant([[1.2, 0.4, -1.0, -1.1]])
    labels = constant_op.constant([[1.0, 0.0, 0.0, 1.0]])
    losses = loss_ops.hinge_loss(logits, labels)
    self.assertFalse(loss_ops.get_losses())
    loss_ops.add_loss(math_ops.reduce_mean(losses), loss_collection=None)
    self.assertFalse(loss_ops.get_losses())
    with self.cached_session():
      self.assertAllClose(losses.eval(), [[0.0, 1.4, 0.0, 2.1]], atol=1e-3)

  def testNoCollectLosses(self):
    logits = constant_op.constant([[1.2, 0.4, -1.0, -1.1]])
    labels = constant_op.constant([[1.0, 0.0, 0.0, 1.0]])
    self.assertFalse(loss_ops.get_losses())
    with arg_scope([loss_ops.add_loss], loss_collection=None):
      loss_ops.absolute_difference(logits, labels)
      loss_ops.log_loss(logits, labels)
      loss_ops.mean_squared_error(logits, labels)
      loss_ops.sigmoid_cross_entropy(logits, labels)
      loss_ops.softmax_cross_entropy(logits, labels)
    self.assertFalse(loss_ops.get_losses())

  def testNoCollectLossesBatch2(self):
    logits = constant_op.constant([[1.2, 0.4, -1.0, -1.1]] * 2)
    labels = constant_op.constant([[1.0, 0.0, 0.0, 1.0]] * 2)
    self.assertFalse(loss_ops.get_losses())
    with arg_scope([loss_ops.add_loss], loss_collection=None):
      loss_ops.absolute_difference(logits, labels)
      loss_ops.log_loss(logits, labels)
      loss_ops.mean_squared_error(logits, labels)
      loss_ops.sigmoid_cross_entropy(logits, labels)
      loss_ops.softmax_cross_entropy(logits, labels)
    self.assertFalse(loss_ops.get_losses())


if __name__ == '__main__':
  test.main()
