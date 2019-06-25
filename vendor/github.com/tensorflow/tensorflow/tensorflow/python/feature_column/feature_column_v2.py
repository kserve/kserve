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
"""This API defines FeatureColumn abstraction.

FeatureColumns provide a high level abstraction for ingesting and representing
features. FeatureColumns are also the primary way of encoding features for
canned `tf.estimator.Estimator`s.

When using FeatureColumns with `Estimators`, the type of feature column you
should choose depends on (1) the feature type and (2) the model type.

1. Feature type:

  * Continuous features can be represented by `numeric_column`.
  * Categorical features can be represented by any `categorical_column_with_*`
  column:
    - `categorical_column_with_vocabulary_list`
    - `categorical_column_with_vocabulary_file`
    - `categorical_column_with_hash_bucket`
    - `categorical_column_with_identity`
    - `weighted_categorical_column`

2. Model type:

  * Deep neural network models (`DNNClassifier`, `DNNRegressor`).

    Continuous features can be directly fed into deep neural network models.

      age_column = numeric_column("age")

    To feed sparse features into DNN models, wrap the column with
    `embedding_column` or `indicator_column`. `indicator_column` is recommended
    for features with only a few possible values. For features with many
    possible values, to reduce the size of your model, `embedding_column` is
    recommended.

      embedded_dept_column = embedding_column(
          categorical_column_with_vocabulary_list(
              "department", ["math", "philosophy", ...]), dimension=10)

  * Wide (aka linear) models (`LinearClassifier`, `LinearRegressor`).

    Sparse features can be fed directly into linear models. They behave like an
    indicator column but with an efficient implementation.

      dept_column = categorical_column_with_vocabulary_list("department",
          ["math", "philosophy", "english"])

    It is recommended that continuous features be bucketized before being
    fed into linear models.

      bucketized_age_column = bucketized_column(
          source_column=age_column,
          boundaries=[18, 25, 30, 35, 40, 45, 50, 55, 60, 65])

    Sparse features can be crossed (also known as conjuncted or combined) in
    order to form non-linearities, and then fed into linear models.

      cross_dept_age_column = crossed_column(
          columns=["department", bucketized_age_column],
          hash_bucket_size=1000)

Example of building canned `Estimator`s using FeatureColumns:

  ```python
  # Define features and transformations
  deep_feature_columns = [age_column, embedded_dept_column]
  wide_feature_columns = [dept_column, bucketized_age_column,
      cross_dept_age_column]

  # Build deep model
  estimator = DNNClassifier(
      feature_columns=deep_feature_columns,
      hidden_units=[500, 250, 50])
  estimator.train(...)

  # Or build a wide model
  estimator = LinearClassifier(
      feature_columns=wide_feature_columns)
  estimator.train(...)

  # Or build a wide and deep model!
  estimator = DNNLinearCombinedClassifier(
      linear_feature_columns=wide_feature_columns,
      dnn_feature_columns=deep_feature_columns,
      dnn_hidden_units=[500, 250, 50])
  estimator.train(...)
  ```


FeatureColumns can also be transformed into a generic input layer for
custom models using `input_layer`.

Example of building model using FeatureColumns, this can be used in a
`model_fn` which is given to the {tf.estimator.Estimator}:

  ```python
  # Building model via layers

  deep_feature_columns = [age_column, embedded_dept_column]
  columns_to_tensor = parse_feature_columns_from_examples(
      serialized=my_data,
      feature_columns=deep_feature_columns)
  first_layer = input_layer(
      features=columns_to_tensor,
      feature_columns=deep_feature_columns)
  second_layer = fully_connected(first_layer, ...)
  ```

NOTE: Functions prefixed with "_" indicate experimental or private parts of
the API subject to change, and should not be relied upon!
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import abc
import collections
import math

import numpy as np
import six


from tensorflow.python.eager import context
from tensorflow.python.feature_column import feature_column as fc_old
from tensorflow.python.framework import dtypes
from tensorflow.python.framework import ops
from tensorflow.python.framework import sparse_tensor as sparse_tensor_lib
from tensorflow.python.framework import tensor_shape
from tensorflow.python.keras.engine import training
from tensorflow.python.keras.engine.base_layer import Layer
# TODO(b/118385027): Dependency on keras can be problematic if Keras moves out
# of the main repo.
from tensorflow.python.keras import utils
from tensorflow.python.ops import array_ops
from tensorflow.python.ops import check_ops
from tensorflow.python.ops import control_flow_ops
from tensorflow.python.ops import embedding_ops
from tensorflow.python.ops import init_ops
from tensorflow.python.ops import lookup_ops
from tensorflow.python.ops import math_ops
from tensorflow.python.ops import nn_ops
from tensorflow.python.ops import parsing_ops
from tensorflow.python.ops import sparse_ops
from tensorflow.python.ops import string_ops
from tensorflow.python.ops import variable_scope
from tensorflow.python.ops import variables
from tensorflow.python.platform import gfile
from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.training import checkpoint_utils
from tensorflow.python.training.checkpointable import tracking
from tensorflow.python.util import deprecation
from tensorflow.python.util import nest
from tensorflow.python.util.tf_export import tf_export


_FEATURE_COLUMN_DEPRECATION_DATE = '2018-11-30'
_FEATURE_COLUMN_DEPRECATION = ('The old _FeatureColumn APIs are being '
                               'deprecated. Please use the new FeatureColumn '
                               'APIs instead.')


class StateManager(object):
  """Manages the state associated with FeatureColumns.

  Some `FeatureColumn`s create variables or resources to assist their
  computation. The `StateManager` is responsible for creating and storing these
  objects since `FeatureColumn`s are supposed to be stateless configuration
  only.
  """

  def create_variable(self,
                      feature_column,
                      name,
                      shape,
                      dtype=None,
                      trainable=True,
                      use_resource=True,
                      initializer=None):
    """Creates a new variable.

    Args:
      feature_column: A `FeatureColumn` object this variable corresponds to.
      name: variable name.
      shape: variable shape.
      dtype: The type of the variable. Defaults to `self.dtype` or `float32`.
      trainable: Whether this variable is trainable or not.
      use_resource: If true, we use resource variables. Otherwise we use
        RefVariable.
      initializer: initializer instance (callable).

    Returns:
      The created variable.
    """
    del feature_column, name, shape, dtype, trainable, use_resource, initializer
    raise NotImplementedError('StateManager.create_variable')

  def add_variable(self, feature_column, var):
    """Adds an existing variable to the state.

    Args:
      feature_column: A `FeatureColumn` object to associate this variable with.
      var: The variable.
    """
    del feature_column, var
    raise NotImplementedError('StateManager.add_variable')

  def get_variable(self, feature_column, name):
    """Returns an existing variable.

    Args:
      feature_column: A `FeatureColumn` object this variable corresponds to.
      name: variable name.
    """
    del feature_column, name
    raise NotImplementedError('StateManager.get_var')

  def add_resource(self, feature_column, name, resource):
    """Creates a new resource.

    Resources can be things such as tables etc.

    Args:
      feature_column: A `FeatureColumn` object this resource corresponds to.
      name: Name of the resource.
      resource: The resource.

    Returns:
      The created resource.
    """
    del feature_column, name, resource
    raise NotImplementedError('StateManager.add_resource')

  def get_resource(self, feature_column, name):
    """Returns an already created resource.

    Resources can be things such as tables etc.

    Args:
      feature_column: A `FeatureColumn` object this variable corresponds to.
      name: Name of the resource.
    """
    del feature_column, name
    raise NotImplementedError('StateManager.get_resource')


class _StateManagerImpl(StateManager):
  """Manages the state of DenseFeatures and LinearLayer."""

  def __init__(self, layer, trainable):
    """Creates an _StateManagerImpl object.

    Args:
      layer: The input layer this state manager is associated with.
      trainable: Whether by default, variables created are trainable or not.
    """
    self._trainable = trainable
    self._layer = layer
    self._cols_to_vars_map = collections.defaultdict(lambda: {})

  def create_variable(self,
                      feature_column,
                      name,
                      shape,
                      dtype=None,
                      trainable=True,
                      use_resource=True,
                      initializer=None):
    if name in self._cols_to_vars_map[feature_column]:
      raise ValueError('Variable already exists.')

    var = self._layer.add_variable(
        name=name,
        shape=shape,
        dtype=dtype,
        initializer=initializer,
        trainable=self._trainable and trainable,
        use_resource=use_resource,
        # TODO(rohanj): Get rid of this hack once we have a mechanism for
        # specifying a default partitioner for an entire layer. In that case,
        # the default getter for Layers should work.
        getter=variable_scope.get_variable)
    self._cols_to_vars_map[feature_column][name] = var
    return var

  def get_variable(self, feature_column, name):
    if name in self._cols_to_vars_map[feature_column]:
      return self._cols_to_vars_map[feature_column][name]
    raise ValueError('Variable does not exist.')


@tf_export('keras.layers.DenseFeatures', v1=[])
class DenseFeatures(Layer):
  """A layer that produces a dense `Tensor` based on given `feature_columns`.

  Generally a single example in training data is described with FeatureColumns.
  At the first layer of the model, this column oriented data should be converted
  to a single `Tensor`.

  This layer can be called multiple times with different features.

  Example:

  ```python
  price = numeric_column('price')
  keywords_embedded = embedding_column(
      categorical_column_with_hash_bucket("keywords", 10K), dimensions=16)
  columns = [price, keywords_embedded, ...]
  feature_layer = DenseFeatures(columns)

  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = feature_layer(features)
  for units in [128, 64, 32]:
    dense_tensor = tf.layers.dense(dense_tensor, units, tf.nn.relu)
  prediction = tf.layers.dense(dense_tensor, 1).
  ```
  """

  def __init__(self,
               feature_columns,
               trainable=True,
               name=None,
               **kwargs):
    """Constructs a DenseFeatures.

    Args:
      feature_columns: An iterable containing the FeatureColumns to use as
        inputs to your model. All items should be instances of classes derived
        from `DenseColumn` such as `numeric_column`, `embedding_column`,
        `bucketized_column`, `indicator_column`. If you have categorical
        features, you can wrap them with an `embedding_column` or
        `indicator_column`.
      trainable: If `True` also add the variable to the graph collection
        `GraphKeys.TRAINABLE_VARIABLES` (see `tf.Variable`).
      name: Name to give to the DenseFeatures.
      **kwargs: Keyword arguments to construct a layer.

    Raises:
      ValueError: if an item in `feature_columns` is not a `DenseColumn`.
    """
    super(DenseFeatures, self).__init__(
        name=name, trainable=trainable, **kwargs)

    self._feature_columns = _normalize_feature_columns(feature_columns)
    self._feature_columns = sorted(self._feature_columns, key=lambda x: x.name)
    self._state_manager = _StateManagerImpl(self, self.trainable)
    for column in self._feature_columns:
      if not isinstance(column, DenseColumn):
        raise ValueError(
            'Items of feature_columns must be a DenseColumn. '
            'You can wrap a categorical column with an '
            'embedding_column or indicator_column. Given: {}'.format(column))

  @property
  def _is_feature_layer(self):
    return True

  def build(self, _):
    for column in self._feature_columns:
      with variable_scope._pure_variable_scope(self.name):  # pylint: disable=protected-access
        with variable_scope._pure_variable_scope(column.name):  # pylint: disable=protected-access
          column.create_state(self._state_manager)
      super(DenseFeatures, self).build(None)

  def call(self, features, cols_to_output_tensors=None):
    """Returns a dense tensor corresponding to the `feature_columns`.

    Args:
      features: A mapping from key to tensors. `FeatureColumn`s look up via
        these keys. For example `numeric_column('price')` will look at 'price'
        key in this dict. Values can be a `SparseTensor` or a `Tensor` depends
        on corresponding `FeatureColumn`.
      cols_to_output_tensors: If not `None`, this will be filled with a dict
        mapping feature columns to output tensors created.

    Returns:
      A `Tensor` which represents input layer of a model. Its shape
      is (batch_size, first_layer_dimension) and its dtype is `float32`.
      first_layer_dimension is determined based on given `feature_columns`.

    Raises:
      ValueError: If features are not a dictionary.
    """
    if not isinstance(features, dict):
      raise ValueError('We expected a dictionary here. Instead we got: ',
                       features)
    transformation_cache = FeatureTransformationCache(features)
    output_tensors = []
    ordered_columns = []
    for column in self._feature_columns:
      with ops.name_scope(column.name):
        ordered_columns.append(column)
        tensor = column.get_dense_tensor(transformation_cache,
                                         self._state_manager)
        num_elements = column.variable_shape.num_elements()
        batch_size = array_ops.shape(tensor)[0]
        tensor = array_ops.reshape(tensor, shape=(batch_size, num_elements))
        output_tensors.append(tensor)
        if cols_to_output_tensors is not None:
          cols_to_output_tensors[column] = tensor

    _verify_static_batch_size_equality(output_tensors, ordered_columns)
    return array_ops.concat(output_tensors, 1)

  def compute_output_shape(self, input_shape):
    total_elements = 0
    for column in self._feature_columns:
      total_elements += column.variable_shape.num_elements()
    return (input_shape[0], total_elements)


class _LinearModelLayer(Layer):
  """Layer that contains logic for `LinearModel`."""

  def __init__(self,
               feature_columns,
               units=1,
               sparse_combiner='sum',
               trainable=True,
               name=None,
               **kwargs):
    super(_LinearModelLayer, self).__init__(
        name=name, trainable=trainable, **kwargs)

    self._feature_columns = _normalize_feature_columns(feature_columns)
    self._feature_columns = sorted(self._feature_columns, key=lambda x: x.name)
    for column in self._feature_columns:
      if not isinstance(column, (DenseColumn, CategoricalColumn)):
        raise ValueError(
            'Items of feature_columns must be either a '
            'DenseColumn or CategoricalColumn. Given: {}'.format(column))

    self._units = units
    self._sparse_combiner = sparse_combiner

    self._state_manager = _StateManagerImpl(self, self.trainable)
    self.bias = None

  def build(self, _):
    # We need variable scopes for now because we want the variable partitioning
    # information to percolate down. We also use _pure_variable_scope's here
    # since we want to open up a name_scope in the `call` method while creating
    # the ops.
    with variable_scope._pure_variable_scope(self.name):  # pylint: disable=protected-access
      for column in self._feature_columns:
        with variable_scope._pure_variable_scope(column.name):  # pylint: disable=protected-access
          # Create the state for each feature column
          column.create_state(self._state_manager)

          # Create a weight variable for each column.
          if isinstance(column, CategoricalColumn):
            first_dim = column.num_buckets
          else:
            first_dim = column.variable_shape.num_elements()
          self._state_manager.create_variable(
              column,
              name='weights',
              dtype=dtypes.float32,
              shape=(first_dim, self._units),
              initializer=init_ops.zeros_initializer(),
              trainable=self.trainable)

      # Create a bias variable.
      self.bias = self.add_variable(
          name='bias_weights',
          dtype=dtypes.float32,
          shape=[self._units],
          initializer=init_ops.zeros_initializer(),
          trainable=self.trainable,
          use_resource=True,
          # TODO(rohanj): Get rid of this hack once we have a mechanism for
          # specifying a default partitioner for an entire layer. In that case,
          # the default getter for Layers should work.
          getter=variable_scope.get_variable)

    super(_LinearModelLayer, self).build(None)

  def call(self, features):
    if not isinstance(features, dict):
      raise ValueError('We expected a dictionary here. Instead we got: {}'
                       .format(features))
    with ops.name_scope(self.name):
      transformation_cache = FeatureTransformationCache(features)
      weighted_sums = []
      for column in self._feature_columns:
        with ops.name_scope(column.name):
          # All the weights used in the linear model are owned by the state
          # manager associated with this Linear Model.
          weight_var = self._state_manager.get_variable(column, 'weights')

          weighted_sum = _create_weighted_sum(
              column=column,
              transformation_cache=transformation_cache,
              state_manager=self._state_manager,
              sparse_combiner=self._sparse_combiner,
              weight_var=weight_var)
          weighted_sums.append(weighted_sum)

      _verify_static_batch_size_equality(weighted_sums, self._feature_columns)
      predictions_no_bias = math_ops.add_n(
          weighted_sums, name='weighted_sum_no_bias')
      predictions = nn_ops.bias_add(
          predictions_no_bias, self.bias, name='weighted_sum')
      return predictions


@tf_export('keras.layers.LinearModel', v1=[])
class LinearModel(training.Model):
  """Produces a linear prediction `Tensor` based on given `feature_columns`.

  This layer generates a weighted sum based on output dimension `units`.
  Weighted sum refers to logits in classification problems. It refers to the
  prediction itself for linear regression problems.

  Note on supported columns: `LinearLayer` treats categorical columns as
  `indicator_column`s. To be specific, assume the input as `SparseTensor` looks
  like:

  ```python
    shape = [2, 2]
    {
        [0, 0]: "a"
        [1, 0]: "b"
        [1, 1]: "c"
    }
  ```
  `linear_model` assigns weights for the presence of "a", "b", "c' implicitly,
  just like `indicator_column`, while `input_layer` explicitly requires wrapping
  each of categorical columns with an `embedding_column` or an
  `indicator_column`.

  Example of usage:

  ```python
  price = numeric_column('price')
  price_buckets = bucketized_column(price, boundaries=[0., 10., 100., 1000.])
  keywords = categorical_column_with_hash_bucket("keywords", 10K)
  keywords_price = crossed_column('keywords', price_buckets, ...)
  columns = [price_buckets, keywords, keywords_price ...]
  linear_model = LinearLayer(columns)

  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  prediction = linear_model(features)
  ```
  """

  def __init__(self,
               feature_columns,
               units=1,
               sparse_combiner='sum',
               trainable=True,
               name=None,
               **kwargs):
    """Constructs a LinearLayer.

    Args:
      feature_columns: An iterable containing the FeatureColumns to use as
        inputs to your model. All items should be instances of classes derived
        from `_FeatureColumn`s.
      units: An integer, dimensionality of the output space. Default value is 1.
      sparse_combiner: A string specifying how to reduce if a categorical column
        is multivalent. Except `numeric_column`, almost all columns passed to
        `linear_model` are considered as categorical columns.  It combines each
        categorical column independently. Currently "mean", "sqrtn" and "sum"
        are supported, with "sum" the default for linear model. "sqrtn" often
        achieves good accuracy, in particular with bag-of-words columns.
          * "sum": do not normalize features in the column
          * "mean": do l1 normalization on features in the column
          * "sqrtn": do l2 normalization on features in the column
        For example, for two features represented as the categorical columns:

          ```python
          # Feature 1

          shape = [2, 2]
          {
              [0, 0]: "a"
              [0, 1]: "b"
              [1, 0]: "c"
          }

          # Feature 2

          shape = [2, 3]
          {
              [0, 0]: "d"
              [1, 0]: "e"
              [1, 1]: "f"
              [1, 2]: "g"
          }
          ```

        with `sparse_combiner` as "mean", the linear model outputs conceptly are
        ```
        y_0 = 1.0 / 2.0 * ( w_a + w_ b) + w_c + b_0
        y_1 = w_d + 1.0 / 3.0 * ( w_e + w_ f + w_g) + b_1
        ```
        where `y_i` is the output, `b_i` is the bias, and `w_x` is the weight
        assigned to the presence of `x` in the input features.
      trainable: If `True` also add the variable to the graph collection
        `GraphKeys.TRAINABLE_VARIABLES` (see `tf.Variable`).
      name: Name to give to the Linear Model. All variables and ops created will
        be scoped by this name.
      **kwargs: Keyword arguments to construct a layer.

    Raises:
      ValueError: if an item in `feature_columns` is neither a `DenseColumn`
        nor `CategoricalColumn`.
    """

    super(LinearModel, self).__init__(name=name, **kwargs)
    self.layer = _LinearModelLayer(
        feature_columns,
        units,
        sparse_combiner,
        trainable,
        name=self.name,
        **kwargs)

  def call(self, features):
    """Returns a `Tensor` the represents the predictions of a linear model.

    Args:
      features: A mapping from key to tensors. `_FeatureColumn`s look up via
        these keys. For example `numeric_column('price')` will look at 'price'
        key in this dict. Values are `Tensor` or `SparseTensor` depending on
        corresponding `_FeatureColumn`.

    Returns:
      A `Tensor` which represents predictions/logits of a linear model. Its
      shape is (batch_size, units) and its dtype is `float32`.

    Raises:
      ValueError: If features are not a dictionary.
    """
    return self.layer(features)

  @property
  def bias(self):
    return self.layer.bias


def _transform_features_v2(features, feature_columns, state_manager):
  """Returns transformed features based on features columns passed in.

  Please note that most probably you would not need to use this function. Please
  check `input_layer` and `linear_model` to see whether they will
  satisfy your use case or not.

  Example:

  ```python
  # Define features and transformations
  crosses_a_x_b = crossed_column(
      columns=["sparse_feature_a", "sparse_feature_b"], hash_bucket_size=10000)
  price_buckets = bucketized_column(
      source_column=numeric_column("price"), boundaries=[...])

  columns = [crosses_a_x_b, price_buckets]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  transformed = transform_features(features=features, feature_columns=columns)

  assertCountEqual(columns, transformed.keys())
  ```

  Args:
    features: A mapping from key to tensors. `FeatureColumn`s look up via these
      keys. For example `numeric_column('price')` will look at 'price' key in
      this dict. Values can be a `SparseTensor` or a `Tensor` depends on
      corresponding `FeatureColumn`.
    feature_columns: An iterable containing all the `FeatureColumn`s.
    state_manager: A StateManager object that holds the FeatureColumn state.

  Returns:
    A `dict` mapping `FeatureColumn` to `Tensor` and `SparseTensor` values.
  """
  feature_columns = _normalize_feature_columns(feature_columns)
  outputs = {}
  with ops.name_scope(
      None, default_name='transform_features', values=features.values()):
    transformation_cache = FeatureTransformationCache(features)
    for column in sorted(feature_columns, key=lambda x: x.name):
      with ops.name_scope(None, default_name=column.name):
        outputs[column] = transformation_cache.get(column, state_manager)
  return outputs


@tf_export('feature_column.make_parse_example_spec', v1=[])
def make_parse_example_spec_v2(feature_columns):
  """Creates parsing spec dictionary from input feature_columns.

  The returned dictionary can be used as arg 'features' in `tf.parse_example`.

  Typical usage example:

  ```python
  # Define features and transformations
  feature_a = categorical_column_with_vocabulary_file(...)
  feature_b = numeric_column(...)
  feature_c_bucketized = bucketized_column(numeric_column("feature_c"), ...)
  feature_a_x_feature_c = crossed_column(
      columns=["feature_a", feature_c_bucketized], ...)

  feature_columns = set(
      [feature_b, feature_c_bucketized, feature_a_x_feature_c])
  features = tf.parse_example(
      serialized=serialized_examples,
      features=make_parse_example_spec(feature_columns))
  ```

  For the above example, make_parse_example_spec would return the dict:

  ```python
  {
      "feature_a": parsing_ops.VarLenFeature(tf.string),
      "feature_b": parsing_ops.FixedLenFeature([1], dtype=tf.float32),
      "feature_c": parsing_ops.FixedLenFeature([1], dtype=tf.float32)
  }
  ```

  Args:
    feature_columns: An iterable containing all feature columns. All items
      should be instances of classes derived from `FeatureColumn`.

  Returns:
    A dict mapping each feature key to a `FixedLenFeature` or `VarLenFeature`
    value.

  Raises:
    ValueError: If any of the given `feature_columns` is not a `FeatureColumn`
      instance.
  """
  result = {}
  for column in feature_columns:
    if not isinstance(column, FeatureColumn):
      raise ValueError('All feature_columns must be FeatureColumn instances. '
                       'Given: {}'.format(column))
    config = column.parse_example_spec
    for key, value in six.iteritems(config):
      if key in result and value != result[key]:
        raise ValueError(
            'feature_columns contain different parse_spec for key '
            '{}. Given {} and {}'.format(key, value, result[key]))
    result.update(config)
  return result


@tf_export('feature_column.embedding_column')
def embedding_column(categorical_column,
                     dimension,
                     combiner='mean',
                     initializer=None,
                     ckpt_to_load_from=None,
                     tensor_name_in_ckpt=None,
                     max_norm=None,
                     trainable=True):
  """`DenseColumn` that converts from sparse, categorical input.

  Use this when your inputs are sparse, but you want to convert them to a dense
  representation (e.g., to feed to a DNN).

  Inputs must be a `CategoricalColumn` created by any of the
  `categorical_column_*` function. Here is an example of using
  `embedding_column` with `DNNClassifier`:

  ```python
  video_id = categorical_column_with_identity(
      key='video_id', num_buckets=1000000, default_value=0)
  columns = [embedding_column(video_id, 9),...]

  estimator = tf.estimator.DNNClassifier(feature_columns=columns, ...)

  label_column = ...
  def input_fn():
    features = tf.parse_example(
        ..., features=make_parse_example_spec(columns + [label_column]))
    labels = features.pop(label_column.name)
    return features, labels

  estimator.train(input_fn=input_fn, steps=100)
  ```

  Here is an example using `embedding_column` with model_fn:

  ```python
  def model_fn(features, ...):
    video_id = categorical_column_with_identity(
        key='video_id', num_buckets=1000000, default_value=0)
    columns = [embedding_column(video_id, 9),...]
    dense_tensor = input_layer(features, columns)
    # Form DNN layers, calculate loss, and return EstimatorSpec.
    ...
  ```

  Args:
    categorical_column: A `CategoricalColumn` created by a
      `categorical_column_with_*` function. This column produces the sparse IDs
      that are inputs to the embedding lookup.
    dimension: An integer specifying dimension of the embedding, must be > 0.
    combiner: A string specifying how to reduce if there are multiple entries in
      a single row. Currently 'mean', 'sqrtn' and 'sum' are supported, with
      'mean' the default. 'sqrtn' often achieves good accuracy, in particular
      with bag-of-words columns. Each of this can be thought as example level
      normalizations on the column. For more information, see
      `tf.embedding_lookup_sparse`.
    initializer: A variable initializer function to be used in embedding
      variable initialization. If not specified, defaults to
      `tf.truncated_normal_initializer` with mean `0.0` and standard deviation
      `1/sqrt(dimension)`.
    ckpt_to_load_from: String representing checkpoint name/pattern from which to
      restore column weights. Required if `tensor_name_in_ckpt` is not `None`.
    tensor_name_in_ckpt: Name of the `Tensor` in `ckpt_to_load_from` from which
      to restore the column weights. Required if `ckpt_to_load_from` is not
      `None`.
    max_norm: If not `None`, embedding values are l2-normalized to this value.
    trainable: Whether or not the embedding is trainable. Default is True.

  Returns:
    `DenseColumn` that converts from sparse input.

  Raises:
    ValueError: if `dimension` not > 0.
    ValueError: if exactly one of `ckpt_to_load_from` and `tensor_name_in_ckpt`
      is specified.
    ValueError: if `initializer` is specified and is not callable.
    RuntimeError: If eager execution is enabled.
  """
  if (dimension is None) or (dimension < 1):
    raise ValueError('Invalid dimension {}.'.format(dimension))
  if (ckpt_to_load_from is None) != (tensor_name_in_ckpt is None):
    raise ValueError('Must specify both `ckpt_to_load_from` and '
                     '`tensor_name_in_ckpt` or none of them.')

  if (initializer is not None) and (not callable(initializer)):
    raise ValueError('initializer must be callable if specified. '
                     'Embedding of column_name: {}'.format(
                         categorical_column.name))
  if initializer is None:
    initializer = init_ops.truncated_normal_initializer(
        mean=0.0, stddev=1 / math.sqrt(dimension))

  return EmbeddingColumn(
      categorical_column=categorical_column,
      dimension=dimension,
      combiner=combiner,
      initializer=initializer,
      ckpt_to_load_from=ckpt_to_load_from,
      tensor_name_in_ckpt=tensor_name_in_ckpt,
      max_norm=max_norm,
      trainable=trainable)


@tf_export(v1=['feature_column.shared_embedding_columns'])
def shared_embedding_columns(categorical_columns,
                             dimension,
                             combiner='mean',
                             initializer=None,
                             shared_embedding_collection_name=None,
                             ckpt_to_load_from=None,
                             tensor_name_in_ckpt=None,
                             max_norm=None,
                             trainable=True):
  """List of dense columns that convert from sparse, categorical input.

  This is similar to `embedding_column`, except that it produces a list of
  embedding columns that share the same embedding weights.

  Use this when your inputs are sparse and of the same type (e.g. watched and
  impression video IDs that share the same vocabulary), and you want to convert
  them to a dense representation (e.g., to feed to a DNN).

  Inputs must be a list of categorical columns created by any of the
  `categorical_column_*` function. They must all be of the same type and have
  the same arguments except `key`. E.g. they can be
  categorical_column_with_vocabulary_file with the same vocabulary_file. Some or
  all columns could also be weighted_categorical_column.

  Here is an example embedding of two features for a DNNClassifier model:

  ```python
  watched_video_id = categorical_column_with_vocabulary_file(
      'watched_video_id', video_vocabulary_file, video_vocabulary_size)
  impression_video_id = categorical_column_with_vocabulary_file(
      'impression_video_id', video_vocabulary_file, video_vocabulary_size)
  columns = shared_embedding_columns(
      [watched_video_id, impression_video_id], dimension=10)

  estimator = tf.estimator.DNNClassifier(feature_columns=columns, ...)

  label_column = ...
  def input_fn():
    features = tf.parse_example(
        ..., features=make_parse_example_spec(columns + [label_column]))
    labels = features.pop(label_column.name)
    return features, labels

  estimator.train(input_fn=input_fn, steps=100)
  ```

  Here is an example using `shared_embedding_columns` with model_fn:

  ```python
  def model_fn(features, ...):
    watched_video_id = categorical_column_with_vocabulary_file(
        'watched_video_id', video_vocabulary_file, video_vocabulary_size)
    impression_video_id = categorical_column_with_vocabulary_file(
        'impression_video_id', video_vocabulary_file, video_vocabulary_size)
    columns = shared_embedding_columns(
        [watched_video_id, impression_video_id], dimension=10)
    dense_tensor = input_layer(features, columns)
    # Form DNN layers, calculate loss, and return EstimatorSpec.
    ...
  ```

  Args:
    categorical_columns: List of categorical columns created by a
      `categorical_column_with_*` function. These columns produce the sparse IDs
      that are inputs to the embedding lookup. All columns must be of the same
      type and have the same arguments except `key`. E.g. they can be
      categorical_column_with_vocabulary_file with the same vocabulary_file.
      Some or all columns could also be weighted_categorical_column.
    dimension: An integer specifying dimension of the embedding, must be > 0.
    combiner: A string specifying how to reduce if there are multiple entries in
      a single row. Currently 'mean', 'sqrtn' and 'sum' are supported, with
      'mean' the default. 'sqrtn' often achieves good accuracy, in particular
      with bag-of-words columns. Each of this can be thought as example level
      normalizations on the column. For more information, see
      `tf.embedding_lookup_sparse`.
    initializer: A variable initializer function to be used in embedding
      variable initialization. If not specified, defaults to
      `tf.truncated_normal_initializer` with mean `0.0` and standard deviation
      `1/sqrt(dimension)`.
    shared_embedding_collection_name: Optional name of the collection where
      shared embedding weights are added. If not given, a reasonable name will
      be chosen based on the names of `categorical_columns`. This is also used
      in `variable_scope` when creating shared embedding weights.
    ckpt_to_load_from: String representing checkpoint name/pattern from which to
      restore column weights. Required if `tensor_name_in_ckpt` is not `None`.
    tensor_name_in_ckpt: Name of the `Tensor` in `ckpt_to_load_from` from which
      to restore the column weights. Required if `ckpt_to_load_from` is not
      `None`.
    max_norm: If not `None`, each embedding is clipped if its l2-norm is larger
      than this value, before combining.
    trainable: Whether or not the embedding is trainable. Default is True.

  Returns:
    A list of dense columns that converts from sparse input. The order of
    results follows the ordering of `categorical_columns`.

  Raises:
    ValueError: if `dimension` not > 0.
    ValueError: if any of the given `categorical_columns` is of different type
      or has different arguments than the others.
    ValueError: if exactly one of `ckpt_to_load_from` and `tensor_name_in_ckpt`
      is specified.
    ValueError: if `initializer` is specified and is not callable.
    RuntimeError: if eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('shared_embedding_columns are not supported when eager '
                       'execution is enabled.')

  if (dimension is None) or (dimension < 1):
    raise ValueError('Invalid dimension {}.'.format(dimension))
  if (ckpt_to_load_from is None) != (tensor_name_in_ckpt is None):
    raise ValueError('Must specify both `ckpt_to_load_from` and '
                     '`tensor_name_in_ckpt` or none of them.')

  if (initializer is not None) and (not callable(initializer)):
    raise ValueError('initializer must be callable if specified.')
  if initializer is None:
    initializer = init_ops.truncated_normal_initializer(
        mean=0.0, stddev=1. / math.sqrt(dimension))

  # Sort the columns so the default collection name is deterministic even if the
  # user passes columns from an unsorted collection, such as dict.values().
  sorted_columns = sorted(categorical_columns, key=lambda x: x.name)

  c0 = sorted_columns[0]
  num_buckets = c0._num_buckets  # pylint: disable=protected-access
  if not isinstance(c0, fc_old._CategoricalColumn):  # pylint: disable=protected-access
    raise ValueError(
        'All categorical_columns must be subclasses of _CategoricalColumn. '
        'Given: {}, of type: {}'.format(c0, type(c0)))
  if isinstance(c0,
                (fc_old._WeightedCategoricalColumn, WeightedCategoricalColumn)):  # pylint: disable=protected-access
    c0 = c0.categorical_column
  for c in sorted_columns[1:]:
    if isinstance(
        c, (fc_old._WeightedCategoricalColumn, WeightedCategoricalColumn)):  # pylint: disable=protected-access
      c = c.categorical_column
    if not isinstance(c, type(c0)):
      raise ValueError(
          'To use shared_embedding_column, all categorical_columns must have '
          'the same type, or be weighted_categorical_column of the same type. '
          'Given column: {} of type: {} does not match given column: {} of '
          'type: {}'.format(c0, type(c0), c, type(c)))
    if num_buckets != c._num_buckets:  # pylint: disable=protected-access
      raise ValueError(
          'To use shared_embedding_column, all categorical_columns must have '
          'the same number of buckets. Given column: {} with buckets: {} does  '
          'not match column: {} with buckets: {}'.format(
              c0, num_buckets, c, c._num_buckets))  # pylint: disable=protected-access

  if not shared_embedding_collection_name:
    shared_embedding_collection_name = '_'.join(c.name for c in sorted_columns)
    shared_embedding_collection_name += '_shared_embedding'

  result = []
  for column in categorical_columns:
    result.append(
        fc_old._SharedEmbeddingColumn(  # pylint: disable=protected-access
            categorical_column=column,
            initializer=initializer,
            dimension=dimension,
            combiner=combiner,
            shared_embedding_collection_name=shared_embedding_collection_name,
            ckpt_to_load_from=ckpt_to_load_from,
            tensor_name_in_ckpt=tensor_name_in_ckpt,
            max_norm=max_norm,
            trainable=trainable))

  return result


