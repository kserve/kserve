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

"""Failure diagnostics checks without a cluster: python -m pytest test/test_e2e_diagnostics.py.

Fixture and hook checks use the run_e2e harness from test/conftest.py.
"""

import json
import subprocess
from datetime import datetime, timezone

import pytest

from test.e2e import conftest
from test.e2e.common import diagnostics

STARTED_AT = datetime(2026, 10, 26, 9, 7, 26, tzinfo=timezone.utc)
NODEID = (
    "predictor/test_multi_model_serving.py::test_mms_sklearn_kserve[v2-gs://bucket]"
)

POD = {
    "metadata": {"name": "isvc-predictor-00001-abc"},
    "spec": {
        "initContainers": [{"name": "storage-initializer"}],
        "containers": [{"name": "kserve-container"}, {"name": "queue-proxy"}],
    },
    "status": {
        "initContainerStatuses": [{"name": "storage-initializer", "restartCount": 0}],
        "containerStatuses": [
            {"name": "kserve-container", "restartCount": 3},
            {"name": "queue-proxy", "restartCount": 0},
        ],
    },
}

CREATED = ("create_namespace", "worker")
DELETED = ("delete_namespace", "worker")
CLEANUP = [("delete", "worker"), ("wait", "worker")]

FAILING_TEST = """
def test_fails(test_namespace):
    assert False
"""


def captured(test_name):
    return ("collect", "worker", test_name, "master")


def test_failed_test_is_captured_before_cleanup(run_e2e, e2e_calls):
    run_e2e(FAILING_TEST).assert_outcomes(failed=1)
    assert e2e_calls == [CREATED, captured("test_fails"), *CLEANUP, DELETED]


def test_setup_failure_after_namespace_is_captured(run_e2e, e2e_calls):
    result = run_e2e(
        """
        import pytest

        @pytest.fixture
        def broken(test_namespace):
            raise RuntimeError("setup broke")

        def test_setup_fails(broken):
            pass
        """
    )
    result.assert_outcomes(errors=1)
    assert e2e_calls == [CREATED, captured("test_setup_fails"), *CLEANUP, DELETED]


def test_passed_and_skipped_tests_only_clean_up(run_e2e, e2e_calls):
    result = run_e2e(
        """
        import pytest

        @pytest.fixture
        def skips_in_setup(test_namespace):
            pytest.skip("setup")

        def test_passes(test_namespace):
            pass

        def test_skips_in_call(test_namespace):
            pytest.skip("call")

        def test_skips_in_setup(skips_in_setup):
            pass
        """
    )
    result.assert_outcomes(passed=1, skipped=2)
    assert e2e_calls == [CREATED, *CLEANUP * 3, DELETED]


def test_failure_is_captured_when_resources_are_preserved(
    run_e2e, e2e_calls, monkeypatch
):
    monkeypatch.setattr(conftest, "skip_resource_deletion", lambda: True)
    run_e2e(FAILING_TEST).assert_outcomes(failed=1)
    assert e2e_calls == [CREATED, captured("test_fails")]


def test_collector_error_keeps_the_test_outcome(
    run_e2e, e2e_calls, monkeypatch, tmp_path
):
    monkeypatch.setattr(
        conftest, "collect_failure_diagnostics", diagnostics.collect_failure_diagnostics
    )
    monkeypatch.setenv(diagnostics.DIAGNOSTICS_DIR_ENV, str(tmp_path))

    def broken_collector(out):
        raise RuntimeError("collector broke")

    monkeypatch.setattr(diagnostics, "_Collector", broken_collector)
    run_e2e(FAILING_TEST).assert_outcomes(failed=1, errors=0)
    assert e2e_calls == [CREATED, *CLEANUP, DELETED]


def test_report_hook_serves_other_fixtures(run_e2e):
    # llmisvc's per-test namespace fixture reads rep_call from the root hook.
    result = run_e2e(
        """
        import pytest

        @pytest.fixture
        def per_test_namespace(request):
            yield
            assert request.node.rep_call.failed

        def test_fails(per_test_namespace):
            assert False
        """
    )
    result.assert_outcomes(failed=1, errors=0)


