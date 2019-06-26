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
"""Implementation of Gaussian mixture model (GMM) clustering using tf.Learn."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import time
import numpy as np

from tensorflow.contrib import framework
from tensorflow.contrib.factorization.python.ops import gmm_ops
from tensorflow.contrib.framework.python.framework import checkpoint_utils
from tensorflow.contrib.learn.python.learn.estimators import estimator
from tensorflow.contrib.learn.python.learn.estimators import model_fn as model_fn_lib
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import ops
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import logging_ops as logging
from tensorflow.python.ops import state_ops
from tensorflow.python.ops.control_flow_ops import with_dependencies
from tensorflow.python.training import session_run_hook
from tensorflow.python.training import training_util


def _streaming_sum(scalar_tensor):
  """Create a sum metric and update op."""
  sum_metric = framework.local_variable(constant_op.constant(0.0))
  sum_update = sum_metric.assign_add(scalar_tensor)
  return sum_metric, sum_update


class _InitializeClustersHook(session_run_hook.SessionRunHook):
  """Initializes clusters or waits for cluster initialization."""

  def __init__(self, init_op, is_initialized_op, is_chief):
    self._init_op = init_op
    self._is_chief = is_chief
    self._is_initialized_op = is_initialized_op

  def after_create_session(self, session, _):
    assert self._init_op.graph == ops.get_default_graph()
    assert self._is_initialized_op.graph == self._init_op.graph
    while True:
      try:
        if session.run(self._is_initialized_op):
          break
        elif self._is_chief:
          session.run(self._init_op)
        else:
          time.sleep(1)
      except RuntimeError as e:
        logging.info(e)


class GMM(estimator.Estimator):
  """An estimator for GMM clustering."""
  SCORES = 'scores'
  LOG_LIKELIHOOD = 'loss'
  ASSIGNMENTS = 'assignments'

  def __init__(self,
               num_clusters,
               model_dir=None,
               random_seed=0,
               params='wmc',
               initial_clusters='random',
               covariance_type='full',
               config=None):
    """Creates a model for running GMM training and inference.

    Args:
      num_clusters: number of clusters to train.
      model_dir: the directory to save the model results and log files.
      random_seed: Python integer. Seed for PRNG used to initialize centers.
      params: Controls which parameters are updated in the training process.
        Can contain any combination of "w" for weights, "m" for means,
        and "c" for covars.
      initial_clusters: specifies how to initialize the clusters for training.
        See gmm_ops.gmm for the possible values.
      covariance_type: one of "full", "diag".
      config: See Estimator
    """
    self._num_clusters = num_clusters
    self._params = params
    self._training_initial_clusters = initial_clusters
    self._covariance_type = covariance_type
    self._training_graph = None
    self._random_seed = random_seed
    super(GMM, self).__init__(
        model_fn=self._model_builder(), model_dir=model_dir, config=config)

  def predict_assignments(self, input_fn=None, batch_size=None, outputs=None):
    """See BaseEstimator.predict."""
    results = self.predict(input_fn=input_fn,
                           batch_size=batch_size,
                           outputs=outputs)
    for result in results:
      yield result[GMM.ASSIGNMENTS]

  def score(self, input_fn=None, batch_size=None, steps=None):
    """Predict total log-likelihood.

    Args:
      input_fn: see predict.
      batch_size: see predict.
      steps: see predict.

    Returns:
      Total log-likelihood.
    """
    results = self.evaluate(input_fn=input_fn, batch_size=batch_size,
                            steps=steps)
    return np.log(np.sum(np.exp(results[GMM.SCORES])))

  def weights(self):
    """Returns the cluster weights."""
    return checkpoint_utils.load_variable(
        self.model_dir, gmm_ops.GmmAlgorithm.CLUSTERS_WEIGHT)

  def clusters(self):
    """Returns cluster centers."""
    clusters = checkpoint_utils.load_variable(
        self.model_dir, gmm_ops.GmmAlgorithm.CLUSTERS_VARIABLE)
    return np.squeeze(clusters, 1)

  def covariances(self):
    """Returns the covariances."""
    return checkpoint_utils.load_variable(
        self.model_dir, gmm_ops.GmmAlgorithm.CLUSTERS_COVS_VARIABLE)

  def _parse_tensor_or_dict(self, features):
    if isinstance(features, dict):
      return array_ops.concat([features[k] for k in sorted(features.keys())],
                              1)
    return features

  def _model_builder(self):
    """Creates a model function."""

    def _model_fn(features, labels, mode, config):
      """Model function."""
      assert labels is None, labels
      (loss,
       scores,
       model_predictions,
       training_op,
       init_op,
       is_initialized) = gmm_ops.gmm(self._parse_tensor_or_dict(features),
                                     self._training_initial_clusters,
                                     self._num_clusters, self._random_seed,
                                     self._covariance_type,
                                     self._params)
      incr_step = state_ops.assign_add(training_util.get_global_step(), 1)
      training_op = with_dependencies([training_op, incr_step], loss)
      training_hooks = [_InitializeClustersHook(
          init_op, is_initialized, config.is_chief)]
      predictions = {
          GMM.ASSIGNMENTS: model_predictions[0][0],
      }
      eval_metric_ops = {
          GMM.SCORES: scores,
          GMM.LOG_LIKELIHOOD: _streaming_sum(loss),
      }
      return model_fn_lib.ModelFnOps(mode=mode, predictions=predictions,
                                     eval_metric_ops=eval_metric_ops,
                                     loss=loss, train_op=training_op,
                                     training_hooks=training_hooks)

    return _model_fn
