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

from unittest import mock

import psutil

from kserve.utils.utils import cpu_count


def test_cpu_count_handles_none_from_os(monkeypatch):
    """os.cpu_count() may return None; cpu_count() must still return a valid int.

    Regression: with os.cpu_count() == None the ``min(count, affinity_count)``
    comparison raised TypeError, which was swallowed by the surrounding
    ``except Exception``, so cpu_count() returned None instead of a usable
    worker count (and the affinity/cgroups limits were silently ignored).
    """
    monkeypatch.setattr("os.cpu_count", lambda: None)
    # cpu_affinity is not defined on every platform (e.g. macOS), so add it.
    monkeypatch.setattr(
        psutil.Process, "cpu_affinity", lambda self: [0, 1, 2, 3], raising=False
    )

    result = cpu_count()
    assert isinstance(result, int)
    assert result >= 1


def test_cpu_count_respects_affinity(monkeypatch):
    """When affinity is smaller than the host CPU count, it wins."""
    monkeypatch.setattr("os.cpu_count", lambda: 8)
    monkeypatch.setattr(
        psutil.Process, "cpu_affinity", lambda self: [0, 1], raising=False
    )
    assert cpu_count() == 2
