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

"""Failure diagnostics for tests in a reused worker namespace.

Per-test cleanup deletes a failed test's InferenceServices before the job-level
status check runs, so the revision, pod and event state that explains the failure
is captured here first. Collection is best effort: every command is bounded, and
errors are written next to the output instead of raised, so they never replace
the test failure.
"""

import json
import logging
import os
import re
import subprocess
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Optional

import yaml

logger = logging.getLogger("e2e.diagnostics")

DIAGNOSTICS_DIR_ENV = "E2E_DIAGNOSTICS_DIR"

COMMAND_TIMEOUT_SECONDS = 30
BUDGET_SECONDS = 180
CONTAINER_LOG_LIMIT_BYTES = 1 << 20
CONTROLLER_LOG_LIMIT_BYTES = 4 << 20

# Fully qualified, so `services` cannot resolve to core Services. One command per
# kind: a single `kubectl get a,b` fails as a whole when a CRD is not installed
# (no Knative in raw deployment mode).
NAMESPACED_KINDS = (
    "inferenceservices.serving.kserve.io",
    "trainedmodels.serving.kserve.io",
    "services.serving.knative.dev",
    "configurations.serving.knative.dev",
    "revisions.serving.knative.dev",
    "podautoscalers.autoscaling.internal.knative.dev",
    "deployments.apps",
    "replicasets.apps",
)

CONTROLLERS = (
    ("kserve", "control-plane=kserve-controller-manager"),
    ("knative-serving", "app=controller"),
    ("knative-serving", "app=autoscaler"),
    ("knative-serving", "app=activator"),
    ("knative-serving", "app=webhook"),
)

# Kubelet reports at most 50 images per node (nodeStatusMaxImages), largest first.
NODE_IMAGES_JSONPATH = (
    '{range .items[*]}{.metadata.name}{"\\n"}'
    '{range .status.images[*]}{"  "}{.sizeBytes}{"\\t"}{.names}{"\\n"}{end}{end}'
)


def setup_or_call_failed(node) -> bool:
    """Whether setup or call failed, from reports stashed by pytest_runtest_makereport."""
    return any(
        getattr(getattr(node, f"rep_{when}", None), "failed", False)
        for when in ("setup", "call")
    )


def collect_failure_diagnostics(
    namespace: str, nodeid: str, worker_id: str, started_at: datetime
) -> Optional[Path]:
    """Capture a failed test's namespace and control-plane state.

    Disabled unless E2E_DIAGNOSTICS_DIR is set. Returns the output directory.
    """
    base = os.environ.get(DIAGNOSTICS_DIR_ENV)
    if not base:
        return None
    try:
        out = Path(base) / _safe_name(worker_id) / _test_dir_name(nodeid, started_at)
        out.mkdir(parents=True, exist_ok=True)
        _Collector(out).collect(namespace, nodeid, worker_id, started_at)
        logger.info("Captured failure diagnostics for %s in %s", nodeid, out)
        return out
    except Exception as e:  # noqa: BLE001 - must not replace the test failure
        logger.warning("Failed to capture diagnostics for %s: %s", nodeid, e)
        return None


def _safe_name(value: str) -> str:
    # upload-artifact rejects " : < > | * ? in paths.
    return re.sub(r"[^A-Za-z0-9._-]+", "_", value).strip("_")[:150] or "unnamed"


def _test_dir_name(nodeid: str, started_at: datetime) -> str:
    return f"{_safe_name(nodeid)}-{started_at.strftime('%Y%m%dT%H%M%SZ')}"


def _rfc3339(ts: datetime) -> str:
    return ts.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


