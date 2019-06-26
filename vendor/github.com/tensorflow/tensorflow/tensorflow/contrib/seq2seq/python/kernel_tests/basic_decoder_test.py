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
"""Tests for contrib.seq2seq.python.seq2seq.basic_decoder."""
# pylint: disable=unused-import,g-bad-import-order
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function
# pylint: enable=unused-import

import numpy as np

from tensorflow.contrib.seq2seq.python.ops import helper as helper_py
from tensorflow.contrib.seq2seq.python.ops import basic_decoder

from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import tensor_shape
from tensorflow.python.layers import core as layers_core
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import rnn_cell
from tensorflow.python.ops import variables
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops.distributions import bernoulli
from tensorflow.python.ops.distributions import categorical
from tensorflow.python.platform import test
# pylint: enable=g-import-not-at-top


class BasicDecoderTest(test.TestCase):

  def _testStepWithTrainingHelper(self, use_output_layer):
    sequence_length = [3, 4, 3, 1, 0]
    batch_size = 5
    max_time = 8
    input_depth = 7
    cell_depth = 10
    output_layer_depth = 3

    with self.session(use_gpu=True) as sess:
      inputs = np.random.randn(batch_size, max_time,
                               input_depth).astype(np.float32)
      cell = rnn_cell.LSTMCell(cell_depth)
      helper = helper_py.TrainingHelper(
          inputs, sequence_length, time_major=False)
      if use_output_layer:
        output_layer = layers_core.Dense(output_layer_depth, use_bias=False)
        expected_output_depth = output_layer_depth
      else:
        output_layer = None
        expected_output_depth = cell_depth
      my_decoder = basic_decoder.BasicDecoder(
          cell=cell,
          helper=helper,
          initial_state=cell.zero_state(
              dtype=dtypes.float32, batch_size=batch_size),
          output_layer=output_layer)
      output_size = my_decoder.output_size
      output_dtype = my_decoder.output_dtype
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(expected_output_depth,
                                           tensor_shape.TensorShape([])),
          output_size)
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
          output_dtype)

      (first_finished, first_inputs, first_state) = my_decoder.initialize()
      (step_outputs, step_state, step_next_inputs,
       step_finished) = my_decoder.step(
           constant_op.constant(0), first_inputs, first_state)
      batch_size_t = my_decoder.batch_size

      self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(
          isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
      self.assertEqual((batch_size, expected_output_depth),
                       step_outputs[0].get_shape())
      self.assertEqual((batch_size,), step_outputs[1].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

      if use_output_layer:
        # The output layer was accessed
        self.assertEqual(len(output_layer.variables), 1)

      sess.run(variables.global_variables_initializer())
      sess_results = sess.run({
          "batch_size": batch_size_t,
          "first_finished": first_finished,
          "first_inputs": first_inputs,
          "first_state": first_state,
          "step_outputs": step_outputs,
          "step_state": step_state,
          "step_next_inputs": step_next_inputs,
          "step_finished": step_finished
      })

      self.assertAllEqual([False, False, False, False, True],
                          sess_results["first_finished"])
      self.assertAllEqual([False, False, False, True, True],
                          sess_results["step_finished"])
      self.assertEqual(output_dtype.sample_id,
                       sess_results["step_outputs"].sample_id.dtype)
      self.assertAllEqual(
          np.argmax(sess_results["step_outputs"].rnn_output, -1),
          sess_results["step_outputs"].sample_id)

  def testStepWithTrainingHelperNoOutputLayer(self):
    self._testStepWithTrainingHelper(use_output_layer=False)

  def testStepWithTrainingHelperWithOutputLayer(self):
    self._testStepWithTrainingHelper(use_output_layer=True)

  def testStepWithGreedyEmbeddingHelper(self):
    batch_size = 5
    vocabulary_size = 7
    cell_depth = vocabulary_size  # cell's logits must match vocabulary size
    input_depth = 10
    start_tokens = np.random.randint(0, vocabulary_size, size=batch_size)
    end_token = 1

    with self.session(use_gpu=True) as sess:
      embeddings = np.random.randn(vocabulary_size,
                                   input_depth).astype(np.float32)
      cell = rnn_cell.LSTMCell(vocabulary_size)
      helper = helper_py.GreedyEmbeddingHelper(embeddings, start_tokens,
                                               end_token)
      my_decoder = basic_decoder.BasicDecoder(
          cell=cell,
          helper=helper,
          initial_state=cell.zero_state(
              dtype=dtypes.float32, batch_size=batch_size))
      output_size = my_decoder.output_size
      output_dtype = my_decoder.output_dtype
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(cell_depth,
                                           tensor_shape.TensorShape([])),
          output_size)
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
          output_dtype)

      (first_finished, first_inputs, first_state) = my_decoder.initialize()
      (step_outputs, step_state, step_next_inputs,
       step_finished) = my_decoder.step(
           constant_op.constant(0), first_inputs, first_state)
      batch_size_t = my_decoder.batch_size

      self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(
          isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
      self.assertEqual((batch_size, cell_depth), step_outputs[0].get_shape())
      self.assertEqual((batch_size,), step_outputs[1].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

      sess.run(variables.global_variables_initializer())
      sess_results = sess.run({
          "batch_size": batch_size_t,
          "first_finished": first_finished,
          "first_inputs": first_inputs,
          "first_state": first_state,
          "step_outputs": step_outputs,
          "step_state": step_state,
          "step_next_inputs": step_next_inputs,
          "step_finished": step_finished
      })

      expected_sample_ids = np.argmax(
          sess_results["step_outputs"].rnn_output, -1)
      expected_step_finished = (expected_sample_ids == end_token)
      expected_step_next_inputs = embeddings[expected_sample_ids]
      self.assertAllEqual([False, False, False, False, False],
                          sess_results["first_finished"])
      self.assertAllEqual(expected_step_finished, sess_results["step_finished"])
      self.assertEqual(output_dtype.sample_id,
                       sess_results["step_outputs"].sample_id.dtype)
      self.assertAllEqual(expected_sample_ids,
                          sess_results["step_outputs"].sample_id)
      self.assertAllEqual(expected_step_next_inputs,
                          sess_results["step_next_inputs"])

  def testStepWithSampleEmbeddingHelper(self):
    batch_size = 5
    vocabulary_size = 7
    cell_depth = vocabulary_size  # cell's logits must match vocabulary size
    input_depth = 10
    np.random.seed(0)
    start_tokens = np.random.randint(0, vocabulary_size, size=batch_size)
    end_token = 1

    with self.session(use_gpu=True) as sess:
      with variable_scope.variable_scope(
          "testStepWithSampleEmbeddingHelper",
          initializer=init_ops.constant_initializer(0.01)):
        embeddings = np.random.randn(vocabulary_size,
                                     input_depth).astype(np.float32)
        cell = rnn_cell.LSTMCell(vocabulary_size)
        helper = helper_py.SampleEmbeddingHelper(embeddings, start_tokens,
                                                 end_token, seed=0)
        my_decoder = basic_decoder.BasicDecoder(
            cell=cell,
            helper=helper,
            initial_state=cell.zero_state(
                dtype=dtypes.float32, batch_size=batch_size))
        output_size = my_decoder.output_size
        output_dtype = my_decoder.output_dtype
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(cell_depth,
                                             tensor_shape.TensorShape([])),
            output_size)
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
            output_dtype)

        (first_finished, first_inputs, first_state) = my_decoder.initialize()
        (step_outputs, step_state, step_next_inputs,
         step_finished) = my_decoder.step(
             constant_op.constant(0), first_inputs, first_state)
        batch_size_t = my_decoder.batch_size

        self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(
            isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
        self.assertEqual((batch_size, cell_depth), step_outputs[0].get_shape())
        self.assertEqual((batch_size,), step_outputs[1].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

        sess.run(variables.global_variables_initializer())
        sess_results = sess.run({
            "batch_size": batch_size_t,
            "first_finished": first_finished,
            "first_inputs": first_inputs,
            "first_state": first_state,
            "step_outputs": step_outputs,
            "step_state": step_state,
            "step_next_inputs": step_next_inputs,
            "step_finished": step_finished
        })

        sample_ids = sess_results["step_outputs"].sample_id
        self.assertEqual(output_dtype.sample_id, sample_ids.dtype)
        expected_step_finished = (sample_ids == end_token)
        expected_step_next_inputs = embeddings[sample_ids]
        self.assertAllEqual(expected_step_finished,
                            sess_results["step_finished"])
        self.assertAllEqual(expected_step_next_inputs,
                            sess_results["step_next_inputs"])

  def testStepWithScheduledEmbeddingTrainingHelper(self):
    sequence_length = [3, 4, 3, 1, 0]
    batch_size = 5
    max_time = 8
    input_depth = 7
    vocabulary_size = 10

    with self.session(use_gpu=True) as sess:
      inputs = np.random.randn(
          batch_size, max_time, input_depth).astype(np.float32)
      embeddings = np.random.randn(
          vocabulary_size, input_depth).astype(np.float32)
      half = constant_op.constant(0.5)
      cell = rnn_cell.LSTMCell(vocabulary_size)
      helper = helper_py.ScheduledEmbeddingTrainingHelper(
          inputs=inputs,
          sequence_length=sequence_length,
          embedding=embeddings,
          sampling_probability=half,
          time_major=False)
      my_decoder = basic_decoder.BasicDecoder(
          cell=cell,
          helper=helper,
          initial_state=cell.zero_state(
              dtype=dtypes.float32, batch_size=batch_size))
      output_size = my_decoder.output_size
      output_dtype = my_decoder.output_dtype
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(vocabulary_size,
                                           tensor_shape.TensorShape([])),
          output_size)
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
          output_dtype)

      (first_finished, first_inputs, first_state) = my_decoder.initialize()
      (step_outputs, step_state, step_next_inputs,
       step_finished) = my_decoder.step(
           constant_op.constant(0), first_inputs, first_state)
      batch_size_t = my_decoder.batch_size

      self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(
          isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
      self.assertEqual((batch_size, vocabulary_size),
                       step_outputs[0].get_shape())
      self.assertEqual((batch_size,), step_outputs[1].get_shape())
      self.assertEqual((batch_size, vocabulary_size),
                       first_state[0].get_shape())
      self.assertEqual((batch_size, vocabulary_size),
                       first_state[1].get_shape())
      self.assertEqual((batch_size, vocabulary_size),
                       step_state[0].get_shape())
      self.assertEqual((batch_size, vocabulary_size),
                       step_state[1].get_shape())
      self.assertEqual((batch_size, input_depth),
                       step_next_inputs.get_shape())

      sess.run(variables.global_variables_initializer())
      sess_results = sess.run({
          "batch_size": batch_size_t,
          "first_finished": first_finished,
          "first_inputs": first_inputs,
          "first_state": first_state,
          "step_outputs": step_outputs,
          "step_state": step_state,
          "step_next_inputs": step_next_inputs,
          "step_finished": step_finished
      })

      self.assertAllEqual([False, False, False, False, True],
                          sess_results["first_finished"])
      self.assertAllEqual([False, False, False, True, True],
                          sess_results["step_finished"])
      sample_ids = sess_results["step_outputs"].sample_id
      self.assertEqual(output_dtype.sample_id, sample_ids.dtype)
      batch_where_not_sampling = np.where(sample_ids == -1)
      batch_where_sampling = np.where(sample_ids > -1)
      self.assertAllClose(
          sess_results["step_next_inputs"][batch_where_sampling],
          embeddings[sample_ids[batch_where_sampling]])
      self.assertAllClose(
          sess_results["step_next_inputs"][batch_where_not_sampling],
          np.squeeze(inputs[batch_where_not_sampling, 1]))

  def _testStepWithScheduledOutputTrainingHelper(
      self, sampling_probability, use_next_inputs_fn, use_auxiliary_inputs):
    sequence_length = [3, 4, 3, 1, 0]
    batch_size = 5
    max_time = 8
    input_depth = 7
    cell_depth = input_depth
    if use_auxiliary_inputs:
      auxiliary_input_depth = 4
      auxiliary_inputs = np.random.randn(
          batch_size, max_time, auxiliary_input_depth).astype(np.float32)
    else:
      auxiliary_inputs = None

    with self.session(use_gpu=True) as sess:
      inputs = np.random.randn(batch_size, max_time,
                               input_depth).astype(np.float32)
      cell = rnn_cell.LSTMCell(cell_depth)
      sampling_probability = constant_op.constant(sampling_probability)

      if use_next_inputs_fn:
        def next_inputs_fn(outputs):
          # Use deterministic function for test.
          samples = math_ops.argmax(outputs, axis=1)
          return array_ops.one_hot(samples, cell_depth, dtype=dtypes.float32)
      else:
        next_inputs_fn = None

      helper = helper_py.ScheduledOutputTrainingHelper(
          inputs=inputs,
          sequence_length=sequence_length,
          sampling_probability=sampling_probability,
          time_major=False,
          next_inputs_fn=next_inputs_fn,
          auxiliary_inputs=auxiliary_inputs)

      my_decoder = basic_decoder.BasicDecoder(
          cell=cell,
          helper=helper,
          initial_state=cell.zero_state(
              dtype=dtypes.float32, batch_size=batch_size))

      output_size = my_decoder.output_size
      output_dtype = my_decoder.output_dtype
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(cell_depth,
                                           tensor_shape.TensorShape([])),
          output_size)
      self.assertEqual(
          basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
          output_dtype)

      (first_finished, first_inputs, first_state) = my_decoder.initialize()
      (step_outputs, step_state, step_next_inputs,
       step_finished) = my_decoder.step(
           constant_op.constant(0), first_inputs, first_state)

      if use_next_inputs_fn:
        output_after_next_inputs_fn = next_inputs_fn(step_outputs.rnn_output)

      batch_size_t = my_decoder.batch_size

      self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
      self.assertTrue(
          isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
      self.assertEqual((batch_size, cell_depth), step_outputs[0].get_shape())
      self.assertEqual((batch_size,), step_outputs[1].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
      self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

      sess.run(variables.global_variables_initializer())

      fetches = {
          "batch_size": batch_size_t,
          "first_finished": first_finished,
          "first_inputs": first_inputs,
          "first_state": first_state,
          "step_outputs": step_outputs,
          "step_state": step_state,
          "step_next_inputs": step_next_inputs,
          "step_finished": step_finished
      }
      if use_next_inputs_fn:
        fetches["output_after_next_inputs_fn"] = output_after_next_inputs_fn

      sess_results = sess.run(fetches)

      self.assertAllEqual([False, False, False, False, True],
                          sess_results["first_finished"])
      self.assertAllEqual([False, False, False, True, True],
                          sess_results["step_finished"])

      sample_ids = sess_results["step_outputs"].sample_id
      self.assertEqual(output_dtype.sample_id, sample_ids.dtype)
      batch_where_not_sampling = np.where(np.logical_not(sample_ids))
      batch_where_sampling = np.where(sample_ids)

      auxiliary_inputs_to_concat = (
          auxiliary_inputs[:, 1] if use_auxiliary_inputs else
          np.array([]).reshape(batch_size, 0).astype(np.float32))

      expected_next_sampling_inputs = np.concatenate(
          (sess_results["output_after_next_inputs_fn"][batch_where_sampling]
           if use_next_inputs_fn else
           sess_results["step_outputs"].rnn_output[batch_where_sampling],
           auxiliary_inputs_to_concat[batch_where_sampling]),
          axis=-1)
      self.assertAllClose(
          sess_results["step_next_inputs"][batch_where_sampling],
          expected_next_sampling_inputs)

      self.assertAllClose(
          sess_results["step_next_inputs"][batch_where_not_sampling],
          np.concatenate(
              (np.squeeze(inputs[batch_where_not_sampling, 1], axis=0),
               auxiliary_inputs_to_concat[batch_where_not_sampling]),
              axis=-1))

  def testStepWithScheduledOutputTrainingHelperWithoutNextInputsFnOrAuxInputs(
      self):
    self._testStepWithScheduledOutputTrainingHelper(
        sampling_probability=0.5, use_next_inputs_fn=False,
        use_auxiliary_inputs=False)

  def testStepWithScheduledOutputTrainingHelperWithNextInputsFn(self):
    self._testStepWithScheduledOutputTrainingHelper(
        sampling_probability=0.5, use_next_inputs_fn=True,
        use_auxiliary_inputs=False)

  def testStepWithScheduledOutputTrainingHelperWithAuxiliaryInputs(self):
    self._testStepWithScheduledOutputTrainingHelper(
        sampling_probability=0.5, use_next_inputs_fn=False,
        use_auxiliary_inputs=True)

  def testStepWithScheduledOutputTrainingHelperWithNextInputsFnAndAuxInputs(
      self):
    self._testStepWithScheduledOutputTrainingHelper(
        sampling_probability=0.5, use_next_inputs_fn=True,
        use_auxiliary_inputs=True)

  def testStepWithScheduledOutputTrainingHelperWithNoSampling(self):
    self._testStepWithScheduledOutputTrainingHelper(
        sampling_probability=0.0, use_next_inputs_fn=True,
        use_auxiliary_inputs=True)

  def testStepWithInferenceHelperCategorical(self):
    batch_size = 5
    vocabulary_size = 7
    cell_depth = vocabulary_size
    start_token = 0
    end_token = 6

    start_inputs = array_ops.one_hot(
        np.ones(batch_size) * start_token,
        vocabulary_size)

    # The sample function samples categorically from the logits.
    sample_fn = lambda x: categorical.Categorical(logits=x).sample()
    # The next inputs are a one-hot encoding of the sampled labels.
    next_inputs_fn = (
        lambda x: array_ops.one_hot(x, vocabulary_size, dtype=dtypes.float32))
    end_fn = lambda sample_ids: math_ops.equal(sample_ids, end_token)

    with self.session(use_gpu=True) as sess:
      with variable_scope.variable_scope(
          "testStepWithInferenceHelper",
          initializer=init_ops.constant_initializer(0.01)):
        cell = rnn_cell.LSTMCell(vocabulary_size)
        helper = helper_py.InferenceHelper(
            sample_fn, sample_shape=(), sample_dtype=dtypes.int32,
            start_inputs=start_inputs, end_fn=end_fn,
            next_inputs_fn=next_inputs_fn)
        my_decoder = basic_decoder.BasicDecoder(
            cell=cell,
            helper=helper,
            initial_state=cell.zero_state(
                dtype=dtypes.float32, batch_size=batch_size))
        output_size = my_decoder.output_size
        output_dtype = my_decoder.output_dtype
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(cell_depth,
                                             tensor_shape.TensorShape([])),
            output_size)
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.int32),
            output_dtype)

        (first_finished, first_inputs, first_state) = my_decoder.initialize()
        (step_outputs, step_state, step_next_inputs,
         step_finished) = my_decoder.step(
             constant_op.constant(0), first_inputs, first_state)
        batch_size_t = my_decoder.batch_size

        self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(
            isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
        self.assertEqual((batch_size, cell_depth), step_outputs[0].get_shape())
        self.assertEqual((batch_size,), step_outputs[1].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

        sess.run(variables.global_variables_initializer())
        sess_results = sess.run({
            "batch_size": batch_size_t,
            "first_finished": first_finished,
            "first_inputs": first_inputs,
            "first_state": first_state,
            "step_outputs": step_outputs,
            "step_state": step_state,
            "step_next_inputs": step_next_inputs,
            "step_finished": step_finished
        })

        sample_ids = sess_results["step_outputs"].sample_id
        self.assertEqual(output_dtype.sample_id, sample_ids.dtype)
        expected_step_finished = (sample_ids == end_token)
        expected_step_next_inputs = np.zeros((batch_size, vocabulary_size))
        expected_step_next_inputs[np.arange(batch_size), sample_ids] = 1.0
        self.assertAllEqual(expected_step_finished,
                            sess_results["step_finished"])
        self.assertAllEqual(expected_step_next_inputs,
                            sess_results["step_next_inputs"])

  def testStepWithInferenceHelperMultilabel(self):
    batch_size = 5
    vocabulary_size = 7
    cell_depth = vocabulary_size
    start_token = 0
    end_token = 6

    start_inputs = array_ops.one_hot(
        np.ones(batch_size) * start_token,
        vocabulary_size)

    # The sample function samples independent bernoullis from the logits.
    sample_fn = (
        lambda x: bernoulli.Bernoulli(logits=x, dtype=dtypes.bool).sample())
    # The next inputs are a one-hot encoding of the sampled labels.
    next_inputs_fn = math_ops.to_float
    end_fn = lambda sample_ids: sample_ids[:, end_token]

    with self.session(use_gpu=True) as sess:
      with variable_scope.variable_scope(
          "testStepWithInferenceHelper",
          initializer=init_ops.constant_initializer(0.01)):
        cell = rnn_cell.LSTMCell(vocabulary_size)
        helper = helper_py.InferenceHelper(
            sample_fn, sample_shape=[cell_depth], sample_dtype=dtypes.bool,
            start_inputs=start_inputs, end_fn=end_fn,
            next_inputs_fn=next_inputs_fn)
        my_decoder = basic_decoder.BasicDecoder(
            cell=cell,
            helper=helper,
            initial_state=cell.zero_state(
                dtype=dtypes.float32, batch_size=batch_size))
        output_size = my_decoder.output_size
        output_dtype = my_decoder.output_dtype
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(cell_depth, cell_depth),
            output_size)
        self.assertEqual(
            basic_decoder.BasicDecoderOutput(dtypes.float32, dtypes.bool),
            output_dtype)

        (first_finished, first_inputs, first_state) = my_decoder.initialize()
        (step_outputs, step_state, step_next_inputs,
         step_finished) = my_decoder.step(
             constant_op.constant(0), first_inputs, first_state)
        batch_size_t = my_decoder.batch_size

        self.assertTrue(isinstance(first_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(isinstance(step_state, rnn_cell.LSTMStateTuple))
        self.assertTrue(
            isinstance(step_outputs, basic_decoder.BasicDecoderOutput))
        self.assertEqual((batch_size, cell_depth), step_outputs[0].get_shape())
        self.assertEqual((batch_size, cell_depth), step_outputs[1].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), first_state[1].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[0].get_shape())
        self.assertEqual((batch_size, cell_depth), step_state[1].get_shape())

        sess.run(variables.global_variables_initializer())
        sess_results = sess.run({
            "batch_size": batch_size_t,
            "first_finished": first_finished,
            "first_inputs": first_inputs,
            "first_state": first_state,
            "step_outputs": step_outputs,
            "step_state": step_state,
            "step_next_inputs": step_next_inputs,
            "step_finished": step_finished
        })

        sample_ids = sess_results["step_outputs"].sample_id
        self.assertEqual(output_dtype.sample_id, sample_ids.dtype)
        expected_step_finished = sample_ids[:, end_token]
        expected_step_next_inputs = sample_ids.astype(np.float32)
        self.assertAllEqual(expected_step_finished,
                            sess_results["step_finished"])
        self.assertAllEqual(expected_step_next_inputs,
                            sess_results["step_next_inputs"])


if __name__ == "__main__":
  test.main()
