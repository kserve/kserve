# Copyright 2025 The KServe Authors.
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

import logging

import pytest
from pathlib import Path

# Fixture factory - not called explicitly, but must be imported for pytest to discover it.
from .fixtures import test_case  # noqa: F401
from .namespace import (
    create_test_namespace,
    delete_test_namespace,
    generate_namespace_name,
    provision_namespace_secrets,
    skip_deletion,
    skip_deletion_on_failure,
)
from .fixtures import inject_k8s_proxy
from .traffic import TrafficDriver

logger = logging.getLogger(__name__)

_LLMISVC_DIR = Path(__file__).parent
_AUTOSCALING_STEM_PREFIX = "test_llm_autoscaling_"

# Files that should NOT receive ``llmisvc_core`` automatically.
# Autoscaling files (``test_llm_autoscaling_<variant>.py``) are handled
# separately below; add other special-case filenames here.
_LLMISVC_CORE_EXCLUDED = {"test_llm_tracing.py"}


@pytest.fixture
def traffic_driver():
    """Factory fixture - creates TrafficDrivers, auto-starts, auto-stops on teardown."""
    drivers: list[TrafficDriver] = []

    def factory(url: str, *, warmup: bool = False, **kwargs) -> TrafficDriver:
        driver = TrafficDriver(url, **kwargs)
        drivers.append(driver)
        driver.start(warmup=warmup)
        return driver

    yield factory

    for d in reversed(drivers):
        if d.is_running:
            d.stop()


def _auto_assign_group_markers(items):
    """Auto-assign group markers based on file naming convention.

    Every test collected from the ``llmisvc/`` directory automatically receives
    the ``llminferenceservice`` marker.  Additionally:

    * ``test_llm_autoscaling_<variant>.py`` -> ``llmisvc_autoscaling`` +
      ``autoscaling_<variant>`` (e.g. ``autoscaling_wva``, ``autoscaling_keda``).
    * ``test_llm_tracing.py`` -> skipped from ``llmisvc_core`` (has its own
      ``tracing`` marker).
    * Everything else -> ``llmisvc_core``.
    """
    for item in items:
        if not item.path.is_relative_to(_LLMISVC_DIR):
            continue

        item.add_marker(pytest.mark.llminferenceservice)

        stem = item.path.stem  # filename without .py

        if stem.startswith(_AUTOSCALING_STEM_PREFIX):
            variant = stem[len(_AUTOSCALING_STEM_PREFIX) :]
            item.add_marker(pytest.mark.llmisvc_autoscaling)
            item.add_marker(getattr(pytest.mark, f"autoscaling_{variant}"))
        elif item.path.name not in _LLMISVC_CORE_EXCLUDED:
            item.add_marker(pytest.mark.llmisvc_core)


@pytest.hookimpl(tryfirst=True, hookwrapper=True)
def pytest_runtest_makereport(item, call):
    """Stash test outcome on the node so fixtures can check it at teardown."""
    outcome = yield
    rep = outcome.get_result()
    setattr(item, f"rep_{rep.when}", rep)


def pytest_configure(config):
    """Register dynamic autoscaling variant markers derived from filenames.

    Static markers are declared in pytest.ini. This hook only handles markers
    that are generated from the filesystem (autoscaling_<variant>) so that
    --strict-markers works without manual upkeep.
    """
    for path in _LLMISVC_DIR.glob(f"{_AUTOSCALING_STEM_PREFIX}*.py"):
        variant = path.stem[len(_AUTOSCALING_STEM_PREFIX) :]
        config.addinivalue_line(
            "markers",
            f"autoscaling_{variant}: auto-discovered autoscaling variant marker",
        )
    config.addinivalue_line(
        "markers",
        "traffic: continuous traffic test (uses TrafficDriver)",
    )


def pytest_collection_modifyitems(config, items):
    _auto_assign_group_markers(items)

    for item in items:
        # only touch parameterized tests
        if not hasattr(item, "callspec"):
            continue

        # if there's no [...] suffix (i.e. not parametrized), skip
        if "[" not in item.nodeid:
            continue
        base, rest = item.nodeid.split("[", 1)
        rest = rest.rstrip("]")

        cluster_marks = [
            m.name for m in item.iter_markers() if m.name.startswith("cluster_")
        ]
        if not cluster_marks:
            continue

        new_id = "-".join(cluster_marks + [rest])
        item._nodeid = f"{base}[{new_id}]"


@pytest.fixture(scope="function")
def test_namespace(request):
    """Create a per-test namespace with secrets, clean up after the test."""
    inject_k8s_proxy()
    ns = generate_namespace_name(request.node.name)
    create_test_namespace(ns)
    provision_namespace_secrets(ns)
    yield ns
    if skip_deletion():
        return
    rep_call = getattr(request.node, "rep_call", None)
    rep_setup = getattr(request.node, "rep_setup", None)
    failed = (rep_call and rep_call.failed) or (rep_setup and rep_setup.failed)
    if failed and skip_deletion_on_failure():
        logger.info("Skipping deletion of namespace %s (SKIP_DELETION_ON_FAILURE)", ns)
        return
    delete_test_namespace(ns)


@pytest.fixture
def flow_control_auth():
    """Auth provider hook for downstream deployments.

    Override this fixture in a downstream conftest.py to enable auth pipeline
    testing. Return a dict with:

        {
            "annotations": dict,   # LLMISVC metadata annotations to enable auth
            "setup": callable,     # (kserve_client, service_name) -> token_str
            "cleanup": callable,   # (kserve_client, service_name) -> None
        }

    When this fixture returns None (upstream default), the auth verification
    section is skipped.
    """
    return None