class _Collector:
    def __init__(self, out: Path):
        self.out = out
        self.deadline = time.monotonic() + BUDGET_SECONDS
        self.errors: List[str] = []

    def collect(
        self, namespace: str, nodeid: str, worker_id: str, started_at: datetime
    ) -> None:
        sections = (
            lambda: self._write_meta(namespace, nodeid, worker_id, started_at),
            lambda: self._collect_namespace(namespace),
            lambda: self._collect_cluster(started_at),
        )
        for section in sections:
            try:
                section()
            except Exception as e:  # noqa: BLE001 - keep the remaining sections
                self.errors.append(f"section failed: {e!r}")
        if self.errors:
            (self.out / "errors.txt").write_text("\n".join(self.errors) + "\n")

    def _write_meta(
        self, namespace: str, nodeid: str, worker_id: str, started_at: datetime
    ) -> None:
        # The workflow merges the base branch after checkout, so the tested tree
        # is HEAD and its parents, not the PR head SHA.
        tested_commit = self._run(["git", "log", "-1", "--format=%H %P"])
        meta = {
            "nodeid": nodeid,
            "worker": worker_id,
            "namespace": namespace,
            "test_started_at": _rfc3339(started_at),
            "collected_at": _rfc3339(datetime.now(timezone.utc)),
            "tested_commit_and_parents": (tested_commit or "").strip(),
        }
        for key in (
            "GITHUB_RUN_ID",
            "GITHUB_RUN_ATTEMPT",
            "GITHUB_JOB",
            "GITHUB_REF",
            "GITHUB_HEAD_REF",
            "TAG",
        ):
            meta[key] = os.environ.get(key, "")
        (self.out / "meta.json").write_text(json.dumps(meta, indent=2) + "\n")

    def _collect_namespace(self, namespace: str) -> None:
        for kind in NAMESPACED_KINDS:
            self._kubectl(f"{kind}.yaml", ["get", kind, "-n", namespace, "-o", "yaml"])
        self._kubectl(
            "events.txt",
            [
                "get",
                "events",
                "-n",
                namespace,
                "-o",
                "wide",
                "--sort-by=.metadata.creationTimestamp",
            ],
        )
        self._kubectl("pods-describe.txt", ["describe", "pods", "-n", namespace])

        pods_json = self._kubectl(None, ["get", "pods", "-n", namespace, "-o", "json"])
        if pods_json is None:
            return
        pods = json.loads(pods_json).get("items", [])
        (self.out / "pods.yaml").write_text(yaml.safe_dump({"items": pods}))
        for pod in pods:
            self._collect_pod_logs(namespace, pod)

    def _collect_pod_logs(self, namespace: str, pod: dict) -> None:
        name = pod["metadata"]["name"]
        status = pod.get("status", {})
        restarts = {
            s["name"]: s.get("restartCount", 0)
            for s in status.get("initContainerStatuses", [])
            + status.get("containerStatuses", [])
        }
        spec = pod.get("spec", {})
        for container in spec.get("initContainers", []) + spec.get("containers", []):
            cname = container["name"]
            args = [
                "logs",
                "-n",
                namespace,
                name,
                "-c",
                cname,
                f"--limit-bytes={CONTAINER_LOG_LIMIT_BYTES}",
            ]
            self._kubectl(f"logs/{name}.{cname}.log", args)
            # --previous fails when the container never restarted.
            if restarts.get(cname, 0) > 0:
                self._kubectl(
                    f"logs/{name}.{cname}.previous.log", args + ["--previous"]
                )

    def _collect_cluster(self, started_at: datetime) -> None:
        self._kubectl("nodes-describe.txt", ["describe", "nodes"])
        self._kubectl(
            "node-images.txt",
            ["get", "nodes", "-o", f"jsonpath={NODE_IMAGES_JSONPATH}"],
        )
        for namespace, selector in CONTROLLERS:
            self._kubectl(
                f"controllers/{namespace}.{selector.split('=')[-1]}.log",
                [
                    "logs",
                    "-n",
                    namespace,
                    "-l",
                    selector,
                    "--all-containers",
                    "--prefix",
                    # With -l, kubectl logs defaults to the last 10 lines.
                    "--tail=-1",
                    f"--since-time={_rfc3339(started_at)}",
                    f"--limit-bytes={CONTROLLER_LOG_LIMIT_BYTES}",
                ],
            )

    def _kubectl(self, filename: Optional[str], args: List[str]) -> Optional[str]:
        timeout = f"--request-timeout={COMMAND_TIMEOUT_SECONDS}s"
        output = self._run(["kubectl", timeout] + args)
        if output is not None and filename:
            path = self.out / filename
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(output)
        return output

    def _run(self, cmd: List[str]) -> Optional[str]:
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            self.errors.append(f"skipped, budget spent: {' '.join(cmd)}")
            return None
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=min(COMMAND_TIMEOUT_SECONDS, remaining),
                check=False,
            )
        except (OSError, subprocess.TimeoutExpired) as e:
            self.errors.append(f"{' '.join(cmd)}: {e}")
            return None
        if result.returncode != 0:
            self.errors.append(
                f"{' '.join(cmd)}: exit {result.returncode}: {result.stderr.strip()[-500:]}"
            )
            return None
        return result.stdout
