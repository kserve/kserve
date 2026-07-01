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

import pytest

# Fixture factory - not called explicitly, but must be imported for pytest to discover it.
from .fixtures import test_case  # noqa: F401


# This hook is used to ensure that the test names are unique and to ensure that
# the test names are consistent with the cluster marks.
def pytest_collection_modifyitems(config, items):
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


def pytest_configure(config):
    config.addinivalue_line(
        "markers", "llminferenceservice: mark test as an LLM inference service test"
    )
    config.addinivalue_line("markers", "autoscaling: mark test as an autoscaling test")
    config.addinivalue_line(
        "markers", "autoscaling_hpa: mark test as an HPA autoscaling test"
    )
    config.addinivalue_line(
        "markers", "autoscaling_keda: mark test as a KEDA autoscaling test"
    )
    config.addinivalue_line(
        "markers", "model_routing: mark test as a model-based routing test"
    )
    config.addinivalue_line("markers", "lora: mark test as a LoRA adapter test")
    config.addinivalue_line("markers", "pvc_storage: mark test as a PVC storage test")
    config.addinivalue_line(
        "markers", "tracing: mark test as a distributed tracing test"
    )
    config.addinivalue_line("markers", "flow_control: mark test as a flow control test")


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
