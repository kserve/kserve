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
"""Ops and estimators that enable explicit kernel methods in TensorFlow.

@@KernelLinearClassifier
@@RandomFourierFeatureMapper
@@sparse_multiclass_hinge_loss
"""
from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.kernel_methods.python.kernel_estimators import KernelLinearClassifier
from tensorflow.contrib.kernel_methods.python.losses import sparse_multiclass_hinge_loss
from tensorflow.contrib.kernel_methods.python.mappers.random_fourier_features import RandomFourierFeatureMapper

from tensorflow.python.util.all_util import remove_undocumented
remove_undocumented(__name__)
