# Copyright 2023 The KServe Authors.
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

import sys
import types
import pytest
from huggingfaceserver import utils

torch = pytest.importorskip("torch", reason="torch not installed")

# stub kserve.logging
if "kserve.logging" not in sys.modules:
    kserve_pkg = types.ModuleType("kserve")
    kserve_logging = types.ModuleType("kserve.logging")

    class _DummyLogger:
        def info(self, *a, **k):
            pass

        def warning(self, *a, **k):
            pass

        def error(self, *a, **k):
            pass

    kserve_logging.logger = _DummyLogger()
    sys.modules["kserve"] = kserve_pkg
    sys.modules["kserve.logging"] = kserve_logging


def test_mean_pooling_none_mask_cpu():
    x = torch.ones(1, 3, 4)  # [B,S,H]
    out = utils._mean_pooling((x,), None)
    assert torch.allclose(out, torch.ones(1, 4))