@tf_export('feature_column.shared_embedding_columns', v1=[])
def shared_embedding_columns_v2(categorical_columns,
                                dimension,
                                combiner='mean',
                                initializer=None,
                                shared_embedding_collection_name=None,
                                ckpt_to_load_from=None,
                                tensor_name_in_ckpt=None,
                                max_norm=None,
                                trainable=True):
  """List of dense columns that convert from sparse, categorical input.

  This is similar to `embedding_column`, except that it produces a list of
  embedding columns that share the same embedding weights.

  Use this when your inputs are sparse and of the same type (e.g. watched and
  impression video IDs that share the same vocabulary), and you want to convert
  them to a dense representation (e.g., to feed to a DNN).

  Inputs must be a list of categorical columns created by any of the
  `categorical_column_*` function. They must all be of the same type and have
  the same arguments except `key`. E.g. they can be
  categorical_column_with_vocabulary_file with the same vocabulary_file. Some or
  all columns could also be weighted_categorical_column.

  Here is an example embedding of two features for a DNNClassifier model:

  ```python
  watched_video_id = categorical_column_with_vocabulary_file(
      'watched_video_id', video_vocabulary_file, video_vocabulary_size)
  impression_video_id = categorical_column_with_vocabulary_file(
      'impression_video_id', video_vocabulary_file, video_vocabulary_size)
  columns = shared_embedding_columns(
      [watched_video_id, impression_video_id], dimension=10)

  estimator = tf.estimator.DNNClassifier(feature_columns=columns, ...)

  label_column = ...
  def input_fn():
    features = tf.parse_example(
        ..., features=make_parse_example_spec(columns + [label_column]))
    labels = features.pop(label_column.name)
    return features, labels

  estimator.train(input_fn=input_fn, steps=100)
  ```

  Here is an example using `shared_embedding_columns` with model_fn:

  ```python
  def model_fn(features, ...):
    watched_video_id = categorical_column_with_vocabulary_file(
        'watched_video_id', video_vocabulary_file, video_vocabulary_size)
    impression_video_id = categorical_column_with_vocabulary_file(
        'impression_video_id', video_vocabulary_file, video_vocabulary_size)
    columns = shared_embedding_columns(
        [watched_video_id, impression_video_id], dimension=10)
    dense_tensor = input_layer(features, columns)
    # Form DNN layers, calculate loss, and return EstimatorSpec.
    ...
  ```

  Args:
    categorical_columns: List of categorical columns created by a
      `categorical_column_with_*` function. These columns produce the sparse IDs
      that are inputs to the embedding lookup. All columns must be of the same
      type and have the same arguments except `key`. E.g. they can be
      categorical_column_with_vocabulary_file with the same vocabulary_file.
      Some or all columns could also be weighted_categorical_column.
    dimension: An integer specifying dimension of the embedding, must be > 0.
    combiner: A string specifying how to reduce if there are multiple entries
      in a single row. Currently 'mean', 'sqrtn' and 'sum' are supported, with
      'mean' the default. 'sqrtn' often achieves good accuracy, in particular
      with bag-of-words columns. Each of this can be thought as example level
      normalizations on the column. For more information, see
      `tf.embedding_lookup_sparse`.
    initializer: A variable initializer function to be used in embedding
      variable initialization. If not specified, defaults to
      `tf.truncated_normal_initializer` with mean `0.0` and standard deviation
      `1/sqrt(dimension)`.
    shared_embedding_collection_name: Optional collective name of these columns.
      If not given, a reasonable name will be chosen based on the names of
      `categorical_columns`.
    ckpt_to_load_from: String representing checkpoint name/pattern from which to
      restore column weights. Required if `tensor_name_in_ckpt` is not `None`.
    tensor_name_in_ckpt: Name of the `Tensor` in `ckpt_to_load_from` from
      which to restore the column weights. Required if `ckpt_to_load_from` is
      not `None`.
    max_norm: If not `None`, each embedding is clipped if its l2-norm is
      larger than this value, before combining.
    trainable: Whether or not the embedding is trainable. Default is True.

  Returns:
    A list of dense columns that converts from sparse input. The order of
    results follows the ordering of `categorical_columns`.

  Raises:
    ValueError: if `dimension` not > 0.
    ValueError: if any of the given `categorical_columns` is of different type
      or has different arguments than the others.
    ValueError: if exactly one of `ckpt_to_load_from` and `tensor_name_in_ckpt`
      is specified.
    ValueError: if `initializer` is specified and is not callable.
    RuntimeError: if eager execution is enabled.
  """
  if context.executing_eagerly():
    raise RuntimeError('shared_embedding_columns are not supported when eager '
                       'execution is enabled.')

  if (dimension is None) or (dimension < 1):
    raise ValueError('Invalid dimension {}.'.format(dimension))
  if (ckpt_to_load_from is None) != (tensor_name_in_ckpt is None):
    raise ValueError('Must specify both `ckpt_to_load_from` and '
                     '`tensor_name_in_ckpt` or none of them.')

  if (initializer is not None) and (not callable(initializer)):
    raise ValueError('initializer must be callable if specified.')
  if initializer is None:
    initializer = init_ops.truncated_normal_initializer(
        mean=0.0, stddev=1. / math.sqrt(dimension))

  # Sort the columns so the default collection name is deterministic even if the
  # user passes columns from an unsorted collection, such as dict.values().
  sorted_columns = sorted(categorical_columns, key=lambda x: x.name)

  c0 = sorted_columns[0]
  num_buckets = c0.num_buckets
  if not isinstance(c0, CategoricalColumn):
    raise ValueError(
        'All categorical_columns must be subclasses of CategoricalColumn. '
        'Given: {}, of type: {}'.format(c0, type(c0)))
  if isinstance(c0, WeightedCategoricalColumn):
    c0 = c0.categorical_column
  for c in sorted_columns[1:]:
    if isinstance(c, WeightedCategoricalColumn):
      c = c.categorical_column
    if not isinstance(c, type(c0)):
      raise ValueError(
          'To use shared_embedding_column, all categorical_columns must have '
          'the same type, or be weighted_categorical_column of the same type. '
          'Given column: {} of type: {} does not match given column: {} of '
          'type: {}'.format(c0, type(c0), c, type(c)))
    if num_buckets != c.num_buckets:
      raise ValueError(
          'To use shared_embedding_column, all categorical_columns must have '
          'the same number of buckets. Given column: {} with buckets: {} does  '
          'not match column: {} with buckets: {}'.format(
              c0, num_buckets, c, c.num_buckets))

  if not shared_embedding_collection_name:
    shared_embedding_collection_name = '_'.join(c.name for c in sorted_columns)
    shared_embedding_collection_name += '_shared_embedding'

  column_creator = SharedEmbeddingColumnCreator(
      dimension, initializer, ckpt_to_load_from, tensor_name_in_ckpt,
      num_buckets, trainable, shared_embedding_collection_name)

  result = []
  for column in categorical_columns:
    result.append(
        column_creator(
            categorical_column=column, combiner=combiner, max_norm=max_norm))

  return result


