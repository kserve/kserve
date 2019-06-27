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
"""Tests for DNNEstimators."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import functools
import json
import tempfile

import numpy as np

from tensorflow.contrib.layers.python.layers import feature_column
from tensorflow.contrib.learn.python.learn import experiment
from tensorflow.contrib.learn.python.learn.datasets import base
from tensorflow.contrib.learn.python.learn.estimators import _sklearn
from tensorflow.contrib.learn.python.learn.estimators import dnn
from tensorflow.contrib.learn.python.learn.estimators import dnn_linear_combined
from tensorflow.contrib.learn.python.learn.estimators import estimator
from tensorflow.contrib.learn.python.learn.estimators import estimator_test_utils
from tensorflow.contrib.learn.python.learn.estimators import head as head_lib
from tensorflow.contrib.learn.python.learn.estimators import model_fn
from tensorflow.contrib.learn.python.learn.estimators import run_config
from tensorflow.contrib.learn.python.learn.estimators import test_data
from tensorflow.contrib.learn.python.learn.metric_spec import MetricSpec
from tensorflow.contrib.metrics.python.ops import metric_ops
from tensorflow.python.feature_column import feature_column_lib as fc_core
from tensorflow.python.framework import constant_op
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import sparse_tensor
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.platform import test
from tensorflow.python.training import input as input_lib
from tensorflow.python.training import monitored_session
from tensorflow.python.training import server_lib


class EmbeddingMultiplierTest(test.TestCase):
  """dnn_model_fn tests."""

  def testRaisesNonEmbeddingColumn(self):
    one_hot_language = feature_column.one_hot_column(
        feature_column.sparse_column_with_hash_bucket('language', 10))

    params = {
        'feature_columns': [one_hot_language],
        'head': head_lib.multi_class_head(2),
        'hidden_units': [1],
        # Set lr mult to 0. to keep embeddings constant.
        'embedding_lr_multipliers': {
            one_hot_language: 0.0
        },
    }
    features = {
        'language':
            sparse_tensor.SparseTensor(
                values=['en', 'fr', 'zh'],
                indices=[[0, 0], [1, 0], [2, 0]],
                dense_shape=[3, 1]),
    }
    labels = constant_op.constant([[0], [0], [0]], dtype=dtypes.int32)
    with self.assertRaisesRegexp(ValueError,
                                 'can only be defined for embedding columns'):
      dnn._dnn_model_fn(features, labels, model_fn.ModeKeys.TRAIN, params)

  def testMultipliesGradient(self):
    embedding_language = feature_column.embedding_column(
        feature_column.sparse_column_with_hash_bucket('language', 10),
        dimension=1,
        initializer=init_ops.constant_initializer(0.1))
    embedding_wire = feature_column.embedding_column(
        feature_column.sparse_column_with_hash_bucket('wire', 10),
        dimension=1,
        initializer=init_ops.constant_initializer(0.1))

    params = {
        'feature_columns': [embedding_language, embedding_wire],
        'head': head_lib.multi_class_head(2),
        'hidden_units': [1],
        # Set lr mult to 0. to keep embeddings constant.
        'embedding_lr_multipliers': {
            embedding_language: 0.0
        },
    }
    features = {
        'language':
            sparse_tensor.SparseTensor(
                values=['en', 'fr', 'zh'],
                indices=[[0, 0], [1, 0], [2, 0]],
                dense_shape=[3, 1]),
        'wire':
            sparse_tensor.SparseTensor(
                values=['omar', 'stringer', 'marlo'],
                indices=[[0, 0], [1, 0], [2, 0]],
                dense_shape=[3, 1]),
    }
    labels = constant_op.constant([[0], [0], [0]], dtype=dtypes.int32)
    model_ops = dnn._dnn_model_fn(features, labels, model_fn.ModeKeys.TRAIN,
                                  params)
    with monitored_session.MonitoredSession() as sess:
      language_var = dnn_linear_combined._get_embedding_variable(
          embedding_language, 'dnn', 'dnn/input_from_feature_columns')
      wire_var = dnn_linear_combined._get_embedding_variable(
          embedding_wire, 'dnn', 'dnn/input_from_feature_columns')
      for _ in range(2):
        _, language_value, wire_value = sess.run(
            [model_ops.train_op, language_var, wire_var])
      initial_value = np.full_like(language_value, 0.1)
      self.assertTrue(np.all(np.isclose(language_value, initial_value)))
      self.assertFalse(np.all(np.isclose(wire_value, initial_value)))


class ActivationFunctionTest(test.TestCase):

  def _getModelForActivation(self, activation_fn):
    embedding_language = feature_column.embedding_column(
        feature_column.sparse_column_with_hash_bucket('language', 10),
        dimension=1,
        initializer=init_ops.constant_initializer(0.1))
    params = {
        'feature_columns': [embedding_language],
        'head': head_lib.multi_class_head(2),
        'hidden_units': [1],
        'activation_fn': activation_fn,
    }
    features = {
        'language':
            sparse_tensor.SparseTensor(
                values=['en', 'fr', 'zh'],
                indices=[[0, 0], [1, 0], [2, 0]],
                dense_shape=[3, 1]),
    }
    labels = constant_op.constant([[0], [0], [0]], dtype=dtypes.int32)
    return dnn._dnn_model_fn(features, labels, model_fn.ModeKeys.TRAIN, params)

  def testValidActivation(self):
    _ = self._getModelForActivation('relu')

  def testRaisesOnBadActivationName(self):
    with self.assertRaisesRegexp(ValueError,
                                 'Activation name should be one of'):
      self._getModelForActivation('max_pool')


class DNNEstimatorTest(test.TestCase):

  def _assertInRange(self, expected_min, expected_max, actual):
    self.assertLessEqual(expected_min, actual)
    self.assertGreaterEqual(expected_max, actual)

  def testExperimentIntegration(self):
    exp = experiment.Experiment(
        estimator=dnn.DNNClassifier(
            n_classes=3,
            feature_columns=[
                feature_column.real_valued_column(
                    'feature', dimension=4)
            ],
            hidden_units=[3, 3]),
        train_input_fn=test_data.iris_input_multiclass_fn,
        eval_input_fn=test_data.iris_input_multiclass_fn)
    exp.test()

  def testEstimatorContract(self):
    estimator_test_utils.assert_estimator_contract(self, dnn.DNNEstimator)

  def testTrainWithWeights(self):
    """Tests training with given weight column."""

    def _input_fn_train():
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      # First row has more weight than others. Model should fit (y=x) better
      # than (y=Not(x)) due to the relative higher weight of the first row.
      labels = constant_op.constant([[1], [0], [0], [0]])
      features = {
          'x': array_ops.ones(shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[100.], [3.], [2.], [2.]])
      }
      return features, labels

    def _input_fn_eval():
      # Create 4 rows (y = x)
      labels = constant_op.constant([[1], [1], [1], [1]])
      features = {
          'x': array_ops.ones(shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[1.], [1.], [1.], [1.]])
      }
      return features, labels

    dnn_estimator = dnn.DNNEstimator(
        head=head_lib.multi_class_head(2, weight_column_name='w'),
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    dnn_estimator.fit(input_fn=_input_fn_train, steps=5)
    scores = dnn_estimator.evaluate(input_fn=_input_fn_eval, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])


class DNNClassifierTest(test.TestCase):

  def testExperimentIntegration(self):
    exp = experiment.Experiment(
        estimator=dnn.DNNClassifier(
            n_classes=3,
            feature_columns=[
                feature_column.real_valued_column(
                    'feature', dimension=4)
            ],
            hidden_units=[3, 3]),
        train_input_fn=test_data.iris_input_multiclass_fn,
        eval_input_fn=test_data.iris_input_multiclass_fn)
    exp.test()

  def _assertInRange(self, expected_min, expected_max, actual):
    self.assertLessEqual(expected_min, actual)
    self.assertGreaterEqual(expected_max, actual)

  def testEstimatorContract(self):
    estimator_test_utils.assert_estimator_contract(self, dnn.DNNClassifier)

  def testEmbeddingMultiplier(self):
    embedding_language = feature_column.embedding_column(
        feature_column.sparse_column_with_hash_bucket('language', 10),
        dimension=1,
        initializer=init_ops.constant_initializer(0.1))
    classifier = dnn.DNNClassifier(
        feature_columns=[embedding_language],
        hidden_units=[3, 3],
        embedding_lr_multipliers={embedding_language: 0.8})
    self.assertEqual({
        embedding_language: 0.8
    }, classifier.params['embedding_lr_multipliers'])

  def testInputPartitionSize(self):
    def _input_fn_float_label(num_epochs=None):
      features = {
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      labels = constant_op.constant([[0.8], [0.], [0.2]], dtype=dtypes.float32)
      return features, labels

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(language_column, dimension=1),
    ]

    # Set num_ps_replica to be 10 and the min slice size to be extremely small,
    # so as to ensure that there'll be 10 partititions produced.
    config = run_config.RunConfig(tf_random_seed=1)
    config._num_ps_replicas = 10
    classifier = dnn.DNNClassifier(
        n_classes=2,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        optimizer='Adagrad',
        config=config,
        input_layer_min_slice_size=1)

    # Ensure the param is passed in.
    self.assertEqual(1, classifier.params['input_layer_min_slice_size'])

    # Ensure the partition count is 10.
    classifier.fit(input_fn=_input_fn_float_label, steps=50)
    partition_count = 0
    for name in classifier.get_variable_names():
      if 'language_embedding' in name and 'Adagrad' in name:
        partition_count += 1
    self.assertEqual(10, partition_count)

  def testLogisticRegression_MatrixData(self):
    """Tests binary classification using matrix data as input."""
    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    input_fn = test_data.iris_input_logistic_fn
    classifier.fit(input_fn=input_fn, steps=5)
    scores = classifier.evaluate(input_fn=input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)

  def testLogisticRegression_MatrixData_Labels1D(self):
    """Same as the last test, but label shape is [100] instead of [100, 1]."""

    def _input_fn():
      iris = test_data.prepare_iris_data_for_logistic_regression()
      return {
          'feature': constant_op.constant(
              iris.data, dtype=dtypes.float32)
      }, constant_op.constant(
          iris.target, shape=[100], dtype=dtypes.int32)

    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=5)
    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)

  def testLogisticRegression_NpMatrixData(self):
    """Tests binary classification using numpy matrix data as input."""
    iris = test_data.prepare_iris_data_for_logistic_regression()
    train_x = iris.data
    train_y = iris.target
    feature_columns = [feature_column.real_valued_column('', dimension=4)]
    classifier = dnn.DNNClassifier(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(x=train_x, y=train_y, steps=5)
    scores = classifier.evaluate(x=train_x, y=train_y, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])

  def _assertBinaryPredictions(self, expected_len, predictions):
    self.assertEqual(expected_len, len(predictions))
    for prediction in predictions:
      self.assertIn(prediction, (0, 1))

  def _assertClassificationPredictions(
      self, expected_len, n_classes, predictions):
    self.assertEqual(expected_len, len(predictions))
    for prediction in predictions:
      self.assertIn(prediction, range(n_classes))

  def _assertProbabilities(self, expected_batch_size, expected_n_classes,
                           probabilities):
    self.assertEqual(expected_batch_size, len(probabilities))
    for b in range(expected_batch_size):
      self.assertEqual(expected_n_classes, len(probabilities[b]))
      for i in range(expected_n_classes):
        self._assertInRange(0.0, 1.0, probabilities[b][i])

  def testEstimatorWithCoreFeatureColumns(self):

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [0.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    language_column = fc_core.categorical_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        fc_core.embedding_column(language_column, dimension=1),
        fc_core.numeric_column('age')
    ]

    classifier = dnn.DNNClassifier(
        n_classes=2,
        feature_columns=feature_columns,
        hidden_units=[10, 10],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=50)

    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predicted_classes = list(
        classifier.predict_classes(input_fn=predict_input_fn, as_iterable=True))
    self._assertBinaryPredictions(3, predicted_classes)
    predictions = list(
        classifier.predict(input_fn=predict_input_fn, as_iterable=True))
    self.assertAllEqual(predicted_classes, predictions)

  def testLogisticRegression_TensorData(self):
    """Tests binary classification using tensor data as input."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [0.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            language_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    classifier = dnn.DNNClassifier(
        n_classes=2,
        feature_columns=feature_columns,
        hidden_units=[10, 10],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=50)

    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predicted_classes = list(
        classifier.predict_classes(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertBinaryPredictions(3, predicted_classes)
    predictions = list(
        classifier.predict(input_fn=predict_input_fn, as_iterable=True))
    self.assertAllEqual(predicted_classes, predictions)

  def testLogisticRegression_FloatLabel(self):
    """Tests binary classification with float labels."""

    def _input_fn_float_label(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[50], [20], [10]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      labels = constant_op.constant([[0.8], [0.], [0.2]], dtype=dtypes.float32)
      return features, labels

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            language_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    classifier = dnn.DNNClassifier(
        n_classes=2,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn_float_label, steps=50)

    predict_input_fn = functools.partial(_input_fn_float_label, num_epochs=1)
    predicted_classes = list(
        classifier.predict_classes(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertBinaryPredictions(3, predicted_classes)
    predictions = list(
        classifier.predict(
            input_fn=predict_input_fn, as_iterable=True))
    self.assertAllEqual(predicted_classes, predictions)
    predictions_proba = list(
        classifier.predict_proba(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertProbabilities(3, 2, predictions_proba)

  def testMultiClass_MatrixData(self):
    """Tests multi-class classification using matrix data as input."""
    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    input_fn = test_data.iris_input_multiclass_fn
    classifier.fit(input_fn=input_fn, steps=200)
    scores = classifier.evaluate(input_fn=input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)

  def testMultiClass_MatrixData_Labels1D(self):
    """Same as the last test, but label shape is [150] instead of [150, 1]."""

    def _input_fn():
      iris = base.load_iris()
      return {
          'feature': constant_op.constant(
              iris.data, dtype=dtypes.float32)
      }, constant_op.constant(
          iris.target, shape=[150], dtype=dtypes.int32)

    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=200)
    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])

  def testMultiClass_NpMatrixData(self):
    """Tests multi-class classification using numpy matrix data as input."""
    iris = base.load_iris()
    train_x = iris.data
    train_y = iris.target
    feature_columns = [feature_column.real_valued_column('', dimension=4)]
    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(x=train_x, y=train_y, steps=200)
    scores = classifier.evaluate(x=train_x, y=train_y, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])

  def testMultiClassLabelKeys(self):
    """Tests n_classes > 2 with label_keys vocabulary for labels."""
    # Byte literals needed for python3 test to pass.
    label_keys = [b'label0', b'label1', b'label2']

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [0.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      labels = constant_op.constant(
          [[label_keys[1]], [label_keys[0]], [label_keys[0]]],
          dtype=dtypes.string)
      return features, labels

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            language_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=feature_columns,
        hidden_units=[10, 10],
        label_keys=label_keys,
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=50)

    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predicted_classes = list(
        classifier.predict_classes(
            input_fn=predict_input_fn, as_iterable=True))
    self.assertEqual(3, len(predicted_classes))
    for pred in predicted_classes:
      self.assertIn(pred, label_keys)
    predictions = list(
        classifier.predict(input_fn=predict_input_fn, as_iterable=True))
    self.assertAllEqual(predicted_classes, predictions)

  def testLoss(self):
    """Tests loss calculation."""

    def _input_fn_train():
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      # The logistic prediction should be (y = 0.25).
      labels = constant_op.constant([[1], [0], [0], [0]])
      features = {'x': array_ops.ones(shape=[4, 1], dtype=dtypes.float32),}
      return features, labels

    classifier = dnn.DNNClassifier(
        n_classes=2,
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn_train, steps=5)
    scores = classifier.evaluate(input_fn=_input_fn_train, steps=1)
    self.assertIn('loss', scores)

  def testLossWithWeights(self):
    """Tests loss calculation with weights."""

    def _input_fn_train():
      # 4 rows with equal weight, one of them (y = x), three of them (y=Not(x))
      # The logistic prediction should be (y = 0.25).
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[1.], [1.], [1.], [1.]])
      }
      return features, labels

    def _input_fn_eval():
      # 4 rows, with different weights.
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[7.], [1.], [1.], [1.]])
      }
      return features, labels

    classifier = dnn.DNNClassifier(
        weight_column_name='w',
        n_classes=2,
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn_train, steps=5)
    scores = classifier.evaluate(input_fn=_input_fn_eval, steps=1)
    self.assertIn('loss', scores)

  def testTrainWithWeights(self):
    """Tests training with given weight column."""

    def _input_fn_train():
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      # First row has more weight than others. Model should fit (y=x) better
      # than (y=Not(x)) due to the relative higher weight of the first row.
      labels = constant_op.constant([[1], [0], [0], [0]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[100.], [3.], [2.], [2.]])
      }
      return features, labels

    def _input_fn_eval():
      # Create 4 rows (y = x)
      labels = constant_op.constant([[1], [1], [1], [1]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[1.], [1.], [1.], [1.]])
      }
      return features, labels

    classifier = dnn.DNNClassifier(
        weight_column_name='w',
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn_train, steps=5)
    scores = classifier.evaluate(input_fn=_input_fn_eval, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])

  def testPredict_AsIterableFalse(self):
    """Tests predict and predict_prob methods with as_iterable=False."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1)
    ]

    n_classes = 3
    classifier = dnn.DNNClassifier(
        n_classes=n_classes,
        feature_columns=feature_columns,
        hidden_units=[10, 10],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=100)

    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)
    predicted_classes = classifier.predict_classes(
        input_fn=_input_fn, as_iterable=False)
    self._assertClassificationPredictions(3, n_classes, predicted_classes)
    predictions = classifier.predict(input_fn=_input_fn, as_iterable=False)
    self.assertAllEqual(predicted_classes, predictions)
    probabilities = classifier.predict_proba(
        input_fn=_input_fn, as_iterable=False)
    self._assertProbabilities(3, n_classes, probabilities)

  def testPredict_AsIterable(self):
    """Tests predict and predict_prob methods with as_iterable=True."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            language_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    n_classes = 3
    classifier = dnn.DNNClassifier(
        n_classes=n_classes,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=300)

    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predicted_classes = list(
        classifier.predict_classes(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertClassificationPredictions(3, n_classes, predicted_classes)
    predictions = list(
        classifier.predict(
            input_fn=predict_input_fn, as_iterable=True))
    self.assertAllEqual(predicted_classes, predictions)
    predicted_proba = list(
        classifier.predict_proba(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertProbabilities(3, n_classes, predicted_proba)

  def testCustomMetrics(self):
    """Tests custom evaluation metrics."""

    def _input_fn(num_epochs=None):
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      labels = constant_op.constant([[1], [0], [0], [0]])
      features = {
          'x':
              input_lib.limit_epochs(
                  array_ops.ones(
                      shape=[4, 1], dtype=dtypes.float32),
                  num_epochs=num_epochs),
      }
      return features, labels

    def _my_metric_op(predictions, labels):
      # For the case of binary classification, the 2nd column of "predictions"
      # denotes the model predictions.
      labels = math_ops.to_float(labels)
      predictions = array_ops.strided_slice(
          predictions, [0, 1], [-1, 2], end_mask=1)
      labels = math_ops.cast(labels, predictions.dtype)
      return math_ops.reduce_sum(math_ops.multiply(predictions, labels))

    classifier = dnn.DNNClassifier(
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=5)
    scores = classifier.evaluate(
        input_fn=_input_fn,
        steps=5,
        metrics={
            'my_accuracy':
                MetricSpec(
                    metric_fn=metric_ops.streaming_accuracy,
                    prediction_key='classes'),
            'my_precision':
                MetricSpec(
                    metric_fn=metric_ops.streaming_precision,
                    prediction_key='classes'),
            'my_metric':
                MetricSpec(
                    metric_fn=_my_metric_op, prediction_key='probabilities')
        })
    self.assertTrue(
        set(['loss', 'my_accuracy', 'my_precision', 'my_metric']).issubset(
            set(scores.keys())))
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predictions = np.array(list(classifier.predict_classes(
        input_fn=predict_input_fn)))
    self.assertEqual(
        _sklearn.accuracy_score([1, 0, 0, 0], predictions),
        scores['my_accuracy'])

    # Test the case where the 2nd element of the key is neither "classes" nor
    # "probabilities".
    with self.assertRaisesRegexp(KeyError, 'bad_type'):
      classifier.evaluate(
          input_fn=_input_fn,
          steps=5,
          metrics={
              'bad_name':
                  MetricSpec(
                      metric_fn=metric_ops.streaming_auc,
                      prediction_key='bad_type')
          })

  def testTrainSaveLoad(self):
    """Tests that insures you can save and reload a trained model."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1)
    ]

    model_dir = tempfile.mkdtemp()
    classifier = dnn.DNNClassifier(
        model_dir=model_dir,
        n_classes=3,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    classifier.fit(input_fn=_input_fn, steps=5)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predictions1 = classifier.predict_classes(input_fn=predict_input_fn)
    del classifier

    classifier2 = dnn.DNNClassifier(
        model_dir=model_dir,
        n_classes=3,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))
    predictions2 = classifier2.predict_classes(input_fn=predict_input_fn)
    self.assertEqual(list(predictions1), list(predictions2))

  def testTrainWithPartitionedVariables(self):
    """Tests training with partitioned variables."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [.2], [.1]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([[1], [0], [0]], dtype=dtypes.int32)

    # The given hash_bucket_size results in variables larger than the
    # default min_slice_size attribute, so the variables are partitioned.
    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=2e7)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1)
    ]

    tf_config = {
        'cluster': {
            run_config.TaskType.PS: ['fake_ps_0', 'fake_ps_1']
        }
    }
    with test.mock.patch.dict('os.environ',
                              {'TF_CONFIG': json.dumps(tf_config)}):
      config = run_config.RunConfig(tf_random_seed=1)
      # Because we did not start a distributed cluster, we need to pass an
      # empty ClusterSpec, otherwise the device_setter will look for
      # distributed jobs, such as "/job:ps" which are not present.
      config._cluster_spec = server_lib.ClusterSpec({})

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=config)

    classifier.fit(input_fn=_input_fn, steps=5)
    scores = classifier.evaluate(input_fn=_input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)

  def testExport(self):
    """Tests export model for servo."""

    def input_fn():
      return {
          'age':
              constant_op.constant([1]),
          'language':
              sparse_tensor.SparseTensor(
                  values=['english'], indices=[[0, 0]], dense_shape=[1, 1])
      }, constant_op.constant([[1]])

    language = feature_column.sparse_column_with_hash_bucket('language', 100)
    feature_columns = [
        feature_column.real_valued_column('age'),
        feature_column.embedding_column(
            language, dimension=1)
    ]

    classifier = dnn.DNNClassifier(
        feature_columns=feature_columns, hidden_units=[3, 3])
    classifier.fit(input_fn=input_fn, steps=5)

    export_dir = tempfile.mkdtemp()
    classifier.export(export_dir)

  def testEnableCenteredBias(self):
    """Tests that we can enable centered bias."""
    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=cont_features,
        hidden_units=[3, 3],
        enable_centered_bias=True,
        config=run_config.RunConfig(tf_random_seed=1))

    input_fn = test_data.iris_input_multiclass_fn
    classifier.fit(input_fn=input_fn, steps=5)
    self.assertIn('dnn/multi_class_head/centered_bias_weight',
                  classifier.get_variable_names())
    scores = classifier.evaluate(input_fn=input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)

  def testDisableCenteredBias(self):
    """Tests that we can disable centered bias."""
    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    classifier = dnn.DNNClassifier(
        n_classes=3,
        feature_columns=cont_features,
        hidden_units=[3, 3],
        enable_centered_bias=False,
        config=run_config.RunConfig(tf_random_seed=1))

    input_fn = test_data.iris_input_multiclass_fn
    classifier.fit(input_fn=input_fn, steps=5)
    self.assertNotIn('centered_bias_weight', classifier.get_variable_names())
    scores = classifier.evaluate(input_fn=input_fn, steps=1)
    self._assertInRange(0.0, 1.0, scores['accuracy'])
    self.assertIn('loss', scores)


