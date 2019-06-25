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
"""Utilities for unit-testing Keras."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import collections
import functools
import itertools
import unittest

from absl.testing import parameterized

from tensorflow.python import keras
from tensorflow.python import tf2
from tensorflow.python.eager import context
from tensorflow.python.keras import testing_utils
from tensorflow.python.platform import test
from tensorflow.python.util import nest


class TestCase(test.TestCase, parameterized.TestCase):

  def tearDown(self):
    keras.backend.clear_session()
    super(TestCase, self).tearDown()


# TODO(kaftan): Possibly enable 'subclass_custom_build' when tests begin to pass
# it. Or perhaps make 'subclass' always use a custom build method.
def run_with_all_model_types(
    test_or_class=None,
    exclude_models=None):
  """Execute the decorated test with all Keras model types.

  This decorator is intended to be applied either to individual test methods in
  a `keras_parameterized.TestCase` class, or directly to a test class that
  extends it. Doing so will cause the contents of the individual test
  method (or all test methods in the class) to be executed multiple times - once
  for each Keras model type.

  The Keras model types are: ['functional', 'subclass', 'sequential']

  Note: if stacking this decorator with absl.testing's parameterized decorators,
  those should be at the bottom of the stack.

  Various methods in `testing_utils` to get models will auto-generate a model
  of the currently active Keras model type. This allows unittests to confirm
  the equivalence between different Keras models.

  For example, consider the following unittest:

  ```python
  class MyTests(testing_utils.KerasTestCase):

    @testing_utils.run_with_all_model_types(
      exclude_models = ['sequential'])
    def test_foo(self):
      model = testing_utils.get_small_mlp(1, 4, input_dim=3)
      optimizer = RMSPropOptimizer(learning_rate=0.001)
      loss = 'mse'
      metrics = ['mae']
      model.compile(optimizer, loss, metrics=metrics)

      inputs = np.zeros((10, 3))
      targets = np.zeros((10, 4))
      dataset = dataset_ops.Dataset.from_tensor_slices((inputs, targets))
      dataset = dataset.repeat(100)
      dataset = dataset.batch(10)

      model.fit(dataset, epochs=1, steps_per_epoch=2, verbose=1)

  if __name__ == "__main__":
    tf.test.main()
  ```

  This test tries building a small mlp as both a functional model and as a
  subclass model.

  We can also annotate the whole class if we want this to apply to all tests in
  the class:
  ```python
  @testing_utils.run_with_all_model_types(exclude_models = ['sequential'])
  class MyTests(testing_utils.KerasTestCase):

    def test_foo(self):
      model = testing_utils.get_small_mlp(1, 4, input_dim=3)
      optimizer = RMSPropOptimizer(learning_rate=0.001)
      loss = 'mse'
      metrics = ['mae']
      model.compile(optimizer, loss, metrics=metrics)

      inputs = np.zeros((10, 3))
      targets = np.zeros((10, 4))
      dataset = dataset_ops.Dataset.from_tensor_slices((inputs, targets))
      dataset = dataset.repeat(100)
      dataset = dataset.batch(10)

      model.fit(dataset, epochs=1, steps_per_epoch=2, verbose=1)

  if __name__ == "__main__":
    tf.test.main()
  ```


  Args:
    test_or_class: test method or class to be annotated. If None,
      this method returns a decorator that can be applied to a test method or
      test class. If it is not None this returns the decorator applied to the
      test or class.
    exclude_models: A collection of Keras model types to not run.
      (May also be a single model type not wrapped in a collection).
      Defaults to None.

  Returns:
    Returns a decorator that will run the decorated test method multiple times:
    once for each desired Keras model type.

  Raises:
    ImportError: If abseil parameterized is not installed or not included as
      a target dependency.
  """
  model_types = ['functional', 'subclass', 'sequential']
  params = [('_%s' % model, model) for model in model_types
            if model not in nest.flatten(exclude_models)]

  def single_method_decorator(f):
    """Decorator that constructs the test cases."""
    # Use named_parameters so it can be individually run from the command line
    @parameterized.named_parameters(*params)
    @functools.wraps(f)
    def decorated(self, model_type, *args, **kwargs):
      """A run of a single test case w/ the specified model type."""
      with testing_utils.model_type_scope(model_type):
        f(self, *args, **kwargs)

    return decorated

  return _test_or_class_decorator(test_or_class, single_method_decorator)


def run_all_keras_modes(
    test_or_class=None,
    config=None,
    always_skip_v1=False):
  """Execute the decorated test with all keras execution modes.

  This decorator is intended to be applied either to individual test methods in
  a `keras_parameterized.TestCase` class, or directly to a test class that
  extends it. Doing so will cause the contents of the individual test
  method (or all test methods in the class) to be executed multiple times -
  once executing in legacy graph mode, once running eagerly and with
  `should_run_eagerly` returning True, and once running eagerly with
  `should_run_eagerly` returning False.

  If Tensorflow v2 behavior is enabled, legacy graph mode will be skipped, and
  the test will only run twice.

  Note: if stacking this decorator with absl.testing's parameterized decorators,
  those should be at the bottom of the stack.

  For example, consider the following unittest:

  ```python
  class MyTests(testing_utils.KerasTestCase):

    @testing_utils.run_all_keras_modes
    def test_foo(self):
      model = testing_utils.get_small_functional_mlp(1, 4, input_dim=3)
      optimizer = RMSPropOptimizer(learning_rate=0.001)
      loss = 'mse'
      metrics = ['mae']
      model.compile(optimizer, loss, metrics=metrics,
                    run_eagerly=testing_utils.should_run_eagerly())

      inputs = np.zeros((10, 3))
      targets = np.zeros((10, 4))
      dataset = dataset_ops.Dataset.from_tensor_slices((inputs, targets))
      dataset = dataset.repeat(100)
      dataset = dataset.batch(10)

      model.fit(dataset, epochs=1, steps_per_epoch=2, verbose=1)

  if __name__ == "__main__":
    tf.test.main()
  ```

  This test will try compiling & fitting the small functional mlp using all
  three Keras execution modes.

  Args:
    test_or_class: test method or class to be annotated. If None,
      this method returns a decorator that can be applied to a test method or
      test class. If it is not None this returns the decorator applied to the
      test or class.
    config: An optional config_pb2.ConfigProto to use to configure the
      session when executing graphs.
    always_skip_v1: If True, does not try running the legacy graph mode even
      when Tensorflow v2 behavior is not enabled.

  Returns:
    Returns a decorator that will run the decorated test method multiple times.

  Raises:
    ImportError: If abseil parameterized is not installed or not included as
      a target dependency.
  """
  params = [('_v2_eager', 'v2_eager'),
            ('_v2_function', 'v2_function')]
  if not (always_skip_v1 or tf2.enabled()):
    params.append(('_v1_graph', 'v1_graph'))

  def single_method_decorator(f):
    """Decorator that constructs the test cases."""

    # Use named_parameters so it can be individually run from the command line
    @parameterized.named_parameters(*params)
    @functools.wraps(f)
    def decorated(self, run_mode, *args, **kwargs):
      """A run of a single test case w/ specified run mode."""
      if run_mode == 'v1_graph':
        with context.graph_mode(), testing_utils.run_eagerly_scope(False):
          with self.test_session(use_gpu=True, config=config):
            f(self, *args, **kwargs)
      elif run_mode == 'v2_function':
        with context.eager_mode():
          with testing_utils.run_eagerly_scope(False):
            f(self, *args, **kwargs)
      elif run_mode == 'v2_eager':
        with context.eager_mode():
          with testing_utils.run_eagerly_scope(True):
            f(self, *args, **kwargs)
      else:
        return ValueError('Unknown run mode %s' % run_mode)

    return decorated

  return _test_or_class_decorator(test_or_class, single_method_decorator)


def _test_or_class_decorator(test_or_class, single_method_decorator):
  """Decorate a test or class with a decorator intended for one method.

  If the test_or_class is a class:
    This will apply the decorator to all test methods in the class.

  If the test_or_class is an iterable of already-parameterized test cases:
    This will apply the decorator to all the cases, and then flatten the
    resulting cross-product of test cases. This allows stacking the Keras
    parameterized decorators w/ each other, and to apply them to test methods
    that have already been marked with an absl parameterized decorator.

  Otherwise, treat the obj as a single method and apply the decorator directly.

  Args:
    test_or_class: A test method (that may have already been decorated with a
      parameterized decorator, or a test class that extends
      keras_parameterized.TestCase
    single_method_decorator:
      A parameterized decorator intended for a single test method.
  Returns:
    The decorated result.
  """
  def _decorate_test_or_class(obj):
    if isinstance(obj, collections.Iterable):
      return itertools.chain.from_iterable(
          single_method_decorator(method) for method in obj)
    if isinstance(obj, type):
      cls = obj
      for name, value in cls.__dict__.copy().items():
        if callable(value) and name.startswith(
            unittest.TestLoader.testMethodPrefix):
          setattr(cls, name, single_method_decorator(value))

      cls = type(cls).__new__(type(cls), cls.__name__, cls.__bases__,
                              cls.__dict__.copy())
      return cls

    return single_method_decorator(obj)

  if test_or_class is not None:
    return _decorate_test_or_class(test_or_class)

  return _decorate_test_or_class
