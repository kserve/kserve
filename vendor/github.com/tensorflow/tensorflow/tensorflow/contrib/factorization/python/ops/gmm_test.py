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
"""Tests for ops.gmm."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import numpy as np

from tensorflow.contrib.factorization.python.ops import gmm as gmm_lib
from tensorflow.contrib.learn.python.learn.estimators import kmeans
from tensorflow.contrib.learn.python.learn.estimators import run_config
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import random_seed as random_seed_lib
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import data_flow_ops
from tensorflow.python.ops import random_ops
from tensorflow.python.platform import test
from tensorflow.python.training import queue_runner


class GMMTest(test.TestCase):

  def input_fn(self, batch_size=None, points=None):
    batch_size = batch_size or self.batch_size
    points = points if points is not None else self.points
    num_points = points.shape[0]

    def _fn():
      x = constant_op.constant(points)
      if batch_size == num_points:
        return x, None
      indices = random_ops.random_uniform(constant_op.constant([batch_size]),
                                          minval=0, maxval=num_points-1,
                                          dtype=dtypes.int32,
                                          seed=10)
      return array_ops.gather(x, indices), None
    return _fn

  def setUp(self):
    np.random.seed(3)
    random_seed_lib.set_random_seed(2)
    self.num_centers = 2
    self.num_dims = 2
    self.num_points = 4000
    self.batch_size = self.num_points
    self.true_centers = self.make_random_centers(self.num_centers,
                                                 self.num_dims)
    self.points, self.assignments = self.make_random_points(
        self.true_centers, self.num_points)

    # Use initial means from kmeans (just like scikit-learn does).
    clusterer = kmeans.KMeansClustering(num_clusters=self.num_centers)
    clusterer.fit(input_fn=lambda: (constant_op.constant(self.points), None),
                  steps=30)
    self.initial_means = clusterer.clusters()

  @staticmethod
  def make_random_centers(num_centers, num_dims):
    return np.round(
        np.random.rand(num_centers, num_dims).astype(np.float32) * 500)

  @staticmethod
  def make_random_points(centers, num_points):
    num_centers, num_dims = centers.shape
    assignments = np.random.choice(num_centers, num_points)
    offsets = np.round(
        np.random.randn(num_points, num_dims).astype(np.float32) * 20)
    points = centers[assignments] + offsets
    return (points, assignments)

  def test_weights(self):
    """Tests the shape of the weights."""
    gmm = gmm_lib.GMM(self.num_centers,
                      initial_clusters=self.initial_means,
                      random_seed=4,
                      config=run_config.RunConfig(tf_random_seed=2))
    gmm.fit(input_fn=self.input_fn(), steps=0)
    weights = gmm.weights()
    self.assertAllEqual(list(weights.shape), [self.num_centers])

  def test_clusters(self):
    """Tests the shape of the clusters."""
    gmm = gmm_lib.GMM(self.num_centers,
                      initial_clusters=self.initial_means,
                      random_seed=4,
                      config=run_config.RunConfig(tf_random_seed=2))
    gmm.fit(input_fn=self.input_fn(), steps=0)
    clusters = gmm.clusters()
    self.assertAllEqual(list(clusters.shape), [self.num_centers, self.num_dims])

  def test_fit(self):
    gmm = gmm_lib.GMM(self.num_centers,
                      initial_clusters='random',
                      random_seed=4,
                      config=run_config.RunConfig(tf_random_seed=2))
    gmm.fit(input_fn=self.input_fn(), steps=1)
    score1 = gmm.score(input_fn=self.input_fn(batch_size=self.num_points),
                       steps=1)
    gmm.fit(input_fn=self.input_fn(), steps=10)
    score2 = gmm.score(input_fn=self.input_fn(batch_size=self.num_points),
                       steps=1)
    self.assertLess(score1, score2)

  def test_infer(self):
    gmm = gmm_lib.GMM(self.num_centers,
                      initial_clusters=self.initial_means,
                      random_seed=4,
                      config=run_config.RunConfig(tf_random_seed=2))
    gmm.fit(input_fn=self.input_fn(), steps=60)
    clusters = gmm.clusters()

    # Make a small test set
    num_points = 40
    points, true_assignments = self.make_random_points(clusters, num_points)

    assignments = []
    for item in gmm.predict_assignments(
        input_fn=self.input_fn(points=points, batch_size=num_points)):
      assignments.append(item)
    assignments = np.ravel(assignments)
    self.assertAllEqual(true_assignments, assignments)

  def _compare_with_sklearn(self, cov_type):
    # sklearn version.
    iterations = 40
    np.random.seed(5)
    sklearn_assignments = np.asarray([0, 0, 1, 0, 0, 0, 1, 0, 0, 1])
    sklearn_means = np.asarray([[144.83417719, 254.20130341],
                                [274.38754816, 353.16074346]])
    sklearn_covs = np.asarray([[[395.0081194, -4.50389512],
                                [-4.50389512, 408.27543989]],
                               [[385.17484203, -31.27834935],
                                [-31.27834935, 391.74249925]]])

    # skflow version.
    gmm = gmm_lib.GMM(self.num_centers,
                      initial_clusters=self.initial_means,
                      covariance_type=cov_type,
                      config=run_config.RunConfig(tf_random_seed=2))
    gmm.fit(input_fn=self.input_fn(), steps=iterations)
    points = self.points[:10, :]
    skflow_assignments = []
    for item in gmm.predict_assignments(
        input_fn=self.input_fn(points=points, batch_size=10)):
      skflow_assignments.append(item)
    self.assertAllClose(sklearn_assignments,
                        np.ravel(skflow_assignments).astype(int))
    self.assertAllClose(sklearn_means, gmm.clusters())
    if cov_type == 'full':
      self.assertAllClose(sklearn_covs, gmm.covariances(), rtol=0.01)
    else:
      for d in [0, 1]:
        self.assertAllClose(
            np.diag(sklearn_covs[d]), gmm.covariances()[d, :], rtol=0.01)

  def test_compare_full(self):
    self._compare_with_sklearn('full')

  def test_compare_diag(self):
    self._compare_with_sklearn('diag')

  def test_random_input_large(self):
    # sklearn version.
    iterations = 5  # that should be enough to know whether this diverges
    np.random.seed(5)
    num_classes = 20
    x = np.array([[np.random.random() for _ in range(100)]
                  for _ in range(num_classes)], dtype=np.float32)

    # skflow version.
    gmm = gmm_lib.GMM(num_classes,
                      covariance_type='full',
                      config=run_config.RunConfig(tf_random_seed=2))

    def get_input_fn(x):
      def input_fn():
        return constant_op.constant(x.astype(np.float32)), None
      return input_fn

    gmm.fit(input_fn=get_input_fn(x), steps=iterations)
    self.assertFalse(np.isnan(gmm.clusters()).any())


class GMMTestQueues(test.TestCase):

  def input_fn(self):
    def _fn():
      queue = data_flow_ops.FIFOQueue(capacity=10,
                                      dtypes=dtypes.float32,
                                      shapes=[10, 3])
      enqueue_op = queue.enqueue(array_ops.zeros([10, 3], dtype=dtypes.float32))
      queue_runner.add_queue_runner(queue_runner.QueueRunner(queue,
                                                             [enqueue_op]))
      return queue.dequeue(), None
    return _fn

  # This test makes sure that there are no deadlocks when using a QueueRunner.
  # Note that since cluster initialization is dependent on inputs, if input
  # is generated using a QueueRunner, one has to make sure that these runners
  # are started before the initialization.
  def test_queues(self):
    gmm = gmm_lib.GMM(2, covariance_type='diag')
    gmm.fit(input_fn=self.input_fn(), steps=1)


if __name__ == '__main__':
  test.main()
