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
"""Tests for tensorflow.ctc_ops.ctc_decoder_ops."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import errors_impl
from tensorflow.python.framework import ops
from tensorflow.python.framework import random_seed
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.framework import test_util
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import ctc_ops
from tensorflow.python.ops import gradients_impl
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.ops import sparse_ops
from tensorflow.python.platform import test


def SimpleSparseTensorFrom(x):
  """Create a very simple SparseTensor with dimensions (batch, time).

  Args:
    x: a list of lists of type int

  Returns:
    x_ix and x_val, the indices and values of the SparseTensor<2>.
  """
  x_ix = []
  x_val = []
  for batch_i, batch in enumerate(x):
    for time, val in enumerate(batch):
      x_ix.append([batch_i, time])
      x_val.append(val)
  x_shape = [len(x), np.asarray(x_ix).max(0)[1] + 1]
  x_ix = constant_op.constant(x_ix, dtypes.int64)
  x_val = constant_op.constant(x_val, dtypes.int32)
  x_shape = constant_op.constant(x_shape, dtypes.int64)

  return sparse_tensor.SparseTensor(x_ix, x_val, x_shape)


def _ctc_loss_v2(labels, inputs, sequence_length,
                 preprocess_collapse_repeated=False,
                 ctc_merge_repeated=True,
                 ignore_longer_outputs_than_inputs=False,
                 time_major=True):
  """Call ctc_loss_v2 with v1 args."""
  assert not preprocess_collapse_repeated
  assert ctc_merge_repeated
  assert not ignore_longer_outputs_than_inputs
  return ctc_ops.ctc_loss_v2(
      labels=labels,
      logits=inputs,
      logit_length=sequence_length,
      label_length=None,
      blank_index=-1,
      logits_time_major=time_major)


class CTCLossTest(test.TestCase):

  def _testCTCLoss(self,
                   inputs,
                   seq_lens,
                   labels,
                   loss_truth,
                   grad_truth,
                   expected_err_re=None):
    self.assertEquals(len(inputs), len(grad_truth))

    inputs_t = constant_op.constant(inputs)

    with self.cached_session(use_gpu=False) as sess:
      loss = _ctc_loss_v2(
          inputs=inputs_t, labels=labels, sequence_length=seq_lens)
      grad = gradients_impl.gradients(loss, [inputs_t])[0]

      self.assertShapeEqual(loss_truth, loss)
      self.assertShapeEqual(grad_truth, grad)

      if expected_err_re is None:
        (tf_loss, tf_grad) = self.evaluate([loss, grad])
        self.assertAllClose(tf_loss, loss_truth, atol=1e-6)
        self.assertAllClose(tf_grad, grad_truth, atol=1e-6)
      else:
        with self.assertRaisesOpError(expected_err_re):
          self.evaluate([loss, grad])

  @test_util.run_v1_only("b/120545219")
  def testBasic(self):
    """Test two batch entries."""
    # Input and ground truth from Alex Graves' implementation.
    #
    #### Batch entry 0 #####
    # targets: 0 1 2 1 0
    # outputs:
    # 0 0.633766 0.221185 0.0917319 0.0129757 0.0142857 0.0260553
    # 1 0.111121 0.588392 0.278779 0.0055756 0.00569609 0.010436
    # 2 0.0357786 0.633813 0.321418 0.00249248 0.00272882 0.0037688
    # 3 0.0663296 0.643849 0.280111 0.00283995 0.0035545 0.00331533
    # 4 0.458235 0.396634 0.123377 0.00648837 0.00903441 0.00623107
    # alpha:
    # 0 -3.64753 -0.456075 -inf -inf -inf -inf -inf -inf -inf -inf -inf
    # 1 -inf -inf -inf -0.986437 -inf -inf -inf -inf -inf -inf -inf
    # 2 -inf -inf -inf -inf -inf -2.12145 -inf -inf -inf -inf -inf
    # 3 -inf -inf -inf -inf -inf -inf -inf -2.56174 -inf -inf -inf
    # 4 -inf -inf -inf -inf -inf -inf -inf -inf -inf -3.34211 -inf
    # beta:
    # 0 -inf -2.88604 -inf -inf -inf -inf -inf -inf -inf -inf -inf
    # 1 -inf -inf -inf -2.35568 -inf -inf -inf -inf -inf -inf -inf
    # 2 -inf -inf -inf -inf -inf -1.22066 -inf -inf -inf -inf -inf
    # 3 -inf -inf -inf -inf -inf -inf -inf -0.780373 -inf -inf -inf
    # 4 -inf -inf -inf -inf -inf -inf -inf -inf -inf 0 0
    # prob: -3.34211
    # outputDerivs:
    # 0 -0.366234 0.221185 0.0917319 0.0129757 0.0142857 0.0260553
    # 1 0.111121 -0.411608 0.278779 0.0055756 0.00569609 0.010436
    # 2 0.0357786 0.633813 -0.678582 0.00249248 0.00272882 0.0037688
    # 3 0.0663296 -0.356151 0.280111 0.00283995 0.0035545 0.00331533
    # 4 -0.541765 0.396634 0.123377 0.00648837 0.00903441 0.00623107
    #
    #### Batch entry 1 #####
    #
    # targets: 0 1 1 0
    # outputs:
    # 0 0.30176 0.28562 0.0831517 0.0862751 0.0816851 0.161508
    # 1 0.24082 0.397533 0.0557226 0.0546814 0.0557528 0.19549
    # 2 0.230246 0.450868 0.0389607 0.038309 0.0391602 0.202456
    # 3 0.280884 0.429522 0.0326593 0.0339046 0.0326856 0.190345
    # 4 0.423286 0.315517 0.0338439 0.0393744 0.0339315 0.154046
    # alpha:
    # 0 -1.8232 -1.19812 -inf -inf -inf -inf -inf -inf -inf
    # 1 -inf -2.19315 -2.83037 -2.1206 -inf -inf -inf -inf -inf
    # 2 -inf -inf -inf -2.03268 -3.71783 -inf -inf -inf -inf
    # 3 -inf -inf -inf -inf -inf -4.56292 -inf -inf -inf
    # 4 -inf -inf -inf -inf -inf -inf -inf -5.42262 -inf
    # beta:
    # 0 -inf -4.2245 -inf -inf -inf -inf -inf -inf -inf
    # 1 -inf -inf -inf -3.30202 -inf -inf -inf -inf -inf
    # 2 -inf -inf -inf -inf -1.70479 -0.856738 -inf -inf -inf
    # 3 -inf -inf -inf -inf -inf -0.859706 -0.859706 -0.549337 -inf
    # 4 -inf -inf -inf -inf -inf -inf -inf 0 0
    # prob: -5.42262
    # outputDerivs:
    # 0 -0.69824 0.28562 0.0831517 0.0862751 0.0816851 0.161508
    # 1 0.24082 -0.602467 0.0557226 0.0546814 0.0557528 0.19549
    # 2 0.230246 0.450868 0.0389607 0.038309 0.0391602 -0.797544
    # 3 0.280884 -0.570478 0.0326593 0.0339046 0.0326856 0.190345
    # 4 -0.576714 0.315517 0.0338439 0.0393744 0.0339315 0.154046

    # max_time_steps == 7
    depth = 6

    # seq_len_0 == 5
    targets_0 = [0, 1, 2, 1, 0]
    loss_log_prob_0 = -3.34211
    # dimensions are time x depth
    input_prob_matrix_0 = np.asarray(
        [[0.633766, 0.221185, 0.0917319, 0.0129757, 0.0142857, 0.0260553],
         [0.111121, 0.588392, 0.278779, 0.0055756, 0.00569609, 0.010436],
         [0.0357786, 0.633813, 0.321418, 0.00249248, 0.00272882, 0.0037688],
         [0.0663296, 0.643849, 0.280111, 0.00283995, 0.0035545, 0.00331533],
         [0.458235, 0.396634, 0.123377, 0.00648837, 0.00903441, 0.00623107]],
        dtype=np.float32)
    input_log_prob_matrix_0 = np.log(input_prob_matrix_0)
    gradient_log_prob_0 = np.asarray(
        [[-0.366234, 0.221185, 0.0917319, 0.0129757, 0.0142857, 0.0260553],
         [0.111121, -0.411608, 0.278779, 0.0055756, 0.00569609, 0.010436],
         [0.0357786, 0.633813, -0.678582, 0.00249248, 0.00272882, 0.0037688],
         [0.0663296, -0.356151, 0.280111, 0.00283995, 0.0035545, 0.00331533],
         [-0.541765, 0.396634, 0.123377, 0.00648837, 0.00903441, 0.00623107]],
        dtype=np.float32)

    # seq_len_1 == 5
    targets_1 = [0, 1, 1, 0]
    loss_log_prob_1 = -5.42262
    # dimensions are time x depth

    input_prob_matrix_1 = np.asarray(
        [[0.30176, 0.28562, 0.0831517, 0.0862751, 0.0816851, 0.161508],
         [0.24082, 0.397533, 0.0557226, 0.0546814, 0.0557528, 0.19549],
         [0.230246, 0.450868, 0.0389607, 0.038309, 0.0391602, 0.202456],
         [0.280884, 0.429522, 0.0326593, 0.0339046, 0.0326856, 0.190345],
         [0.423286, 0.315517, 0.0338439, 0.0393744, 0.0339315, 0.154046]],
        dtype=np.float32)
    input_log_prob_matrix_1 = np.log(input_prob_matrix_1)
    gradient_log_prob_1 = np.asarray(
        [[-0.69824, 0.28562, 0.0831517, 0.0862751, 0.0816851, 0.161508],
         [0.24082, -0.602467, 0.0557226, 0.0546814, 0.0557528, 0.19549],
         [0.230246, 0.450868, 0.0389607, 0.038309, 0.0391602, -0.797544],
         [0.280884, -0.570478, 0.0326593, 0.0339046, 0.0326856, 0.190345],
         [-0.576714, 0.315517, 0.0338439, 0.0393744, 0.0339315, 0.154046]],
        dtype=np.float32)

    # len max_time_steps array of 2 x depth matrices
    inputs = [
        np.vstack(
            [input_log_prob_matrix_0[t, :], input_log_prob_matrix_1[t, :]])
        for t in range(5)
    ] + 2 * [np.nan * np.ones((2, depth), np.float32)]

    # convert inputs into [max_time x batch_size x depth tensor] Tensor
    inputs = np.asarray(inputs, dtype=np.float32)

    # len batch_size array of label vectors
    labels = SimpleSparseTensorFrom([targets_0, targets_1])

    # batch_size length vector of sequence_lengths
    seq_lens = np.array([5, 5], dtype=np.int32)

    # output: batch_size length vector of negative log probabilities
    loss_truth = np.array([-loss_log_prob_0, -loss_log_prob_1], np.float32)

    # output: len max_time_steps array of 2 x depth matrices
    grad_truth = [
        np.vstack([gradient_log_prob_0[t, :], gradient_log_prob_1[t, :]])
        for t in range(5)
    ] + 2 * [np.zeros((2, depth), np.float32)]

    # convert grad_truth into [max_time x batch_size x depth] Tensor
    grad_truth = np.asarray(grad_truth, dtype=np.float32)

    self._testCTCLoss(inputs, seq_lens, labels, loss_truth, grad_truth)

  def test_time_major(self):
    """Testing time_major param.


    testing if transposing and setting time_major=False will result in the same
    loss
    """
    # [max_time x batch_size x depth tensor]
    inputs = np.random.randn(2, 2, 3).astype(np.float32)
    labels = SimpleSparseTensorFrom([[0, 1], [1, 0]])
    seq_lens = np.array([2, 2], dtype=np.int32)

    inputs_t = constant_op.constant(inputs)

    # Transposing tensor to [batch_size x max_time x depth tensor]
    inputs_t_transposed = constant_op.constant(inputs.transpose(1, 0, 2))

    with self.session(use_gpu=False) as sess:
      loss = _ctc_loss_v2(
          inputs=inputs_t, labels=labels, sequence_length=seq_lens)
      loss_transposed = _ctc_loss_v2(
          inputs=inputs_t_transposed,
          labels=labels,
          sequence_length=seq_lens,
          time_major=False)

      (tf_loss, tf_loss_transposed) = self.evaluate([loss, loss_transposed])
      self.assertAllEqual(tf_loss, tf_loss_transposed)

  @test_util.run_v1_only("b/120545219")
  def testInvalidSecondGradient(self):
    inputs = np.random.randn(2, 2, 3).astype(np.float32)
    inputs_t = constant_op.constant(inputs)
    labels = SimpleSparseTensorFrom([[0, 1], [1, 0]])
    seq_lens = np.array([2, 2], dtype=np.int32)
    v = [1.0]

    with self.session(use_gpu=False):
      loss = _ctc_loss_v2(
          inputs=inputs_t, labels=labels, sequence_length=seq_lens)
      # Taking ths second gradient should fail, since it is not
      # yet supported.
      with self.assertRaisesRegexp(LookupError,
                                   "explicitly disabled"):
        _ = gradients_impl._hessian_vector_product(loss, [inputs_t], v)

  @test_util.run_v1_only("b/120545219")
  def testEmptyBatch(self):
    inputs = constant_op.constant([], dtype=dtypes.float32, shape=(1, 0, 2))
    sequence_lengths = constant_op.constant([], dtype=dtypes.int32)
    labels = sparse_tensor.SparseTensor(
        indices=constant_op.constant([], shape=(0, 2), dtype=dtypes.int64),
        values=constant_op.constant([], shape=(0,), dtype=dtypes.int32),
        dense_shape=[5, 5])

    with self.session(use_gpu=False) as sess:
      with self.assertRaisesRegexp(errors_impl.InvalidArgumentError,
                                   "batch_size must not be 0"):
        sess.run(_ctc_loss_v2(labels, inputs, sequence_lengths))


class CTCLossTestV2(test.TestCase):

  @test_util.run_v1_only("b/120545219")
  def testCtcLossV2(self):
    random_seed.set_random_seed(5)

    batch_size = 8
    num_labels = 6
    max_label_length = 5
    num_frames = 12

    labels = random_ops.random_uniform(
        [batch_size, max_label_length], minval=1, maxval=num_labels,
        dtype=dtypes.int64)
    logits = random_ops.random_uniform([num_frames, batch_size, num_labels])

    label_length = random_ops.random_uniform(
        [batch_size], minval=2, maxval=max_label_length, dtype=dtypes.int64)
    label_mask = array_ops.sequence_mask(
        label_length, maxlen=max_label_length, dtype=label_length.dtype)
    labels *= label_mask
    logit_length = [num_frames] * batch_size

    ref_loss = ctc_ops.ctc_loss_v2(
        labels=labels,
        logits=logits,
        label_length=label_length,
        logit_length=logit_length)
    ref_grad = gradients_impl.gradients(ref_loss, [logits])

    sparse_labels = ctc_ops.dense_labels_to_sparse(labels, label_length)

    def assert_same_loss_and_grads(loss):
      with self.cached_session() as sess:
        self.assertAllClose(*self.evaluate([loss, ref_loss]))
        grad = gradients_impl.gradients(loss, [logits])
        self.assertAllClose(
            *self.evaluate([grad, ref_grad]), rtol=2e-06, atol=2e-06)

    assert_same_loss_and_grads(
        ctc_ops.ctc_loss_v2(
            labels=sparse_labels,
            logits=logits,
            label_length=label_length,
            logit_length=logit_length,
            blank_index=0))

  @test_util.run_v1_only("b/120545219")
  def testCtcLossDenseIsSameAsCtcLoss(self):
    with ops.device("/GPU:0" if test.is_gpu_available() else "/CPU:0"):
      random_seed.set_random_seed(5)

      batch_size = 8
      num_labels = 6
      label_length = 5
      num_frames = 12
      logits = random_ops.random_uniform([num_frames, batch_size, num_labels])
      labels = random_ops.random_uniform(
          [batch_size, label_length], minval=1, maxval=num_labels,
          dtype=dtypes.int64)

      label_lengths = random_ops.random_uniform(
          [batch_size], minval=2, maxval=label_length, dtype=dtypes.int64)
      label_mask = array_ops.sequence_mask(
          label_lengths, maxlen=label_length, dtype=label_lengths.dtype)
      labels *= label_mask

      logit_lengths = [num_frames] * batch_size

      ctc_loss = ctc_ops.ctc_loss_dense(
          labels=labels,
          logits=logits,
          label_length=label_lengths,
          logit_length=logit_lengths)
      ctc_loss_grads = gradients_impl.gradients(ctc_loss, [logits])[0]

      # Shift labels down by one (move blank from 0 to num_labels -1)
      tf_ctc_loss_labels = math_ops.cast(labels, dtypes.int32) - 1
      tf_nn_ctc_logits = array_ops.concat([
          logits[:, :, 1:],
          logits[:, :, 0:1],
      ], axis=2)

      tf_ctc_loss_labels = ctc_ops.dense_labels_to_sparse(
          tf_ctc_loss_labels, label_lengths)

      tf_nn_ctc_loss = ctc_ops.ctc_loss(
          labels=tf_ctc_loss_labels,
          inputs=tf_nn_ctc_logits,
          sequence_length=logit_lengths,
          time_major=True)
      tf_nn_ctc_grads = gradients_impl.gradients(tf_nn_ctc_loss, [logits])[0]

      with self.cached_session() as sess:
        for _ in range(32):
          self.assertAllClose(*self.evaluate([ctc_loss, tf_nn_ctc_loss]))
          self.assertAllClose(
              *self.evaluate([ctc_loss_grads, tf_nn_ctc_grads]),
              rtol=2e-06,
              atol=2e-06)

  @test_util.run_v1_only("b/120545219")
  def testCtcLossDenseUniqueFastPathIsSameAsCtcLoss(self):
    random_seed.set_random_seed(5)

    batch_size = 8
    num_labels = 6
    label_length = 5
    num_frames = 12
    logits = random_ops.random_uniform([num_frames, batch_size, num_labels])
    labels = random_ops.random_uniform(
        [batch_size, label_length], minval=1, maxval=num_labels,
        dtype=dtypes.int64)

    label_lengths = random_ops.random_uniform(
        [batch_size], minval=2, maxval=label_length, dtype=dtypes.int64)
    label_mask = array_ops.sequence_mask(
        label_lengths, maxlen=label_length, dtype=label_lengths.dtype)
    labels *= label_mask

    logit_lengths = [num_frames] * batch_size

    ctc_loss = ctc_ops.ctc_loss_dense(
        labels=labels,
        logits=logits,
        label_length=label_lengths,
        logit_length=logit_lengths,
        unique=ctc_ops.ctc_unique_labels(labels))
    ctc_loss_grads = gradients_impl.gradients(ctc_loss, [logits])[0]

    # Shift labels down by one (move blank from 0 to num_labels -1)
    tf_ctc_loss_labels = math_ops.cast(labels, dtypes.int32) - 1
    tf_nn_ctc_logits = array_ops.concat([
        logits[:, :, 1:],
        logits[:, :, 0:1],
    ], axis=2)

    tf_ctc_loss_labels = ctc_ops.dense_labels_to_sparse(
        tf_ctc_loss_labels, label_lengths)

    tf_nn_ctc_loss = ctc_ops.ctc_loss(
        labels=tf_ctc_loss_labels,
        inputs=tf_nn_ctc_logits,
        sequence_length=logit_lengths,
        time_major=True)
    tf_nn_ctc_grads = gradients_impl.gradients(tf_nn_ctc_loss, [logits])[0]

    with self.cached_session() as sess:
      for _ in range(32):
        self.assertAllClose(*self.evaluate([ctc_loss, tf_nn_ctc_loss]))
        self.assertAllClose(
            *self.evaluate([ctc_loss_grads, tf_nn_ctc_grads]),
            rtol=2e-06,
            atol=2e-06)

  @test_util.run_v1_only("b/120545219")
  def testCtcLossDenseWithBlankIndexIsSameAsCtcLoss(self):
    random_seed.set_random_seed(5)

    batch_size = 8
    num_labels = 6
    label_length = 5
    num_frames = 12
    logits = random_ops.random_uniform([num_frames, batch_size, num_labels])
    labels = random_ops.random_uniform(
        [batch_size, label_length], minval=0, maxval=num_labels-1,
        dtype=dtypes.int64)

    label_lengths = random_ops.random_uniform(
        [batch_size], minval=2, maxval=label_length, dtype=dtypes.int64)
    label_mask = array_ops.sequence_mask(
        label_lengths, maxlen=label_length, dtype=label_lengths.dtype)
    labels *= label_mask

    logit_lengths = [num_frames] * batch_size

    tf_ctc_loss_labels = math_ops.cast(labels, dtypes.int32)
    tf_ctc_loss_labels = ctc_ops.dense_labels_to_sparse(
        tf_ctc_loss_labels, label_lengths)

    tf_nn_ctc_loss = ctc_ops.ctc_loss(
        labels=tf_ctc_loss_labels,
        inputs=logits,
        sequence_length=logit_lengths,
        time_major=True)
    tf_nn_ctc_grads = gradients_impl.gradients(tf_nn_ctc_loss, [logits])[0]

    # Shift the blank logits/labels to be somewhere in the middle.
    blank_index = 2
    shifted_logits = array_ops.concat([
        logits[:, :, :blank_index],
        logits[:, :, -1:],
        logits[:, :, blank_index:-1],
    ], axis=2)
    shifted_labels = array_ops.where(labels < blank_index, labels, labels + 1)

    ctc_loss = ctc_ops.ctc_loss_dense(
        labels=shifted_labels,
        logits=shifted_logits,
        label_length=label_lengths,
        logit_length=logit_lengths,
        blank_index=blank_index)
    ctc_loss_grads = gradients_impl.gradients(ctc_loss, [logits])[0]

    with self.cached_session() as sess:
      for _ in range(32):
        self.assertAllClose(*self.evaluate([ctc_loss, tf_nn_ctc_loss]))
        self.assertAllClose(
            *self.evaluate([ctc_loss_grads, tf_nn_ctc_grads]),
            rtol=2e-06,
            atol=2e-06)

  @test_util.run_v1_only("b/120545219")
  def testCtcLossDenseWithNegativeBlankIndexIsSameAsCtcLoss(self):
    with ops.device("/GPU:0" if test.is_gpu_available() else "/CPU:0"):
      random_seed.set_random_seed(5)

      batch_size = 8
      num_labels = 6
      label_length = 5
      num_frames = 12
      logits = random_ops.random_uniform([num_frames, batch_size, num_labels])
      labels = random_ops.random_uniform(
          [batch_size, label_length], minval=0, maxval=num_labels-1,
          dtype=dtypes.int64)

      label_lengths = random_ops.random_uniform(
          [batch_size], minval=2, maxval=label_length, dtype=dtypes.int64)
      label_mask = array_ops.sequence_mask(
          label_lengths, maxlen=label_length, dtype=label_lengths.dtype)
      labels *= label_mask

      logit_lengths = [num_frames] * batch_size

      ctc_loss = ctc_ops.ctc_loss_dense(
          labels=labels,
          logits=logits,
          label_length=label_lengths,
          logit_length=logit_lengths,
          blank_index=-1)
      ctc_loss_grads = gradients_impl.gradients(ctc_loss, [logits])[0]

      tf_ctc_loss_labels = math_ops.cast(labels, dtypes.int32)
      tf_ctc_loss_labels = ctc_ops.dense_labels_to_sparse(
          tf_ctc_loss_labels, label_lengths)

      tf_nn_ctc_loss = ctc_ops.ctc_loss(
          labels=tf_ctc_loss_labels,
          inputs=logits,
          sequence_length=logit_lengths,
          time_major=True)
      tf_nn_ctc_grads = gradients_impl.gradients(tf_nn_ctc_loss, [logits])[0]

      with self.cached_session() as sess:
        for _ in range(32):
          self.assertAllClose(*self.evaluate([ctc_loss, tf_nn_ctc_loss]))
          self.assertAllClose(
              *self.evaluate([ctc_loss_grads, tf_nn_ctc_grads]),
              rtol=2e-06,
              atol=2e-06)

  def testCollapseRepeated(self):
    collapsed, new_seq_lengths = ctc_ops.collapse_repeated(
        labels=[[1, 3, 3, 3, 0],
                [1, 4, 4, 4, 0],
                [4, 2, 2, 9, 4]],
        seq_length=[4, 5, 5])
    self.assertAllEqual(new_seq_lengths, [2, 3, 4])
    self.assertAllEqual(
        collapsed,
        [[1, 3, 0, 0],
         [1, 4, 0, 0],
         [4, 2, 9, 4]])

  def testCollapseRepeatedPreservesDtypes(self):
    collapsed, new_seq_lengths = ctc_ops.collapse_repeated(
        labels=constant_op.constant(
            [[1, 3, 3, 3, 0],
             [1, 4, 4, 4, 0],
             [4, 2, 2, 9, 4]],
            dtype=dtypes.int64),
        seq_length=constant_op.constant([4, 5, 5], dtype=dtypes.int64))
    self.assertEqual(new_seq_lengths.dtype, dtypes.int64)
    self.assertEqual(collapsed.dtype, dtypes.int64)
    self.assertAllEqual(new_seq_lengths, [2, 3, 4])
    self.assertAllEqual(
        collapsed,
        [[1, 3, 0, 0],
         [1, 4, 0, 0],
         [4, 2, 9, 4]])

  def testCollapseRepeatedExtraPadding(self):
    collapsed, new_seq_lengths = ctc_ops.collapse_repeated(
        labels=[[1, 3, 3, 3, 0, 0, 0],
                [1, 4, 4, 4, 0, 1, 2],
                [4, 2, 2, 9, 4, 0, 0]],
        seq_length=[4, 5, 5])
    self.assertAllEqual(new_seq_lengths, [2, 3, 4])
    self.assertAllEqual(
        collapsed,
        [[1, 3, 0, 0],
         [1, 4, 0, 0],
         [4, 2, 9, 4]])

  def testCollapseRepeatedFrontRepeats(self):
    collapsed, new_seq_lengths = ctc_ops.collapse_repeated(
        labels=[[1, 1, 1, 2, 2],
                [1, 1, 1, 2, 2],
                [1, 1, 1, 2, 2]],
        seq_length=[5, 4, 3])
    self.assertAllEqual(new_seq_lengths, [2, 2, 1])
    self.assertAllEqual(
        collapsed,
        [[1, 2],
         [1, 2],
         [1, 0]])

  def testCollapseRepeatedAllLabelsTheSame(self):
    collapsed, new_seq_lengths = ctc_ops.collapse_repeated(
        labels=[[1, 1, 1, 1, 1],
                [1, 1, 1, 1, 1],
                [1, 1, 1, 1, 1]],
        seq_length=[4, 5, 1])
    self.assertAllEqual(new_seq_lengths, [1, 1, 1])
    self.assertAllEqual(
        collapsed,
        [[1],
         [1],
         [1]])

  def testDenseSequencesToSparse(self):
    labels = [[1, 3, 3, 3, 0],
              [1, 4, 4, 4, 0],
              [4, 2, 2, 9, 4]]
    length = [4, 5, 5]
    sparse = ctc_ops.dense_labels_to_sparse(labels, length)
    new_dense = sparse_ops.sparse_tensor_to_dense(sparse)

    self.assertAllEqual(labels, new_dense)

    padded_labels = [[1, 3, 3, 3, 0, 0, 0, 0],
                     [1, 4, 4, 4, 0, 0, 0, 0],
                     [4, 2, 2, 9, 4, 0, 0, 0]]
    length = [4, 5, 5]
    sparse = ctc_ops.dense_labels_to_sparse(padded_labels, length)
    padded_dense = sparse_ops.sparse_tensor_to_dense(sparse)

    self.assertAllEqual(padded_dense, new_dense)

  def testUnique(self):
    labels = [
        [3, 4, 4, 3],
        [1, 1, 1, 0],
    ]
    unique, idx = ctc_ops.ctc_unique_labels(labels)
    self.assertAllEqual([
        [3, 4, 0, 0],
        [1, 0, 0, 0],
    ], unique)
    self.assertAllEqual([
        [0, 1, 1, 0],
        [0, 0, 0, 1],
    ], idx)

  def testSumStates(self):
    idx = [
        [0, 1, 0, 1],
        [0, 0, 0, 1],
    ]
    states = math_ops.log([
        [[1.0, 2.0, 3.0, 4.0],
         [5.0, 6.0, 7.0, 8.0]],
        [[0.1, 0.2, 0.3, 0.4],
         [0.5, 0.6, 0.7, 0.8]],
    ])
    sum_of_states = math_ops.exp(ctc_ops._sum_states(idx, states))
    self.assertAllClose([
        [[4.0, 6.0, 0.0, 0.0],
         [18.0, 8.0, 0.0, 0.0]],
        [[0.4, 0.6, 0.0, 0.0],
         [1.8, 0.8, 0.0, 0.0]]
    ], sum_of_states)

  def testStateToOlabel(self):
    labels = [
        [3, 4, 3, 4],
        [1, 1, 1, 0],
    ]
    num_labels = 8

    # 3 frames, 2 batch, 10 states (5 label, 5 blank).
    states = [
        [[0.11, 0.12, 0.13, 0.14, 0.15, 0.16, 0.17, 0.18, 0.19, 0.20],
         [0.21, 0.22, 0.23, 0.24, 0.25, 0.26, 0.27, 0.28, 0.29, 0.30]],
        [[1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2.0],
         [2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 3.0]],
        [[11.0, 12.0, 13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0, 20.0],
         [21.0, 22.0, 23.0, 24.0, 25.0, 26.0, 27.0, 28.0, 29.0, 30.0]],
    ]
    labels = ops.convert_to_tensor(labels)
    states = math_ops.log(states)
    olabel = ctc_ops._state_to_olabel(labels, num_labels, states)
    olabel = math_ops.exp(olabel)
    blank = olabel[:, :, 0]
    self.assertAllClose(blank, [
        [0.16 + 0.17 + 0.18 + 0.19 + 0.20,
         0.26 + 0.27 + 0.28 + 0.29 + 0.30],
        [1.6 + 1.7 + 1.8 + 1.9 + 2.0,
         2.6 + 2.7 + 2.8 + 2.9 + 3.0],
        [16.0 + 17.0 + 18.0 + 19.0 + 20.0,
         26.0 + 27.0 + 28.0 + 29.0 + 30.0]
    ])
    self.assertAllClose(olabel[:, :, 1:], [
        [[0.0, 0.0, 0.12 + 0.14, 0.13 + 0.15, 0.0, 0.0, 0.0],
         [0.22 + 0.23 + 0.24, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
        [[0.0, 0.0, 1.2 + 1.4, 1.3 + 1.5, 0.0, 0.0, 0.0],
         [2.2 + 2.3 + 2.4, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
        [[0.0, 0.0, 12.0 + 14.0, 13.0 + 15.0, 0.0, 0.0, 0.0],
         [22.0 + 23.0 + 24.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
    ])

  def testStateToOlabelUnique(self):
    labels = [
        [3, 4, 3, 4],
        [1, 1, 1, 0],
    ]
    num_labels = 8

    # 3 frames, 2 batch, 10 states (5 label, 5 blank).
    states = [
        [[0.11, 0.12, 0.13, 0.14, 0.15, 0.16, 0.17, 0.18, 0.19, 0.20],
         [0.21, 0.22, 0.23, 0.24, 0.25, 0.26, 0.27, 0.28, 0.29, 0.30]],
        [[1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8, 1.9, 2.0],
         [2.1, 2.2, 2.3, 2.4, 2.5, 2.6, 2.7, 2.8, 2.9, 3.0]],
        [[11.0, 12.0, 13.0, 14.0, 15.0, 16.0, 17.0, 18.0, 19.0, 20.0],
         [21.0, 22.0, 23.0, 24.0, 25.0, 26.0, 27.0, 28.0, 29.0, 30.0]],
    ]
    labels = ops.convert_to_tensor(labels)
    states = math_ops.log(states)
    olabel = ctc_ops._state_to_olabel_unique(
        labels, num_labels, states, ctc_ops.ctc_unique_labels(labels))
    olabel = math_ops.exp(olabel)
    blank = olabel[:, :, 0]
    self.assertAllClose(blank, [
        [0.16 + 0.17 + 0.18 + 0.19 + 0.20,
         0.26 + 0.27 + 0.28 + 0.29 + 0.30],
        [1.6 + 1.7 + 1.8 + 1.9 + 2.0,
         2.6 + 2.7 + 2.8 + 2.9 + 3.0],
        [16.0 + 17.0 + 18.0 + 19.0 + 20.0,
         26.0 + 27.0 + 28.0 + 29.0 + 30.0]])
    self.assertAllClose(olabel[:, :, 1:], [
        [[0.0, 0.0, 0.12 + 0.14, 0.13 + 0.15, 0.0, 0.0, 0.0],
         [0.22 + 0.23 + 0.24, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
        [[0.0, 0.0, 1.2 + 1.4, 1.3 + 1.5, 0.0, 0.0, 0.0],
         [2.2 + 2.3 + 2.4, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
        [[0.0, 0.0, 12.0 + 14.0, 13.0 + 15.0, 0.0, 0.0, 0.0],
         [22.0 + 23.0 + 24.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0]],
    ])

  @test_util.run_deprecated_v1
  def testScan(self):
    with ops.device("/GPU:0" if test.is_gpu_available() else "/CPU:0"):
      out = ctc_ops._scan(
          lambda accum, elem: accum + elem,
          constant_op.constant([1.0, 2.0, 3.0]), 23.0)
      self.assertAllEqual([24.0, 26.0, 29.0], out)

      out = ctc_ops._scan(
          lambda a, e: a + e,
          constant_op.constant([1.0, 2.0, 3.0]), 23.0,
          inclusive=True)
      self.assertAllEqual([23.0, 24.0, 26.0, 29.0], out)

      out = ctc_ops._scan(
          lambda a, e: a + e,
          constant_op.constant([1.0, 2.0, 3.0]), 23.0,
          reverse=True)
      self.assertAllEqual([29.0, 28.0, 26.0], out)

      out = ctc_ops._scan(
          lambda a, e: a + e,
          constant_op.constant([1.0, 2.0, 3.0]), 23.0,
          reverse=True,
          inclusive=True)
      self.assertAllEqual([29.0, 28.0, 26.0, 23.0], out)

      out = ctc_ops._scan(
          lambda a, e: a + e,
          constant_op.constant([[0.0, 1.0], [2.0, 3.0], [4.0, 5.0]]),
          constant_op.constant([23.0, 24.0]))
      self.assertAllEqual([[23.0, 25.0], [25.0, 28.0], [29.0, 33.0]], out)

  @test_util.run_deprecated_v1
  def testScanCapturesVariables(self):
    with self.cached_session() as sess:
      x = random_ops.random_uniform([])
      fn = lambda accum, elem: accum + x * elem
      out = ctc_ops._scan(fn, constant_op.constant([0.0, 1.0, 2.0]), 23.0)
      self.assertAllClose(*sess.run([
          [23.0 + x * 0.0, 23.0 + x * 1.0, 23.0 + x * 3.0], out
      ]))

  @test_util.run_deprecated_v1
  def testScanMultipleAccumulators(self):
    with ops.device("/GPU:0" if test.is_gpu_available() else "/CPU:0"):
      def fn(accum, elem):
        accum_a, accum_b = accum
        return accum_a + elem, accum_b * elem
      out = ctc_ops._scan(
          fn, constant_op.constant([1.0, 2.0, 3.0]),
          (23.0, constant_op.constant([1.0, 2.0])))
      a, b = out
      self.assertAllEqual([24.0, 26.0, 29.0], a)
      self.assertAllEqual([[1.0, 2.0], [2.0, 4.0], [6.0, 12.0]], b)

  @test_util.run_deprecated_v1
  def testScanMultipleElements(self):
    with ops.device("/GPU:0" if test.is_gpu_available() else "/CPU:0"):
      def fn(accum, elem):
        elem_a, elem_b = elem
        return accum + (elem_a * elem_b)
      elems_a = constant_op.constant([1.0, 2.0, 3.0])
      elems_b = constant_op.constant([[1.0, 2.0], [2.0, 3.0], [3.0, 4.0]])
      out = ctc_ops._scan(
          fn, (elems_a, elems_b),
          initial=constant_op.constant([0.0, 0.0]))
      self.assertAllEqual(
          [[1.0, 2.0], [5.0, 8.0], [14.0, 20.0]], out)

if __name__ == "__main__":
  test.main()
