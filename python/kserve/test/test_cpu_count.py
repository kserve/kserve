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

import builtins
import io
from unittest import mock

import psutil
import pytest

from kserve.utils import utils

V1_QUOTA = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us"
V1_PERIOD = "/sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us"
PROC_CGROUP = "/proc/self/cgroup"
ROOT_CPU_MAX = "/sys/fs/cgroup/cpu.max"


@pytest.fixture
def fake_files(monkeypatch):
    """Serve the given path -> content map for open(); any other path is absent."""
    real_open = builtins.open
    files = {}

    def fake_open(path, *args, **kwargs):
        if path in files:
            return io.StringIO(files[path])
        if isinstance(path, str) and path.startswith(
            ("/sys/fs/cgroup", "/proc/self/cgroup")
        ):
            raise FileNotFoundError(path)
        return real_open(path, *args, **kwargs)

    monkeypatch.setattr(builtins, "open", fake_open)
    monkeypatch.setattr(utils.sys, "platform", "linux")
    monkeypatch.setattr(utils.os, "cpu_count", lambda: 32)
    with mock.patch.object(utils.psutil, "Process") as process:
        process.return_value.cpu_affinity.return_value = list(range(32))
        yield files


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

    result = utils.cpu_count()
    assert isinstance(result, int)
    assert result >= 1


def test_cpu_count_respects_affinity(fake_files):
    """When affinity is smaller than the host CPU count, it wins."""
    utils.psutil.Process.return_value.cpu_affinity.return_value = [0, 1]
    assert utils.cpu_count() == 2


def test_cgroup_v2_quota_limits_cpu_count(fake_files):
    fake_files[ROOT_CPU_MAX] = "200000 100000\n"
    assert utils.cpu_count() == 2


def test_cgroup_v2_uses_the_process_cgroup_path(fake_files):
    fake_files[PROC_CGROUP] = "0::/kubepods/pod-1/container-1\n"
    fake_files["/sys/fs/cgroup/kubepods/pod-1/container-1/cpu.max"] = "400000 100000\n"
    assert utils.cpu_count() == 4


def test_cgroup_v2_max_means_no_limit(fake_files):
    fake_files[ROOT_CPU_MAX] = "max 100000\n"
    assert utils.cpu_count() == 32


def test_cgroup_v2_limit_above_host_count_is_ignored(fake_files):
    fake_files[ROOT_CPU_MAX] = "6400000 100000\n"
    assert utils.cpu_count() == 32


def test_cgroup_v1_quota_still_applies(fake_files):
    fake_files[V1_QUOTA] = "300000\n"
    fake_files[V1_PERIOD] = "100000\n"
    assert utils.cpu_count() == 3


def test_cgroup_v1_unlimited_quota_is_ignored(fake_files):
    fake_files[V1_QUOTA] = "-1\n"
    fake_files[V1_PERIOD] = "100000\n"
    assert utils.cpu_count() == 32


def test_no_cgroup_files_returns_host_count(fake_files):
    assert utils.cpu_count() == 32


def test_cpu_affinity_is_still_respected(fake_files):
    fake_files[ROOT_CPU_MAX] = "800000 100000\n"
    with mock.patch.object(utils.psutil, "Process") as process:
        process.return_value.cpu_affinity.return_value = [0, 1, 2]
        assert utils.cpu_count() == 3
