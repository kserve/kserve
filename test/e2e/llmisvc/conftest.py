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
from pathlib import Path

# Fixture factory - not called explicitly, but must be imported for pytest to discover it.
from .fixtures import test_case  # noqa: F401

_LLMISVC_DIR = Path(__file__).parent
_AUTOSCALING_STEM_PREFIX = "test_llm_autoscaling_"

# Files that should NOT receive ``llmisvc_core`` automatically.
# Autoscaling files (``test_llm_autoscaling_<variant>.py``) are handled
# separately below; add other special-case filenames here.
_LLMISVC_CORE_EXCLUDED = {"test_llm_tracing.py"}


def _auto_assign_group_markers(items):
    """Auto-assign group markers based on file naming convention.

    * ``test_llm_autoscaling_<variant>.py`` -> ``llmisvc_autoscaling`` +
      ``autoscaling_<variant>`` (e.g. ``autoscaling_wva``, ``autoscaling_keda``).
    * ``test_llm_tracing.py`` -> skipped (has its own ``tracing`` marker).
    * Everything else -> ``llmisvc_core``.
    """
    for item in items:
        if not item.path.is_relative_to(_LLMISVC_DIR):
            continue
        stem = item.path.stem  # filename without .py

        if stem.startswith(_AUTOSCALING_STEM_PREFIX):
            variant = stem[len(_AUTOSCALING_STEM_PREFIX) :]
            item.add_marker(pytest.mark.llmisvc_autoscaling)
            item.add_marker(getattr(pytest.mark, f"autoscaling_{variant}"))
        elif item.path.name not in _LLMISVC_CORE_EXCLUDED:
            item.add_marker(pytest.mark.llmisvc_core)


# This hook is used to ensure that the test names are unique and to ensure that
# the test names are consistent with the cluster marks.
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


def pytest_configure(config):
    config.addinivalue_line(
        "markers", "llminferenceservice: mark test as an LLM inference service test"
    )
    config.addinivalue_line("markers", "llmisvc_core: mark test as a core LLMISVC test")
    config.addinivalue_line("markers", "autoscaling: mark test as an autoscaling test")
    config.addinivalue_line(
        "markers", "autoscaling_wva: mark test as a WVA autoscaling test"
    )
    config.addinivalue_line(
        "markers",
        "llmisvc_autoscaling: mark test as an LLMISVC autoscaling test",
    )
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
