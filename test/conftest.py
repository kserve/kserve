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

"""Harness for the e2e fixture checks in test/test_e2e_*.py.

The e2e conftest is loaded as a plugin into an inner pytest session with its
cluster calls replaced by recorders, so fixture order and test outcomes are
pytest's own. e2e runs never load this file: test/e2e/pytest.ini makes test/e2e
their rootdir.

Imports use e2e.*, the names real e2e runs give these modules (pytest puts test/
on sys.path). test.e2e breaks on Pythons that ship the standard library's test
package, which wins over this directory because it has an __init__.py.
"""

import pytest

from e2e import conftest as e2e_conftest

pytest_plugins = ["pytester"]

# Shadows the kserve logging setup, which reconfigures global logging.
INNER_CONFTEST = """
import pytest


@pytest.fixture(scope="session", autouse=True)
def configure_logger():
    pass
"""


@pytest.fixture
def e2e_calls(monkeypatch):
    """Cluster calls the e2e fixtures make, in order. The worker namespace is "worker"."""
    calls = []

    def record(name):
        return lambda *args: calls.append(
            (name,) + tuple(a for a in args if isinstance(a, str))
        )

    monkeypatch.setattr(e2e_conftest, "get_core_api", lambda: None)
    monkeypatch.setattr(e2e_conftest, "skip_resource_deletion", lambda: False)
    monkeypatch.setattr(e2e_conftest, "worker_namespace_name", lambda worker: "worker")
    monkeypatch.setattr(e2e_conftest, "create_namespace", record("create_namespace"))
    monkeypatch.setattr(e2e_conftest, "provision_secrets", lambda api, ns: None)
    monkeypatch.setattr(
        e2e_conftest, "provision_serving_runtimes", lambda api, ns: None
    )
    monkeypatch.setattr(e2e_conftest, "delete_namespace", record("delete_namespace"))
    monkeypatch.setattr(e2e_conftest, "cleanup_isvcs", record("delete"))
    monkeypatch.setattr(e2e_conftest, "wait_pods_terminated", record("wait"))
    monkeypatch.setattr(
        e2e_conftest,
        "collect_failure_diagnostics",
        lambda ns, nodeid, worker, started: calls.append(
            ("collect", ns, nodeid.split("::")[-1], worker)
        ),
    )
    return calls


@pytest.fixture
def run_e2e(pytester):
    """Run a test module against the real e2e conftest and return the result."""

    def run(source):
        pytester.makeconftest(INNER_CONFTEST)
        pytester.makepyfile(test_inner=source)
        return pytester.runpytest(
            "-p",
            "e2e.conftest",
            "-p",
            "no:cacheprovider",
            # Silences pytest-asyncio; no inner test uses an async fixture.
            "-o",
            "asyncio_default_fixture_loop_scope=function",
        )

    return run
