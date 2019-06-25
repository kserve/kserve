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
"""Utils for Estimator (deprecated).

This module and all its submodules are deprecated. See
[contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
for migration instructions.
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.python.util import tf_inspect


def assert_estimator_contract(tester, estimator_class):
  """Asserts whether given estimator satisfies the expected contract.

  This doesn't check every details of contract. This test is used for that a
  function is not forgotten to implement in a precanned Estimator.

  Args:
    tester: A tf.test.TestCase.
    estimator_class: 'type' object of pre-canned estimator.
  """
  attributes = tf_inspect.getmembers(estimator_class)
  attribute_names = [a[0] for a in attributes]

  tester.assertTrue('config' in attribute_names)
  tester.assertTrue('evaluate' in attribute_names)
  tester.assertTrue('export' in attribute_names)
  tester.assertTrue('fit' in attribute_names)
  tester.assertTrue('get_variable_names' in attribute_names)
  tester.assertTrue('get_variable_value' in attribute_names)
  tester.assertTrue('model_dir' in attribute_names)
  tester.assertTrue('predict' in attribute_names)


def assert_in_range(min_value, max_value, key, metrics):
  actual_value = metrics[key]
  if actual_value < min_value:
    raise ValueError('%s: %s < %s.' % (key, actual_value, min_value))
  if actual_value > max_value:
    raise ValueError('%s: %s > %s.' % (key, actual_value, max_value))