class FakeKubectl:
    def __init__(self):
        self.commands = []

    def __call__(self, cmd, **kwargs):
        self.commands.append(cmd)
        if cmd[0] == "git":
            return subprocess.CompletedProcess(cmd, 0, "merge base head\n", "")
        if cmd[2:] == ["get", "pods", "-n", "worker", "-o", "json"]:
            return subprocess.CompletedProcess(cmd, 0, json.dumps({"items": [POD]}), "")
        return subprocess.CompletedProcess(cmd, 0, " ".join(cmd[2:]), "")


@pytest.fixture
def kubectl(monkeypatch, tmp_path):
    monkeypatch.setenv(diagnostics.DIAGNOSTICS_DIR_ENV, str(tmp_path))
    fake = FakeKubectl()
    monkeypatch.setattr(diagnostics.subprocess, "run", fake)
    return fake


def collect(worker="gw0"):
    return diagnostics.collect_failure_diagnostics("worker", NODEID, worker, STARTED_AT)


def test_disabled_without_output_dir(kubectl, monkeypatch):
    monkeypatch.setenv(diagnostics.DIAGNOSTICS_DIR_ENV, "")
    assert collect() is None
    assert kubectl.commands == []


def test_captures_namespace_and_control_plane_state(kubectl):
    out = collect()

    meta = json.loads((out / "meta.json").read_text())
    assert meta["nodeid"] == NODEID
    assert meta["worker"] == "gw0"
    assert meta["namespace"] == "worker"
    assert meta["tested_commit_and_parents"] == "merge base head"
    assert "GITHUB_RUN_ATTEMPT" in meta

    for name in (
        "revisions.serving.knative.dev.yaml",
        "podautoscalers.autoscaling.internal.knative.dev.yaml",
        "events.txt",
        "pods-describe.txt",
        "pods.yaml",
        "nodes-describe.txt",
        "node-images.txt",
        "logs/isvc-predictor-00001-abc.storage-initializer.log",
        "logs/isvc-predictor-00001-abc.kserve-container.log",
        "logs/isvc-predictor-00001-abc.kserve-container.previous.log",
        "controllers/knative-serving.autoscaler.log",
    ):
        assert (out / name).is_file(), name
    assert not (out / "logs/isvc-predictor-00001-abc.queue-proxy.previous.log").exists()
    assert not (out / "errors.txt").exists()

    controller_logs = [c for c in kubectl.commands if "-l" in c]
    assert controller_logs
    for cmd in controller_logs:
        assert "--since-time=2026-10-26T09:07:26Z" in cmd
        assert "--tail=-1" in cmd
    for cmd in kubectl.commands:
        assert all("secret" not in arg for arg in cmd), cmd
        if "logs" in cmd:
            assert any(a.startswith("--limit-bytes=") for a in cmd), cmd


def test_command_errors_are_recorded_not_raised(kubectl, monkeypatch):
    def run(cmd, **kwargs):
        if "revisions.serving.knative.dev" in cmd:
            raise subprocess.TimeoutExpired(cmd, 30)
        if "describe" in cmd:
            return subprocess.CompletedProcess(cmd, 1, "", "connection refused")
        if cmd[0] == "git":
            raise FileNotFoundError("git")
        return kubectl(cmd)

    monkeypatch.setattr(diagnostics.subprocess, "run", run)
    out = collect()

    errors = (out / "errors.txt").read_text()
    assert "timed out" in errors
    assert "connection refused" in errors
    assert "git" in errors
    assert (out / "logs/isvc-predictor-00001-abc.kserve-container.log").is_file()


def test_spent_budget_skips_remaining_commands(kubectl, monkeypatch):
    clock = iter([0] + [diagnostics.BUDGET_SECONDS + 1] * 100)
    monkeypatch.setattr(diagnostics.time, "monotonic", lambda: next(clock))
    out = collect()
    assert kubectl.commands == []
    assert "budget spent" in (out / "errors.txt").read_text()


def test_unexpected_error_does_not_raise(kubectl, monkeypatch, caplog):
    def broken_collector(out):
        raise RuntimeError("boom")

    monkeypatch.setattr(diagnostics, "_Collector", broken_collector)
    assert collect() is None
    assert "boom" in caplog.text


def test_workers_get_separate_upload_safe_directories(kubectl, tmp_path):
    first = collect(worker="gw0")
    second = collect(worker="gw1")
    assert first != second
    for out in (first, second):
        relative = str(out.relative_to(tmp_path))
        assert not set(relative) & set('":<>|*?[]'), relative