@tf_export('feature_column.numeric_column')
def numeric_column(key,
                   shape=(1,),
                   default_value=None,
                   dtype=dtypes.float32,
                   normalizer_fn=None):
  """Represents real valued or numerical features.

  Example:

  ```python
  price = numeric_column('price')
  columns = [price, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)

  # or
  bucketized_price = bucketized_column(price, boundaries=[...])
  columns = [bucketized_price, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the
      column name and the dictionary key for feature parsing configs, feature
      `Tensor` objects, and feature columns.
    shape: An iterable of integers specifies the shape of the `Tensor`. An
      integer can be given which means a single dimension `Tensor` with given
      width. The `Tensor` representing the column will have the shape of
      [batch_size] + `shape`.
    default_value: A single value compatible with `dtype` or an iterable of
      values compatible with `dtype` which the column takes on during
      `tf.Example` parsing if data is missing. A default value of `None` will
      cause `tf.parse_example` to fail if an example does not contain this
      column. If a single value is provided, the same value will be applied as
      the default value for every item. If an iterable of values is provided,
      the shape of the `default_value` should be equal to the given `shape`.
    dtype: defines the type of values. Default value is `tf.float32`. Must be a
      non-quantized, real integer or floating point type.
    normalizer_fn: If not `None`, a function that can be used to normalize the
      value of the tensor after `default_value` is applied for parsing.
      Normalizer function takes the input `Tensor` as its argument, and returns
      the output `Tensor`. (e.g. lambda x: (x - 3.0) / 4.2). Please note that
      even though the most common use case of this function is normalization, it
      can be used for any kind of Tensorflow transformations.

  Returns:
    A `NumericColumn`.

  Raises:
    TypeError: if any dimension in shape is not an int
    ValueError: if any dimension in shape is not a positive integer
    TypeError: if `default_value` is an iterable but not compatible with `shape`
    TypeError: if `default_value` is not compatible with `dtype`.
    ValueError: if `dtype` is not convertible to `tf.float32`.
  """
  shape = _check_shape(shape, key)
  if not (dtype.is_integer or dtype.is_floating):
    raise ValueError('dtype must be convertible to float. '
                     'dtype: {}, key: {}'.format(dtype, key))
  default_value = _check_default_value(shape, default_value, dtype, key)

  if normalizer_fn is not None and not callable(normalizer_fn):
    raise TypeError(
        'normalizer_fn must be a callable. Given: {}'.format(normalizer_fn))

  _assert_key_is_string(key)
  return NumericColumn(
      key,
      shape=shape,
      default_value=default_value,
      dtype=dtype,
      normalizer_fn=normalizer_fn)


@tf_export('feature_column.bucketized_column')
def bucketized_column(source_column, boundaries):
  """Represents discretized dense input.

  Buckets include the left boundary, and exclude the right boundary. Namely,
  `boundaries=[0., 1., 2.]` generates buckets `(-inf, 0.)`, `[0., 1.)`,
  `[1., 2.)`, and `[2., +inf)`.

  For example, if the inputs are

  ```python
  boundaries = [0, 10, 100]
  input tensor = [[-5, 10000]
                  [150,   10]
                  [5,    100]]
  ```

  then the output will be

  ```python
  output = [[0, 3]
            [3, 2]
            [1, 3]]
  ```

  Example:

  ```python
  price = numeric_column('price')
  bucketized_price = bucketized_column(price, boundaries=[...])
  columns = [bucketized_price, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)

  # or
  columns = [bucketized_price, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  `bucketized_column` can also be crossed with another categorical column using
  `crossed_column`:

  ```python
  price = numeric_column('price')
  # bucketized_column converts numerical feature to a categorical one.
  bucketized_price = bucketized_column(price, boundaries=[...])
  # 'keywords' is a string feature.
  price_x_keywords = crossed_column([bucketized_price, 'keywords'], 50K)
  columns = [price_x_keywords, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  Args:
    source_column: A one-dimensional dense column which is generated with
      `numeric_column`.
    boundaries: A sorted list or tuple of floats specifying the boundaries.

  Returns:
    A `BucketizedColumn`.

  Raises:
    ValueError: If `source_column` is not a numeric column, or if it is not
      one-dimensional.
    ValueError: If `boundaries` is not a sorted list or tuple.
  """
  if not isinstance(source_column, (NumericColumn, fc_old._NumericColumn)):  # pylint: disable=protected-access
    raise ValueError(
        'source_column must be a column generated with numeric_column(). '
        'Given: {}'.format(source_column))
  if len(source_column.shape) > 1:
    raise ValueError(
        'source_column must be one-dimensional column. '
        'Given: {}'.format(source_column))
  if (not boundaries or
      not (isinstance(boundaries, list) or isinstance(boundaries, tuple))):
    raise ValueError('boundaries must be a sorted list.')
  for i in range(len(boundaries) - 1):
    if boundaries[i] >= boundaries[i + 1]:
      raise ValueError('boundaries must be a sorted list.')
  return BucketizedColumn(source_column, tuple(boundaries))


def _assert_string_or_int(dtype, prefix):
  if (dtype != dtypes.string) and (not dtype.is_integer):
    raise ValueError(
        '{} dtype must be string or integer. dtype: {}.'.format(prefix, dtype))


def _assert_key_is_string(key):
  if not isinstance(key, six.string_types):
    raise ValueError(
        'key must be a string. Got: type {}. Given key: {}.'.format(
            type(key), key))


@tf_export('feature_column.categorical_column_with_hash_bucket')
def categorical_column_with_hash_bucket(key,
                                        hash_bucket_size,
                                        dtype=dtypes.string):
  """Represents sparse feature where ids are set by hashing.

  Use this when your sparse features are in string or integer format, and you
  want to distribute your inputs into a finite number of buckets by hashing.
  output_id = Hash(input_feature_string) % bucket_size for string type input.
  For int type input, the value is converted to its string representation first
  and then hashed by the same formula.

  For input dictionary `features`, `features[key]` is either `Tensor` or
  `SparseTensor`. If `Tensor`, missing values can be represented by `-1` for int
  and `''` for string, which will be dropped by this feature column.

  Example:

  ```python
  keywords = categorical_column_with_hash_bucket("keywords", 10K)
  columns = [keywords, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)

  # or
  keywords_embedded = embedding_column(keywords, 16)
  columns = [keywords_embedded, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the
      column name and the dictionary key for feature parsing configs, feature
      `Tensor` objects, and feature columns.
    hash_bucket_size: An int > 1. The number of buckets.
    dtype: The type of features. Only string and integer types are supported.

  Returns:
    A `HashedCategoricalColumn`.

  Raises:
    ValueError: `hash_bucket_size` is not greater than 1.
    ValueError: `dtype` is neither string nor integer.
  """
  if hash_bucket_size is None:
    raise ValueError('hash_bucket_size must be set. ' 'key: {}'.format(key))

  if hash_bucket_size < 1:
    raise ValueError('hash_bucket_size must be at least 1. '
                     'hash_bucket_size: {}, key: {}'.format(
                         hash_bucket_size, key))

  _assert_key_is_string(key)
  _assert_string_or_int(dtype, prefix='column_name: {}'.format(key))

  return HashedCategoricalColumn(key, hash_bucket_size, dtype)


@tf_export(v1=['feature_column.categorical_column_with_vocabulary_file'])
def categorical_column_with_vocabulary_file(key,
                                            vocabulary_file,
                                            vocabulary_size=None,
                                            num_oov_buckets=0,
                                            default_value=None,
                                            dtype=dtypes.string):
  """A `CategoricalColumn` with a vocabulary file.

  Use this when your inputs are in string or integer format, and you have a
  vocabulary file that maps each value to an integer ID. By default,
  out-of-vocabulary values are ignored. Use either (but not both) of
  `num_oov_buckets` and `default_value` to specify how to include
  out-of-vocabulary values.

  For input dictionary `features`, `features[key]` is either `Tensor` or
  `SparseTensor`. If `Tensor`, missing values can be represented by `-1` for int
  and `''` for string, which will be dropped by this feature column.

  Example with `num_oov_buckets`:
  File '/us/states.txt' contains 50 lines, each with a 2-character U.S. state
  abbreviation. All inputs with values in that file are assigned an ID 0-49,
  corresponding to its line number. All other values are hashed and assigned an
  ID 50-54.

  ```python
  states = categorical_column_with_vocabulary_file(
      key='states', vocabulary_file='/us/states.txt', vocabulary_size=50,
      num_oov_buckets=5)
  columns = [states, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  Example with `default_value`:
  File '/us/states.txt' contains 51 lines - the first line is 'XX', and the
  other 50 each have a 2-character U.S. state abbreviation. Both a literal 'XX'
  in input, and other values missing from the file, will be assigned ID 0. All
  others are assigned the corresponding line number 1-50.

  ```python
  states = categorical_column_with_vocabulary_file(
      key='states', vocabulary_file='/us/states.txt', vocabulary_size=51,
      default_value=0)
  columns = [states, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  And to make an embedding with either:

  ```python
  columns = [embedding_column(states, 3),...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the
      column name and the dictionary key for feature parsing configs, feature
      `Tensor` objects, and feature columns.
    vocabulary_file: The vocabulary file name.
    vocabulary_size: Number of the elements in the vocabulary. This must be no
      greater than length of `vocabulary_file`, if less than length, later
      values are ignored. If None, it is set to the length of `vocabulary_file`.
    num_oov_buckets: Non-negative integer, the number of out-of-vocabulary
      buckets. All out-of-vocabulary inputs will be assigned IDs in the range
      `[vocabulary_size, vocabulary_size+num_oov_buckets)` based on a hash of
      the input value. A positive `num_oov_buckets` can not be specified with
      `default_value`.
    default_value: The integer ID value to return for out-of-vocabulary feature
      values, defaults to `-1`. This can not be specified with a positive
      `num_oov_buckets`.
    dtype: The type of features. Only string and integer types are supported.

  Returns:
    A `CategoricalColumn` with a vocabulary file.

  Raises:
    ValueError: `vocabulary_file` is missing or cannot be opened.
    ValueError: `vocabulary_size` is missing or < 1.
    ValueError: `num_oov_buckets` is a negative integer.
    ValueError: `num_oov_buckets` and `default_value` are both specified.
    ValueError: `dtype` is neither string nor integer.
  """
  return categorical_column_with_vocabulary_file_v2(
      key, vocabulary_file, vocabulary_size,
      dtype, default_value,
      num_oov_buckets)


@tf_export('feature_column.categorical_column_with_vocabulary_file', v1=[])
def categorical_column_with_vocabulary_file_v2(key,
                                               vocabulary_file,
                                               vocabulary_size=None,
                                               dtype=dtypes.string,
                                               default_value=None,
                                               num_oov_buckets=0):
  """A `CategoricalColumn` with a vocabulary file.

  Use this when your inputs are in string or integer format, and you have a
  vocabulary file that maps each value to an integer ID. By default,
  out-of-vocabulary values are ignored. Use either (but not both) of
  `num_oov_buckets` and `default_value` to specify how to include
  out-of-vocabulary values.

  For input dictionary `features`, `features[key]` is either `Tensor` or
  `SparseTensor`. If `Tensor`, missing values can be represented by `-1` for int
  and `''` for string, which will be dropped by this feature column.

  Example with `num_oov_buckets`:
  File '/us/states.txt' contains 50 lines, each with a 2-character U.S. state
  abbreviation. All inputs with values in that file are assigned an ID 0-49,
  corresponding to its line number. All other values are hashed and assigned an
  ID 50-54.

  ```python
  states = categorical_column_with_vocabulary_file(
      key='states', vocabulary_file='/us/states.txt', vocabulary_size=50,
      num_oov_buckets=5)
  columns = [states, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  Example with `default_value`:
  File '/us/states.txt' contains 51 lines - the first line is 'XX', and the
  other 50 each have a 2-character U.S. state abbreviation. Both a literal 'XX'
  in input, and other values missing from the file, will be assigned ID 0. All
  others are assigned the corresponding line number 1-50.

  ```python
  states = categorical_column_with_vocabulary_file(
      key='states', vocabulary_file='/us/states.txt', vocabulary_size=51,
      default_value=0)
  columns = [states, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  And to make an embedding with either:

  ```python
  columns = [embedding_column(states, 3),...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the
      column name and the dictionary key for feature parsing configs, feature
      `Tensor` objects, and feature columns.
    vocabulary_file: The vocabulary file name.
    vocabulary_size: Number of the elements in the vocabulary. This must be no
      greater than length of `vocabulary_file`, if less than length, later
      values are ignored. If None, it is set to the length of `vocabulary_file`.
    dtype: The type of features. Only string and integer types are supported.
    default_value: The integer ID value to return for out-of-vocabulary feature
      values, defaults to `-1`. This can not be specified with a positive
      `num_oov_buckets`.
    num_oov_buckets: Non-negative integer, the number of out-of-vocabulary
      buckets. All out-of-vocabulary inputs will be assigned IDs in the range
      `[vocabulary_size, vocabulary_size+num_oov_buckets)` based on a hash of
      the input value. A positive `num_oov_buckets` can not be specified with
      `default_value`.

  Returns:
    A `CategoricalColumn` with a vocabulary file.

  Raises:
    ValueError: `vocabulary_file` is missing or cannot be opened.
    ValueError: `vocabulary_size` is missing or < 1.
    ValueError: `num_oov_buckets` is a negative integer.
    ValueError: `num_oov_buckets` and `default_value` are both specified.
    ValueError: `dtype` is neither string nor integer.
  """
  if not vocabulary_file:
    raise ValueError('Missing vocabulary_file in {}.'.format(key))

  if vocabulary_size is None:
    if not gfile.Exists(vocabulary_file):
      raise ValueError('vocabulary_file in {} does not exist.'.format(key))

    with gfile.GFile(vocabulary_file) as f:
      vocabulary_size = sum(1 for _ in f)
    logging.info(
        'vocabulary_size = %d in %s is inferred from the number of elements '
        'in the vocabulary_file %s.', vocabulary_size, key, vocabulary_file)

  # `vocabulary_size` isn't required for lookup, but it is for `_num_buckets`.
  if vocabulary_size < 1:
    raise ValueError('Invalid vocabulary_size in {}.'.format(key))
  if num_oov_buckets:
    if default_value is not None:
      raise ValueError(
          'Can\'t specify both num_oov_buckets and default_value in {}.'.format(
              key))
    if num_oov_buckets < 0:
      raise ValueError('Invalid num_oov_buckets {} in {}.'.format(
          num_oov_buckets, key))
  _assert_string_or_int(dtype, prefix='column_name: {}'.format(key))
  _assert_key_is_string(key)
  return VocabularyFileCategoricalColumn(
      key=key,
      vocabulary_file=vocabulary_file,
      vocabulary_size=vocabulary_size,
      num_oov_buckets=0 if num_oov_buckets is None else num_oov_buckets,
      default_value=-1 if default_value is None else default_value,
      dtype=dtype)


