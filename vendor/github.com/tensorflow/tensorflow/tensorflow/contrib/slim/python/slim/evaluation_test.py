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
"""Tests for slim.evaluation."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import glob
import os
import shutil
import time

import numpy as np

from tensorflow.contrib.framework.python.ops import variables as variables_lib
from tensorflow.contrib.slim.python.slim import evaluation
from tensorflow.contrib.training.python.training import evaluation as evaluation_lib
from tensorflow.core.protobuf import saver_pb2
from tensorflow.python.debug.lib import debug_data
from tensorflow.python.debug.wrappers import hooks
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import metrics
from tensorflow.python.ops import variables
from tensorflow.python.platform import flags
from tensorflow.python.platform import gfile
from tensorflow.python.platform import test
from tensorflow.python.summary import summary_iterator
from tensorflow.python.training import input  # pylint: disable=redefined-builtin
from tensorflow.python.training import saver as saver_lib
from tensorflow.python.training import session_run_hook


FLAGS = flags.FLAGS


def GenerateTestData(num_classes, batch_size):
  inputs = np.random.rand(batch_size, num_classes)

  np.random.seed(0)
  labels = np.random.randint(low=0, high=num_classes, size=batch_size)
  labels = labels.reshape((batch_size,))
  return inputs, labels


def TestModel(inputs):
  scale = variables.Variable(1.0, trainable=False)

  # Scaling the outputs wont change the result...
  outputs = math_ops.multiply(inputs, scale)
  return math_ops.argmax(outputs, 1), scale


def GroundTruthAccuracy(inputs, labels, batch_size):
  predictions = np.argmax(inputs, 1)
  num_correct = np.sum(predictions == labels)
  return float(num_correct) / batch_size


class EvaluationTest(test.TestCase):

  def setUp(self):
    super(EvaluationTest, self).setUp()

    num_classes = 8
    batch_size = 16
    inputs, labels = GenerateTestData(num_classes, batch_size)
    self._expected_accuracy = GroundTruthAccuracy(inputs, labels, batch_size)

    self._global_step = variables_lib.get_or_create_global_step()
    self._inputs = constant_op.constant(inputs, dtype=dtypes.float32)
    self._labels = constant_op.constant(labels, dtype=dtypes.int64)
    self._predictions, self._scale = TestModel(self._inputs)

  def testFinalOpsOnEvaluationLoop(self):
    value_op, update_op = metrics.accuracy(
        labels=self._labels, predictions=self._predictions)
    init_op = control_flow_ops.group(variables.global_variables_initializer(),
                                     variables.local_variables_initializer())
    # Create checkpoint and log directories:
    chkpt_dir = os.path.join(self.get_temp_dir(), 'tmp_logs/')
    gfile.MakeDirs(chkpt_dir)
    logdir = os.path.join(self.get_temp_dir(), 'tmp_logs2/')
    gfile.MakeDirs(logdir)

    # Save initialized variables to a checkpoint directory:
    saver = saver_lib.Saver()
    with self.cached_session() as sess:
      init_op.run()
      saver.save(sess, os.path.join(chkpt_dir, 'chkpt'))

    class Object(object):

      def __init__(self):
        self.hook_was_run = False

    obj = Object()

    # Create a custom session run hook.
    class CustomHook(session_run_hook.SessionRunHook):

      def __init__(self, obj):
        self.obj = obj

      def end(self, session):
        self.obj.hook_was_run = True

    # Now, run the evaluation loop:
    accuracy_value = evaluation.evaluation_loop(
        '',
        chkpt_dir,
        logdir,
        eval_op=update_op,
        final_op=value_op,
        hooks=[CustomHook(obj)],
        max_number_of_evaluations=1)
    self.assertAlmostEqual(accuracy_value, self._expected_accuracy)

    # Validate that custom hook ran.
    self.assertTrue(obj.hook_was_run)

  def _create_names_to_metrics(self, predictions, labels):
    accuracy0, update_op0 = metrics.accuracy(
        labels=labels, predictions=predictions)
    accuracy1, update_op1 = metrics.accuracy(
        labels=labels, predictions=predictions + 1)

    names_to_values = {'Accuracy': accuracy0, 'Another_accuracy': accuracy1}
    names_to_updates = {'Accuracy': update_op0, 'Another_accuracy': update_op1}
    return names_to_values, names_to_updates

  def _verify_summaries(self, output_dir, names_to_values):
    """Verifies that the given `names_to_values` are found in the summaries.

    Args:
      output_dir: An existing directory where summaries are found.
      names_to_values: A dictionary of strings to values.
    """
    # Check that the results were saved. The events file may have additional
    # entries, e.g. the event version stamp, so have to parse things a bit.
    output_filepath = glob.glob(os.path.join(output_dir, '*'))
    self.assertEqual(len(output_filepath), 1)

    events = summary_iterator.summary_iterator(output_filepath[0])
    summaries = [e.summary for e in events if e.summary.value]
    values = []
    for summary in summaries:
      for value in summary.value:
        values.append(value)
    saved_results = {v.tag: v.simple_value for v in values}
    for name in names_to_values:
      self.assertAlmostEqual(names_to_values[name], saved_results[name])

  def testLatestCheckpointReturnsNoneAfterTimeout(self):
    start = time.time()
    ret = evaluation_lib.wait_for_new_checkpoint(
        '/non-existent-dir', 'foo', timeout=1.0, seconds_to_sleep=0.5)
    end = time.time()
    self.assertIsNone(ret)
    # We've waited one time.
    self.assertGreater(end, start + 0.5)
    # The timeout kicked in.
    self.assertLess(end, start + 1.1)

  def testTimeoutFnOnEvaluationLoop(self):
    # We require a mutable object (e.g. list but not an int) to maintain state
    # across calls of a nested function.
    timeout_fn_calls = [0]
    def _TimeoutFn():
      timeout_fn_calls[0] += 1
      return timeout_fn_calls[0] >= 3
    # Need not do any evaluation, but should just call timeout_fn repeatedly.
    evaluation.evaluation_loop('', '', '', timeout=0, timeout_fn=_TimeoutFn)
    self.assertEqual(timeout_fn_calls[0], 3)

  def testMonitorCheckpointsLoopTimeout(self):
    ret = list(
        evaluation_lib.checkpoints_iterator(
            '/non-existent-dir', timeout=0))
    self.assertEqual(ret, [])

  def testWithEpochLimit(self):
    predictions_limited = input.limit_epochs(self._predictions, num_epochs=1)
    labels_limited = input.limit_epochs(self._labels, num_epochs=1)

    value_op, update_op = metrics.accuracy(
        labels=labels_limited, predictions=predictions_limited)

    init_op = control_flow_ops.group(variables.global_variables_initializer(),
                                     variables.local_variables_initializer())
    # Create checkpoint and log directories:
    chkpt_dir = os.path.join(self.get_temp_dir(), 'tmp_logs/')
    gfile.MakeDirs(chkpt_dir)
    logdir = os.path.join(self.get_temp_dir(), 'tmp_logs2/')
    gfile.MakeDirs(logdir)

    # Save initialized variables to a checkpoint directory:
    saver = saver_lib.Saver()
    with self.cached_session() as sess:
      init_op.run()
      saver.save(sess, os.path.join(chkpt_dir, 'chkpt'))

    # Now, run the evaluation loop:
    accuracy_value = evaluation.evaluation_loop(
        '', chkpt_dir, logdir, eval_op=update_op, final_op=value_op,
        max_number_of_evaluations=1, num_evals=10000)
    self.assertAlmostEqual(accuracy_value, self._expected_accuracy)


class SingleEvaluationTest(test.TestCase):

  def setUp(self):
    super(SingleEvaluationTest, self).setUp()

    num_classes = 8
    batch_size = 16
    inputs, labels = GenerateTestData(num_classes, batch_size)
    self._expected_accuracy = GroundTruthAccuracy(inputs, labels, batch_size)

    self._global_step = variables_lib.get_or_create_global_step()
    self._inputs = constant_op.constant(inputs, dtype=dtypes.float32)
    self._labels = constant_op.constant(labels, dtype=dtypes.int64)
    self._predictions, self._scale = TestModel(self._inputs)

  def testErrorRaisedIfCheckpointDoesntExist(self):
    checkpoint_path = os.path.join(self.get_temp_dir(),
                                   'this_file_doesnt_exist')
    log_dir = os.path.join(self.get_temp_dir(), 'error_raised')
    with self.assertRaises(ValueError):
      evaluation.evaluate_once('', checkpoint_path, log_dir)

  def _prepareCheckpoint(self, checkpoint_path):
    init_op = control_flow_ops.group(variables.global_variables_initializer(),
                                     variables.local_variables_initializer())
    saver = saver_lib.Saver(write_version=saver_pb2.SaverDef.V1)
    with self.cached_session() as sess:
      sess.run(init_op)
      saver.save(sess, checkpoint_path)

  def testRestoredModelPerformance(self):
    checkpoint_path = os.path.join(self.get_temp_dir(), 'model.ckpt')
    log_dir = os.path.join(self.get_temp_dir(), 'log_dir1/')

    # First, save out the current model to a checkpoint:
    self._prepareCheckpoint(checkpoint_path)

    # Next, determine the metric to evaluate:
    value_op, update_op = metrics.accuracy(
        labels=self._labels, predictions=self._predictions)

    # Run the evaluation and verify the results:
    accuracy_value = evaluation.evaluate_once(
        '', checkpoint_path, log_dir, eval_op=update_op, final_op=value_op)
    self.assertAlmostEqual(accuracy_value, self._expected_accuracy)

  def testAdditionalHooks(self):
    checkpoint_path = os.path.join(self.get_temp_dir(), 'model.ckpt')
    log_dir = os.path.join(self.get_temp_dir(), 'log_dir1/')

    # First, save out the current model to a checkpoint:
    self._prepareCheckpoint(checkpoint_path)

    # Next, determine the metric to evaluate:
    value_op, update_op = metrics.accuracy(
        labels=self._labels, predictions=self._predictions)

    dumping_root = os.path.join(self.get_temp_dir(), 'tfdbg_dump_dir')
    dumping_hook = hooks.DumpingDebugHook(dumping_root, log_usage=False)
    try:
      # Run the evaluation and verify the results:
      accuracy_value = evaluation.evaluate_once(
          '', checkpoint_path, log_dir, eval_op=update_op, final_op=value_op,
          hooks=[dumping_hook])
      self.assertAlmostEqual(accuracy_value, self._expected_accuracy)

      dump = debug_data.DebugDumpDir(
          glob.glob(os.path.join(dumping_root, 'run_*'))[0])
      # Here we simply assert that the dumped data has been loaded and is
      # non-empty. We do not care about the detailed model-internal tensors or
      # their values.
      self.assertTrue(dump.dumped_tensor_data)
    finally:
      if os.path.isdir(dumping_root):
        shutil.rmtree(dumping_root)


if __name__ == '__main__':
  test.main()
