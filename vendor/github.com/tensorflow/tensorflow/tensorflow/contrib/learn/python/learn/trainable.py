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
"""`Trainable` interface (deprecated).

This module and all its submodules are deprecated. See
[contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
for migration instructions.
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import abc

import six


@six.add_metaclass(abc.ABCMeta)
class Trainable(object):
  """Interface for objects that are trainable by, e.g., `Experiment`.

  THIS CLASS IS DEPRECATED.
  """

  @abc.abstractmethod
  def fit(self,
          x=None,
          y=None,
          input_fn=None,
          steps=None,
          batch_size=None,
          monitors=None,
          max_steps=None):
    """Trains a model given training data `x` predictions and `y` labels.

    Args:
      x: Matrix of shape [n_samples, n_features...] or the dictionary of
        Matrices.
         Can be iterator that returns arrays of features or dictionary of arrays
           of features.
         The training input samples for fitting the model. If set, `input_fn`
           must be `None`.
      y: Vector or matrix [n_samples] or [n_samples, n_outputs] or the
        dictionary of same.
         Can be iterator that returns array of labels or dictionary of array of
           labels.
         The training label values (class labels in classification, real numbers
           in regression).
         If set, `input_fn` must be `None`. Note: For classification, label
           values must
         be integers representing the class index (i.e. values from 0 to
         n_classes-1).
      input_fn: Input function returning a tuple of:
          features - `Tensor` or dictionary of string feature name to `Tensor`.
          labels - `Tensor` or dictionary of `Tensor` with labels.
        If input_fn is set, `x`, `y`, and `batch_size` must be `None`.
      steps: Number of steps for which to train model. If `None`, train forever.
        'steps' works incrementally. If you call two times fit(steps=10) then
        training occurs in total 20 steps. If you don't want to have incremental
        behavior please set `max_steps` instead. If set, `max_steps` must be
        `None`.
      batch_size: minibatch size to use on the input, defaults to first
        dimension of `x`. Must be `None` if `input_fn` is provided.
      monitors: List of `BaseMonitor` subclass instances. Used for callbacks
        inside the training loop.
      max_steps: Number of total steps for which to train model. If `None`,
        train forever. If set, `steps` must be `None`.

        Two calls to `fit(steps=100)` means 200 training
        iterations. On the other hand, two calls to `fit(max_steps=100)` means
        that the second call will not do any iteration since first call did
        all 100 steps.

    Returns:
      `self`, for chaining.
    """
    raise NotImplementedError