@tf_export('feature_column.categorical_column_with_vocabulary_list')
def categorical_column_with_vocabulary_list(key,
                                            vocabulary_list,
                                            dtype=None,
                                            default_value=-1,
                                            num_oov_buckets=0):
  """A `CategoricalColumn` with in-memory vocabulary.

  Use this when your inputs are in string or integer format, and you have an
  in-memory vocabulary mapping each value to an integer ID. By default,
  out-of-vocabulary values are ignored. Use either (but not both) of
  `num_oov_buckets` and `default_value` to specify how to include
  out-of-vocabulary values.

  For input dictionary `features`, `features[key]` is either `Tensor` or
  `SparseTensor`. If `Tensor`, missing values can be represented by `-1` for int
  and `''` for string, which will be dropped by this feature column.

  Example with `num_oov_buckets`:
  In the following example, each input in `vocabulary_list` is assigned an ID
  0-3 corresponding to its index (e.g., input 'B' produces output 2). All other
  inputs are hashed and assigned an ID 4-5.

  ```python
  colors = categorical_column_with_vocabulary_list(
      key='colors', vocabulary_list=('R', 'G', 'B', 'Y'),
      num_oov_buckets=2)
  columns = [colors, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  Example with `default_value`:
  In the following example, each input in `vocabulary_list` is assigned an ID
  0-4 corresponding to its index (e.g., input 'B' produces output 3). All other
  inputs are assigned `default_value` 0.


  ```python
  colors = categorical_column_with_vocabulary_list(
      key='colors', vocabulary_list=('X', 'R', 'G', 'B', 'Y'), default_value=0)
  columns = [colors, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  And to make an embedding with either:

  ```python
  columns = [embedding_column(colors, 3),...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the column
      name and the dictionary key for feature parsing configs, feature `Tensor`
      objects, and feature columns.
    vocabulary_list: An ordered iterable defining the vocabulary. Each feature
      is mapped to the index of its value (if present) in `vocabulary_list`.
      Must be castable to `dtype`.
    dtype: The type of features. Only string and integer types are supported. If
      `None`, it will be inferred from `vocabulary_list`.
    default_value: The integer ID value to return for out-of-vocabulary feature
      values, defaults to `-1`. This can not be specified with a positive
      `num_oov_buckets`.
    num_oov_buckets: Non-negative integer, the number of out-of-vocabulary
      buckets. All out-of-vocabulary inputs will be assigned IDs in the range
      `[len(vocabulary_list), len(vocabulary_list)+num_oov_buckets)` based on a
      hash of the input value. A positive `num_oov_buckets` can not be specified
      with `default_value`.

  Returns:
    A `CategoricalColumn` with in-memory vocabulary.

  Raises:
    ValueError: if `vocabulary_list` is empty, or contains duplicate keys.
    ValueError: `num_oov_buckets` is a negative integer.
    ValueError: `num_oov_buckets` and `default_value` are both specified.
    ValueError: if `dtype` is not integer or string.
  """
  if (vocabulary_list is None) or (len(vocabulary_list) < 1):
    raise ValueError(
        'vocabulary_list {} must be non-empty, column_name: {}'.format(
            vocabulary_list, key))
  if len(set(vocabulary_list)) != len(vocabulary_list):
    raise ValueError(
        'Duplicate keys in vocabulary_list {}, column_name: {}'.format(
            vocabulary_list, key))
  vocabulary_dtype = dtypes.as_dtype(np.array(vocabulary_list).dtype)
  if num_oov_buckets:
    if default_value != -1:
      raise ValueError(
          'Can\'t specify both num_oov_buckets and default_value in {}.'.format(
              key))
    if num_oov_buckets < 0:
      raise ValueError('Invalid num_oov_buckets {} in {}.'.format(
          num_oov_buckets, key))
  _assert_string_or_int(
      vocabulary_dtype, prefix='column_name: {} vocabulary'.format(key))
  if dtype is None:
    dtype = vocabulary_dtype
  elif dtype.is_integer != vocabulary_dtype.is_integer:
    raise ValueError(
        'dtype {} and vocabulary dtype {} do not match, column_name: {}'.format(
            dtype, vocabulary_dtype, key))
  _assert_string_or_int(dtype, prefix='column_name: {}'.format(key))
  _assert_key_is_string(key)

  return VocabularyListCategoricalColumn(
      key=key,
      vocabulary_list=tuple(vocabulary_list),
      dtype=dtype,
      default_value=default_value,
      num_oov_buckets=num_oov_buckets)


@tf_export('feature_column.categorical_column_with_identity')
def categorical_column_with_identity(key, num_buckets, default_value=None):
  """A `CategoricalColumn` that returns identity values.

  Use this when your inputs are integers in the range `[0, num_buckets)`, and
  you want to use the input value itself as the categorical ID. Values outside
  this range will result in `default_value` if specified, otherwise it will
  fail.

  Typically, this is used for contiguous ranges of integer indexes, but
  it doesn't have to be. This might be inefficient, however, if many of IDs
  are unused. Consider `categorical_column_with_hash_bucket` in that case.

  For input dictionary `features`, `features[key]` is either `Tensor` or
  `SparseTensor`. If `Tensor`, missing values can be represented by `-1` for int
  and `''` for string, which will be dropped by this feature column.

  In the following examples, each input in the range `[0, 1000000)` is assigned
  the same value. All other inputs are assigned `default_value` 0. Note that a
  literal 0 in inputs will result in the same default ID.

  Linear model:

  ```python
  video_id = categorical_column_with_identity(
      key='video_id', num_buckets=1000000, default_value=0)
  columns = [video_id, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  Embedding for a DNN model:

  ```python
  columns = [embedding_column(video_id, 9),...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)
  ```

  Args:
    key: A unique string identifying the input feature. It is used as the
      column name and the dictionary key for feature parsing configs, feature
      `Tensor` objects, and feature columns.
    num_buckets: Range of inputs and outputs is `[0, num_buckets)`.
    default_value: If `None`, this column's graph operations will fail for
      out-of-range inputs. Otherwise, this value must be in the range
      `[0, num_buckets)`, and will replace inputs in that range.

  Returns:
    A `CategoricalColumn` that returns identity values.

  Raises:
    ValueError: if `num_buckets` is less than one.
    ValueError: if `default_value` is not in range `[0, num_buckets)`.
  """
  if num_buckets < 1:
    raise ValueError(
        'num_buckets {} < 1, column_name {}'.format(num_buckets, key))
  if (default_value is not None) and (
      (default_value < 0) or (default_value >= num_buckets)):
    raise ValueError(
        'default_value {} not in range [0, {}), column_name {}'.format(
            default_value, num_buckets, key))
  _assert_key_is_string(key)
  return IdentityCategoricalColumn(
      key=key, number_buckets=num_buckets, default_value=default_value)


@tf_export('feature_column.indicator_column')
def indicator_column(categorical_column):
  """Represents multi-hot representation of given categorical column.

  - For DNN model, `indicator_column` can be used to wrap any
    `categorical_column_*` (e.g., to feed to DNN). Consider to Use
    `embedding_column` if the number of buckets/unique(values) are large.

  - For Wide (aka linear) model, `indicator_column` is the internal
    representation for categorical column when passing categorical column
    directly (as any element in feature_columns) to `linear_model`. See
    `linear_model` for details.

  ```python
  name = indicator_column(categorical_column_with_vocabulary_list(
      'name', ['bob', 'george', 'wanda'])
  columns = [name, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  dense_tensor = input_layer(features, columns)

  dense_tensor == [[1, 0, 0]]  # If "name" bytes_list is ["bob"]
  dense_tensor == [[1, 0, 1]]  # If "name" bytes_list is ["bob", "wanda"]
  dense_tensor == [[2, 0, 0]]  # If "name" bytes_list is ["bob", "bob"]
  ```

  Args:
    categorical_column: A `CategoricalColumn` which is created by
      `categorical_column_with_*` or `crossed_column` functions.

  Returns:
    An `IndicatorColumn`.
  """
  return IndicatorColumn(categorical_column)


@tf_export('feature_column.weighted_categorical_column')
def weighted_categorical_column(categorical_column,
                                weight_feature_key,
                                dtype=dtypes.float32):
  """Applies weight values to a `CategoricalColumn`.

  Use this when each of your sparse inputs has both an ID and a value. For
  example, if you're representing text documents as a collection of word
  frequencies, you can provide 2 parallel sparse input features ('terms' and
  'frequencies' below).

  Example:

  Input `tf.Example` objects:

  ```proto
  [
    features {
      feature {
        key: "terms"
        value {bytes_list {value: "very" value: "model"}}
      }
      feature {
        key: "frequencies"
        value {float_list {value: 0.3 value: 0.1}}
      }
    },
    features {
      feature {
        key: "terms"
        value {bytes_list {value: "when" value: "course" value: "human"}}
      }
      feature {
        key: "frequencies"
        value {float_list {value: 0.4 value: 0.1 value: 0.2}}
      }
    }
  ]
  ```

  ```python
  categorical_column = categorical_column_with_hash_bucket(
      column_name='terms', hash_bucket_size=1000)
  weighted_column = weighted_categorical_column(
      categorical_column=categorical_column, weight_feature_key='frequencies')
  columns = [weighted_column, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction, _, _ = linear_model(features, columns)
  ```

  This assumes the input dictionary contains a `SparseTensor` for key
  'terms', and a `SparseTensor` for key 'frequencies'. These 2 tensors must have
  the same indices and dense shape.

  Args:
    categorical_column: A `CategoricalColumn` created by
      `categorical_column_with_*` functions.
    weight_feature_key: String key for weight values.
    dtype: Type of weights, such as `tf.float32`. Only float and integer weights
      are supported.

  Returns:
    A `CategoricalColumn` composed of two sparse features: one represents id,
    the other represents weight (value) of the id feature in that example.

  Raises:
    ValueError: if `dtype` is not convertible to float.
  """
  if (dtype is None) or not (dtype.is_integer or dtype.is_floating):
    raise ValueError('dtype {} is not convertible to float.'.format(dtype))
  return WeightedCategoricalColumn(
      categorical_column=categorical_column,
      weight_feature_key=weight_feature_key,
      dtype=dtype)


@tf_export('feature_column.crossed_column')
def crossed_column(keys, hash_bucket_size, hash_key=None):
  """Returns a column for performing crosses of categorical features.

  Crossed features will be hashed according to `hash_bucket_size`. Conceptually,
  the transformation can be thought of as:
    Hash(cartesian product of features) % `hash_bucket_size`

  For example, if the input features are:

  * SparseTensor referred by first key:

    ```python
    shape = [2, 2]
    {
        [0, 0]: "a"
        [1, 0]: "b"
        [1, 1]: "c"
    }
    ```

  * SparseTensor referred by second key:

    ```python
    shape = [2, 1]
    {
        [0, 0]: "d"
        [1, 0]: "e"
    }
    ```

  then crossed feature will look like:

  ```python
   shape = [2, 2]
  {
      [0, 0]: Hash64("d", Hash64("a")) % hash_bucket_size
      [1, 0]: Hash64("e", Hash64("b")) % hash_bucket_size
      [1, 1]: Hash64("e", Hash64("c")) % hash_bucket_size
  }
  ```

  Here is an example to create a linear model with crosses of string features:

  ```python
  keywords_x_doc_terms = crossed_column(['keywords', 'doc_terms'], 50K)
  columns = [keywords_x_doc_terms, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  You could also use vocabulary lookup before crossing:

  ```python
  keywords = categorical_column_with_vocabulary_file(
      'keywords', '/path/to/vocabulary/file', vocabulary_size=1K)
  keywords_x_doc_terms = crossed_column([keywords, 'doc_terms'], 50K)
  columns = [keywords_x_doc_terms, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  If an input feature is of numeric type, you can use
  `categorical_column_with_identity`, or `bucketized_column`, as in the example:

  ```python
  # vertical_id is an integer categorical feature.
  vertical_id = categorical_column_with_identity('vertical_id', 10K)
  price = numeric_column('price')
  # bucketized_column converts numerical feature to a categorical one.
  bucketized_price = bucketized_column(price, boundaries=[...])
  vertical_id_x_price = crossed_column([vertical_id, bucketized_price], 50K)
  columns = [vertical_id_x_price, ...]
  features = tf.parse_example(..., features=make_parse_example_spec(columns))
  linear_prediction = linear_model(features, columns)
  ```

  To use crossed column in DNN model, you need to add it in an embedding column
  as in this example:

  ```python
  vertical_id_x_price = crossed_column([vertical_id, bucketized_price], 50K)
  vertical_id_x_price_embedded = embedding_column(vertical_id_x_price, 10)
  dense_tensor = input_layer(features, [vertical_id_x_price_embedded, ...])
  ```

  Args:
    keys: An iterable identifying the features to be crossed. Each element can
      be either:
      * string: Will use the corresponding feature which must be of string type.
      * `CategoricalColumn`: Will use the transformed tensor produced by this
        column. Does not support hashed categorical column.
    hash_bucket_size: An int > 1. The number of buckets.
    hash_key: Specify the hash_key that will be used by the `FingerprintCat64`
      function to combine the crosses fingerprints on SparseCrossOp (optional).

  Returns:
    A `CrossedColumn`.

  Raises:
    ValueError: If `len(keys) < 2`.
    ValueError: If any of the keys is neither a string nor `CategoricalColumn`.
    ValueError: If any of the keys is `HashedCategoricalColumn`.
    ValueError: If `hash_bucket_size < 1`.
  """
  if not hash_bucket_size or hash_bucket_size < 1:
    raise ValueError('hash_bucket_size must be > 1. '
                     'hash_bucket_size: {}'.format(hash_bucket_size))
  if not keys or len(keys) < 2:
    raise ValueError(
        'keys must be a list with length > 1. Given: {}'.format(keys))
  for key in keys:
    if (not isinstance(key, six.string_types) and
        not isinstance(key, (CategoricalColumn, fc_old._CategoricalColumn))):  # pylint: disable=protected-access
      raise ValueError(
          'Unsupported key type. All keys must be either string, or '
          'categorical column except HashedCategoricalColumn. '
          'Given: {}'.format(key))
    if isinstance(key,
                  (HashedCategoricalColumn, fc_old._HashedCategoricalColumn)):  # pylint: disable=protected-access
      raise ValueError(
          'categorical_column_with_hash_bucket is not supported for crossing. '
          'Hashing before crossing will increase probability of collision. '
          'Instead, use the feature name as a string. Given: {}'.format(key))
  return CrossedColumn(
      keys=tuple(keys), hash_bucket_size=hash_bucket_size, hash_key=hash_key)


