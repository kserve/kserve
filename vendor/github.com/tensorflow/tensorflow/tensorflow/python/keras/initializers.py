# Copyright 2015 The TensorFlow Authors. All Rights Reserved.
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
"""Keras initializer serialization / deserialization.
"""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import six

from tensorflow.python.framework import dtypes
from tensorflow.python.keras.utils.generic_utils import deserialize_keras_object
from tensorflow.python.keras.utils.generic_utils import serialize_keras_object

# These imports are brought in so that keras.initializers.deserialize
# has them available in module_objects.
from tensorflow.python.ops.init_ops import Constant
from tensorflow.python.ops.init_ops import GlorotNormal
from tensorflow.python.ops.init_ops import GlorotUniform
from tensorflow.python.ops.init_ops import he_normal  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import he_uniform  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import Identity
from tensorflow.python.ops.init_ops import Initializer  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import lecun_normal  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import lecun_uniform  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import Ones
from tensorflow.python.ops.init_ops import Orthogonal
from tensorflow.python.ops.init_ops import RandomNormal as TFRandomNormal
from tensorflow.python.ops.init_ops import RandomUniform as TFRandomUniform
from tensorflow.python.ops.init_ops import TruncatedNormal as TFTruncatedNormal
from tensorflow.python.ops.init_ops import VarianceScaling  # pylint: disable=unused-import
from tensorflow.python.ops.init_ops import Zeros

from tensorflow.python.util.tf_export import tf_export


@tf_export('keras.initializers.TruncatedNormal',
           'keras.initializers.truncated_normal')
class TruncatedNormal(TFTruncatedNormal):
  """Initializer that generates a truncated normal distribution.

  These values are similar to values from a `random_normal_initializer`
  except that values more than two standard deviations from the mean
  are discarded and re-drawn. This is the recommended initializer for
  neural network weights and filters.

  Args:
    mean: a python scalar or a scalar tensor. Mean of the random values to
      generate. Defaults to 0.
    stddev: a python scalar or a scalar tensor. Standard deviation of the random
      values to generate. Defaults to 0.05.
    seed: A Python integer. Used to create random seeds. See
      `tf.set_random_seed` for behavior.
    dtype: The data type. Only floating point types are supported.
  """

  def __init__(self, mean=0.0, stddev=0.05, seed=None, dtype=dtypes.float32):
    super(TruncatedNormal, self).__init__(
        mean=mean, stddev=stddev, seed=seed, dtype=dtype)


@tf_export('keras.initializers.RandomUniform', 'keras.initializers.uniform',
           'keras.initializers.random_uniform')
class RandomUniform(TFRandomUniform):
  """Initializer that generates tensors with a uniform distribution.

  Args:
    minval: A python scalar or a scalar tensor. Lower bound of the range of
      random values to generate. Defaults to -0.05.
    maxval: A python scalar or a scalar tensor. Upper bound of the range of
      random values to generate. Defaults to 0.05.
    seed: A Python integer. Used to create random seeds. See
      `tf.set_random_seed` for behavior.
    dtype: The data type.
  """

  def __init__(self, minval=-0.05, maxval=0.05, seed=None,
               dtype=dtypes.float32):
    super(RandomUniform, self).__init__(
        minval=minval, maxval=maxval, seed=seed, dtype=dtype)


@tf_export('keras.initializers.RandomNormal', 'keras.initializers.normal',
           'keras.initializers.random_normal')
class RandomNormal(TFRandomNormal):
  """Initializer that generates tensors with a normal distribution.

  Args:
    mean: a python scalar or a scalar tensor. Mean of the random values to
      generate. Defaults to 0.
    stddev: a python scalar or a scalar tensor. Standard deviation of the random
      values to generate. Defaults to 0.05.
    seed: A Python integer. Used to create random seeds. See
      `tf.set_random_seed` for behavior.
    dtype: The data type. Only floating point types are supported.

  Returns:
      RandomNormal instance.
  """

  def __init__(self, mean=0.0, stddev=0.05, seed=None, dtype=dtypes.float32):
    super(RandomNormal, self).__init__(
        mean=mean, stddev=stddev, seed=seed, dtype=dtype)


# Compatibility aliases

# pylint: disable=invalid-name
zero = zeros = Zeros
one = ones = Ones
constant = Constant
uniform = random_uniform = RandomUniform
normal = random_normal = RandomNormal
truncated_normal = TruncatedNormal
identity = Identity
orthogonal = Orthogonal
glorot_normal = GlorotNormal
glorot_uniform = GlorotUniform


# Utility functions


@tf_export('keras.initializers.serialize')
def serialize(initializer):
  return serialize_keras_object(initializer)


@tf_export('keras.initializers.deserialize')
def deserialize(config, custom_objects=None):
  return deserialize_keras_object(
      config,
      module_objects=globals(),
      custom_objects=custom_objects,
      printable_module_name='initializer')


@tf_export('keras.initializers.get')
def get(identifier):
  if identifier is None:
    return None
  if isinstance(identifier, dict):
    return deserialize(identifier)
  elif isinstance(identifier, six.string_types):
    config = {'class_name': str(identifier), 'config': {}}
    return deserialize(config)
  elif callable(identifier):
    return identifier
  else:
    raise ValueError('Could not interpret initializer identifier: ' +
                     str(identifier))


# pylint: enable=invalid-name
