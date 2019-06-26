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
# ===================================================================

"""Utilities for the functionalities."""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import time
import six

from tensorflow.python.platform import tf_logging as logging
from tensorflow.python.training import training

def check_positive_integer(value, name):
  """Checks whether `value` is a positive integer."""
  if not isinstance(value, six.integer_types):
    raise TypeError('{} must be int, got {}'.format(name, type(value)))

  if value <= 0:
    raise ValueError('{} must be positive, got {}'.format(name, value))


# TODO(b/118302029) Remove this copy of MultiHostDatasetInitializerHook after we
# release a tensorflow_estimator with MultiHostDatasetInitializerHook in
# python/estimator/util.py.
class MultiHostDatasetInitializerHook(training.SessionRunHook):
  """Creates a SessionRunHook that initializes all passed iterators."""

  def __init__(self, dataset_initializers):
    self._initializers = dataset_initializers

  def after_create_session(self, session, coord):
    del coord
    start = time.time()
    session.run(self._initializers)
    logging.info('Initialized dataset iterators in %d seconds',
                 time.time() - start)