@six.add_metaclass(abc.ABCMeta)
class FeatureColumn(object):
  """Represents a feature column abstraction.

  WARNING: Do not subclass this layer unless you know what you are doing:
  the API is subject to future changes.

  To distinguish between the concept of a feature family and a specific binary
  feature within a family, we refer to a feature family like "country" as a
  feature column. For example, we can have a feature in a `tf.Example` format:
    {key: "country",  value: [ "US" ]}
  In this example the value of feature is "US" and "country" refers to the
  column of the feature.

  This class is an abstract class. Users should not create instances of this.
  """

  @abc.abstractproperty
  def name(self):
    """Returns string. Used for naming."""
    pass

  @abc.abstractmethod
  def transform_feature(self, transformation_cache, state_manager):
    """Returns intermediate representation (usually a `Tensor`).

    Uses `transformation_cache` to create an intermediate representation
    (usually a `Tensor`) that other feature columns can use.

    Example usage of `transformation_cache`:
    Let's say a Feature column depends on raw feature ('raw') and another
    `FeatureColumn` (input_fc). To access corresponding `Tensor`s,
    transformation_cache will be used as follows:

    ```python
    raw_tensor = transformation_cache.get('raw', state_manager)
    fc_tensor = transformation_cache.get(input_fc, state_manager)
    ```

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Transformed feature `Tensor`.
    """
    pass

  @abc.abstractproperty
  def parse_example_spec(self):
    """Returns a `tf.Example` parsing spec as dict.

    It is used for get_parsing_spec for `tf.parse_example`. Returned spec is a
    dict from keys ('string') to `VarLenFeature`, `FixedLenFeature`, and other
    supported objects. Please check documentation of `tf.parse_example` for all
    supported spec objects.

    Let's say a Feature column depends on raw feature ('raw') and another
    `FeatureColumn` (input_fc). One possible implementation of
    parse_example_spec is as follows:

    ```python
    spec = {'raw': tf.FixedLenFeature(...)}
    spec.update(input_fc.parse_example_spec)
    return spec
    ```
    """
    pass

  def create_state(self, state_manager):
    """Uses the `state_manager` to create state for the FeatureColumn.

    Args:
      state_manager: A `StateManager` to create / access resources such as
        lookup tables and variables.
    """
    pass

  @abc.abstractproperty
  def _is_v2_column(self):
    """Returns whether this FeatureColumn is fully conformant to the new API.

    This is needed for composition type cases where an EmbeddingColumn etc.
    might take in old categorical columns as input and then we want to use the
    old API.
    """
    pass

  @abc.abstractproperty
  def parents(self):
    """Returns a list of immediate raw feature and FeatureColumn dependencies.

    For example:
    # For the following feature columns
    a = numeric_column('f1')
    c = crossed_column(a, 'f2')
    # The expected parents are:
    a.parents = ['f1']
    c.parents = [a, 'f2']
    """
    pass

  @abc.abstractmethod
  def _get_config(self):
    """Returns the config of the feature column.

    A FeatureColumn config is a Python dictionary (serializable) containing the
    configuration of a FeatureColumn. The same FeatureColumn can be
    reinstantiated later from this configuration.

    The config of a feature column does not include information about feature
    columns depending on it nor the FeatureColumn class name.

    Example with (de)serialization practices followed in this file:
    ```python
    class SerializationExampleFeatureColumn(
        FeatureColumn, collections.namedtuple(
            'SerializationExampleFeatureColumn',
            ('dimension', 'parent', 'dtype', 'normalizer_fn'))):

      def _get_config(self):
        # Create a dict from the namedtuple.
        # Python attribute literals can be directly copied from / to the config.
        # For example 'dimension', assuming it is an integer literal.
        config = dict(zip(self._fields, self))

        # (De)serialization of parent FeatureColumns should use the provided
        # (de)serialize_feature_column() methods that take care of de-duping.
        config['parent'] = serialize_feature_column(self.parent)

        # Many objects provide custom (de)serialization e.g: for tf.DType
        # tf.DType.name, tf.as_dtype() can be used.
        config['dtype'] = self.dtype.name

        # Non-trivial dependencies should be Keras-(de)serializable.
        config['normalizer_fn'] = utils.serialize_keras_object(
            self.normalizer_fn)

        return config

      @classmethod
      def _from_config(cls, config, custom_objects=None, columns_by_name=None):
        # This should do the inverse transform from `_get_config` and construct
        # the namedtuple.
        kwargs = config.copy()
        kwargs['parent'] = deserialize_feature_column(
            config['parent'], custom_objects, columns_by_name)
        kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
        kwargs['normalizer_fn'] = utils.deserialize_keras_object(
          config['normalizer_fn'], custom_objects=custom_objects)
        return cls(**kwargs)

    ```
    Returns:
      A serializable Dict that can be used to deserialize the object with
      from_config.
    """
    pass

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """Creates a FeatureColumn from its config.

    This method should be the reverse of `_get_config`, capable of instantiating
    the same FeatureColumn from the config dictionary. See `_get_config` for an
    example of common (de)serialization practices followed in this file.

    TODO(b/118939620): This is a private method until consensus is reached on
    supporting object deserialization deduping within Keras.

    Args:
      config: A Dict config acquired with `_get_config`.
      custom_objects: Optional dictionary mapping names (strings) to custom
        classes or functions to be considered during deserialization.
      columns_by_name: A Dict[String, FeatureColumn] of existing columns in
        order to avoid duplication. Should be passed to any calls to
        deserialize_feature_column().

    Returns:
      A FeatureColumn for the input config.
    """
    pass


class DenseColumn(FeatureColumn):
  """Represents a column which can be represented as `Tensor`.

  Some examples of this type are: numeric_column, embedding_column,
  indicator_column.
  """

  @abc.abstractproperty
  def variable_shape(self):
    """`TensorShape` of `get_dense_tensor`, without batch dimension."""
    pass

  @abc.abstractmethod
  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns a `Tensor`.

    The output of this function will be used by model-builder-functions. For
    example the pseudo code of `input_layer` will be like:

    ```python
    def input_layer(features, feature_columns, ...):
      outputs = [fc.get_dense_tensor(...) for fc in feature_columns]
      return tf.concat(outputs)
    ```

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      `Tensor` of shape [batch_size] + `variable_shape`.
    """
    pass


def is_feature_column_v2(feature_columns):
  """Returns True if all feature columns are V2."""
  for feature_column in feature_columns:
    if not isinstance(feature_column, FeatureColumn):
      return False
    if not feature_column._is_v2_column:  # pylint: disable=protected-access
      return False
  return True


def _create_weighted_sum(column, transformation_cache, state_manager,
                         sparse_combiner, weight_var):
  """Creates a weighted sum for a dense/categorical column for linear_model."""
  if isinstance(column, CategoricalColumn):
    return _create_categorical_column_weighted_sum(
        column=column,
        transformation_cache=transformation_cache,
        state_manager=state_manager,
        sparse_combiner=sparse_combiner,
        weight_var=weight_var)
  else:
    return _create_dense_column_weighted_sum(
        column=column,
        transformation_cache=transformation_cache,
        state_manager=state_manager,
        weight_var=weight_var)


def _create_dense_column_weighted_sum(column, transformation_cache,
                                      state_manager, weight_var):
  """Create a weighted sum of a dense column for linear_model."""
  tensor = column.get_dense_tensor(transformation_cache, state_manager)
  num_elements = column.variable_shape.num_elements()
  batch_size = array_ops.shape(tensor)[0]
  tensor = array_ops.reshape(tensor, shape=(batch_size, num_elements))
  return math_ops.matmul(tensor, weight_var, name='weighted_sum')


class CategoricalColumn(FeatureColumn):
  """Represents a categorical feature.

  A categorical feature typically handled with a `tf.SparseTensor` of IDs.
  """

  IdWeightPair = collections.namedtuple(  # pylint: disable=invalid-name
      'IdWeightPair', ('id_tensor', 'weight_tensor'))

  @abc.abstractproperty
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    pass

  @abc.abstractmethod
  def get_sparse_tensors(self, transformation_cache, state_manager):
    """Returns an IdWeightPair.

    `IdWeightPair` is a pair of `SparseTensor`s which represents ids and
    weights.

    `IdWeightPair.id_tensor` is typically a `batch_size` x `num_buckets`
    `SparseTensor` of `int64`. `IdWeightPair.weight_tensor` is either a
    `SparseTensor` of `float` or `None` to indicate all weights should be
    taken to be 1. If specified, `weight_tensor` must have exactly the same
    shape and indices as `sp_ids`. Expected `SparseTensor` is same as parsing
    output of a `VarLenFeature` which is a ragged matrix.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.
    """
    pass


def _create_categorical_column_weighted_sum(
    column, transformation_cache, state_manager, sparse_combiner, weight_var):
  # pylint: disable=g-doc-return-or-yield,g-doc-args
  """Create a weighted sum of a categorical column for linear_model.

  Note to maintainer: As implementation details, the weighted sum is
  implemented via embedding_lookup_sparse toward efficiency. Mathematically,
  they are the same.

  To be specific, conceptually, categorical column can be treated as multi-hot
  vector. Say:

  ```python
    x = [0 0 1]  # categorical column input
    w = [a b c]  # weights
  ```
  The weighted sum is `c` in this case, which is same as `w[2]`.

  Another example is

  ```python
    x = [0 1 1]  # categorical column input
    w = [a b c]  # weights
  ```
  The weighted sum is `b + c` in this case, which is same as `w[2] + w[3]`.

  For both cases, we can implement weighted sum via embedding_lookup with
  sparse_combiner = "sum".
  """

  sparse_tensors = column.get_sparse_tensors(transformation_cache,
                                             state_manager)
  id_tensor = sparse_ops.sparse_reshape(sparse_tensors.id_tensor, [
      array_ops.shape(sparse_tensors.id_tensor)[0], -1
  ])
  weight_tensor = sparse_tensors.weight_tensor
  if weight_tensor is not None:
    weight_tensor = sparse_ops.sparse_reshape(
        weight_tensor, [array_ops.shape(weight_tensor)[0], -1])

  return embedding_ops.safe_embedding_lookup_sparse(
      weight_var,
      id_tensor,
      sparse_weights=weight_tensor,
      combiner=sparse_combiner,
      name='weighted_sum')


class SequenceDenseColumn(FeatureColumn):
  """Represents dense sequence data."""

  TensorSequenceLengthPair = collections.namedtuple(  # pylint: disable=invalid-name
      'TensorSequenceLengthPair', ('dense_tensor', 'sequence_length'))

  @abc.abstractmethod
  def get_sequence_dense_tensor(self, transformation_cache, state_manager):
    """Returns a `TensorSequenceLengthPair`.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.
    """
    pass


class FeatureTransformationCache(object):
  """Handles caching of transformations while building the model.

  `FeatureColumn` specifies how to digest an input column to the network. Some
  feature columns require data transformations. This class caches those
  transformations.

  Some features may be used in more than one place. For example, one can use a
  bucketized feature by itself and a cross with it. In that case we
  should create only one bucketization op instead of creating ops for each
  feature column separately. To handle re-use of transformed columns,
  `FeatureTransformationCache` caches all previously transformed columns.

  Example:
  We're trying to use the following `FeatureColumn`s:

  ```python
  bucketized_age = fc.bucketized_column(fc.numeric_column("age"), ...)
  keywords = fc.categorical_column_with_hash_buckets("keywords", ...)
  age_X_keywords = fc.crossed_column([bucketized_age, "keywords"])
  ... = linear_model(features,
                          [bucketized_age, keywords, age_X_keywords]
  ```

  If we transform each column independently, then we'll get duplication of
  bucketization (one for cross, one for bucketization itself).
  The `FeatureTransformationCache` eliminates this duplication.
  """

  def __init__(self, features):
    """Creates a `FeatureTransformationCache`.

    Args:
      features: A mapping from feature column to objects that are `Tensor` or
        `SparseTensor`, or can be converted to same via
        `sparse_tensor.convert_to_tensor_or_sparse_tensor`. A `string` key
        signifies a base feature (not-transformed). A `FeatureColumn` key
        means that this `Tensor` is the output of an existing `FeatureColumn`
        which can be reused.
    """
    self._features = features.copy()
    self._feature_tensors = {}

  def get(self, key, state_manager):
    """Returns a `Tensor` for the given key.

    A `str` key is used to access a base feature (not-transformed). When a
    `FeatureColumn` is passed, the transformed feature is returned if it
    already exists, otherwise the given `FeatureColumn` is asked to provide its
    transformed output, which is then cached.

    Args:
      key: a `str` or a `FeatureColumn`.
      state_manager: A StateManager object that holds the FeatureColumn state.

    Returns:
      The transformed `Tensor` corresponding to the `key`.

    Raises:
      ValueError: if key is not found or a transformed `Tensor` cannot be
        computed.
    """
    if key in self._feature_tensors:
      # FeatureColumn is already transformed or converted.
      return self._feature_tensors[key]

    if key in self._features:
      feature_tensor = self._get_raw_feature_as_tensor(key)
      self._feature_tensors[key] = feature_tensor
      return feature_tensor

    if isinstance(key, six.string_types):
      raise ValueError('Feature {} is not in features dictionary.'.format(key))

    if not isinstance(key, FeatureColumn):
      raise TypeError('"key" must be either a "str" or "FeatureColumn". '
                      'Provided: {}'.format(key))

    column = key
    logging.debug('Transforming feature_column %s.', column)
    transformed = column.transform_feature(self, state_manager)
    if transformed is None:
      raise ValueError('Column {} is not supported.'.format(column.name))
    self._feature_tensors[column] = transformed
    return transformed

  def _get_raw_feature_as_tensor(self, key):
    """Gets the raw_feature (keyed by `key`) as `tensor`.

    The raw feature is converted to (sparse) tensor and maybe expand dim.

    For both `Tensor` and `SparseTensor`, the rank will be expanded (to 2) if
    the rank is 1. This supports dynamic rank also. For rank 0 raw feature, will
    error out as it is not supported.

    Args:
      key: A `str` key to access the raw feature.

    Returns:
      A `Tensor` or `SparseTensor`.

    Raises:
      ValueError: if the raw feature has rank 0.
    """
    raw_feature = self._features[key]
    feature_tensor = sparse_tensor_lib.convert_to_tensor_or_sparse_tensor(
        raw_feature)

    def expand_dims(input_tensor):
      # Input_tensor must have rank 1.
      if isinstance(input_tensor, sparse_tensor_lib.SparseTensor):
        return sparse_ops.sparse_reshape(
            input_tensor, [array_ops.shape(input_tensor)[0], 1])
      else:
        return array_ops.expand_dims(input_tensor, -1)

    rank = feature_tensor.get_shape().ndims
    if rank is not None:
      if rank == 0:
        raise ValueError(
            'Feature (key: {}) cannot have rank 0. Give: {}'.format(
                key, feature_tensor))
      return feature_tensor if rank != 1 else expand_dims(feature_tensor)

    # Handle dynamic rank.
    with ops.control_dependencies([
        check_ops.assert_positive(
            array_ops.rank(feature_tensor),
            message='Feature (key: {}) cannot have rank 0. Given: {}'.format(
                key, feature_tensor))]):
      return control_flow_ops.cond(
          math_ops.equal(1, array_ops.rank(feature_tensor)),
          lambda: expand_dims(feature_tensor),
          lambda: feature_tensor)


# TODO(ptucker): Move to third_party/tensorflow/python/ops/sparse_ops.py
def _to_sparse_input_and_drop_ignore_values(input_tensor, ignore_value=None):
  """Converts a `Tensor` to a `SparseTensor`, dropping ignore_value cells.

  If `input_tensor` is already a `SparseTensor`, just return it.

  Args:
    input_tensor: A string or integer `Tensor`.
    ignore_value: Entries in `dense_tensor` equal to this value will be
      absent from the resulting `SparseTensor`. If `None`, default value of
      `dense_tensor`'s dtype will be used ('' for `str`, -1 for `int`).

  Returns:
    A `SparseTensor` with the same shape as `input_tensor`.

  Raises:
    ValueError: when `input_tensor`'s rank is `None`.
  """
  input_tensor = sparse_tensor_lib.convert_to_tensor_or_sparse_tensor(
      input_tensor)
  if isinstance(input_tensor, sparse_tensor_lib.SparseTensor):
    return input_tensor
  with ops.name_scope(None, 'to_sparse_input', (input_tensor, ignore_value,)):
    if ignore_value is None:
      if input_tensor.dtype == dtypes.string:
        # Exception due to TF strings are converted to numpy objects by default.
        ignore_value = ''
      elif input_tensor.dtype.is_integer:
        ignore_value = -1  # -1 has a special meaning of missing feature
      else:
        # NOTE: `as_numpy_dtype` is a property, so with the parentheses this is
        # constructing a new numpy object of the given type, which yields the
        # default value for that type.
        ignore_value = input_tensor.dtype.as_numpy_dtype()
    ignore_value = math_ops.cast(
        ignore_value, input_tensor.dtype, name='ignore_value')
    indices = array_ops.where(
        math_ops.not_equal(input_tensor, ignore_value), name='indices')
    return sparse_tensor_lib.SparseTensor(
        indices=indices,
        values=array_ops.gather_nd(input_tensor, indices, name='values'),
        dense_shape=array_ops.shape(
            input_tensor, out_type=dtypes.int64, name='dense_shape'))


def _normalize_feature_columns(feature_columns):
  """Normalizes the `feature_columns` input.

  This method converts the `feature_columns` to list type as best as it can. In
  addition, verifies the type and other parts of feature_columns, required by
  downstream library.

  Args:
    feature_columns: The raw feature columns, usually passed by users.

  Returns:
    The normalized feature column list.

  Raises:
    ValueError: for any invalid inputs, such as empty, duplicated names, etc.
  """
  if isinstance(feature_columns, FeatureColumn):
    feature_columns = [feature_columns]

  if isinstance(feature_columns, collections.Iterator):
    feature_columns = list(feature_columns)

  if isinstance(feature_columns, dict):
    raise ValueError('Expected feature_columns to be iterable, found dict.')

  for column in feature_columns:
    if not isinstance(column, FeatureColumn):
      raise ValueError('Items of feature_columns must be a FeatureColumn. '
                       'Given (type {}): {}.'.format(type(column), column))
  if not feature_columns:
    raise ValueError('feature_columns must not be empty.')
  name_to_column = dict()
  for column in feature_columns:
    if column.name in name_to_column:
      raise ValueError('Duplicate feature column name found for columns: {} '
                       'and {}. This usually means that these columns refer to '
                       'same base feature. Either one must be discarded or a '
                       'duplicated but renamed item must be inserted in '
                       'features dict.'.format(column,
                                               name_to_column[column.name]))
    name_to_column[column.name] = column

  return feature_columns


class NumericColumn(
    DenseColumn,
    fc_old._DenseColumn,  # pylint: disable=protected-access
    collections.namedtuple(
        'NumericColumn',
        ('key', 'shape', 'default_value', 'dtype', 'normalizer_fn'))):
  """see `numeric_column`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.key

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return {
        self.key:
            parsing_ops.FixedLenFeature(self.shape, self.dtype,
                                        self.default_value)
    }

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def _transform_input_tensor(self, input_tensor):
    if isinstance(input_tensor, sparse_tensor_lib.SparseTensor):
      raise ValueError(
          'The corresponding Tensor of numerical column must be a Tensor. '
          'SparseTensor is not supported. key: {}'.format(self.key))
    if self.normalizer_fn is not None:
      input_tensor = self.normalizer_fn(input_tensor)
    return math_ops.to_float(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    input_tensor = inputs.get(self.key)
    return self._transform_input_tensor(input_tensor)

  def transform_feature(self, transformation_cache, state_manager):
    """See `FeatureColumn` base class.

    In this case, we apply the `normalizer_fn` to the input tensor.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Normalized input tensor.
    Raises:
      ValueError: If a SparseTensor is passed in.
    """
    input_tensor = transformation_cache.get(self.key, state_manager)
    return self._transform_input_tensor(input_tensor)

  @property
  def variable_shape(self):
    """See `DenseColumn` base class."""
    return tensor_shape.TensorShape(self.shape)

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _variable_shape(self):
    return self.variable_shape

  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns dense `Tensor` representing numeric feature.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Dense `Tensor` created within `transform_feature`.
    """
    # Feature has been already transformed. Return the intermediate
    # representation created by _transform_feature.
    return transformation_cache.get(self, state_manager)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_dense_tensor(self, inputs, weight_collections=None, trainable=None):
    del weight_collections
    del trainable
    return inputs.get(self)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['normalizer_fn'] = utils.serialize_keras_object(self.normalizer_fn)
    config['dtype'] = self.dtype.name
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    # TODO(b/118820158): Simplify if deserialize_keras_object supports None.
    if config['normalizer_fn']:
      kwargs['normalizer_fn'] = utils.deserialize_keras_object(
          config['normalizer_fn'], custom_objects=custom_objects)
    else:
      kwargs['normalizer_fn'] = None
    kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
    return cls(**kwargs)


class BucketizedColumn(
    DenseColumn,
    CategoricalColumn,
    fc_old._DenseColumn,  # pylint: disable=protected-access
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('BucketizedColumn',
                           ('source_column', 'boundaries'))):
  """See `bucketized_column`."""

  @property
  def _is_v2_column(self):
    return (isinstance(self.source_column, FeatureColumn) and
            self.source_column._is_v2_column)  # pylint: disable=protected-access

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return '{}_bucketized'.format(self.source_column.name)

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return self.source_column.parse_example_spec

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.source_column._parse_example_spec  # pylint: disable=protected-access

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    """Returns bucketized categorical `source_column` tensor."""
    source_tensor = inputs.get(self.source_column)
    return math_ops._bucketize(  # pylint: disable=protected-access
        source_tensor,
        boundaries=self.boundaries)

  def transform_feature(self, transformation_cache, state_manager):
    """Returns bucketized categorical `source_column` tensor."""
    source_tensor = transformation_cache.get(self.source_column, state_manager)
    return math_ops._bucketize(  # pylint: disable=protected-access
        source_tensor,
        boundaries=self.boundaries)

  @property
  def variable_shape(self):
    """See `DenseColumn` base class."""
    return tensor_shape.TensorShape(
        tuple(self.source_column.shape) + (len(self.boundaries) + 1,))

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _variable_shape(self):
    return self.variable_shape

  def _get_dense_tensor_for_input_tensor(self, input_tensor):
    return array_ops.one_hot(
        indices=math_ops.to_int64(input_tensor),
        depth=len(self.boundaries) + 1,
        on_value=1.,
        off_value=0.)

  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns one hot encoded dense `Tensor`."""
    input_tensor = transformation_cache.get(self, state_manager)
    return self._get_dense_tensor_for_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_dense_tensor(self, inputs, weight_collections=None, trainable=None):
    del weight_collections
    del trainable
    input_tensor = inputs.get(self)
    return self._get_dense_tensor_for_input_tensor(input_tensor)

  @property
  def num_buckets(self):
    """See `CategoricalColumn` base class."""
    # By construction, source_column is always one-dimensional.
    return (len(self.boundaries) + 1) * self.source_column.shape[0]

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def _get_sparse_tensors_for_input_tensor(self, input_tensor):
    batch_size = array_ops.shape(input_tensor)[0]
    # By construction, source_column is always one-dimensional.
    source_dimension = self.source_column.shape[0]

    i1 = array_ops.reshape(
        array_ops.tile(
            array_ops.expand_dims(math_ops.range(0, batch_size), 1),
            [1, source_dimension]),
        (-1,))
    i2 = array_ops.tile(math_ops.range(0, source_dimension), [batch_size])
    # Flatten the bucket indices and unique them across dimensions
    # E.g. 2nd dimension indices will range from k to 2*k-1 with k buckets
    bucket_indices = (
        array_ops.reshape(input_tensor, (-1,)) +
        (len(self.boundaries) + 1) * i2)

    indices = math_ops.to_int64(array_ops.transpose(array_ops.stack((i1, i2))))
    dense_shape = math_ops.to_int64(array_ops.stack(
        [batch_size, source_dimension]))
    sparse_tensor = sparse_tensor_lib.SparseTensor(
        indices=indices,
        values=bucket_indices,
        dense_shape=dense_shape)
    return CategoricalColumn.IdWeightPair(sparse_tensor, None)

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """Converts dense inputs to SparseTensor so downstream code can use it."""
    input_tensor = transformation_cache.get(self, state_manager)
    return self._get_sparse_tensors_for_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    """Converts dense inputs to SparseTensor so downstream code can use it."""
    del weight_collections
    del trainable
    input_tensor = inputs.get(self)
    return self._get_sparse_tensors_for_input_tensor(input_tensor)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.source_column]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['source_column'] = serialize_feature_column(self.source_column)
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['source_column'] = deserialize_feature_column(
        config['source_column'], custom_objects, columns_by_name)
    return cls(**kwargs)


