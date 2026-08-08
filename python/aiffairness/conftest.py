# Copyright 2026 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Test setup for the aifserver package.

model.py imports aif360, kserve and nest_asyncio at module load. Those are heavy
runtime dependencies; the unit tests only exercise AIFModel's own request
handling, so we install lightweight stand-ins here (before the package is
imported during collection) to keep the tests hermetic and fast.
"""

import sys
import types
from unittest import mock

# kserve.Model must be a real class because AIFModel subclasses it.
_kserve = types.ModuleType("kserve")


class _Model:
    def __init__(self, name):
        self.name = name


_kserve.Model = _Model
sys.modules.setdefault("kserve", _kserve)

_nest_asyncio = types.ModuleType("nest_asyncio")
_nest_asyncio.apply = lambda: None
sys.modules.setdefault("nest_asyncio", _nest_asyncio)

# aif360 pieces are constructed but never asserted on in these tests.
_metrics = types.ModuleType("aif360.metrics")
_metrics.BinaryLabelDatasetMetric = mock.MagicMock(name="BinaryLabelDatasetMetric")
_datasets = types.ModuleType("aif360.datasets")
_datasets.BinaryLabelDataset = mock.MagicMock(name="BinaryLabelDataset")
sys.modules.setdefault("aif360", types.ModuleType("aif360"))
sys.modules.setdefault("aif360.metrics", _metrics)
sys.modules.setdefault("aif360.datasets", _datasets)
