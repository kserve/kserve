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
"""Ops for lookup operations.

@@string_to_index
@@string_to_index_table_from_file
@@string_to_index_table_from_tensor
@@index_table_from_file
@@index_table_from_tensor
@@index_to_string
@@index_to_string_table_from_file
@@index_to_string_table_from_tensor
@@LookupInterface
@@InitializableLookupTableBase
@@IdTableWithHashBuckets
@@HashTable
@@MutableHashTable
@@MutableDenseHashTable
@@TableInitializerBase
@@KeyValueTensorInitializer
@@TextFileIndex
@@TextFileInitializer
@@TextFileIdTableInitializer
@@TextFileStringTableInitializer

@@HasherSpec
@@StrongHashSpec
@@FastHashSpec
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

# pylint: disable=unused-import,wildcard-import
from tensorflow.contrib.lookup.lookup_ops import *
# pylint: enable=unused-import,wildcard-import

from tensorflow.python.util.all_util import remove_undocumented
remove_undocumented(__name__)