class EmbeddingColumn(
    DenseColumn,
    SequenceDenseColumn,
    fc_old._DenseColumn,  # pylint: disable=protected-access
    fc_old._SequenceDenseColumn,  # pylint: disable=protected-access
    collections.namedtuple(
        'EmbeddingColumn',
        ('categorical_column', 'dimension', 'combiner', 'initializer',
         'ckpt_to_load_from', 'tensor_name_in_ckpt', 'max_norm', 'trainable'))):
  """See `embedding_column`."""

  @property
  def _is_v2_column(self):
    return (isinstance(self.categorical_column, FeatureColumn) and
            self.categorical_column._is_v2_column)  # pylint: disable=protected-access

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return '{}_embedding'.format(self.categorical_column.name)

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return self.categorical_column.parse_example_spec

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.categorical_column._parse_example_spec  # pylint: disable=protected-access

  def transform_feature(self, transformation_cache, state_manager):
    """Transforms underlying `categorical_column`."""
    return transformation_cache.get(self.categorical_column, state_manager)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    return inputs.get(self.categorical_column)

  @property
  def variable_shape(self):
    """See `DenseColumn` base class."""
    return tensor_shape.vector(self.dimension)

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _variable_shape(self):
    return self.variable_shape

  def create_state(self, state_manager):
    """Creates the embedding lookup variable."""
    embedding_shape = (self.categorical_column._num_buckets, self.dimension)  # pylint: disable=protected-access
    state_manager.create_variable(
        self,
        name='embedding_weights',
        shape=embedding_shape,
        dtype=dtypes.float32,
        trainable=self.trainable,
        # TODO(rohanj): Make this True when b/118500434 is fixed.
        use_resource=False,
        initializer=self.initializer)

  def _get_dense_tensor_internal_helper(self, sparse_tensors,
                                        embedding_weights):
    sparse_ids = sparse_tensors.id_tensor
    sparse_weights = sparse_tensors.weight_tensor

    if self.ckpt_to_load_from is not None:
      to_restore = embedding_weights
      if isinstance(to_restore, variables.PartitionedVariable):
        to_restore = to_restore._get_variable_list()  # pylint: disable=protected-access
      checkpoint_utils.init_from_checkpoint(self.ckpt_to_load_from, {
          self.tensor_name_in_ckpt: to_restore
      })

    # Return embedding lookup result.
    return embedding_ops.safe_embedding_lookup_sparse(
        embedding_weights=embedding_weights,
        sparse_ids=sparse_ids,
        sparse_weights=sparse_weights,
        combiner=self.combiner,
        name='%s_weights' % self.name,
        max_norm=self.max_norm)

  def _get_dense_tensor_internal(self, sparse_tensors, state_manager):
    """Private method that follows the signature of get_dense_tensor."""
    embedding_weights = state_manager.get_variable(
        self, name='embedding_weights')
    return self._get_dense_tensor_internal_helper(sparse_tensors,
                                                  embedding_weights)

  def _old_get_dense_tensor_internal(self, sparse_tensors, weight_collections,
                                     trainable):
    """Private method that follows the signature of _get_dense_tensor."""
    embedding_shape = (self.categorical_column._num_buckets, self.dimension)  # pylint: disable=protected-access
    if (weight_collections and
        ops.GraphKeys.GLOBAL_VARIABLES not in weight_collections):
      weight_collections.append(ops.GraphKeys.GLOBAL_VARIABLES)
    embedding_weights = variable_scope.get_variable(
        name='embedding_weights',
        shape=embedding_shape,
        dtype=dtypes.float32,
        initializer=self.initializer,
        trainable=self.trainable and trainable,
        collections=weight_collections)
    return self._get_dense_tensor_internal_helper(sparse_tensors,
                                                  embedding_weights)

  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns tensor after doing the embedding lookup.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Embedding lookup tensor.

    Raises:
      ValueError: `categorical_column` is SequenceCategoricalColumn.
    """
    if isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must not be of type SequenceCategoricalColumn. '
          'Suggested fix A: If you wish to use input_layer, use a '
          'non-sequence categorical_column_with_*. '
          'Suggested fix B: If you wish to create sequence input, use '
          'sequence_input_layer instead of input_layer. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    # Get sparse IDs and weights.
    sparse_tensors = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    return self._get_dense_tensor_internal(sparse_tensors, state_manager)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_dense_tensor(self, inputs, weight_collections=None, trainable=None):
    if isinstance(
        self.categorical_column,
        (SequenceCategoricalColumn, fc_old._SequenceCategoricalColumn)):  # pylint: disable=protected-access
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must not be of type _SequenceCategoricalColumn. '
          'Suggested fix A: If you wish to use input_layer, use a '
          'non-sequence categorical_column_with_*. '
          'Suggested fix B: If you wish to create sequence input, use '
          'sequence_input_layer instead of input_layer. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    sparse_tensors = self.categorical_column._get_sparse_tensors(  # pylint: disable=protected-access
        inputs, weight_collections, trainable)
    return self._old_get_dense_tensor_internal(sparse_tensors,
                                               weight_collections, trainable)

  def get_sequence_dense_tensor(self, transformation_cache, state_manager):
    """See `SequenceDenseColumn` base class."""
    if not isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must be of type SequenceCategoricalColumn '
          'to use sequence_input_layer. '
          'Suggested fix: Use one of sequence_categorical_column_with_*. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    sparse_tensors = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    dense_tensor = self._get_dense_tensor_internal(sparse_tensors,
                                                   state_manager)
    sequence_length = fc_old._sequence_length_from_sparse_tensor(  # pylint: disable=protected-access
        sparse_tensors.id_tensor)
    return SequenceDenseColumn.TensorSequenceLengthPair(
        dense_tensor=dense_tensor, sequence_length=sequence_length)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sequence_dense_tensor(self,
                                 inputs,
                                 weight_collections=None,
                                 trainable=None):
    if not isinstance(
        self.categorical_column,
        (SequenceCategoricalColumn, fc_old._SequenceCategoricalColumn)):  # pylint: disable=protected-access
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must be of type _SequenceCategoricalColumn '
          'to use sequence_input_layer. '
          'Suggested fix: Use one of sequence_categorical_column_with_*. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    sparse_tensors = self.categorical_column._get_sparse_tensors(inputs)  # pylint: disable=protected-access
    dense_tensor = self._old_get_dense_tensor_internal(
        sparse_tensors,
        weight_collections=weight_collections,
        trainable=trainable)
    sequence_length = fc_old._sequence_length_from_sparse_tensor(  # pylint: disable=protected-access
        sparse_tensors.id_tensor)
    return SequenceDenseColumn.TensorSequenceLengthPair(
        dense_tensor=dense_tensor, sequence_length=sequence_length)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.categorical_column]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['categorical_column'] = serialize_feature_column(
        self.categorical_column)
    config['initializer'] = utils.serialize_keras_object(self.initializer)
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['categorical_column'] = deserialize_feature_column(
        config['categorical_column'], custom_objects, columns_by_name)
    # TODO(b/118820158): Simplify if deserialize_keras_object supports None.
    if config['initializer']:
      kwargs['initializer'] = utils.deserialize_keras_object(
          config['initializer'], custom_objects=custom_objects)
    else:
      kwargs['initializer'] = None
    return cls(**kwargs)


def _raise_shared_embedding_column_error():
  raise ValueError('SharedEmbeddingColumns are not supported in '
                   '`linear_model` or `input_layer`. Please use '
                   '`DenseFeatures` or `LinearModel` instead.')


class SharedEmbeddingColumnCreator(tracking.Checkpointable):

  def __init__(self,
               dimension,
               initializer,
               ckpt_to_load_from,
               tensor_name_in_ckpt,
               num_buckets,
               trainable,
               name='shared_embedding_column_creator'):
    self._dimension = dimension
    self._initializer = initializer
    self._ckpt_to_load_from = ckpt_to_load_from
    self._tensor_name_in_ckpt = tensor_name_in_ckpt
    self._num_buckets = num_buckets
    self._trainable = trainable
    self._name = name
    # Map from graph keys to embedding_weight variables.
    self._embedding_weights = {}

  def __call__(self, categorical_column, combiner, max_norm):
    return SharedEmbeddingColumn(categorical_column, self, combiner, max_norm)

  @property
  def embedding_weights(self):
    key = ops.get_default_graph()._graph_key  # pylint: disable=protected-access
    if key not in self._embedding_weights:
      embedding_shape = (self._num_buckets, self._dimension)
      var = variable_scope.get_variable(
          name=self._name,
          shape=embedding_shape,
          dtype=dtypes.float32,
          initializer=self._initializer,
          trainable=self._trainable)

      if self._ckpt_to_load_from is not None:
        to_restore = var
        if isinstance(to_restore, variables.PartitionedVariable):
          to_restore = to_restore._get_variable_list()  # pylint: disable=protected-access
        checkpoint_utils.init_from_checkpoint(
            self._ckpt_to_load_from, {self._tensor_name_in_ckpt: to_restore})
      self._embedding_weights[key] = var
    return self._embedding_weights[key]

  @property
  def dimension(self):
    return self._dimension


class SharedEmbeddingColumn(
    DenseColumn,
    SequenceDenseColumn,
    fc_old._DenseColumn,  # pylint: disable=protected-access
    fc_old._SequenceDenseColumn,  # pylint: disable=protected-access
    collections.namedtuple(
        'SharedEmbeddingColumn',
        ('categorical_column', 'shared_embedding_column_creator', 'combiner',
         'max_norm'))):
  """See `embedding_column`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return '{}_shared_embedding'.format(self.categorical_column.name)

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return self.categorical_column.parse_example_spec

  @property
  def _parse_example_spec(self):
    return _raise_shared_embedding_column_error()

  def transform_feature(self, transformation_cache, state_manager):
    """See `FeatureColumn` base class."""
    return transformation_cache.get(self.categorical_column, state_manager)

  def _transform_feature(self, inputs):
    return _raise_shared_embedding_column_error()

  @property
  def variable_shape(self):
    """See `DenseColumn` base class."""
    return tensor_shape.vector(self.shared_embedding_column_creator.dimension)

  @property
  def _variable_shape(self):
    return _raise_shared_embedding_column_error()

  def _get_dense_tensor_internal(self, transformation_cache, state_manager):
    """Private method that follows the signature of _get_dense_tensor."""
    # This method is called from a variable_scope with name _var_scope_name,
    # which is shared among all shared embeddings. Open a name_scope here, so
    # that the ops for different columns have distinct names.
    with ops.name_scope(None, default_name=self.name):
      # Get sparse IDs and weights.
      sparse_tensors = self.categorical_column.get_sparse_tensors(
          transformation_cache, state_manager)
      sparse_ids = sparse_tensors.id_tensor
      sparse_weights = sparse_tensors.weight_tensor

      embedding_weights = self.shared_embedding_column_creator.embedding_weights

      # Return embedding lookup result.
      return embedding_ops.safe_embedding_lookup_sparse(
          embedding_weights=embedding_weights,
          sparse_ids=sparse_ids,
          sparse_weights=sparse_weights,
          combiner=self.combiner,
          name='%s_weights' % self.name,
          max_norm=self.max_norm)

  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns the embedding lookup result."""
    if isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must not be of type SequenceCategoricalColumn. '
          'Suggested fix A: If you wish to use input_layer, use a '
          'non-sequence categorical_column_with_*. '
          'Suggested fix B: If you wish to create sequence input, use '
          'SequenceFeatureLayer instead of FeatureLayer. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    return self._get_dense_tensor_internal(transformation_cache, state_manager)

  def _get_dense_tensor(self, inputs, weight_collections=None, trainable=None):
    return _raise_shared_embedding_column_error()

  def get_sequence_dense_tensor(self, transformation_cache, state_manager):
    """See `SequenceDenseColumn` base class."""
    if not isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In embedding_column: {}. '
          'categorical_column must be of type SequenceCategoricalColumn '
          'to use SequenceFeatureLayer. '
          'Suggested fix: Use one of sequence_categorical_column_with_*. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    dense_tensor = self._get_dense_tensor_internal(transformation_cache,
                                                   state_manager)
    sparse_tensors = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    sequence_length = fc_old._sequence_length_from_sparse_tensor(  # pylint: disable=protected-access
        sparse_tensors.id_tensor)
    return SequenceDenseColumn.TensorSequenceLengthPair(
        dense_tensor=dense_tensor, sequence_length=sequence_length)

  def _get_sequence_dense_tensor(self,
                                 inputs,
                                 weight_collections=None,
                                 trainable=None):
    return _raise_shared_embedding_column_error()

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.categorical_column]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    raise NotImplementedError()

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    raise NotImplementedError()


def _create_tuple(shape, value):
  """Returns a tuple with given shape and filled with value."""
  if shape:
    return tuple([_create_tuple(shape[1:], value) for _ in range(shape[0])])
  return value


def _as_tuple(value):
  if not nest.is_sequence(value):
    return value
  return tuple([_as_tuple(v) for v in value])


def _check_shape(shape, key):
  """Returns shape if it's valid, raises error otherwise."""
  assert shape is not None
  if not nest.is_sequence(shape):
    shape = [shape]
  shape = tuple(shape)
  for dimension in shape:
    if not isinstance(dimension, int):
      raise TypeError('shape dimensions must be integer. '
                      'shape: {}, key: {}'.format(shape, key))
    if dimension < 1:
      raise ValueError('shape dimensions must be greater than 0. '
                       'shape: {}, key: {}'.format(shape, key))
  return shape


def _is_shape_and_default_value_compatible(default_value, shape):
  """Verifies compatibility of shape and default_value."""
  # Invalid condition:
  #  * if default_value is not a scalar and shape is empty
  #  * or if default_value is an iterable and shape is not empty
  if nest.is_sequence(default_value) != bool(shape):
    return False
  if not shape:
    return True
  if len(default_value) != shape[0]:
    return False
  for i in range(shape[0]):
    if not _is_shape_and_default_value_compatible(default_value[i], shape[1:]):
      return False
  return True


