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
"""A more advanced example, of building an RNN-based time series model."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import functools
from os import path
import tempfile

import numpy
import tensorflow as tf

from tensorflow.contrib.timeseries.python.timeseries import estimators as ts_estimators
from tensorflow.contrib.timeseries.python.timeseries import model as ts_model
from tensorflow.contrib.timeseries.python.timeseries import state_management

try:
  import matplotlib  # pylint: disable=g-import-not-at-top
  matplotlib.use("TkAgg")  # Need Tk for interactive plots.
  from matplotlib import pyplot  # pylint: disable=g-import-not-at-top
  HAS_MATPLOTLIB = True
except ImportError:
  # Plotting requires matplotlib, but the unit test running this code may
  # execute in an environment without it (i.e. matplotlib is not a build
  # dependency). We'd still like to test the TensorFlow-dependent parts of this
  # example.
  HAS_MATPLOTLIB = False

_MODULE_PATH = path.dirname(__file__)
_DATA_FILE = path.join(_MODULE_PATH, "data/multivariate_periods.csv")


class _LSTMModel(ts_model.SequentialTimeSeriesModel):
  """A time series model-building example using an RNNCell."""

  def __init__(self, num_units, num_features, exogenous_feature_columns=None,
               dtype=tf.float32):
    """Initialize/configure the model object.

    Note that we do not start graph building here. Rather, this object is a
    configurable factory for TensorFlow graphs which are run by an Estimator.

    Args:
      num_units: The number of units in the model's LSTMCell.
      num_features: The dimensionality of the time series (features per
        timestep).
      exogenous_feature_columns: A list of `tf.feature_column`s representing
          features which are inputs to the model but are not predicted by
          it. These must then be present for training, evaluation, and
          prediction.
      dtype: The floating point data type to use.
    """
    super(_LSTMModel, self).__init__(
        # Pre-register the metrics we'll be outputting (just a mean here).
        train_output_names=["mean"],
        predict_output_names=["mean"],
        num_features=num_features,
        exogenous_feature_columns=exogenous_feature_columns,
        dtype=dtype)
    self._num_units = num_units
    # Filled in by initialize_graph()
    self._lstm_cell = None
    self._lstm_cell_run = None
    self._predict_from_lstm_output = None

  def initialize_graph(self, input_statistics=None):
    """Save templates for components, which can then be used repeatedly.

    This method is called every time a new graph is created. It's safe to start
    adding ops to the current default graph here, but the graph should be
    constructed from scratch.

    Args:
      input_statistics: A math_utils.InputStatistics object.
    """
    super(_LSTMModel, self).initialize_graph(input_statistics=input_statistics)
    with tf.variable_scope("", use_resource=True):
      # Use ResourceVariables to avoid race conditions.
      self._lstm_cell = tf.nn.rnn_cell.LSTMCell(num_units=self._num_units)
      # Create templates so we don't have to worry about variable reuse.
      self._lstm_cell_run = tf.make_template(
          name_="lstm_cell",
          func_=self._lstm_cell,
          create_scope_now_=True)
      # Transforms LSTM output into mean predictions.
      self._predict_from_lstm_output = tf.make_template(
          name_="predict_from_lstm_output",
          func_=functools.partial(tf.layers.dense, units=self.num_features),
          create_scope_now_=True)

  def get_start_state(self):
    """Return initial state for the time series model."""
    return (
        # Keeps track of the time associated with this state for error checking.
        tf.zeros([], dtype=tf.int64),
        # The previous observation or prediction.
        tf.zeros([self.num_features], dtype=self.dtype),
        # The most recently seen exogenous features.
        tf.zeros(self._get_exogenous_embedding_shape(), dtype=self.dtype),
        # The state of the RNNCell (batch dimension removed since this parent
        # class will broadcast).
        [tf.squeeze(state_element, axis=0)
         for state_element
         in self._lstm_cell.zero_state(batch_size=1, dtype=self.dtype)])

  def _filtering_step(self, current_times, current_values, state, predictions):
    """Update model state based on observations.

    Note that we don't do much here aside from computing a loss. In this case
    it's easier to update the RNN state in _prediction_step, since that covers
    running the RNN both on observations (from this method) and our own
    predictions. This distinction can be important for probabilistic models,
    where repeatedly predicting without filtering should lead to low-confidence
    predictions.

    Args:
      current_times: A [batch size] integer Tensor.
      current_values: A [batch size, self.num_features] floating point Tensor
        with new observations.
      state: The model's state tuple.
      predictions: The output of the previous `_prediction_step`.
    Returns:
      A tuple of new state and a predictions dictionary updated to include a
      loss (note that we could also return other measures of goodness of fit,
      although only "loss" will be optimized).
    """
    state_from_time, prediction, exogenous, lstm_state = state
    with tf.control_dependencies(
        [tf.assert_equal(current_times, state_from_time)]):
      # Subtract the mean and divide by the variance of the series.  Slightly
      # more efficient if done for a whole window (using the normalize_features
      # argument to SequentialTimeSeriesModel).
      transformed_values = self._scale_data(current_values)
      # Use mean squared error across features for the loss.
      predictions["loss"] = tf.reduce_mean(
          (prediction - transformed_values) ** 2, axis=-1)
      # Keep track of the new observation in model state. It won't be run
      # through the LSTM until the next _imputation_step.
      new_state_tuple = (current_times, transformed_values,
                         exogenous, lstm_state)
    return (new_state_tuple, predictions)

  def _prediction_step(self, current_times, state):
    """Advance the RNN state using a previous observation or prediction."""
    _, previous_observation_or_prediction, exogenous, lstm_state = state
    # Update LSTM state based on the most recent exogenous and endogenous
    # features.
    inputs = tf.concat([previous_observation_or_prediction, exogenous],
                       axis=-1)
    lstm_output, new_lstm_state = self._lstm_cell_run(
        inputs=inputs, state=lstm_state)
    next_prediction = self._predict_from_lstm_output(lstm_output)
    new_state_tuple = (current_times, next_prediction,
                       exogenous, new_lstm_state)
    return new_state_tuple, {"mean": self._scale_back_data(next_prediction)}

  def _imputation_step(self, current_times, state):
    """Advance model state across a gap."""
    # Does not do anything special if we're jumping across a gap. More advanced
    # models, especially probabilistic ones, would want a special case that
    # depends on the gap size.
    return state

  def _exogenous_input_step(
      self, current_times, current_exogenous_regressors, state):
    """Save exogenous regressors in model state for use in _prediction_step."""
    state_from_time, prediction, _, lstm_state = state
    return (state_from_time, prediction,
            current_exogenous_regressors, lstm_state)


def train_and_predict(
    csv_file_name=_DATA_FILE, training_steps=200, estimator_config=None,
    export_directory=None):
  """Train and predict using a custom time series model."""
  # Construct an Estimator from our LSTM model.
  categorical_column = tf.feature_column.categorical_column_with_hash_bucket(
      key="categorical_exogenous_feature", hash_bucket_size=16)
  exogenous_feature_columns = [
      # Exogenous features are not part of the loss, but can inform
      # predictions. In this example the features have no extra information, but
      # are included as an API example.
      tf.feature_column.numeric_column(
          "2d_exogenous_feature", shape=(2,)),
      tf.feature_column.embedding_column(
          categorical_column=categorical_column, dimension=10)]
  estimator = ts_estimators.TimeSeriesRegressor(
      model=_LSTMModel(num_features=5, num_units=128,
                       exogenous_feature_columns=exogenous_feature_columns),
      optimizer=tf.train.AdamOptimizer(0.001), config=estimator_config,
      # Set state to be saved across windows.
      state_manager=state_management.ChainingStateManager())
  reader = tf.contrib.timeseries.CSVReader(
      csv_file_name,
      column_names=((tf.contrib.timeseries.TrainEvalFeatures.TIMES,)
                    + (tf.contrib.timeseries.TrainEvalFeatures.VALUES,) * 5
                    + ("2d_exogenous_feature",) * 2
                    + ("categorical_exogenous_feature",)),
      # Data types other than for `times` need to be specified if they aren't
      # float32. In this case one of our exogenous features has string dtype.
      column_dtypes=((tf.int64,) + (tf.float32,) * 7 + (tf.string,)))
  train_input_fn = tf.contrib.timeseries.RandomWindowInputFn(
      reader, batch_size=4, window_size=32)
  estimator.train(input_fn=train_input_fn, steps=training_steps)
  evaluation_input_fn = tf.contrib.timeseries.WholeDatasetInputFn(reader)
  evaluation = estimator.evaluate(input_fn=evaluation_input_fn, steps=1)
  # Predict starting after the evaluation
  predict_exogenous_features = {
      "2d_exogenous_feature": numpy.concatenate(
          [numpy.ones([1, 100, 1]), numpy.zeros([1, 100, 1])],
          axis=-1),
      "categorical_exogenous_feature": numpy.array(
          ["strkey"] * 100)[None, :, None]}
  (predictions,) = tuple(estimator.predict(
      input_fn=tf.contrib.timeseries.predict_continuation_input_fn(
          evaluation, steps=100,
          exogenous_features=predict_exogenous_features)))
  times = evaluation["times"][0]
  observed = evaluation["observed"][0, :, :]
  predicted_mean = numpy.squeeze(numpy.concatenate(
      [evaluation["mean"][0], predictions["mean"]], axis=0))
  all_times = numpy.concatenate([times, predictions["times"]], axis=0)

  # Export the model in SavedModel format. We include a bit of extra boilerplate
  # for "cold starting" as if we didn't have any state from the Estimator, which
  # is the case when serving from a SavedModel. If Estimator output is
  # available, the result of "Estimator.evaluate" can be passed directly to
  # `tf.contrib.timeseries.saved_model_utils.predict_continuation` as the
  # `continue_from` argument.
  with tf.Graph().as_default():
    filter_feature_tensors, _ = evaluation_input_fn()
    with tf.train.MonitoredSession() as session:
      # Fetch the series to "warm up" our state, which will allow us to make
      # predictions for its future values. This is just a dictionary of times,
      # values, and exogenous features mapping to numpy arrays. The use of an
      # input_fn is just a convenience for the example; they can also be
      # specified manually.
      filter_features = session.run(filter_feature_tensors)
  if export_directory is None:
    export_directory = tempfile.mkdtemp()
  input_receiver_fn = estimator.build_raw_serving_input_receiver_fn()
  export_location = estimator.export_saved_model(export_directory,
                                                 input_receiver_fn)
  # Warm up and predict using the SavedModel
  with tf.Graph().as_default():
    with tf.Session() as session:
      signatures = tf.saved_model.loader.load(
          session, [tf.saved_model.tag_constants.SERVING], export_location)
      state = tf.contrib.timeseries.saved_model_utils.cold_start_filter(
          signatures=signatures, session=session, features=filter_features)
      saved_model_output = (
          tf.contrib.timeseries.saved_model_utils.predict_continuation(
              continue_from=state, signatures=signatures,
              session=session, steps=100,
              exogenous_features=predict_exogenous_features))
      # The exported model gives the same results as the Estimator.predict()
      # call above.
      numpy.testing.assert_allclose(
          predictions["mean"],
          numpy.squeeze(saved_model_output["mean"], axis=0))
  return times, observed, all_times, predicted_mean


def main(unused_argv):
  if not HAS_MATPLOTLIB:
    raise ImportError(
        "Please install matplotlib to generate a plot from this example.")
  (observed_times, observations,
   all_times, predictions) = train_and_predict()
  pyplot.axvline(99, linestyle="dotted")
  observed_lines = pyplot.plot(
      observed_times, observations, label="Observed", color="k")
  predicted_lines = pyplot.plot(
      all_times, predictions, label="Predicted", color="b")
  pyplot.legend(handles=[observed_lines[0], predicted_lines[0]],
                loc="upper left")
  pyplot.show()


if __name__ == "__main__":
  tf.app.run(main=main)
