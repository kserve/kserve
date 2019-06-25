# -*- coding: utf-8 -*-
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
"""Implementation of the Keras API meant to be a high-level API for TensorFlow.

This module an alias for `tf.keras`, for backwards compatibility.

Detailed documentation and user guides are also available at
[keras.io](https://keras.io).
"""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# pylint: disable=wildcard-import
from tensorflow.contrib.keras.api.keras import *

try:
  from tensorflow.contrib.keras import python  # pylint: disable=g-import-not-at-top
  del python
except ImportError:
  pass

del absolute_import
del division
del print_function