def _check_default_value(shape, default_value, dtype, key):
  """Returns default value as tuple if it's valid, otherwise raises errors.

  This function verifies that `default_value` is compatible with both `shape`
  and `dtype`. If it is not compatible, it raises an error. If it is compatible,
  it casts default_value to a tuple and returns it. `key` is used only
  for error message.

  Args:
    shape: An iterable of integers specifies the shape of the `Tensor`.
    default_value: If a single value is provided, the same value will be applied
      as the default value for every item. If an iterable of values is
      provided, the shape of the `default_value` should be equal to the given
      `shape`.
    dtype: defines the type of values. Default value is `tf.float32`. Must be a
      non-quantized, real integer or floating point type.
    key: Column name, used only for error messages.

  Returns:
    A tuple which will be used as default value.

  Raises:
    TypeError: if `default_value` is an iterable but not compatible with `shape`
    TypeError: if `default_value` is not compatible with `dtype`.
    ValueError: if `dtype` is not convertible to `tf.float32`.
  """
  if default_value is None:
    return None

  if isinstance(default_value, int):
    return _create_tuple(shape, default_value)

  if isinstance(default_value, float) and dtype.is_floating:
    return _create_tuple(shape, default_value)

  if callable(getattr(default_value, 'tolist', None)):  # Handles numpy arrays
    default_value = default_value.tolist()

  if nest.is_sequence(default_value):
    if not _is_shape_and_default_value_compatible(default_value, shape):
      raise ValueError(
          'The shape of default_value must be equal to given shape. '
          'default_value: {}, shape: {}, key: {}'.format(
              default_value, shape, key))
    # Check if the values in the list are all integers or are convertible to
    # floats.
    is_list_all_int = all(
        isinstance(v, int) for v in nest.flatten(default_value))
    is_list_has_float = any(
        isinstance(v, float) for v in nest.flatten(default_value))
    if is_list_all_int:
      return _as_tuple(default_value)
    if is_list_has_float and dtype.is_floating:
      return _as_tuple(default_value)
  raise TypeError('default_value must be compatible with dtype. '
                  'default_value: {}, dtype: {}, key: {}'.format(
                      default_value, dtype, key))


class HashedCategoricalColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('HashedCategoricalColumn',
                           ('key', 'hash_bucket_size', 'dtype'))):
  """see `categorical_column_with_hash_bucket`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.key

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return {self.key: parsing_ops.VarLenFeature(self.dtype)}

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def _transform_input_tensor(self, input_tensor):
    """Hashes the values in the feature_column."""
    if not isinstance(input_tensor, sparse_tensor_lib.SparseTensor):
      raise ValueError('SparseColumn input must be a SparseTensor.')

    _assert_string_or_int(
        input_tensor.dtype,
        prefix='column_name: {} input_tensor'.format(self.key))

    if self.dtype.is_integer != input_tensor.dtype.is_integer:
      raise ValueError(
          'Column dtype and SparseTensors dtype must be compatible. '
          'key: {}, column dtype: {}, tensor dtype: {}'.format(
              self.key, self.dtype, input_tensor.dtype))

    if self.dtype == dtypes.string:
      sparse_values = input_tensor.values
    else:
      sparse_values = string_ops.as_string(input_tensor.values)

    sparse_id_values = string_ops.string_to_hash_bucket_fast(
        sparse_values, self.hash_bucket_size, name='lookup')
    return sparse_tensor_lib.SparseTensor(
        input_tensor.indices, sparse_id_values, input_tensor.dense_shape)

  def transform_feature(self, transformation_cache, state_manager):
    """Hashes the values in the feature_column."""
    input_tensor = _to_sparse_input_and_drop_ignore_values(
        transformation_cache.get(self.key, state_manager))
    return self._transform_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    input_tensor = _to_sparse_input_and_drop_ignore_values(inputs.get(self.key))
    return self._transform_input_tensor(input_tensor)

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return self.hash_bucket_size

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    return CategoricalColumn.IdWeightPair(
        transformation_cache.get(self, state_manager), None)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    del weight_collections
    del trainable
    return CategoricalColumn.IdWeightPair(inputs.get(self), None)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['dtype'] = self.dtype.name
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
    return cls(**kwargs)


class VocabularyFileCategoricalColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('VocabularyFileCategoricalColumn',
                           ('key', 'vocabulary_file', 'vocabulary_size',
                            'num_oov_buckets', 'dtype', 'default_value'))):
  """See `categorical_column_with_vocabulary_file`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.key

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return {self.key: parsing_ops.VarLenFeature(self.dtype)}

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def _transform_input_tensor(self, input_tensor):
    """Creates a lookup table for the vocabulary."""
    if self.dtype.is_integer != input_tensor.dtype.is_integer:
      raise ValueError(
          'Column dtype and SparseTensors dtype must be compatible. '
          'key: {}, column dtype: {}, tensor dtype: {}'.format(
              self.key, self.dtype, input_tensor.dtype))

    _assert_string_or_int(
        input_tensor.dtype,
        prefix='column_name: {} input_tensor'.format(self.key))

    key_dtype = self.dtype
    if input_tensor.dtype.is_integer:
      # `index_table_from_file` requires 64-bit integer keys.
      key_dtype = dtypes.int64
      input_tensor = math_ops.to_int64(input_tensor)

    # TODO(rohanj): Use state manager to manage the index table creation.
    return lookup_ops.index_table_from_file(
        vocabulary_file=self.vocabulary_file,
        num_oov_buckets=self.num_oov_buckets,
        vocab_size=self.vocabulary_size,
        default_value=self.default_value,
        key_dtype=key_dtype,
        name='{}_lookup'.format(self.key)).lookup(input_tensor)

  def transform_feature(self, transformation_cache, state_manager):
    """Creates a lookup table for the vocabulary."""
    input_tensor = _to_sparse_input_and_drop_ignore_values(
        transformation_cache.get(self.key, state_manager))
    return self._transform_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    input_tensor = _to_sparse_input_and_drop_ignore_values(inputs.get(self.key))
    return self._transform_input_tensor(input_tensor)

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return self.vocabulary_size + self.num_oov_buckets

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    return CategoricalColumn.IdWeightPair(
        transformation_cache.get(self, state_manager), None)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    del weight_collections
    del trainable
    return CategoricalColumn.IdWeightPair(inputs.get(self), None)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['dtype'] = self.dtype.name
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
    return cls(**kwargs)


class VocabularyListCategoricalColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple(
        'VocabularyListCategoricalColumn',
        ('key', 'vocabulary_list', 'dtype', 'default_value', 'num_oov_buckets'))
):
  """See `categorical_column_with_vocabulary_list`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.key

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return {self.key: parsing_ops.VarLenFeature(self.dtype)}

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def _transform_input_tensor(self, input_tensor):
    """Creates a lookup table for the vocabulary list."""
    if self.dtype.is_integer != input_tensor.dtype.is_integer:
      raise ValueError(
          'Column dtype and SparseTensors dtype must be compatible. '
          'key: {}, column dtype: {}, tensor dtype: {}'.format(
              self.key, self.dtype, input_tensor.dtype))

    _assert_string_or_int(
        input_tensor.dtype,
        prefix='column_name: {} input_tensor'.format(self.key))

    key_dtype = self.dtype
    if input_tensor.dtype.is_integer:
      # `index_table_from_tensor` requires 64-bit integer keys.
      key_dtype = dtypes.int64
      input_tensor = math_ops.to_int64(input_tensor)

    # TODO(rohanj): Use state manager to manage the index table creation.
    return lookup_ops.index_table_from_tensor(
        vocabulary_list=tuple(self.vocabulary_list),
        default_value=self.default_value,
        num_oov_buckets=self.num_oov_buckets,
        dtype=key_dtype,
        name='{}_lookup'.format(self.key)).lookup(input_tensor)

  def transform_feature(self, transformation_cache, state_manager):
    """Creates a lookup table for the vocabulary list."""
    input_tensor = _to_sparse_input_and_drop_ignore_values(
        transformation_cache.get(self.key, state_manager))
    return self._transform_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    input_tensor = _to_sparse_input_and_drop_ignore_values(inputs.get(self.key))
    return self._transform_input_tensor(input_tensor)

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return len(self.vocabulary_list) + self.num_oov_buckets

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    return CategoricalColumn.IdWeightPair(
        transformation_cache.get(self, state_manager), None)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    del weight_collections
    del trainable
    return CategoricalColumn.IdWeightPair(inputs.get(self), None)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['dtype'] = self.dtype.name
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
    return cls(**kwargs)


class IdentityCategoricalColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('IdentityCategoricalColumn',
                           ('key', 'number_buckets', 'default_value'))):

  """See `categorical_column_with_identity`."""

  @property
  def _is_v2_column(self):
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.key

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return {self.key: parsing_ops.VarLenFeature(dtypes.int64)}

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def _transform_input_tensor(self, input_tensor):
    """Returns a SparseTensor with identity values."""
    if not input_tensor.dtype.is_integer:
      raise ValueError(
          'Invalid input, not integer. key: {} dtype: {}'.format(
              self.key, input_tensor.dtype))

    values = math_ops.to_int64(input_tensor.values, name='values')
    num_buckets = math_ops.to_int64(self.num_buckets, name='num_buckets')
    zero = math_ops.to_int64(0, name='zero')
    if self.default_value is None:
      # Fail if values are out-of-range.
      assert_less = check_ops.assert_less(
          values, num_buckets, data=(values, num_buckets),
          name='assert_less_than_num_buckets')
      assert_greater = check_ops.assert_greater_equal(
          values, zero, data=(values,),
          name='assert_greater_or_equal_0')
      with ops.control_dependencies((assert_less, assert_greater)):
        values = array_ops.identity(values)
    else:
      # Assign default for out-of-range values.
      values = array_ops.where(
          math_ops.logical_or(
              values < zero, values >= num_buckets, name='out_of_range'),
          array_ops.fill(
              dims=array_ops.shape(values),
              value=math_ops.to_int64(self.default_value),
              name='default_values'),
          values)

    return sparse_tensor_lib.SparseTensor(
        indices=input_tensor.indices,
        values=values,
        dense_shape=input_tensor.dense_shape)

  def transform_feature(self, transformation_cache, state_manager):
    """Returns a SparseTensor with identity values."""
    input_tensor = _to_sparse_input_and_drop_ignore_values(
        transformation_cache.get(self.key, state_manager))
    return self._transform_input_tensor(input_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    input_tensor = _to_sparse_input_and_drop_ignore_values(inputs.get(self.key))
    return self._transform_input_tensor(input_tensor)

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return self.number_buckets

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    return CategoricalColumn.IdWeightPair(
        transformation_cache.get(self, state_manager), None)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    del weight_collections
    del trainable
    return CategoricalColumn.IdWeightPair(inputs.get(self), None)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    return dict(zip(self._fields, self))

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    return cls(**config)


class WeightedCategoricalColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple(
        'WeightedCategoricalColumn',
        ('categorical_column', 'weight_feature_key', 'dtype'))):
  """See `weighted_categorical_column`."""

  @property
  def _is_v2_column(self):
    return (isinstance(self.categorical_column, FeatureColumn) and
            self.categorical_column._is_v2_column)  # pylint: disable=protected-access

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return '{}_weighted_by_{}'.format(
        self.categorical_column.name, self.weight_feature_key)

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    config = self.categorical_column.parse_example_spec
    if self.weight_feature_key in config:
      raise ValueError('Parse config {} already exists for {}.'.format(
          config[self.weight_feature_key], self.weight_feature_key))
    config[self.weight_feature_key] = parsing_ops.VarLenFeature(self.dtype)
    return config

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    config = self.categorical_column._parse_example_spec  # pylint: disable=protected-access
    if self.weight_feature_key in config:
      raise ValueError('Parse config {} already exists for {}.'.format(
          config[self.weight_feature_key], self.weight_feature_key))
    config[self.weight_feature_key] = parsing_ops.VarLenFeature(self.dtype)
    return config

  @property
  def num_buckets(self):
    """See `DenseColumn` base class."""
    return self.categorical_column.num_buckets

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.categorical_column._num_buckets  # pylint: disable=protected-access

  def _transform_weight_tensor(self, weight_tensor):
    if weight_tensor is None:
      raise ValueError('Missing weights {}.'.format(self.weight_feature_key))
    weight_tensor = sparse_tensor_lib.convert_to_tensor_or_sparse_tensor(
        weight_tensor)
    if self.dtype != weight_tensor.dtype.base_dtype:
      raise ValueError('Bad dtype, expected {}, but got {}.'.format(
          self.dtype, weight_tensor.dtype))
    if not isinstance(weight_tensor, sparse_tensor_lib.SparseTensor):
      # The weight tensor can be a regular Tensor. In this case, sparsify it.
      weight_tensor = _to_sparse_input_and_drop_ignore_values(
          weight_tensor, ignore_value=0.0)
    if not weight_tensor.dtype.is_floating:
      weight_tensor = math_ops.to_float(weight_tensor)
    return weight_tensor

  def transform_feature(self, transformation_cache, state_manager):
    """Applies weights to tensor generated from `categorical_column`'."""
    print('WeightedCategoricalColumn.transform_feature: ', self.name)
    print('Weight feature key: ', self.weight_feature_key)
    weight_tensor = transformation_cache.get(self.weight_feature_key,
                                             state_manager)
    print('Weight tensor before: ', weight_tensor)
    weight_tensor = self._transform_weight_tensor(weight_tensor)
    print('Weight tensor after: ', weight_tensor)
    return (transformation_cache.get(self.categorical_column, state_manager),
            weight_tensor)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    """Applies weights to tensor generated from `categorical_column`'."""
    weight_tensor = inputs.get(self.weight_feature_key)
    weight_tensor = self._transform_weight_tensor(weight_tensor)
    return (inputs.get(self.categorical_column), weight_tensor)

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    print('WeightedCategoricalColumn.get_sparse_tensors: ', self.name)
    tensors = transformation_cache.get(self, state_manager)
    print('tensors[1]: ', tensors[1])
    return CategoricalColumn.IdWeightPair(tensors[0], tensors[1])

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    del weight_collections
    del trainable
    tensors = inputs.get(self)
    return CategoricalColumn.IdWeightPair(tensors[0], tensors[1])

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.categorical_column, self.weight_feature_key]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['categorical_column'] = serialize_feature_column(
        self.categorical_column)
    config['dtype'] = self.dtype.name
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['categorical_column'] = deserialize_feature_column(
        config['categorical_column'], custom_objects, columns_by_name)
    kwargs['dtype'] = dtypes.as_dtype(config['dtype'])
    return cls(**kwargs)


class CrossedColumn(
    CategoricalColumn,
    fc_old._CategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('CrossedColumn',
                           ('keys', 'hash_bucket_size', 'hash_key'))):
  """See `crossed_column`."""

  @property
  def _is_v2_column(self):
    for key in _collect_leaf_level_keys(self):
      if isinstance(key, six.string_types):
        continue
      if not isinstance(key, FeatureColumn):
        return False
      if not key._is_v2_column:  # pylint: disable=protected-access
        return False
    return True

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    feature_names = []
    for key in _collect_leaf_level_keys(self):
      if isinstance(key, (FeatureColumn, fc_old._FeatureColumn)):  # pylint: disable=protected-access
        feature_names.append(key.name)
      else:  # key must be a string
        feature_names.append(key)
    return '_X_'.join(sorted(feature_names))

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    config = {}
    for key in self.keys:
      if isinstance(key, FeatureColumn):
        config.update(key.parse_example_spec)
      elif isinstance(key, fc_old._FeatureColumn):  # pylint: disable=protected-access
        config.update(key._parse_example_spec)  # pylint: disable=protected-access
      else:  # key must be a string
        config.update({key: parsing_ops.VarLenFeature(dtypes.string)})
    return config

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.parse_example_spec

  def transform_feature(self, transformation_cache, state_manager):
    """Generates a hashed sparse cross from the input tensors."""
    feature_tensors = []
    for key in _collect_leaf_level_keys(self):
      if isinstance(key, six.string_types):
        feature_tensors.append(transformation_cache.get(key, state_manager))
      elif isinstance(key, (fc_old._CategoricalColumn, CategoricalColumn)):  # pylint: disable=protected-access
        ids_and_weights = key.get_sparse_tensors(transformation_cache,
                                                 state_manager)
        if ids_and_weights.weight_tensor is not None:
          raise ValueError(
              'crossed_column does not support weight_tensor, but the given '
              'column populates weight_tensor. '
              'Given column: {}'.format(key.name))
        feature_tensors.append(ids_and_weights.id_tensor)
      else:
        raise ValueError('Unsupported column type. Given: {}'.format(key))
    return sparse_ops.sparse_cross_hashed(
        inputs=feature_tensors,
        num_buckets=self.hash_bucket_size,
        hash_key=self.hash_key)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    """Generates a hashed sparse cross from the input tensors."""
    feature_tensors = []
    for key in _collect_leaf_level_keys(self):
      if isinstance(key, six.string_types):
        feature_tensors.append(inputs.get(key))
      elif isinstance(key, (CategoricalColumn, fc_old._CategoricalColumn)):  # pylint: disable=protected-access
        ids_and_weights = key._get_sparse_tensors(inputs)  # pylint: disable=protected-access
        if ids_and_weights.weight_tensor is not None:
          raise ValueError(
              'crossed_column does not support weight_tensor, but the given '
              'column populates weight_tensor. '
              'Given column: {}'.format(key.name))
        feature_tensors.append(ids_and_weights.id_tensor)
      else:
        raise ValueError('Unsupported column type. Given: {}'.format(key))
    return sparse_ops.sparse_cross_hashed(
        inputs=feature_tensors,
        num_buckets=self.hash_bucket_size,
        hash_key=self.hash_key)

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return self.hash_bucket_size

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.num_buckets

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """See `CategoricalColumn` base class."""
    return CategoricalColumn.IdWeightPair(
        transformation_cache.get(self, state_manager), None)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    """See `CategoricalColumn` base class."""
    del weight_collections
    del trainable
    return CategoricalColumn.IdWeightPair(inputs.get(self), None)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return list(self.keys)

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['keys'] = tuple([serialize_feature_column(fc) for fc in self.keys])
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['keys'] = tuple([
        deserialize_feature_column(c, custom_objects, columns_by_name)
        for c in config['keys']
    ])
    return cls(**kwargs)