class DNNRegressorTest(test.TestCase):

  def testExperimentIntegration(self):
    exp = experiment.Experiment(
        estimator=dnn.DNNRegressor(
            feature_columns=[
                feature_column.real_valued_column(
                    'feature', dimension=4)
            ],
            hidden_units=[3, 3]),
        train_input_fn=test_data.iris_input_logistic_fn,
        eval_input_fn=test_data.iris_input_logistic_fn)
    exp.test()

  def testEstimatorContract(self):
    estimator_test_utils.assert_estimator_contract(self, dnn.DNNRegressor)

  def testRegression_MatrixData(self):
    """Tests regression using matrix data as input."""
    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    regressor = dnn.DNNRegressor(
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    input_fn = test_data.iris_input_logistic_fn
    regressor.fit(input_fn=input_fn, steps=200)
    scores = regressor.evaluate(input_fn=input_fn, steps=1)
    self.assertIn('loss', scores)

  def testRegression_MatrixData_Labels1D(self):
    """Same as the last test, but label shape is [100] instead of [100, 1]."""

    def _input_fn():
      iris = test_data.prepare_iris_data_for_logistic_regression()
      return {
          'feature': constant_op.constant(
              iris.data, dtype=dtypes.float32)
      }, constant_op.constant(
          iris.target, shape=[100], dtype=dtypes.int32)

    cont_features = [feature_column.real_valued_column('feature', dimension=4)]

    regressor = dnn.DNNRegressor(
        feature_columns=cont_features,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=200)
    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)

  def testRegression_NpMatrixData(self):
    """Tests binary classification using numpy matrix data as input."""
    iris = test_data.prepare_iris_data_for_logistic_regression()
    train_x = iris.data
    train_y = iris.target
    feature_columns = [feature_column.real_valued_column('', dimension=4)]
    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(x=train_x, y=train_y, steps=200)
    scores = regressor.evaluate(x=train_x, y=train_y, steps=1)
    self.assertIn('loss', scores)

  def testRegression_TensorData(self):
    """Tests regression using tensor data as input."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[.8], [.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([1., 0., 0.2], dtype=dtypes.float32)

    language_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            language_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=200)

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)

  def testLoss(self):
    """Tests loss calculation."""

    def _input_fn_train():
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      # The algorithm should learn (y = 0.25).
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {'x': array_ops.ones(shape=[4, 1], dtype=dtypes.float32),}
      return features, labels

    regressor = dnn.DNNRegressor(
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn_train, steps=5)
    scores = regressor.evaluate(input_fn=_input_fn_train, steps=1)
    self.assertIn('loss', scores)

  def testLossWithWeights(self):
    """Tests loss calculation with weights."""

    def _input_fn_train():
      # 4 rows with equal weight, one of them (y = x), three of them (y=Not(x))
      # The algorithm should learn (y = 0.25).
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[1.], [1.], [1.], [1.]])
      }
      return features, labels

    def _input_fn_eval():
      # 4 rows, with different weights.
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[7.], [1.], [1.], [1.]])
      }
      return features, labels

    regressor = dnn.DNNRegressor(
        weight_column_name='w',
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn_train, steps=5)
    scores = regressor.evaluate(input_fn=_input_fn_eval, steps=1)
    self.assertIn('loss', scores)

  def testTrainWithWeights(self):
    """Tests training with given weight column."""

    def _input_fn_train():
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      # First row has more weight than others. Model should fit (y=x) better
      # than (y=Not(x)) due to the relative higher weight of the first row.
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[100.], [3.], [2.], [2.]])
      }
      return features, labels

    def _input_fn_eval():
      # Create 4 rows (y = x)
      labels = constant_op.constant([[1.], [1.], [1.], [1.]])
      features = {
          'x': array_ops.ones(
              shape=[4, 1], dtype=dtypes.float32),
          'w': constant_op.constant([[1.], [1.], [1.], [1.]])
      }
      return features, labels

    regressor = dnn.DNNRegressor(
        weight_column_name='w',
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn_train, steps=5)
    scores = regressor.evaluate(input_fn=_input_fn_eval, steps=1)
    self.assertIn('loss', scores)

  def _assertRegressionOutputs(
      self, predictions, expected_shape):
    predictions_nparray = np.array(predictions)
    self.assertAllEqual(expected_shape, predictions_nparray.shape)
    self.assertTrue(np.issubdtype(predictions_nparray.dtype, np.floating))

  def testPredict_AsIterableFalse(self):
    """Tests predict method with as_iterable=False."""
    labels = [1., 0., 0.2]

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant(labels, dtype=dtypes.float32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=200)

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)
    predicted_scores = regressor.predict_scores(
        input_fn=_input_fn, as_iterable=False)
    self._assertRegressionOutputs(predicted_scores, [3])
    predictions = regressor.predict(input_fn=_input_fn, as_iterable=False)
    self.assertAllClose(predicted_scores, predictions)

  def testPredict_AsIterable(self):
    """Tests predict method with as_iterable=True."""
    labels = [1., 0., 0.2]

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant(labels, dtype=dtypes.float32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=200)

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predicted_scores = list(
        regressor.predict_scores(
            input_fn=predict_input_fn, as_iterable=True))
    self._assertRegressionOutputs(predicted_scores, [3])
    predictions = list(
        regressor.predict(input_fn=predict_input_fn, as_iterable=True))
    self.assertAllClose(predicted_scores, predictions)

  def testCustomMetrics(self):
    """Tests custom evaluation metrics."""

    def _input_fn(num_epochs=None):
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x':
              input_lib.limit_epochs(
                  array_ops.ones(
                      shape=[4, 1], dtype=dtypes.float32),
                  num_epochs=num_epochs),
      }
      return features, labels

    def _my_metric_op(predictions, labels):
      return math_ops.reduce_sum(math_ops.multiply(predictions, labels))

    regressor = dnn.DNNRegressor(
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=5)
    scores = regressor.evaluate(
        input_fn=_input_fn,
        steps=1,
        metrics={
            'my_error': metric_ops.streaming_mean_squared_error,
            ('my_metric', 'scores'): _my_metric_op
        })
    self.assertIn('loss', set(scores.keys()))
    self.assertIn('my_error', set(scores.keys()))
    self.assertIn('my_metric', set(scores.keys()))
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predictions = np.array(list(regressor.predict_scores(
        input_fn=predict_input_fn)))
    self.assertAlmostEqual(
        _sklearn.mean_squared_error(np.array([1, 0, 0, 0]), predictions),
        scores['my_error'])

    # Tests the case that the 2nd element of the key is not "scores".
    with self.assertRaises(KeyError):
      regressor.evaluate(
          input_fn=_input_fn,
          steps=1,
          metrics={
              ('my_error', 'predictions'):
                  metric_ops.streaming_mean_squared_error
          })

    # Tests the case where the tuple of the key doesn't have 2 elements.
    with self.assertRaises(ValueError):
      regressor.evaluate(
          input_fn=_input_fn,
          steps=1,
          metrics={
              ('bad_length_name', 'scores', 'bad_length'):
                  metric_ops.streaming_mean_squared_error
          })

  def testCustomMetricsWithMetricSpec(self):
    """Tests custom evaluation metrics that use MetricSpec."""

    def _input_fn(num_epochs=None):
      # Create 4 rows, one of them (y = x), three of them (y=Not(x))
      labels = constant_op.constant([[1.], [0.], [0.], [0.]])
      features = {
          'x':
              input_lib.limit_epochs(
                  array_ops.ones(
                      shape=[4, 1], dtype=dtypes.float32),
                  num_epochs=num_epochs),
      }
      return features, labels

    def _my_metric_op(predictions, labels):
      return math_ops.reduce_sum(math_ops.multiply(predictions, labels))

    regressor = dnn.DNNRegressor(
        feature_columns=[feature_column.real_valued_column('x')],
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=5)
    scores = regressor.evaluate(
        input_fn=_input_fn,
        steps=1,
        metrics={
            'my_error':
                MetricSpec(
                    metric_fn=metric_ops.streaming_mean_squared_error,
                    prediction_key='scores'),
            'my_metric':
                MetricSpec(
                    metric_fn=_my_metric_op, prediction_key='scores')
        })
    self.assertIn('loss', set(scores.keys()))
    self.assertIn('my_error', set(scores.keys()))
    self.assertIn('my_metric', set(scores.keys()))
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predictions = np.array(list(regressor.predict_scores(
        input_fn=predict_input_fn)))
    self.assertAlmostEqual(
        _sklearn.mean_squared_error(np.array([1, 0, 0, 0]), predictions),
        scores['my_error'])

    # Tests the case where the prediction_key is not "scores".
    with self.assertRaisesRegexp(KeyError, 'bad_type'):
      regressor.evaluate(
          input_fn=_input_fn,
          steps=1,
          metrics={
              'bad_name':
                  MetricSpec(
                      metric_fn=metric_ops.streaming_auc,
                      prediction_key='bad_type')
          })

  def testTrainSaveLoad(self):
    """Tests that insures you can save and reload a trained model."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([1., 0., 0.2], dtype=dtypes.float32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    model_dir = tempfile.mkdtemp()
    regressor = dnn.DNNRegressor(
        model_dir=model_dir,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=5)
    predict_input_fn = functools.partial(_input_fn, num_epochs=1)
    predictions = list(regressor.predict_scores(input_fn=predict_input_fn))
    del regressor

    regressor2 = dnn.DNNRegressor(
        model_dir=model_dir,
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        config=run_config.RunConfig(tf_random_seed=1))
    predictions2 = list(regressor2.predict_scores(input_fn=predict_input_fn))
    self.assertAllClose(predictions, predictions2)

  def testTrainWithPartitionedVariables(self):
    """Tests training with partitioned variables."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([1., 0., 0.2], dtype=dtypes.float32)

    # The given hash_bucket_size results in variables larger than the
    # default min_slice_size attribute, so the variables are partitioned.
    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=2e7)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    tf_config = {
        'cluster': {
            run_config.TaskType.PS: ['fake_ps_0', 'fake_ps_1']
        }
    }
    with test.mock.patch.dict('os.environ',
                              {'TF_CONFIG': json.dumps(tf_config)}):
      config = run_config.RunConfig(tf_random_seed=1)
      # Because we did not start a distributed cluster, we need to pass an
      # empty ClusterSpec, otherwise the device_setter will look for
      # distributed jobs, such as "/job:ps" which are not present.
      config._cluster_spec = server_lib.ClusterSpec({})

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns, hidden_units=[3, 3], config=config)

    regressor.fit(input_fn=_input_fn, steps=5)

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)

  def testEnableCenteredBias(self):
    """Tests that we can enable centered bias."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([1., 0., 0.2], dtype=dtypes.float32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        enable_centered_bias=True,
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=5)
    self.assertIn('dnn/regression_head/centered_bias_weight',
                  regressor.get_variable_names())

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)

  def testDisableCenteredBias(self):
    """Tests that we can disable centered bias."""

    def _input_fn(num_epochs=None):
      features = {
          'age':
              input_lib.limit_epochs(
                  constant_op.constant([[0.8], [0.15], [0.]]),
                  num_epochs=num_epochs),
          'language':
              sparse_tensor.SparseTensor(
                  values=input_lib.limit_epochs(
                      ['en', 'fr', 'zh'], num_epochs=num_epochs),
                  indices=[[0, 0], [0, 1], [2, 0]],
                  dense_shape=[3, 2])
      }
      return features, constant_op.constant([1., 0., 0.2], dtype=dtypes.float32)

    sparse_column = feature_column.sparse_column_with_hash_bucket(
        'language', hash_bucket_size=20)
    feature_columns = [
        feature_column.embedding_column(
            sparse_column, dimension=1),
        feature_column.real_valued_column('age')
    ]

    regressor = dnn.DNNRegressor(
        feature_columns=feature_columns,
        hidden_units=[3, 3],
        enable_centered_bias=False,
        config=run_config.RunConfig(tf_random_seed=1))

    regressor.fit(input_fn=_input_fn, steps=5)
    self.assertNotIn('centered_bias_weight', regressor.get_variable_names())

    scores = regressor.evaluate(input_fn=_input_fn, steps=1)
    self.assertIn('loss', scores)


def boston_input_fn():
  boston = base.load_boston()
  features = math_ops.cast(
      array_ops.reshape(constant_op.constant(boston.data), [-1, 13]),
      dtypes.float32)
  labels = math_ops.cast(
      array_ops.reshape(constant_op.constant(boston.target), [-1, 1]),
      dtypes.float32)
  return features, labels


class FeatureColumnTest(test.TestCase):

  def testTrain(self):
    feature_columns = estimator.infer_real_valued_columns_from_input_fn(
        boston_input_fn)
    est = dnn.DNNRegressor(feature_columns=feature_columns, hidden_units=[3, 3])
    est.fit(input_fn=boston_input_fn, steps=1)
    _ = est.evaluate(input_fn=boston_input_fn, steps=1)


if __name__ == '__main__':
  test.main()
