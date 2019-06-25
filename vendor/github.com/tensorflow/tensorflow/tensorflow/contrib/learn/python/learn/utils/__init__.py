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

"""TensorFlow Learn Utils (deprecated).

This module and all its submodules are deprecated. See
[contrib/learn/README.md](https://www.tensorflow.org/code/tensorflow/contrib/learn/README.md)
for migration instructions.
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

from tensorflow.contrib.learn.python.learn.utils.export import export_estimator
from tensorflow.contrib.learn.python.learn.utils.input_fn_utils import build_default_serving_input_fn
from tensorflow.contrib.learn.python.learn.utils.input_fn_utils import build_parsing_serving_input_fn
from tensorflow.contrib.learn.python.learn.utils.input_fn_utils import InputFnOps
from tensorflow.contrib.learn.python.learn.utils.saved_model_export_utils import make_export_strategy