def _collect_leaf_level_keys(cross):
  """Collects base keys by expanding all nested crosses.

  Args:
    cross: A `CrossedColumn`.

  Returns:
    A list of strings or `CategoricalColumn` instances.
  """
  leaf_level_keys = []
  for k in cross.keys:
    if isinstance(k, CrossedColumn):
      leaf_level_keys.extend(_collect_leaf_level_keys(k))
    else:
      leaf_level_keys.append(k)
  return leaf_level_keys


def _prune_invalid_ids(sparse_ids, sparse_weights):
  """Prune invalid IDs (< 0) from the input ids and weights."""
  is_id_valid = math_ops.greater_equal(sparse_ids.values, 0)
  if sparse_weights is not None:
    is_id_valid = math_ops.logical_and(
        is_id_valid,
        array_ops.ones_like(sparse_weights.values, dtype=dtypes.bool))
  sparse_ids = sparse_ops.sparse_retain(sparse_ids, is_id_valid)
  if sparse_weights is not None:
    sparse_weights = sparse_ops.sparse_retain(sparse_weights, is_id_valid)
  return sparse_ids, sparse_weights


def _prune_invalid_weights(sparse_ids, sparse_weights):
  """Prune invalid weights (< 0) from the input ids and weights."""
  if sparse_weights is not None:
    is_weights_valid = math_ops.greater(sparse_weights.values, 0)
    sparse_ids = sparse_ops.sparse_retain(sparse_ids, is_weights_valid)
    sparse_weights = sparse_ops.sparse_retain(sparse_weights, is_weights_valid)
  return sparse_ids, sparse_weights


class IndicatorColumn(
    DenseColumn,
    SequenceDenseColumn,
    fc_old._DenseColumn,  # pylint: disable=protected-access
    fc_old._SequenceDenseColumn,  # pylint: disable=protected-access
    collections.namedtuple('IndicatorColumn', ('categorical_column'))):
  """Represents a one-hot column for use in deep networks.

  Args:
    categorical_column: A `CategoricalColumn` which is created by
      `categorical_column_with_*` function.
  """

  @property
  def _is_v2_column(self):
    return (isinstance(self.categorical_column, FeatureColumn) and
            self.categorical_column._is_v2_column)  # pylint: disable=protected-access

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return '{}_indicator'.format(self.categorical_column.name)

  def _transform_id_weight_pair(self, id_weight_pair):
    id_tensor = id_weight_pair.id_tensor
    weight_tensor = id_weight_pair.weight_tensor

    # If the underlying column is weighted, return the input as a dense tensor.
    if weight_tensor is not None:
      weighted_column = sparse_ops.sparse_merge(
          sp_ids=id_tensor,
          sp_values=weight_tensor,
          vocab_size=int(self._variable_shape[-1]))
      # Remove (?, -1) index.
      weighted_column = sparse_ops.sparse_slice(weighted_column, [0, 0],
                                                weighted_column.dense_shape)
      # Use scatter_nd to merge duplicated indices if existed,
      # instead of sparse_tensor_to_dense.
      return array_ops.scatter_nd(weighted_column.indices,
                                  weighted_column.values,
                                  weighted_column.dense_shape)

    dense_id_tensor = sparse_ops.sparse_tensor_to_dense(
        id_tensor, default_value=-1)

    # One hot must be float for tf.concat reasons since all other inputs to
    # input_layer are float32.
    one_hot_id_tensor = array_ops.one_hot(
        dense_id_tensor,
        depth=self._variable_shape[-1],
        on_value=1.0,
        off_value=0.0)

    # Reduce to get a multi-hot per example.
    return math_ops.reduce_sum(one_hot_id_tensor, axis=[-2])

  def transform_feature(self, transformation_cache, state_manager):
    """Returns dense `Tensor` representing feature.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Transformed feature `Tensor`.

    Raises:
      ValueError: if input rank is not known at graph building time.
    """
    id_weight_pair = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    return self._transform_id_weight_pair(id_weight_pair)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    id_weight_pair = self.categorical_column._get_sparse_tensors(inputs)  # pylint: disable=protected-access
    return self._transform_id_weight_pair(id_weight_pair)

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return self.categorical_column.parse_example_spec

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.categorical_column._parse_example_spec  # pylint: disable=protected-access

  @property
  def variable_shape(self):
    """Returns a `TensorShape` representing the shape of the dense `Tensor`."""
    if isinstance(self.categorical_column, FeatureColumn):
      return tensor_shape.TensorShape([1, self.categorical_column.num_buckets])
    else:
      return tensor_shape.TensorShape([1, self.categorical_column._num_buckets])  # pylint: disable=protected-access

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _variable_shape(self):
    return tensor_shape.TensorShape([1, self.categorical_column._num_buckets])  # pylint: disable=protected-access

  def get_dense_tensor(self, transformation_cache, state_manager):
    """Returns dense `Tensor` representing feature.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.

    Returns:
      Dense `Tensor` created within `transform_feature`.

    Raises:
      ValueError: If `categorical_column` is a `SequenceCategoricalColumn`.
    """
    if isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In indicator_column: {}. '
          'categorical_column must not be of type SequenceCategoricalColumn. '
          'Suggested fix A: If you wish to use input_layer, use a '
          'non-sequence categorical_column_with_*. '
          'Suggested fix B: If you wish to create sequence input, use '
          'sequence_input_layer instead of input_layer. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    # Feature has been already transformed. Return the intermediate
    # representation created by transform_feature.
    return transformation_cache.get(self, state_manager)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_dense_tensor(self, inputs, weight_collections=None, trainable=None):
    del weight_collections
    del trainable
    if isinstance(
        self.categorical_column,
        (SequenceCategoricalColumn, fc_old._SequenceCategoricalColumn)):  # pylint: disable=protected-access
      raise ValueError(
          'In indicator_column: {}. '
          'categorical_column must not be of type _SequenceCategoricalColumn. '
          'Suggested fix A: If you wish to use input_layer, use a '
          'non-sequence categorical_column_with_*. '
          'Suggested fix B: If you wish to create sequence input, use '
          'sequence_input_layer instead of input_layer. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    # Feature has been already transformed. Return the intermediate
    # representation created by transform_feature.
    return inputs.get(self)

  def get_sequence_dense_tensor(self, transformation_cache, state_manager):
    """See `SequenceDenseColumn` base class."""
    if not isinstance(self.categorical_column, SequenceCategoricalColumn):
      raise ValueError(
          'In indicator_column: {}. '
          'categorical_column must be of type SequenceCategoricalColumn '
          'to use sequence_input_layer. '
          'Suggested fix: Use one of sequence_categorical_column_with_*. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    # Feature has been already transformed. Return the intermediate
    # representation created by transform_feature.
    dense_tensor = transformation_cache.get(self, state_manager)
    sparse_tensors = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    sequence_length = fc_old._sequence_length_from_sparse_tensor(  # pylint: disable=protected-access
        sparse_tensors.id_tensor)
    return SequenceDenseColumn.TensorSequenceLengthPair(
        dense_tensor=dense_tensor, sequence_length=sequence_length)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sequence_dense_tensor(self,
                                 inputs,
                                 weight_collections=None,
                                 trainable=None):
    # Do nothing with weight_collections and trainable since no variables are
    # created in this function.
    del weight_collections
    del trainable
    if not isinstance(
        self.categorical_column,
        (SequenceCategoricalColumn, fc_old._SequenceCategoricalColumn)):  # pylint: disable=protected-access
      raise ValueError(
          'In indicator_column: {}. '
          'categorical_column must be of type _SequenceCategoricalColumn '
          'to use sequence_input_layer. '
          'Suggested fix: Use one of sequence_categorical_column_with_*. '
          'Given (type {}): {}'.format(self.name, type(self.categorical_column),
                                       self.categorical_column))
    # Feature has been already transformed. Return the intermediate
    # representation created by _transform_feature.
    dense_tensor = inputs.get(self)
    sparse_tensors = self.categorical_column._get_sparse_tensors(inputs)  # pylint: disable=protected-access
    sequence_length = fc_old._sequence_length_from_sparse_tensor(  # pylint: disable=protected-access
        sparse_tensors.id_tensor)
    return SequenceDenseColumn.TensorSequenceLengthPair(
        dense_tensor=dense_tensor, sequence_length=sequence_length)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.categorical_column]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['categorical_column'] = serialize_feature_column(
        self.categorical_column)
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['categorical_column'] = deserialize_feature_column(
        config['categorical_column'], custom_objects, columns_by_name)
    return cls(**kwargs)


def _verify_static_batch_size_equality(tensors, columns):
  """Verify equality between static batch sizes.

  Args:
    tensors: iterable of input tensors.
    columns: Corresponding feature columns.

  Raises:
    ValueError: in case of mismatched batch sizes.
  """
  # bath_size is a tf.Dimension object.
  expected_batch_size = None
  for i in range(0, len(tensors)):
    batch_size = tensor_shape.Dimension(tensor_shape.dimension_value(
        tensors[i].shape[0]))
    if batch_size.value is not None:
      if expected_batch_size is None:
        bath_size_column_index = i
        expected_batch_size = batch_size
      elif not expected_batch_size.is_compatible_with(batch_size):
        raise ValueError(
            'Batch size (first dimension) of each feature must be same. '
            'Batch size of columns ({}, {}): ({}, {})'.format(
                columns[bath_size_column_index].name, columns[i].name,
                expected_batch_size, batch_size))


class SequenceCategoricalColumn(
    CategoricalColumn,
    fc_old._SequenceCategoricalColumn,  # pylint: disable=protected-access
    collections.namedtuple('SequenceCategoricalColumn',
                           ('categorical_column'))):
  """Represents sequences of categorical data."""

  @property
  def _is_v2_column(self):
    return (isinstance(self.categorical_column, FeatureColumn) and
            self.categorical_column._is_v2_column)  # pylint: disable=protected-access

  @property
  def name(self):
    """See `FeatureColumn` base class."""
    return self.categorical_column.name

  @property
  def parse_example_spec(self):
    """See `FeatureColumn` base class."""
    return self.categorical_column.parse_example_spec

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _parse_example_spec(self):
    return self.categorical_column._parse_example_spec  # pylint: disable=protected-access

  def transform_feature(self, transformation_cache, state_manager):
    """See `FeatureColumn` base class."""
    return self.categorical_column.transform_feature(transformation_cache,
                                                     state_manager)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _transform_feature(self, inputs):
    return self.categorical_column._transform_feature(inputs)  # pylint: disable=protected-access

  @property
  def num_buckets(self):
    """Returns number of buckets in this sparse feature."""
    return self.categorical_column.num_buckets

  @property
  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _num_buckets(self):
    return self.categorical_column._num_buckets  # pylint: disable=protected-access

  def _get_sparse_tensors_helper(self, sparse_tensors):
    id_tensor = sparse_tensors.id_tensor
    weight_tensor = sparse_tensors.weight_tensor
    # Expands third dimension, if necessary so that embeddings are not
    # combined during embedding lookup. If the tensor is already 3D, leave
    # as-is.
    shape = array_ops.shape(id_tensor)
    # Compute the third dimension explicitly instead of setting it to -1, as
    # that doesn't work for dynamically shaped tensors with 0-length at runtime.
    # This happens for empty sequences.
    target_shape = [shape[0], shape[1], math_ops.reduce_prod(shape[2:])]
    id_tensor = sparse_ops.sparse_reshape(id_tensor, target_shape)
    if weight_tensor is not None:
      weight_tensor = sparse_ops.sparse_reshape(weight_tensor, target_shape)
    return CategoricalColumn.IdWeightPair(id_tensor, weight_tensor)

  def get_sparse_tensors(self, transformation_cache, state_manager):
    """Returns an IdWeightPair.

    `IdWeightPair` is a pair of `SparseTensor`s which represents ids and
    weights.

    `IdWeightPair.id_tensor` is typically a `batch_size` x `num_buckets`
    `SparseTensor` of `int64`. `IdWeightPair.weight_tensor` is either a
    `SparseTensor` of `float` or `None` to indicate all weights should be
    taken to be 1. If specified, `weight_tensor` must have exactly the same
    shape and indices as `sp_ids`. Expected `SparseTensor` is same as parsing
    output of a `VarLenFeature` which is a ragged matrix.

    Args:
      transformation_cache: A `FeatureTransformationCache` object to access
        features.
      state_manager: A `StateManager` to create / access resources such as
        lookup tables.
    """
    sparse_tensors = self.categorical_column.get_sparse_tensors(
        transformation_cache, state_manager)
    return self._get_sparse_tensors_helper(sparse_tensors)

  @deprecation.deprecated(_FEATURE_COLUMN_DEPRECATION_DATE,
                          _FEATURE_COLUMN_DEPRECATION)
  def _get_sparse_tensors(self, inputs, weight_collections=None,
                          trainable=None):
    sparse_tensors = self.categorical_column._get_sparse_tensors(inputs)  # pylint: disable=protected-access
    return self._get_sparse_tensors_helper(sparse_tensors)

  @property
  def parents(self):
    """See 'FeatureColumn` base class."""
    return [self.categorical_column]

  def _get_config(self):
    """See 'FeatureColumn` base class."""
    config = dict(zip(self._fields, self))
    config['categorical_column'] = serialize_feature_column(
        self.categorical_column)
    return config

  @classmethod
  def _from_config(cls, config, custom_objects=None, columns_by_name=None):
    """See 'FeatureColumn` base class."""
    _check_config_keys(config, cls._fields)
    kwargs = config.copy()
    kwargs['categorical_column'] = deserialize_feature_column(
        config['categorical_column'], custom_objects, columns_by_name)
    return cls(**kwargs)


# FeatureColumn serialization, deserialization logic.


def _check_config_keys(config, expected_keys):
  """Checks that a config has all expected_keys."""
  if set(config.keys()) != set(expected_keys):
    raise ValueError('Invalid config: {}, expected keys: {}'.format(
        config, expected_keys))


def serialize_feature_column(fc):
  """Serializes a FeatureColumn or a raw string key.

  This method should only be used to serialize parent FeatureColumns when
  implementing FeatureColumn._get_config(), else serialize_feature_columns()
  is preferable.

  This serialization also keeps information of the FeatureColumn class, so
  deserialization is possible without knowing the class type. For example:

  a = numeric_column('x')
  a._get_config() gives:
  {
      'key': 'price',
      'shape': (1,),
      'default_value': None,
      'dtype': 'float32',
      'normalizer_fn': None
  }
  While serialize_feature_column(a) gives:
  {
      'class_name': 'NumericColumn',
      'config': {
          'key': 'price',
          'shape': (1,),
          'default_value': None,
          'dtype': 'float32',
          'normalizer_fn': None
      }
  }

  Args:
    fc: A FeatureColumn or raw feature key string.

  Returns:
    Keras serialization for FeatureColumns, leaves string keys unaffected.

  Raises:
    ValueError if called with input that is not string or FeatureColumn.
  """
  if isinstance(fc, six.string_types):
    return fc
  elif isinstance(fc, FeatureColumn):
    return utils.serialize_keras_class_and_config(fc.__class__.__name__,
                                                  fc._get_config())
  else:
    raise ValueError('Instance: {} is not a FeatureColumn'.format(fc))


def deserialize_feature_column(config,
                               custom_objects=None,
                               columns_by_name=None):
  """Deserializes a `config` generated with `serialize_feature_column`.

  This method should only be used to deserialize parent FeatureColumns when
  implementing FeatureColumn._from_config(), else deserialize_feature_columns()
  is preferable. Returns a FeatureColumn for this config.
  TODO(b/118939620): Simplify code if Keras utils support object deduping.

  Args:
    config: A Dict with the serialization of feature columns acquired by
      `serialize_feature_column`, or a string representing a raw column.
    custom_objects: A Dict from custom_object name to the associated keras
      serializable objects (FeatureColumns, classes or functions).
    columns_by_name: A Dict[String, FeatureColumn] of existing columns in order
      to avoid duplication.

  Raises:
    ValueError if `config` has invalid format (e.g: expected keys missing,
    or refers to unknown classes).

  Returns:
    A FeatureColumn corresponding to the input `config`.
  """
  if isinstance(config, six.string_types):
    return config
  # A dict from class_name to class for all FeatureColumns in this module.
  # FeatureColumns not part of the module can be passed as custom_objects.
  module_feature_column_classes = {
      cls.__name__: cls for cls in [
          BucketizedColumn, EmbeddingColumn, HashedCategoricalColumn,
          IdentityCategoricalColumn, IndicatorColumn, NumericColumn,
          SequenceCategoricalColumn, SequenceDenseColumn, SharedEmbeddingColumn,
          VocabularyFileCategoricalColumn, VocabularyListCategoricalColumn,
          WeightedCategoricalColumn
      ]
  }
  if columns_by_name is None:
    columns_by_name = {}

  (cls, cls_config) = utils.class_and_config_for_serialized_keras_object(
      config,
      module_objects=module_feature_column_classes,
      custom_objects=custom_objects,
      printable_module_name='feature_column_v2')

  if not issubclass(cls, FeatureColumn):
    raise ValueError(
        'Expected FeatureColumn class, instead found: {}'.format(cls))

  # Always deserialize the FeatureColumn, in order to get the name.
  new_instance = cls._from_config(  # pylint: disable=protected-access
      cls_config,
      custom_objects=custom_objects,
      columns_by_name=columns_by_name)

  # If the name already exists, re-use the column from columns_by_name,
  # (new_instance remains unused).
  return columns_by_name.setdefault(new_instance.name, new_instance)


def serialize_feature_columns(feature_columns):
  """Serializes a list of FeatureColumns.

  Returns a list of Keras-style config dicts that represent the input
  FeatureColumns and can be used with `deserialize_feature_columns` for
  reconstructing the original columns.

  Args:
    feature_columns: A list of FeatureColumns.

  Returns:
    Keras serialization for the list of FeatureColumns.

  Raises:
    ValueError if called with input that is not a list of FeatureColumns.
  """
  return [serialize_feature_column(fc) for fc in feature_columns]


def deserialize_feature_columns(configs, custom_objects=None):
  """Deserializes a list of FeatureColumns configs.

  Returns a list of FeatureColumns given a list of config dicts acquired by
  `serialize_feature_columns`.

  Args:
    configs: A list of Dicts with the serialization of feature columns acquired
      by `serialize_feature_columns`.
    custom_objects: A Dict from custom_object name to the associated keras
      serializable objects (FeatureColumns, classes or functions).

  Returns:
    FeatureColumn objects corresponding to the input configs.

  Raises:
    ValueError if called with input that is not a list of FeatureColumns.
  """
  columns_by_name = {}
  return [
      deserialize_feature_column(c, custom_objects, columns_by_name)
      for c in configs
  ]
